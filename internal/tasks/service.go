package tasks

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/tasks", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Post("/", s.handleCreate(userIDFn))
	})
	r.Route("/tasks/{taskID}", func(rr chi.Router) {
		rr.Use(s.attachTaskSpace)
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Patch("/", s.handleUpdate())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Delete("/", s.handleDelete())
	})
}

func (s *Service) attachTaskSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		t, err := s.q.GetTask(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(t.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Title           string  `json:"title"`
	ChannelID       *string `json:"channel_id"`
	SourceArchiveID *string `json:"source_archive_id"`
	AssigneeID      *string `json:"assignee_id"`
	DueAt           *string `json:"due_at"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len(req.Title) < 1 || len(req.Title) > 280 {
			writeError(w, http.StatusBadRequest, "title length")
			return
		}
		params := db.CreateTaskParams{
			SpaceID:         pgUUID(spaceID),
			Title:           req.Title,
			CreatedBy:       pgUUID(uidFn(r.Context())),
			SourceArchiveID: req.SourceArchiveID,
		}
		if req.ChannelID != nil {
			if id, err := uuid.Parse(*req.ChannelID); err == nil {
				params.ChannelID = pgUUID(id)
			}
		}
		if req.AssigneeID != nil {
			if id, err := uuid.Parse(*req.AssigneeID); err == nil {
				params.AssigneeID = pgUUID(id)
			}
		}
		if req.DueAt != nil {
			if t, err := time.Parse(time.RFC3339, *req.DueAt); err == nil {
				params.DueAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
		t, err := s.q.CreateTask(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, taskDTO(t))
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceTasks(r.Context(), db.ListSpaceTasksParams{
			SpaceID: pgUUID(spaceID), Limit: 200,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, t := range rows {
			out = append(out, taskDTO(t))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type updateReq struct {
	Title      *string `json:"title"`
	Status     *string `json:"status"`
	AssigneeID *string `json:"assignee_id"`
	DueAt      *string `json:"due_at"`
}

func (s *Service) handleUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		var req updateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Status != nil && *req.Status != "open" && *req.Status != "done" {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		params := db.UpdateTaskParams{ID: pgUUID(id), Title: req.Title, Status: req.Status}
		if req.AssigneeID != nil {
			if aid, err := uuid.Parse(*req.AssigneeID); err == nil {
				params.AssigneeID = pgUUID(aid)
			}
		}
		if req.DueAt != nil {
			if t, err := time.Parse(time.RFC3339, *req.DueAt); err == nil {
				params.DueAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
		t, err := s.q.UpdateTask(r.Context(), params)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, taskDTO(t))
	}
}

func (s *Service) handleDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "taskID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task id")
			return
		}
		if err := s.q.DeleteTask(r.Context(), pgUUID(id)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func taskDTO(t db.Task) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(t.ID.Bytes).String(),
		"space_id":   uuid.UUID(t.SpaceID.Bytes).String(),
		"title":      t.Title,
		"status":     t.Status,
		"created_at": t.CreatedAt.Time,
	}
	if t.ChannelID.Valid {
		out["channel_id"] = uuid.UUID(t.ChannelID.Bytes).String()
	}
	if t.AssigneeID.Valid {
		out["assignee_id"] = uuid.UUID(t.AssigneeID.Bytes).String()
	}
	if t.SourceArchiveID != nil {
		out["source_archive_id"] = *t.SourceArchiveID
	}
	if t.DueAt.Valid {
		out["due_at"] = t.DueAt.Time
	}
	return out
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
