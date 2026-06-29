package threads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	v       *validator.Validate
	mucHost string
}

func NewService(pool *pgxpool.Pool, mucHost string) *Service {
	return &Service{
		pool:    pool,
		q:       db.New(pool),
		v:       validator.New(validator.WithRequiredStructEnabled()),
		mucHost: mucHost,
	}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/channels/{channelID}/threads", func(rr chi.Router) {
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Post("/", s.handleCreate(userIDFn))
	})
	r.Route("/threads/{threadID}", func(rr chi.Router) {
		rr.Post("/subscribe", s.guardThread(resolver, userIDFn, s.handleSubscribe(userIDFn)))
		rr.Delete("/subscribe", s.guardThread(resolver, userIDFn, s.handleUnsubscribe(userIDFn)))
		rr.Post("/touch", s.guardThread(resolver, userIDFn, s.handleTouch()))
	})
}

type createReq struct {
	RootArchiveID string `json:"root_archive_id" validate:"required,max=255"`
	Title         string `json:"title"           validate:"required,min=1,max=128"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		var req createReq
		if !s.decode(w, r, &req) {
			return
		}
		t, err := s.q.CreateThread(r.Context(), db.CreateThreadParams{
			ChannelID:     pgUUID(channelID),
			RootArchiveID: req.RootArchiveID,
			Title:         req.Title,
			CreatedBy:     pgUUID(uidFn(r.Context())),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}

		_ = s.q.SubscribeThread(r.Context(), db.SubscribeThreadParams{
			UserID:   pgUUID(uidFn(r.Context())),
			ThreadID: t.ID,
		})
		writeJSON(w, http.StatusCreated, threadDTO(t, 0, true))
	}
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		rows, err := s.q.ListChannelThreads(r.Context(), db.ListChannelThreadsParams{
			ViewerID:  pgUUID(uidFn(r.Context())),
			ChannelID: pgUUID(channelID),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, threadRoom(row.Thread.ID))
		}
		counts := s.replyCounts(r.Context(), ids)
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, threadDTO(row.Thread, counts[threadRoom(row.Thread.ID)], row.IsSubscribed))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleSubscribe(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID, ok := uuidParam(w, r, "threadID")
		if !ok {
			return
		}
		if err := s.q.SubscribeThread(r.Context(), db.SubscribeThreadParams{
			UserID:   pgUUID(uidFn(r.Context())),
			ThreadID: pgUUID(threadID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "subscribe failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleUnsubscribe(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID, ok := uuidParam(w, r, "threadID")
		if !ok {
			return
		}
		if err := s.q.UnsubscribeThread(r.Context(), db.UnsubscribeThreadParams{
			UserID:   pgUUID(uidFn(r.Context())),
			ThreadID: pgUUID(threadID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "unsubscribe failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleTouch() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID, ok := uuidParam(w, r, "threadID")
		if !ok {
			return
		}
		t, err := s.q.TouchThreadActivity(r.Context(), pgUUID(threadID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "touch failed")
			return
		}
		writeJSON(w, http.StatusOK, threadDTO(t, 0, false))
	}
}

func (s *Service) guardThread(resolver permissions.Resolver, uidFn permissions.UserIDFunc, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		threadID, ok := uuidParam(w, r, "threadID")
		if !ok {
			return
		}
		t, err := s.q.GetThread(r.Context(), pgUUID(threadID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "thread not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		mc, err := resolver.ResolveChannel(r.Context(), uid, uuid.UUID(t.ChannelID.Bytes))
		if errors.Is(err, permissions.ErrNotMember) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !permissions.Compute(mc).Has(permissions.ViewChannel) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

const bodyPredicate = `(value LIKE '%<body>%' OR value LIKE '%<body %')`

func (s *Service) replyCounts(ctx context.Context, rooms []string) map[string]int64 {
	out := make(map[string]int64, len(rooms))
	if len(rooms) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `
SELECT "user", count(*) FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = ANY($2) AND `+bodyPredicate+`
 GROUP BY "user"
`, s.mucHost, rooms)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var room string
		var n int64
		if err := rows.Scan(&room, &n); err != nil {
			return out
		}
		out[room] = n
	}
	return out
}

func threadRoom(id pgtype.UUID) string {
	return "thread-" + uuid.UUID(id.Bytes).String()
}

func threadDTO(t db.Thread, replyCount int64, isSubscribed bool) map[string]any {
	return map[string]any{
		"id":               uuid.UUID(t.ID.Bytes).String(),
		"channel_id":       uuid.UUID(t.ChannelID.Bytes).String(),
		"root_archive_id":  t.RootArchiveID,
		"title":            t.Title,
		"created_by":       uuid.UUID(t.CreatedBy.Bytes).String(),
		"created_at":       t.CreatedAt.Time,
		"last_activity_at": t.LastActivityAt.Time,
		"reply_count":      replyCount,
		"is_subscribed":    isSubscribed,
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

func uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
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
