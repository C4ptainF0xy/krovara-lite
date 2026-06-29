package members

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/permissions"
)

var (
	ErrNotFound       = errors.New("members: not found")
	ErrCrossSpace     = errors.New("members: role belongs to a different space")
	ErrHierarchy      = errors.New("members: target outranks caller")
	ErrEveryoneAssign = errors.New("members: cannot assign @everyone")
	ErrSelfKick       = errors.New("members: cannot kick yourself; use leave instead")
	ErrOwnerKick      = errors.New("members: cannot kick the space owner")
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	v    *validator.Validate
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
		q:    db.New(pool),
		v:    validator.New(validator.WithRequiredStructEnabled()),
	}
}

func emitMembersUpdate(ctx context.Context, pool *pgxpool.Pool, spaceID uuid.UUID) {
	_ = eventsfeed.Emit(ctx, pool, uuid.Nil, "space_update", map[string]any{
		"space_id": spaceID.String(),
		"what":     "members",
	})
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/members", func(rr chi.Router) {
		rr.Get("/", s.handleList())
		rr.Put("/me/profile", s.handlePutMyProfile(userIDFn))
		rr.Post("/me/leave", s.handleLeave(userIDFn))
		rr.Patch("/{userID}", s.handlePatchNickname(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.KickMembers)).
			Delete("/{userID}", s.handleKick(userIDFn))
	})

	r.With(s.attachMemberSpace).
		With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
		Get("/members/{memberID}/roles", s.handleListMemberRoles())
	r.Route("/members/{memberID}/roles/{roleID}", func(rr chi.Router) {
		rr.Use(s.attachMemberSpace)
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Put("/", s.handleAssignRole(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Delete("/", s.handleRemoveRole(userIDFn))
	})
}

func (s *Service) handleListMemberRoles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memID, err := uuid.Parse(chi.URLParam(r, "memberID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid member id")
			return
		}
		rows, err := s.q.ListMemberRoles(r.Context(), pgUUID(memID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		ids := make([]string, 0, len(rows))
		for _, role := range rows {
			ids = append(ids, uuid.UUID(role.ID.Bytes).String())
		}
		writeJSON(w, http.StatusOK, map[string]any{"role_ids": ids})
	}
}

func (s *Service) handleLeave(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		actor := uidFn(r.Context())
		sp, err := s.q.GetSpace(r.Context(), pgUUID(spaceID))
		if err == nil && sp.OwnerID.Valid && uuid.UUID(sp.OwnerID.Bytes) == actor {
			writeError(w, http.StatusForbidden, "the owner cannot leave; transfer or delete the space")
			return
		}
		mem, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if err := s.q.DeleteMember(r.Context(), mem.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "leave failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "member.leave", mem.ID, nil)
		emitMembersUpdate(r.Context(), s.pool, spaceID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) attachMemberSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		memID, err := uuid.Parse(chi.URLParam(r, "memberID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid member id")
			return
		}
		mem, err := s.q.GetMember(r.Context(), pgUUID(memID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(mem.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type patchNicknameReq struct {
	Nickname *string `json:"nickname" validate:"omitempty,max=32"`
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceMembers(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, m := range rows {
			out = append(out, map[string]any{
				"id":             uuid.UUID(m.ID.Bytes).String(),
				"space_id":       uuid.UUID(m.SpaceID.Bytes).String(),
				"user_id":        uuid.UUID(m.UserID.Bytes).String(),
				"username":       m.Username,
				"nickname":       m.Nickname,
				"avatar_key":     m.AvatarKey,
				"bio":            m.Bio,
				"role_color":     m.RoleColor,
				"role_icon":      m.RoleIcon,
				"hoist_role":     m.HoistRole,
				"hoist_position": m.HoistPosition,
				"joined_at":      m.JoinedAt.Time,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handlePatchNickname(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req patchNicknameReq
		if !s.decode(w, r, &req) {
			return
		}

		mem, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID),
			UserID:  pgUUID(targetUserID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if actor != targetUserID {
			mc, err := permissions.NewPGResolver(s.q).ResolveSpace(r.Context(), actor, spaceID)
			if err != nil || !permissions.Compute(mc).Has(permissions.ManageSpace) {
				writeError(w, http.StatusForbidden, "cannot rename another member")
				return
			}
		}

		updated, err := s.q.UpdateMemberNickname(r.Context(), db.UpdateMemberNicknameParams{
			ID:       mem.ID,
			Nickname: req.Nickname,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "member.nickname", mem.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       uuid.UUID(updated.ID.Bytes).String(),
			"user_id":  uuid.UUID(updated.UserID.Bytes).String(),
			"nickname": updated.Nickname,
		})
	}
}

type putMyProfileReq struct {
	Nickname  *string `json:"nickname"   validate:"omitempty,max=32"`
	AvatarKey *string `json:"avatar_key" validate:"omitempty,max=255"`
	Bio       *string `json:"bio"        validate:"omitempty,max=300"`
}

func (s *Service) handlePutMyProfile(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req putMyProfileReq
		if !s.decode(w, r, &req) {
			return
		}
		if _, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
		}); errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusForbidden, "not a member of this space")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		updated, err := s.q.UpdateMemberSpaceProfile(r.Context(), db.UpdateMemberSpaceProfileParams{
			SpaceID:   pgUUID(spaceID),
			UserID:    pgUUID(actor),
			Nickname:  req.Nickname,
			AvatarKey: req.AvatarKey,
			Bio:       req.Bio,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "member.profile", updated.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":         uuid.UUID(updated.ID.Bytes).String(),
			"user_id":    uuid.UUID(updated.UserID.Bytes).String(),
			"nickname":   updated.Nickname,
			"avatar_key": updated.AvatarKey,
			"bio":        updated.Bio,
		})
	}
}

func (s *Service) handleKick(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		targetUserID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		actor := uidFn(r.Context())
		if actor == targetUserID {
			writeError(w, http.StatusBadRequest, ErrSelfKick.Error())
			return
		}
		sp, err := s.q.GetSpace(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "space lookup failed")
			return
		}
		if sp.OwnerID.Valid && uuid.UUID(sp.OwnerID.Bytes) == targetUserID {
			writeError(w, http.StatusForbidden, ErrOwnerKick.Error())
			return
		}
		target, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID),
			UserID:  pgUUID(targetUserID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if err := s.checkHierarchyMember(r.Context(), actor, spaceID, target.ID); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := s.q.DeleteMember(r.Context(), target.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "kick failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "member.kick", target.ID, nil)
		emitMembersUpdate(r.Context(), s.pool, spaceID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleAssignRole(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memID, roleID, mem, role, ok := s.resolveMemberRole(w, r)
		if !ok {
			return
		}
		if role.IsEveryone != nil && *role.IsEveryone {
			writeError(w, http.StatusBadRequest, ErrEveryoneAssign.Error())
			return
		}
		actor := uidFn(r.Context())
		spaceID := uuid.UUID(mem.SpaceID.Bytes)
		pos := int32(0)
		if role.Position != nil {
			pos = *role.Position
		}
		if err := s.checkHierarchyPosition(r.Context(), actor, spaceID, pos); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := s.q.AssignMemberRole(r.Context(), db.AssignMemberRoleParams{
			MemberID: pgUUID(memID),
			RoleID:   pgUUID(roleID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "assign failed")
			return
		}
		s.logAudit(r.Context(), mem.SpaceID, actor, "member.role.assign", pgUUID(roleID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleRemoveRole(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memID, roleID, mem, role, ok := s.resolveMemberRole(w, r)
		if !ok {
			return
		}
		if role.IsEveryone != nil && *role.IsEveryone {
			writeError(w, http.StatusBadRequest, ErrEveryoneAssign.Error())
			return
		}
		actor := uidFn(r.Context())
		spaceID := uuid.UUID(mem.SpaceID.Bytes)
		pos := int32(0)
		if role.Position != nil {
			pos = *role.Position
		}
		if err := s.checkHierarchyPosition(r.Context(), actor, spaceID, pos); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := s.q.RemoveMemberRole(r.Context(), db.RemoveMemberRoleParams{
			MemberID: pgUUID(memID),
			RoleID:   pgUUID(roleID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "remove failed")
			return
		}
		s.logAudit(r.Context(), mem.SpaceID, actor, "member.role.remove", pgUUID(roleID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) resolveMemberRole(w http.ResponseWriter, r *http.Request) (memID, roleID uuid.UUID, mem db.Member, role db.Role, ok bool) {
	memID, err := uuid.Parse(chi.URLParam(r, "memberID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member id")
		return
	}
	roleID, err = uuid.Parse(chi.URLParam(r, "roleID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid role id")
		return
	}
	mem, err = s.q.GetMember(r.Context(), pgUUID(memID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member lookup failed")
		return
	}
	role, err = s.q.GetRole(r.Context(), pgUUID(roleID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "role lookup failed")
		return
	}
	if mem.SpaceID != role.SpaceID {
		writeError(w, http.StatusBadRequest, ErrCrossSpace.Error())
		return
	}
	ok = true
	return
}

func (s *Service) checkHierarchyPosition(ctx context.Context, actorID, spaceID uuid.UUID, targetPos int32) error {
	sp, err := s.q.GetSpace(ctx, pgUUID(spaceID))
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	if sp.OwnerID.Valid && uuid.UUID(sp.OwnerID.Bytes) == actorID {
		return nil
	}
	mem, err := s.q.GetMemberByUser(ctx, db.GetMemberByUserParams{
		SpaceID: pgUUID(spaceID),
		UserID:  pgUUID(actorID),
	})
	if err != nil {
		return ErrHierarchy
	}
	maxPos, err := s.q.GetMemberMaxRolePosition(ctx, mem.ID)
	if err != nil {
		return fmt.Errorf("max role: %w", err)
	}
	if targetPos >= maxPos {
		return ErrHierarchy
	}
	return nil
}

func (s *Service) checkHierarchyMember(ctx context.Context, actorID, spaceID uuid.UUID, targetMemberID pgtype.UUID) error {
	targetMax, err := s.q.GetMemberMaxRolePosition(ctx, targetMemberID)
	if err != nil {
		return fmt.Errorf("target max role: %w", err)
	}
	return s.checkHierarchyPosition(ctx, actorID, spaceID, targetMax)
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, targetID pgtype.UUID, meta []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID: spaceID, ActorID: pgUUID(actorID), Action: action, TargetID: targetID, Metadata: meta,
	})
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

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
