package events

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	r.Route("/spaces/{spaceID}/events", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Post("/", s.handleCreate(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList(userIDFn))
	})
	r.Route("/events/{eventID}", func(rr chi.Router) {
		rr.Use(s.attachEventSpace)
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Post("/rsvp", s.handleRsvp(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Delete("/", s.handleDelete(userIDFn, resolver))
	})
}

func (s *Service) attachEventSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "eventID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid event id")
			return
		}
		ev, err := s.q.GetEvent(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(ev.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Location    *string `json:"location"`
	StartsAt    string  `json:"starts_at"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		actor := uidFn(r.Context())
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" || len(req.Title) > 200 {
			writeError(w, http.StatusBadRequest, "title is required (max 200)")
			return
		}
		t, err := time.Parse(time.RFC3339, req.StartsAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "starts_at must be RFC3339")
			return
		}
		ev, err := s.q.CreateEvent(r.Context(), db.CreateEventParams{
			SpaceID:     pgUUID(spaceID),
			Title:       req.Title,
			Description: trimPtr(req.Description),
			Location:    trimPtr(req.Location),
			StartsAt:    pgtype.Timestamptz{Time: t, Valid: true},
			CreatedBy:   pgUUID(actor),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, eventDTO(ev, nil, ""))
	}
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		actor := uidFn(r.Context())
		evs, err := s.q.ListSpaceEvents(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(evs))
		for _, ev := range evs {
			counts := map[string]int64{}
			if rows, err := s.q.RsvpCounts(r.Context(), ev.ID); err == nil {
				for _, row := range rows {
					counts[row.Status] = row.N
				}
			}
			mine := ""
			if st, err := s.q.GetMyRsvp(r.Context(), db.GetMyRsvpParams{EventID: ev.ID, UserID: pgUUID(actor)}); err == nil {
				mine = st
			}
			out = append(out, eventDTO(ev, counts, mine))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type rsvpReq struct {
	Status string `json:"status"`
}

var validStatus = map[string]bool{"going": true, "maybe": true, "no": true}

func (s *Service) handleRsvp(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid event id")
			return
		}
		actor := uidFn(r.Context())
		var req rsvpReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if !validStatus[req.Status] {
			writeError(w, http.StatusBadRequest, "status must be going, maybe or no")
			return
		}
		if err := s.q.SetRsvp(r.Context(), db.SetRsvpParams{
			EventID: pgUUID(eventID), UserID: pgUUID(actor), Status: req.Status,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "rsvp failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc, resolver permissions.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID, err := uuid.Parse(chi.URLParam(r, "eventID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid event id")
			return
		}
		actor := uidFn(r.Context())
		ev, err := s.q.GetEvent(r.Context(), pgUUID(eventID))
		if err != nil {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		if uuid.UUID(ev.CreatedBy.Bytes) != actor {
			mc, err := resolver.ResolveSpace(r.Context(), actor, uuid.UUID(ev.SpaceID.Bytes))
			if err != nil || !permissions.Compute(mc).Has(permissions.ManageSpace) {
				writeError(w, http.StatusForbidden, "only the creator or a manager may delete this event")
				return
			}
		}
		if err := s.q.DeleteEvent(r.Context(), pgUUID(eventID)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func eventDTO(ev db.Event, counts map[string]int64, mine string) map[string]any {
	get := func(k string) int64 {
		if counts == nil {
			return 0
		}
		return counts[k]
	}
	out := map[string]any{
		"id":         uuid.UUID(ev.ID.Bytes).String(),
		"space_id":   uuid.UUID(ev.SpaceID.Bytes).String(),
		"title":      ev.Title,
		"starts_at":  ev.StartsAt.Time,
		"created_by": uuid.UUID(ev.CreatedBy.Bytes).String(),
		"created_at": ev.CreatedAt.Time,
		"rsvp":       map[string]int64{"going": get("going"), "maybe": get("maybe"), "no": get("no")},
		"my_rsvp":    mine,
	}
	if ev.Description != nil {
		out["description"] = *ev.Description
	}
	if ev.Location != nil {
		out["location"] = *ev.Location
	}
	return out
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
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
