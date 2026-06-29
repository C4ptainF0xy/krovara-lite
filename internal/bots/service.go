package bots

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const DefaultDomain = "krovara.local"

type Service struct {
	q             *db.Queries
	v             *validator.Validate
	domain        string
	componentsDir string
}

func NewService(pool *pgxpool.Pool, xmppDomain string) *Service {
	if xmppDomain == "" {
		xmppDomain = DefaultDomain
	}
	return &Service{
		q:      db.New(pool),
		v:      validator.New(validator.WithRequiredStructEnabled()),
		domain: xmppDomain,
	}
}

func (s *Service) WithComponentsDir(dir string) *Service {
	s.componentsDir = dir
	return s
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, uidFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/bots", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Post("/", s.handleCreate())
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Get("/", s.handleList())
	})
	r.Route("/bots/{botID}", func(rr chi.Router) {
		rr.Use(s.attachBotSpace)
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Get("/", s.handleGet())
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Delete("/", s.handleDelete())
	})
}

func (s *Service) attachBotSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "botID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bot id")
			return
		}
		row, err := s.q.GetBot(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bot not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(row.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Name string `json:"name" validate:"required,min=1,max=64"`
}

func (s *Service) componentJID(id uuid.UUID) string {
	short := strings.ReplaceAll(id.String(), "-", "")[:8]
	return "bot-" + short + "." + s.domain
}

func (s *Service) handleCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req createReq
		if !s.decode(w, r, &req) {
			return
		}

		botID := uuid.New()
		secret, err := randomSecret()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "secret gen failed")
			return
		}
		hash := HashSecret(secret)

		row, err := s.q.CreateBot(r.Context(), db.CreateBotParams{
			SpaceID:      pgUUID(spaceID),
			Name:         req.Name,
			ComponentJid: s.componentJID(botID),
			SecretHash:   hash,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}

		if err := s.writeComponent(row.ComponentJid, secret); err != nil {
			writeError(w, http.StatusInternalServerError, "create ok but components sync failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":            uuid.UUID(row.ID.Bytes).String(),
			"space_id":      uuid.UUID(row.SpaceID.Bytes).String(),
			"name":          row.Name,
			"component_jid": row.ComponentJid,
			"secret":        secret,
			"connect": map[string]any{
				"host":   "xmpp." + s.domain,
				"port":   5347,
				"domain": row.ComponentJid,
				"secret": secret,
			},
			"created_at": row.CreatedAt.Time,
		})
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceBots(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, b := range rows {
			out = append(out, botDTO(b))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "botID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bot id")
			return
		}
		row, err := s.q.GetBot(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, botDTO(row))
	}
}

func (s *Service) handleDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "botID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bot id")
			return
		}
		row, err := s.q.GetBot(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if err := s.q.DeleteBot(r.Context(), pgUUID(id)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}

		_ = s.removeComponent(row.ComponentJid)
		w.WriteHeader(http.StatusNoContent)
	}
}

func HashSecret(s string) string {
	const salt = "krovara-bot-secret-v1"
	h := argon2.IDKey([]byte(s), []byte(salt), 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(h)
}

func (s *Service) writeComponent(componentJID, secret string) error {
	if s.componentsDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.componentsDir, 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	name := componentFilename(componentJID)
	final := filepath.Join(s.componentsDir, name)
	tmp := final + ".tmp"
	body := fmt.Sprintf(`-- Generated by krovara-api. Do not edit by hand.
-- Bot JID: %s
Component %q
    component_secret = %q
`, componentJID, componentJID, secret)
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (s *Service) removeComponent(componentJID string) error {
	if s.componentsDir == "" {
		return nil
	}
	return os.Remove(filepath.Join(s.componentsDir, componentFilename(componentJID)))
}

func componentFilename(componentJID string) string {
	if i := strings.IndexByte(componentJID, '.'); i > 0 {
		return componentJID[:i] + ".cfg.lua"
	}
	return componentJID + ".cfg.lua"
}

func randomSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func botDTO(b db.Bot) map[string]any {
	return map[string]any{
		"id":            uuid.UUID(b.ID.Bytes).String(),
		"space_id":      uuid.UUID(b.SpaceID.Bytes).String(),
		"name":          b.Name,
		"component_jid": b.ComponentJid,
		"created_at":    b.CreatedAt.Time,
	}
}

func (s *Service) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	if err := s.v.Struct(dst); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

var _ = context.Background
