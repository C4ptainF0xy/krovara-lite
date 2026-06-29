package savedsearch

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const maxPerUser = 50

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, uidFn permissions.UserIDFunc) {
	r.Route("/me/saved-searches", func(rr chi.Router) {
		rr.Get("/", s.handleList(uidFn))
		rr.Post("/", s.handleCreate(uidFn))
		rr.Delete("/{id}", s.handleDelete(uidFn))
	})
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		rows, err := s.q.ListSavedSearches(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, ss := range rows {
			out = append(out, dto(ss))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type createReq struct {
	Name    string  `json:"name"`
	Query   string  `json:"query"`
	SpaceID *string `json:"space_id"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Query = strings.TrimSpace(req.Query)
		if req.Name == "" || len(req.Name) > 80 {
			writeError(w, http.StatusBadRequest, "name is required (max 80)")
			return
		}
		if req.Query == "" || len(req.Query) > 500 {
			writeError(w, http.StatusBadRequest, "query is required (max 500)")
			return
		}

		count, err := s.q.CountSavedSearches(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "count failed")
			return
		}
		if count >= maxPerUser {
			writeError(w, http.StatusConflict, "saved search limit reached")
			return
		}

		var space pgtype.UUID
		if req.SpaceID != nil && *req.SpaceID != "" {
			sid, err := uuid.Parse(*req.SpaceID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid space_id")
				return
			}
			space = pgUUID(sid)
		}

		ss, err := s.q.CreateSavedSearch(r.Context(), db.CreateSavedSearchParams{
			UserID:  pgUUID(uid),
			SpaceID: space,
			Name:    req.Name,
			Query:   req.Query,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, dto(ss))
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		n, err := s.q.DeleteSavedSearch(r.Context(), db.DeleteSavedSearchParams{
			ID: pgUUID(id), UserID: pgUUID(uid),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		if n == 0 {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func dto(ss db.SavedSearch) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(ss.ID.Bytes).String(),
		"name":       ss.Name,
		"query":      ss.Query,
		"space_id":   nil,
		"created_at": ss.CreatedAt.Time,
	}
	if ss.SpaceID.Valid {
		out["space_id"] = uuid.UUID(ss.SpaceID.Bytes).String()
	}
	return out
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
