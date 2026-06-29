package games

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Get("/games", s.handleList())
	r.Post("/games", s.handleSubmit(userIDFn))
}

func (s *Service) AdminRoutes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Group(func(rr chi.Router) {
		rr.Use(s.requireAdmin(userIDFn))
		rr.Get("/admin/games/pending", s.handlePending())
		rr.Post("/admin/games/{gameID}/review", s.handleReview(userIDFn))
	})
}

func (s *Service) requireAdmin(userIDFn permissions.UserIDFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := userIDFn(r.Context())
			if uid == uuid.Nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
			if err != nil || !u.IsAdmin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limit := queryInt(r, "limit", 50, 1, 200)
		rows, err := s.q.ListApprovedGames(r.Context(), db.ListApprovedGamesParams{
			Column1: q, Limit: limit,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, gamesDTO(rows))
	}
}

type submitReq struct {
	Name     string   `json:"name"`
	CoverKey *string  `json:"cover_key"`
	Aliases  []string `json:"aliases"`
}

func (s *Service) handleSubmit(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req submitReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len(req.Name) < 2 || len(req.Name) > 128 {
			writeError(w, http.StatusBadRequest, "name length")
			return
		}
		if req.Aliases == nil {
			req.Aliases = []string{}
		}
		g, err := s.q.SubmitGame(r.Context(), db.SubmitGameParams{
			Name:        req.Name,
			CoverKey:    req.CoverKey,
			SubmittedBy: pgUUID(uidFn(r.Context())),
			Aliases:     req.Aliases,
		})
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "ce jeu existe déjà")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "submit failed")
			return
		}
		writeJSON(w, http.StatusCreated, gameDTO(g))
	}
}

func (s *Service) handlePending() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListGamesByStatus(r.Context(), db.ListGamesByStatusParams{
			Status: "pending", Limit: 200,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, gamesDTO(rows))
	}
}

type reviewReq struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

func (s *Service) handleReview(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "gameID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid game id")
			return
		}
		var req reviewReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		status := "approved"
		var reason *string
		if !req.Approve {
			status = "rejected"
			if req.Reason != "" {
				reason = &req.Reason
			}
		}
		g, err := s.q.ReviewGame(r.Context(), db.ReviewGameParams{
			ID: pgUUID(id), Status: status, ReviewedBy: pgUUID(uidFn(r.Context())), RejectReason: reason,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "game not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "review failed")
			return
		}
		writeJSON(w, http.StatusOK, gameDTO(g))
	}
}

func gameDTO(g db.Game) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(g.ID.Bytes).String(),
		"name":       g.Name,
		"cover_key":  g.CoverKey,
		"status":     g.Status,
		"aliases":    g.Aliases,
		"created_at": g.CreatedAt.Time,
	}
	if g.RejectReason != nil {
		out["reject_reason"] = *g.RejectReason
	}
	return out
}

func gamesDTO(rows []db.Game) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, g := range rows {
		out = append(out, gameDTO(g))
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func queryInt(r *http.Request, key string, def, min, max int32) int32 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if int32(n) < min {
		return min
	}
	if int32(n) > max {
		return max
	}
	return int32(n)
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
