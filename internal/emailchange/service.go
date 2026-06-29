package emailchange

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/email"
	"github.com/krovara/krovara/internal/permissions"
)

const tokenTTL = 2 * time.Hour

type Service struct {
	q      *db.Queries
	v      *validator.Validate
	sender email.Sender
	appURL string
}

func NewService(pool *pgxpool.Pool, sender email.Sender, appURL string) *Service {
	return &Service{
		q:      db.New(pool),
		v:      validator.New(validator.WithRequiredStructEnabled()),
		sender: sender,
		appURL: strings.TrimRight(appURL, "/"),
	}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Post("/me/email", s.handleRequest(userIDFn))
}

func (s *Service) PublicRoutes(r chi.Router) {
	r.Post("/api/account/email/confirm", s.handleConfirm())
}

type requestReq struct {
	NewEmail        string `json:"new_email"        validate:"required,email,max=254"`
	CurrentPassword string `json:"current_password" validate:"required"`
}

func (s *Service) handleRequest(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req requestReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := s.v.Struct(&req); err != nil {
			writeError(w, http.StatusBadRequest, "email invalide")
			return
		}

		newEmail := strings.TrimSpace(req.NewEmail)

		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if u.PasswordHash == nil || *u.PasswordHash == "" {
			writeError(w, http.StatusForbidden, "compte sans mot de passe")
			return
		}
		ok, vErr := auth.VerifyPassword(req.CurrentPassword, *u.PasswordHash)
		if vErr != nil || !ok {
			writeError(w, http.StatusForbidden, "mot de passe actuel incorrect")
			return
		}
		if strings.EqualFold(newEmail, u.Email) {
			writeError(w, http.StatusBadRequest, "adresse identique à l'actuelle")
			return
		}

		if _, err := s.q.GetUserByEmail(r.Context(), newEmail); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if err := s.q.InvalidatePendingEmailChanges(r.Context(), pgUUID(uid)); err != nil {
			writeError(w, http.StatusInternalServerError, "cleanup failed")
			return
		}

		clear, hash, err := newToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token gen failed")
			return
		}
		if _, err := s.q.CreateEmailChangeToken(r.Context(), db.CreateEmailChangeTokenParams{
			UserID:    pgUUID(uid),
			NewEmail:  newEmail,
			TokenHash: hash,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(tokenTTL), Valid: true},
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}

		link := s.appURL + "/confirm-email?token=" + clear
		if err := s.sender.Send(r.Context(), email.Message{
			To:      newEmail,
			Subject: "Confirme ta nouvelle adresse email Krovara",
			HTML:    confirmHTML(u.Username, link),
		}); err != nil {
			slog.Error("email change send", "err", err, "user", uid)
			writeError(w, http.StatusBadGateway, "envoi indisponible")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type confirmReq struct {
	Token string `json:"token" validate:"required"`
}

func (s *Service) handleConfirm() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req confirmReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := s.v.Struct(&req); err != nil {
			writeError(w, http.StatusBadRequest, "token manquant")
			return
		}

		tok, err := s.q.ConsumeEmailChangeToken(r.Context(), hashToken(req.Token))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "lien invalide ou expiré")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "confirm failed")
			return
		}
		if err := s.q.UpdateUserEmail(r.Context(), db.UpdateUserEmailParams{
			ID:    tok.UserID,
			Email: tok.NewEmail,
		}); err != nil {

			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, http.StatusConflict, "adresse déjà utilisée")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"email": tok.NewEmail})
	}
}

func newToken() (clear, hash string, err error) {
	var buf [32]byte
	if _, err = rand.Read(buf[:]); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	clear = hex.EncodeToString(buf[:])
	return clear, hashToken(clear), nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func confirmHTML(username, link string) string {
	return fmt.Sprintf(
		`<p>Bonjour %s,</p>`+
			`<p>Confirme ta nouvelle adresse email pour ton compte Krovara en cliquant sur le lien ci-dessous :</p>`+
			`<p><a href="%s">Confirmer mon adresse email</a></p>`+
			`<p>Ce lien expire dans 2 heures. Si tu n'es pas à l'origine de cette demande, ignore cet email — ton adresse actuelle reste inchangée.</p>`,
		htmlEscape(username), htmlEscape(link))
}
