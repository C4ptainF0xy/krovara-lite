package apitokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const tokenPrefix = "kvt_"

var validScopes = map[string]bool{"read": true, "write": true}

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Get("/me/api-tokens", s.handleList(userIDFn))
	r.Post("/me/api-tokens", s.handleCreate(userIDFn))
	r.Delete("/me/api-tokens/{id}", s.handleDelete(userIDFn))
}

type createReq struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len(req.Name) < 1 || len(req.Name) > 64 {
			writeError(w, http.StatusBadRequest, "name length")
			return
		}
		if len(req.Scopes) == 0 {
			writeError(w, http.StatusBadRequest, "at least one scope required")
			return
		}
		for _, sc := range req.Scopes {
			if !validScopes[sc] {
				writeError(w, http.StatusBadRequest, "invalid scope: "+sc)
				return
			}
		}
		clear, err := randomToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token gen failed")
			return
		}
		row, err := s.q.CreateAPIToken(r.Context(), db.CreateAPITokenParams{
			UserID:    pgUUID(uidFn(r.Context())),
			Name:      req.Name,
			TokenHash: hashToken(clear),
			Prefix:    clear[:len(tokenPrefix)+6],
			Scopes:    req.Scopes,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		out := tokenDTO(row)
		out["token"] = clear
		writeJSON(w, http.StatusCreated, out)
	}
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListUserAPITokens(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, t := range rows {
			out = append(out, tokenDTO(t))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if err := s.q.DeleteAPIToken(r.Context(), db.DeleteAPITokenParams{
			ID: pgUUID(id), UserID: pgUUID(uidFn(r.Context())),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxScopes
)

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(raw, tokenPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		row, err := s.q.GetAPITokenByHash(r.Context(), hashToken(raw))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid api token")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "auth error")
			return
		}
		_ = s.q.TouchAPIToken(r.Context(), row.ID)
		ctx := context.WithValue(r.Context(), ctxUserID, uuid.UUID(row.UserID.Bytes))
		ctx = context.WithValue(ctx, ctxScopes, row.Scopes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes, ok := r.Context().Value(ctxScopes).([]string)
			if ok && !slices.Contains(scopes, scope) {
				writeError(w, http.StatusForbidden, "token missing scope: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TokenUserID(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(ctxUserID).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func tokenDTO(t db.ApiToken) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(t.ID.Bytes).String(),
		"name":       t.Name,
		"prefix":     t.Prefix,
		"scopes":     t.Scopes,
		"created_at": t.CreatedAt.Time,
	}
	if t.LastUsedAt.Valid {
		out["last_used_at"] = t.LastUsedAt.Time
	}
	return out
}

func randomToken() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
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
