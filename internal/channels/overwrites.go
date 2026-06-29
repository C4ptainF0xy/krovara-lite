package channels

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

func (s *Service) OverwriteRoutes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/channels/{channelID}/overwrites", func(rr chi.Router) {
		rr.Use(permissions.RequireChannel(resolver, userIDFn, permissions.ManageRoles))
		rr.Get("/", s.handleListOverwrites())
		rr.Put("/role/{roleID}", s.handleUpsertRoleOverwrite(userIDFn))
		rr.Delete("/role/{roleID}", s.handleDeleteRoleOverwrite(userIDFn))
		rr.Put("/member/{memberID}", s.handleUpsertMemberOverwrite(userIDFn))
		rr.Delete("/member/{memberID}", s.handleDeleteMemberOverwrite(userIDFn))
	})
}

type overwriteReq struct {
	Allow int64 `json:"allow"`
	Deny  int64 `json:"deny"`
}

func validBits(allow, deny int64) bool {
	all := permissions.All.ToInt64()
	return allow&^all == 0 && deny&^all == 0
}

func (s *Service) handleListOverwrites() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		rows, err := s.q.ListChannelOverwrites(r.Context(), pgUUID(id))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, ow := range rows {
			out = append(out, overwriteDTO(ow))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleUpsertRoleOverwrite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chID, ok := parseChannelID(w, r)
		if !ok {
			return
		}
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		var req overwriteReq
		if !s.decode(w, r, &req) {
			return
		}
		if !validBits(req.Allow, req.Deny) {
			writeError(w, http.StatusBadRequest, "permission bits out of range")
			return
		}
		ow, err := s.q.UpsertRoleOverwrite(r.Context(), db.UpsertRoleOverwriteParams{
			ChannelID: pgUUID(chID),
			RoleID:    pgUUID(roleID),
			Allow:     &req.Allow,
			Deny:      &req.Deny,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upsert failed")
			return
		}
		s.auditOverwrite(r, uidFn, chID, "channel.overwrite.role.set", pgUUID(roleID))
		writeJSON(w, http.StatusOK, overwriteDTO(ow))
	}
}

func (s *Service) handleDeleteRoleOverwrite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chID, ok := parseChannelID(w, r)
		if !ok {
			return
		}
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		if err := s.q.DeleteRoleOverwrite(r.Context(), db.DeleteRoleOverwriteParams{
			ChannelID: pgUUID(chID),
			RoleID:    pgUUID(roleID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.auditOverwrite(r, uidFn, chID, "channel.overwrite.role.clear", pgUUID(roleID))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleUpsertMemberOverwrite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chID, ok := parseChannelID(w, r)
		if !ok {
			return
		}
		memberID, err := uuid.Parse(chi.URLParam(r, "memberID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid member id")
			return
		}
		var req overwriteReq
		if !s.decode(w, r, &req) {
			return
		}
		if !validBits(req.Allow, req.Deny) {
			writeError(w, http.StatusBadRequest, "permission bits out of range")
			return
		}
		ow, err := s.q.UpsertMemberOverwrite(r.Context(), db.UpsertMemberOverwriteParams{
			ChannelID: pgUUID(chID),
			MemberID:  pgUUID(memberID),
			Allow:     &req.Allow,
			Deny:      &req.Deny,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upsert failed")
			return
		}
		s.auditOverwrite(r, uidFn, chID, "channel.overwrite.member.set", pgUUID(memberID))
		writeJSON(w, http.StatusOK, overwriteDTO(ow))
	}
}

func (s *Service) handleDeleteMemberOverwrite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chID, ok := parseChannelID(w, r)
		if !ok {
			return
		}
		memberID, err := uuid.Parse(chi.URLParam(r, "memberID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid member id")
			return
		}
		if err := s.q.DeleteMemberOverwrite(r.Context(), db.DeleteMemberOverwriteParams{
			ChannelID: pgUUID(chID),
			MemberID:  pgUUID(memberID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.auditOverwrite(r, uidFn, chID, "channel.overwrite.member.clear", pgUUID(memberID))
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseChannelID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Service) auditOverwrite(r *http.Request, uidFn permissions.UserIDFunc, chID uuid.UUID, action string, target pgtype.UUID) {
	ch, err := s.q.GetChannel(r.Context(), pgUUID(chID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		return
	}
	s.logAudit(r.Context(), ch.SpaceID, uidFn(r.Context()), action, target, nil)
}

func overwriteDTO(ow db.ChannelOverwrite) map[string]any {
	out := map[string]any{
		"allow": int64Val(ow.Allow),
		"deny":  int64Val(ow.Deny),
	}
	if ow.RoleID.Valid {
		out["target_type"] = "role"
		out["target_id"] = uuid.UUID(ow.RoleID.Bytes).String()
	} else if ow.MemberID.Valid {
		out["target_type"] = "member"
		out["target_id"] = uuid.UUID(ow.MemberID.Bytes).String()
	}
	return out
}

func int64Val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
