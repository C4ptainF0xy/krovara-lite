package xmpp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
)

const DefaultTokenTTL = 60 * time.Second

const DefaultDomain = "krovara.local"

var (
	ErrInvalidToken = errors.New("xmpp: token invalid or expired")
	ErrInvalidJID   = errors.New("xmpp: username is not a valid JID node")
)

type Service struct {
	q      db.Querier
	ttl    time.Duration
	domain string
	now    func() time.Time
}

func NewService(q db.Querier) *Service {
	return &Service{
		q:      q,
		ttl:    DefaultTokenTTL,
		domain: DefaultDomain,
		now:    time.Now,
	}
}

func (s *Service) JID(userID uuid.UUID) string {
	return userID.String() + "@" + s.domain
}

func (s *Service) IssueToken(ctx context.Context, userID uuid.UUID) (token string, expiresAt time.Time, err error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("rand: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf[:])
	expiresAt = s.now().Add(s.ttl)
	if _, err := s.q.CreateXMPPToken(ctx, db.CreateXMPPTokenParams{
		Token:     token,
		UserID:    pgUUID(userID),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("store token: %w", err)
	}
	return token, expiresAt, nil
}

func (s *Service) Verify(ctx context.Context, token string, expectedUserID uuid.UUID) (uuid.UUID, error) {
	row, err := s.q.ConsumeXMPPToken(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, err
	}
	owner := uuid.UUID(row.UserID.Bytes)
	if expectedUserID != uuid.Nil && owner != expectedUserID {
		return uuid.Nil, ErrInvalidToken
	}
	return owner, nil
}

func (s *Service) PublicRoutes(r chi.Router, userIDFn func(context.Context) uuid.UUID) {
	r.Post("/xmpp/token", s.handleIssueToken(userIDFn))
}

func (s *Service) InternalRoutes(r chi.Router) {
	r.Get("/xmpp/check_password", s.handleProsodyAuth())
	r.Post("/xmpp/register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}

type tokenResp struct {
	Token     string    `json:"token"`
	JID       string    `json:"jid"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) handleIssueToken(uidFn func(context.Context) uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		token, exp, err := s.IssueToken(r.Context(), uid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "issue failed")
			return
		}
		writeJSON(w, http.StatusOK, tokenResp{
			Token:     token,
			JID:       s.JID(uid),
			ExpiresAt: exp,
		})
	}
}

func (s *Service) handleProsodyAuth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.FormValue("user")
		pass := r.FormValue("pass")
		if user == "" || pass == "" {
			http.Error(w, "false", http.StatusUnauthorized)
			return
		}

		uid, err := uuid.Parse(user)
		if err != nil {
			http.Error(w, "false", http.StatusUnauthorized)
			return
		}
		if _, err := s.Verify(r.Context(), pass, uid); err != nil {
			http.Error(w, "false", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
