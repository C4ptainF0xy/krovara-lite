package moderation

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const maxTimeoutMinutes = 28 * 24 * 60

func (s *Service) TimeoutRoutes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/timeouts", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.KickMembers)).
			Post("/", s.handleTimeout(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.KickMembers)).
			Delete("/{userID}", s.handleRevokeTimeout(userIDFn))
		rr.Get("/me", s.handleMyTimeout(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.KickMembers)).
			Get("/{userID}", s.handleGetTimeout())
	})
	r.With(permissions.RequireSpace(resolver, userIDFn, permissions.KickMembers)).
		Get("/spaces/{spaceID}/mod-actions", s.handleListModActions())
}

type timeoutReq struct {
	UserID  string `json:"user_id"`
	Minutes int    `json:"minutes"`
	Reason  string `json:"reason"`
}

func (s *Service) handleTimeout(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req timeoutReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		target, err := uuid.Parse(req.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if req.Minutes < 1 || req.Minutes > maxTimeoutMinutes {
			writeError(w, http.StatusBadRequest, "invalid duration")
			return
		}
		actor := uidFn(r.Context())
		if target == actor {
			writeError(w, http.StatusBadRequest, "cannot time out yourself")
			return
		}

		sp, err := s.q.GetSpace(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if sp.OwnerID.Valid && uuid.UUID(sp.OwnerID.Bytes) == target {
			writeError(w, http.StatusForbidden, "cannot time out the owner")
			return
		}

		_ = s.q.RevokeTimeout(r.Context(), db.RevokeTimeoutParams{
			SpaceID: pgUUID(spaceID), TargetUser: pgUUID(target),
		})
		expires := time.Now().Add(time.Duration(req.Minutes) * time.Minute)
		var reason *string
		if req.Reason != "" {
			reason = &req.Reason
		}
		act, err := s.q.CreateModAction(r.Context(), db.CreateModActionParams{
			SpaceID:     pgUUID(spaceID),
			TargetUser:  pgUUID(target),
			ModeratorID: pgUUID(actor),
			Action:      "timeout",
			Reason:      reason,
			ExpiresAt:   pgtype.Timestamptz{Time: expires, Valid: true},
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "timeout failed")
			return
		}
		meta, _ := json.Marshal(map[string]any{"target": target.String(), "minutes": req.Minutes})
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "member.timeout", meta)
		writeJSON(w, http.StatusOK, modActionDTO(act))
	}
}

func (s *Service) handleRevokeTimeout(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		target, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if err := s.q.RevokeTimeout(r.Context(), db.RevokeTimeoutParams{
			SpaceID: pgUUID(spaceID), TargetUser: pgUUID(target),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "revoke failed")
			return
		}
		meta, _ := json.Marshal(map[string]any{"target": target.String()})
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "member.timeout_lift", meta)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleMyTimeout(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		act, err := s.q.GetActiveTimeout(r.Context(), db.GetActiveTimeoutParams{
			SpaceID: pgUUID(spaceID), TargetUser: pgUUID(uidFn(r.Context())),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"active": false})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		out := modActionDTO(act)
		out["active"] = true
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleGetTimeout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		target, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		act, err := s.q.GetActiveTimeout(r.Context(), db.GetActiveTimeoutParams{
			SpaceID: pgUUID(spaceID), TargetUser: pgUUID(target),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"active": false})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		out := modActionDTO(act)
		out["active"] = true
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleListModActions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceModActions(r.Context(), db.ListSpaceModActionsParams{
			SpaceID: pgUUID(spaceID), Limit: 200,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, a := range rows {
			out = append(out, modActionDTO(a))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func modActionDTO(a db.ModAction) map[string]any {
	out := map[string]any{
		"id":          uuid.UUID(a.ID.Bytes).String(),
		"space_id":    uuid.UUID(a.SpaceID.Bytes).String(),
		"target_user": uuid.UUID(a.TargetUser.Bytes).String(),
		"action":      a.Action,
		"reason":      a.Reason,
		"is_active":   a.Active,
		"created_at":  a.CreatedAt.Time,
	}
	if a.ModeratorID.Valid {
		out["moderator_id"] = uuid.UUID(a.ModeratorID.Bytes).String()
	}
	if a.ExpiresAt.Valid {
		out["expires_at"] = a.ExpiresAt.Time
	}
	return out
}
