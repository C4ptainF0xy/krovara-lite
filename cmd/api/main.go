package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/krovara/krovara/internal/admin"
	"github.com/krovara/krovara/internal/apitokens"
	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/bans"
	"github.com/krovara/krovara/internal/billing"
	"github.com/krovara/krovara/internal/bots"
	"github.com/krovara/krovara/internal/captcha"
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/discovery"
	"github.com/krovara/krovara/internal/dmgroups"
	"github.com/krovara/krovara/internal/email"
	"github.com/krovara/krovara/internal/emailchange"
	"github.com/krovara/krovara/internal/emojis"
	"github.com/krovara/krovara/internal/envfile"
	"github.com/krovara/krovara/internal/events"
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/files"
	"github.com/krovara/krovara/internal/friends"
	"github.com/krovara/krovara/internal/games"
	"github.com/krovara/krovara/internal/gif"
	"github.com/krovara/krovara/internal/httpx"
	"github.com/krovara/krovara/internal/invites"
	"github.com/krovara/krovara/internal/jobs"
	"github.com/krovara/krovara/internal/joingate"
	"github.com/krovara/krovara/internal/karma"
	"github.com/krovara/krovara/internal/links"
	"github.com/krovara/krovara/internal/members"
	"github.com/krovara/krovara/internal/messages"
	"github.com/krovara/krovara/internal/moderation"
	"github.com/krovara/krovara/internal/notifications"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/polls"
	"github.com/krovara/krovara/internal/profile"
	"github.com/krovara/krovara/internal/push"
	"github.com/krovara/krovara/internal/reports"
	"github.com/krovara/krovara/internal/roles"
	"github.com/krovara/krovara/internal/savedsearch"
	"github.com/krovara/krovara/internal/search"
	"github.com/krovara/krovara/internal/spaces"
	"github.com/krovara/krovara/internal/tasks"
	"github.com/krovara/krovara/internal/threads"
	"github.com/krovara/krovara/internal/voip"
	"github.com/krovara/krovara/internal/webhooks"
	"github.com/krovara/krovara/internal/xmpp"
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		log.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	publicMux, internalMux := buildRouters(pool, cfg, log)

	pub := &http.Server{
		Addr:              cfg.PublicAddr,
		Handler:           publicMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	internal := &http.Server{
		Addr:              cfg.InternalAddr,
		Handler:           internalMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 2)
	go func() { errs <- pub.ListenAndServe() }()
	go func() { errs <- internal.ListenAndServe() }()
	log.Info("api online", "public", cfg.PublicAddr, "internal", cfg.InternalAddr)

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errs:
		log.Error("listener died", "err", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	_ = pub.Shutdown(shutdownCtx)
	_ = internal.Shutdown(shutdownCtx)
}

type config struct {
	DSN          string
	PublicAddr   string
	InternalAddr string
	JWTSecret    []byte
	Google       *auth.ProviderConfig
	GitHub       *auth.ProviderConfig
}

func loadConfig() (*config, error) {
	secret := os.Getenv("KROVARA_JWT_SECRET")
	if secret == "" {
		return nil, errString("KROVARA_JWT_SECRET is required")
	}
	c := &config{
		DSN:          envOr("KROVARA_DB_DSN", "postgres://krovara:krovara@localhost:5432/krovara?sslmode=disable"),
		PublicAddr:   envOr("KROVARA_HTTP_ADDR", ":8080"),
		InternalAddr: envOr("KROVARA_INTERNAL_ADDR", "127.0.0.1:8090"),
		JWTSecret:    []byte(secret),
	}
	if id, sec := os.Getenv("KROVARA_OAUTH_GOOGLE_CLIENT_ID"), os.Getenv("KROVARA_OAUTH_GOOGLE_CLIENT_SECRET"); id != "" && sec != "" {
		c.Google = auth.GoogleProvider(id, sec, os.Getenv("KROVARA_OAUTH_GOOGLE_REDIRECT_URL"))
	}
	if id, sec := os.Getenv("KROVARA_OAUTH_GITHUB_CLIENT_ID"), os.Getenv("KROVARA_OAUTH_GITHUB_CLIENT_SECRET"); id != "" && sec != "" {
		c.GitHub = auth.GitHubProvider(id, sec, os.Getenv("KROVARA_OAUTH_GITHUB_REDIRECT_URL"))
	}
	return c, nil
}

func buildRouters(pool *pgxpool.Pool, cfg *config, log *slog.Logger) (*chi.Mux, *chi.Mux) {
	q := db.New(pool)
	signer := auth.NewJWTSigner(cfg.JWTSecret, 15*time.Minute)
	sessions := auth.NewSessionStore(q, 30*24*time.Hour)
	authSvc := auth.NewService(q, signer, sessions)

	if v := captcha.NewTurnstile(os.Getenv("KROVARA_TURNSTILE_SECRET")); v != nil {
		authSvc.Captcha = v
		log.Info("registration captcha enabled (Turnstile)")
	}

	if k := os.Getenv("KROVARA_SIGNUP_IP_KEY"); k != "" {
		authSvc.SignupIPKey = []byte(k)
		log.Info("signup IP fingerprint enabled (anti-multi-account signal)")
	}

	if os.Getenv("KROVARA_TRUST_PROXY") == "true" {
		auth.SetTrustProxy(true)
		log.Info("trusting proxy IP headers (X-Real-IP / X-Forwarded-For)")
	}
	var oauthSvc *auth.OAuthService
	providers := []*auth.ProviderConfig{}
	if cfg.Google != nil {
		providers = append(providers, cfg.Google)
	}
	if cfg.GitHub != nil {
		providers = append(providers, cfg.GitHub)
	}
	if len(providers) > 0 {
		oauthSvc = auth.NewOAuthService(authSvc, true, providers...)
	}

	resolver := permissions.NewPGResolver(q)
	filesRoot := envOr("KROVARA_FILES_ROOT", "/data/krovara")
	store, err := files.NewLocalStore(filesRoot)
	if err != nil {
		log.Error("files store", "err", err)
		os.Exit(1)
	}

	spacesSvc := spaces.NewService(pool)
	channelsSvc := channels.NewService(pool)
	rolesSvc := roles.NewService(pool)
	membersSvc := members.NewService(pool)
	invitesSvc := invites.NewService(pool)
	mucHost := envOr("KROVARA_XMPP_MUC_HOST", "conference.krovara.local")
	bansSvc := bans.NewService(pool, mucHost)
	reportsSvc := reports.NewService(pool)
	moderationSvc := moderation.NewService(pool, mucHost)
	messagesSvc := messages.NewService(pool, mucHost)
	threadsSvc := threads.NewService(pool, mucHost)
	karmaSvc := karma.NewService(pool)
	joinGateSvc := joingate.NewService(pool)
	profileSvc := profile.NewService(pool)

	appURL := envOr("KROVARA_APP_URL", "http://localhost:8082")

	emailSender := email.New(
		os.Getenv("KROVARA_RESEND_API_KEY"),
		envOr("KROVARA_EMAIL_FROM", "Krovara <onboarding@resend.dev>"),
	)
	emailChangeSvc := emailchange.NewService(pool, emailSender, appURL)
	authSvc.Email = emailSender
	authSvc.AppURL = appURL
	friendsSvc := friends.NewService(pool)
	dmGroupsSvc := dmgroups.NewService(pool)
	notificationsSvc := notifications.NewService(pool)
	gamesSvc := games.NewService(pool)
	tasksSvc := tasks.NewService(pool)
	pollsSvc := polls.NewService(pool)
	eventsSvc := events.NewService(pool)
	eventsfeedSvc := eventsfeed.NewService(pool, log.With("component", "eventsfeed"))
	apiTokensSvc := apitokens.NewService(pool)
	discoverySvc := discovery.NewService(pool)
	emojisSvc := emojis.NewService(pool)

	billingSvc := billing.New(pool, billing.Config{
		SecretKey:     os.Getenv("KROVARA_STRIPE_SECRET_KEY"),
		WebhookSecret: os.Getenv("KROVARA_STRIPE_WEBHOOK_SECRET"),
		PriceID:       os.Getenv("KROVARA_STRIPE_PRICE_ID"),
		SuccessURL:    appURL + "/app/settings/subscription?success=1",
		CancelURL:     appURL + "/app/settings/subscription?canceled=1",
		ReturnURL:     appURL + "/app/settings/subscription",
	})
	savedSearchSvc := savedsearch.NewService(pool)
	adminSvc := admin.NewService(pool)
	linksSvc := links.NewService(q)
	if sb := links.NewSafeBrowsing(os.Getenv("KROVARA_SAFE_BROWSING_KEY")); sb != nil {
		linksSvc.WithSafeBrowsing(sb)
		log.Info("Google Safe Browsing link scanning enabled")
	}

	var fileScanner files.Scanner
	if os.Getenv("KROVARA_CLAMAV_ADDR") != "" {
		if rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{}); err != nil {
			log.Warn("file scan enqueuer", "err", err)
		} else {
			fileScanner = fileScanEnqueuer{rc: rc}
		}
	}
	filesSvc := files.NewService(q, store, fileScanner)
	xmppSvc := xmpp.NewService(q)

	webhooksSvc := webhooks.NewService(pool, nil)
	botsSvc := bots.NewService(pool, envOr("KROVARA_XMPP_DOMAIN", "")).
		WithComponentsDir(os.Getenv("KROVARA_PROSODY_COMPONENTS_DIR"))
	pushSvc := push.NewService(pool, nil)

	var voipSvc *voip.Service
	if sec := os.Getenv("COTURN_AUTH_SECRET"); sec != "" {
		uris := splitCSV(os.Getenv("KROVARA_TURN_URIS"))
		voipSvc = voip.NewService(sec, uris)
	}

	var searchSvc *search.Service
	if host, key := os.Getenv("KROVARA_MEILI_HOST"), os.Getenv("MEILI_MASTER_KEY"); host != "" && key != "" {
		searchSvc = search.NewService(q, host, key)

		if err := searchSvc.EnsureIndex(context.Background()); err != nil {
			log.Warn("search index init", "err", err)
		}
	}

	var gifSvc *gif.Service
	if key := os.Getenv("KROVARA_KLIPY_API_KEY"); key != "" {
		gifSvc = gif.NewService(key)
	}

	pub := chi.NewMux()

	if origins := splitCSV(os.Getenv("KROVARA_CORS_ORIGINS")); len(origins) > 0 {
		pub.Use(httpx.CORS(origins))
		log.Info("CORS enabled", "origins", origins)
	}
	pub.Use(httpx.WithRequestID(log))
	pub.Use(httpx.AccessLog)
	pub.Use(httpx.Metrics(func(r *http.Request) string {
		if rc := chi.RouteContext(r.Context()); rc != nil {
			return rc.RoutePattern()
		}
		return ""
	}))

	pub.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	pub.Handle("/metrics", httpx.MetricsHandler())

	pub.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, oauthSvc) })

	webhooksSvc.PublicRoutes(pub)
	spacesSvc.PublicRoutes(pub)
	if billingSvc != nil {
		billingSvc.PublicRoutes(pub)
	}
	emailChangeSvc.PublicRoutes(pub)
	pub.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer, q))

		g.Use(auth.NewKeyedRateLimitMiddleware(600, time.Minute, func(r *http.Request) string {
			if uid := auth.UserID(r.Context()); uid != uuid.Nil {
				return "u:" + uid.String()
			}
			return ""
		}))
		g.Route("/api", func(api chi.Router) {
			profileSvc.Routes(api, auth.UserID)
			emailChangeSvc.Routes(api, auth.UserID)
			friendsSvc.Routes(api, auth.UserID)
			dmGroupsSvc.Routes(api, auth.UserID)
			notificationsSvc.Routes(api, auth.UserID)
			gamesSvc.Routes(api, auth.UserID)
			gamesSvc.AdminRoutes(api, auth.UserID)
			tasksSvc.Routes(api, resolver, auth.UserID)
			pollsSvc.Routes(api, resolver, auth.UserID)
			eventsSvc.Routes(api, resolver, auth.UserID)
			eventsfeedSvc.Routes(api, auth.UserID)
			apiTokensSvc.Routes(api, auth.UserID)
			savedSearchSvc.Routes(api, auth.UserID)
			discoverySvc.Routes(api, resolver, auth.UserID)
			emojisSvc.Routes(api, resolver, auth.UserID)
			if billingSvc != nil {
				billingSvc.Routes(api, auth.UserID)
			}
			adminSvc.Routes(api, auth.UserID)
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.CategoryRoutes(api, resolver, auth.UserID)
			channelsSvc.OverwriteRoutes(api, resolver, auth.UserID)
			rolesSvc.Routes(api, resolver, auth.UserID)
			membersSvc.Routes(api, resolver, auth.UserID)
			invitesSvc.Routes(api, resolver, auth.UserID)
			bansSvc.Routes(api, resolver, auth.UserID)
			reportsSvc.Routes(api, resolver, auth.UserID)
			moderationSvc.Routes(api, resolver, auth.UserID)
			moderationSvc.TimeoutRoutes(api, resolver, auth.UserID)
			messagesSvc.Routes(api, resolver, auth.UserID)
			threadsSvc.Routes(api, resolver, auth.UserID)
			karmaSvc.Routes(api, auth.UserID)
			joinGateSvc.Routes(api, resolver, auth.UserID)
			filesSvc.Routes(api, files.UserIDFunc(auth.UserID))
			xmppSvc.PublicRoutes(api, auth.UserID)
			if voipSvc != nil {
				voipSvc.Routes(api, auth.UserID)
			}
			if searchSvc != nil {
				searchSvc.Routes(api, resolver, auth.UserID)
			}
			if gifSvc != nil {
				gifSvc.Routes(api)
			}
			linksSvc.Routes(api)
			webhooksSvc.Routes(api, resolver, auth.UserID)
			botsSvc.Routes(api, resolver, auth.UserID)
			pushSvc.Routes(api, auth.UserID)
		})
	})

	internal := chi.NewMux()
	internal.Use(httpx.WithRequestID(log))
	internal.Use(httpx.AccessLog)
	internal.Route("/internal", func(r chi.Router) { xmppSvc.InternalRoutes(r) })
	internal.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	go eventsfeedSvc.Listen(context.Background())

	return pub, internal
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type fileScanEnqueuer struct{ rc *river.Client[pgx.Tx] }

func (e fileScanEnqueuer) Enqueue(ctx context.Context, fileID uuid.UUID) error {
	_, err := e.rc.Insert(ctx, jobs.FileScanArgs{FileID: fileID.String()}, nil)
	return err
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
