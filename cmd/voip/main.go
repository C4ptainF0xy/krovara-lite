package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/envfile"
	"github.com/krovara/krovara/internal/sfu"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := envfile.LoadFirst(".env.dev", ".env"); err != nil {
		log.Warn("envfile", "err", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	signer := auth.NewJWTSigner(cfg.JWTSecret, 0)
	hub := sfu.NewHub(log)
	rtcCfg := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: cfg.ICEServers},
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mux := chi.NewMux()
	mux.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Get("/voip/ws", handleWS(signer, hub, rtcCfg, cfg.AllowedOrigins, log))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	log.Info("voip online", "addr", cfg.Addr)

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("listener died", "err", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdownCtx)
}

type config struct {
	Addr           string
	JWTSecret      []byte
	AllowedOrigins []string
	ICEServers     []string
}

func loadConfig() (*config, error) {
	secret := os.Getenv("KROVARA_JWT_SECRET")
	if secret == "" {
		return nil, errString("KROVARA_JWT_SECRET is required (must match cmd/api)")
	}
	ice := splitCSV(os.Getenv("KROVARA_VOIP_ICE_SERVERS"))
	if len(ice) == 0 {
		ice = []string{"stun:stun.l.google.com:19302"}
	}
	return &config{
		Addr:           envOr("KROVARA_VOIP_ADDR", ":8083"),
		JWTSecret:      []byte(secret),
		AllowedOrigins: splitCSV(os.Getenv("KROVARA_VOIP_ALLOWED_ORIGINS")),
		ICEServers:     ice,
	}, nil
}

func handleWS(signer *auth.JWTSigner, hub *sfu.Hub, rtcCfg webrtc.Configuration, allowedOrigins []string, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		userID, err := signer.Parse(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		opts := &websocket.AcceptOptions{
			OriginPatterns: allowedOrigins,
		}
		if len(allowedOrigins) == 0 {

			opts.InsecureSkipVerify = true
		}

		conn, err := websocket.Accept(w, r, opts)
		if err != nil {
			log.Warn("ws accept", "err", err, "user_id", userID)
			return
		}

		peerID := uuid.NewString()
		peer, err := sfu.NewPeer(sfu.PeerConfig{
			ID:     peerID,
			UserID: userID.String(),
			WS:     conn,
			WebRTC: rtcCfg,
			Log:    log,
		})
		if err != nil {
			log.Error("new peer", "err", err, "user_id", userID)
			_ = conn.Close(websocket.StatusInternalError, "peer init failed")
			return
		}

		log.Info("ws connected", "user_id", userID, "peer_id", peerID)
		if err := peer.Run(r.Context(), hub); err != nil {
			log.Debug("peer run", "err", err, "peer_id", peerID)
		}
		log.Info("ws disconnected", "user_id", userID, "peer_id", peerID)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type errString string

func (e errString) Error() string { return string(e) }

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for len(part) > 0 && (part[0] == ' ' || part[0] == '\t') {
				part = part[1:]
			}
			for len(part) > 0 && (part[len(part)-1] == ' ' || part[len(part)-1] == '\t') {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
