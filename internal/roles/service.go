package roles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

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
	ErrNotFound       = errors.New("roles: not found")
	ErrEveryoneLocked = errors.New("roles: @everyone cannot be modified this way")
	ErrHierarchy      = errors.New("roles: target role is at or above caller's highest role")
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	v    *validator.Validate
}

func NewService(pool *pgxpool.Pool) *Service {
	v := validator.New(validator.WithRequiredStructEnabled())
	_ = v.RegisterValidation("hexcolor6", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		return s == "" || hexColor.MatchString(s)
	})
	return &Service{pool: pool, q: db.New(pool), v: v}
}

func emitRolesUpdate(ctx context.Context, pool *pgxpool.Pool, spaceID uuid.UUID) {
	_ = eventsfeed.Emit(ctx, pool, uuid.Nil, "space_update", map[string]any{
		"space_id": spaceID.String(),
		"what":     "roles",
	})
}

func emitMembersUpdate2(ctx context.Context, pool *pgxpool.Pool, spaceID uuid.UUID) {
	_ = eventsfeed.Emit(ctx, pool, uuid.Nil, "space_update", map[string]any{
		"space_id": spaceID.String(),
		"what":     "members",
	})
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/roles", func(rr chi.Router) {
		rr.Get("/", s.handleList())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Post("/", s.handleCreate(userIDFn))
	})
	r.Route("/roles/{roleID}", func(rr chi.Router) {
		rr.Use(s.attachRoleSpace)
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Patch("/", s.handleUpdate(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Delete("/", s.handleDelete(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Get("/members", s.handleListRoleMembers())

		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageRoles)).
			Post("/members", s.handleBulkAssign(userIDFn))
	})
}

type bulkAssignReq struct {
	MemberIDs []string `json:"member_ids" validate:"omitempty,dive,uuid4"`
	Everyone  bool     `json:"everyone"`
	Action    string   `json:"action" validate:"required,oneof=add remove"`

	ExpiresInSeconds int64 `json:"expires_in_seconds" validate:"omitempty,min=0,max=31536000"`
}

func (s *Service) handleListRoleMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		rows, err := s.q.ListRoleMembers(r.Context(), pgUUID(roleID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, m := range rows {
			out = append(out, map[string]any{
				"member_id": uuid.UUID(m.MemberID.Bytes).String(),
				"user_id":   uuid.UUID(m.UserID.Bytes).String(),
				"username":  m.Username,
				"nickname":  m.Nickname,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleBulkAssign(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		role, err := s.q.GetRole(r.Context(), pgUUID(roleID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if role.IsEveryone != nil && *role.IsEveryone {
			writeError(w, http.StatusBadRequest, "@everyone is assigned implicitly")
			return
		}
		var req bulkAssignReq
		if !s.decode(w, r, &req) {
			return
		}

		curPos := int32(0)
		if role.Position != nil {
			curPos = *role.Position
		}
		if err := s.checkHierarchy(r.Context(), uidFn(r.Context()), uuid.UUID(role.SpaceID.Bytes), curPos); err != nil {
			writeHierarchyErr(w, err)
			return
		}

		var memberIDs []uuid.UUID
		if req.Everyone {
			ids, err := s.q.ListSpaceMemberIDs(r.Context(), role.SpaceID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "member list failed")
				return
			}
			for _, id := range ids {
				memberIDs = append(memberIDs, uuid.UUID(id.Bytes))
			}
		} else {
			for _, raw := range req.MemberIDs {
				id, err := uuid.Parse(raw)
				if err != nil {
					writeError(w, http.StatusBadRequest, "invalid member id")
					return
				}
				memberIDs = append(memberIDs, id)
			}
		}

		var expiresAt pgtype.Timestamptz
		if req.Action == "add" && req.ExpiresInSeconds > 0 {
			expiresAt = pgtype.Timestamptz{
				Time:  time.Now().Add(time.Duration(req.ExpiresInSeconds) * time.Second),
				Valid: true,
			}
		}
		var n int
		for _, mid := range memberIDs {
			if req.Action == "add" {
				if err := s.q.AssignMemberRole(r.Context(), db.AssignMemberRoleParams{
					MemberID: pgUUID(mid), RoleID: pgUUID(roleID), ExpiresAt: expiresAt,
				}); err != nil {
					writeError(w, http.StatusInternalServerError, "assign failed")
					return
				}
			} else {
				if err := s.q.RemoveMemberRole(r.Context(), db.RemoveMemberRoleParams{
					MemberID: pgUUID(mid), RoleID: pgUUID(roleID),
				}); err != nil {
					writeError(w, http.StatusInternalServerError, "remove failed")
					return
				}
			}
			n++
		}
		meta, _ := json.Marshal(map[string]any{"action": req.Action, "count": n})
		s.logAudit(r.Context(), role.SpaceID, uidFn(r.Context()), "role.bulk_assign", role.ID, meta)
		emitRolesUpdate(r.Context(), s.pool, uuid.UUID(role.SpaceID.Bytes))
		emitMembersUpdate2(r.Context(), s.pool, uuid.UUID(role.SpaceID.Bytes))
		writeJSON(w, http.StatusOK, map[string]any{"affected": n})
	}
}

func (s *Service) attachRoleSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		role, err := s.q.GetRole(r.Context(), pgUUID(roleID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(role.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Name        string  `json:"name"        validate:"required,min=1,max=64"`
	Permissions *int64  `json:"permissions"`
	Color       *string `json:"color"       validate:"omitempty,hexcolor6"`
	Position    *int32  `json:"position"`
}

type updateReq struct {
	Name        *string `json:"name"        validate:"omitempty,min=1,max=64"`
	Permissions *int64  `json:"permissions"`
	Color       *string `json:"color"       validate:"omitempty,hexcolor6"`
	Position    *int32  `json:"position"`
	Hoist       *bool   `json:"hoist"`
	Mentionable *bool   `json:"mentionable"`
	IconEmoji   *string `json:"icon_emoji"  validate:"omitempty,max=16"`
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceRoles(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			out = append(out, roleDTO(r))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
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

		if req.Position != nil {
			if err := s.checkHierarchy(r.Context(), uidFn(r.Context()), spaceID, *req.Position); err != nil {
				writeHierarchyErr(w, err)
				return
			}
		}
		isEv := false
		role, err := s.q.CreateRole(r.Context(), db.CreateRoleParams{
			SpaceID:     pgUUID(spaceID),
			Name:        req.Name,
			Permissions: req.Permissions,
			Color:       req.Color,
			Position:    req.Position,
			IsEveryone:  &isEv,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "role.create", role.ID, nil)
		emitRolesUpdate(r.Context(), s.pool, spaceID)
		writeJSON(w, http.StatusCreated, roleDTO(role))
	}
}

func (s *Service) handleUpdate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		role, err := s.q.GetRole(r.Context(), pgUUID(roleID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var req updateReq
		if !s.decode(w, r, &req) {
			return
		}

		if role.IsEveryone != nil && *role.IsEveryone {
			if req.Name != nil || req.Color != nil || req.Position != nil ||
				req.Hoist != nil || req.Mentionable != nil || req.IconEmoji != nil {
				writeError(w, http.StatusBadRequest, "@everyone only accepts permissions changes")
				return
			}
		}
		spaceID := uuid.UUID(role.SpaceID.Bytes)

		curPos := int32(0)
		if role.Position != nil {
			curPos = *role.Position
		}
		if err := s.checkHierarchy(r.Context(), uidFn(r.Context()), spaceID, curPos); err != nil {
			writeHierarchyErr(w, err)
			return
		}

		if req.Position != nil {
			if err := s.checkHierarchy(r.Context(), uidFn(r.Context()), spaceID, *req.Position); err != nil {
				writeHierarchyErr(w, err)
				return
			}
		}
		updated, err := s.q.UpdateRole(r.Context(), db.UpdateRoleParams{
			ID:          pgUUID(roleID),
			Name:        req.Name,
			Permissions: req.Permissions,
			Color:       req.Color,
			Position:    req.Position,
			Hoist:       req.Hoist,
			Mentionable: req.Mentionable,
			IconEmoji:   req.IconEmoji,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.logAudit(r.Context(), role.SpaceID, uidFn(r.Context()), "role.update", role.ID, nil)
		emitRolesUpdate(r.Context(), s.pool, spaceID)
		writeJSON(w, http.StatusOK, roleDTO(updated))
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleID, err := uuid.Parse(chi.URLParam(r, "roleID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role id")
			return
		}
		role, err := s.q.GetRole(r.Context(), pgUUID(roleID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "role not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if role.IsEveryone != nil && *role.IsEveryone {
			writeError(w, http.StatusBadRequest, "@everyone cannot be deleted")
			return
		}
		curPos := int32(0)
		if role.Position != nil {
			curPos = *role.Position
		}
		if err := s.checkHierarchy(r.Context(), uidFn(r.Context()), uuid.UUID(role.SpaceID.Bytes), curPos); err != nil {
			writeHierarchyErr(w, err)
			return
		}
		if err := s.q.DeleteRole(r.Context(), pgUUID(roleID)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.logAudit(r.Context(), role.SpaceID, uidFn(r.Context()), "role.delete", role.ID, nil)
		emitRolesUpdate(r.Context(), s.pool, uuid.UUID(role.SpaceID.Bytes))
		emitMembersUpdate2(r.Context(), s.pool, uuid.UUID(role.SpaceID.Bytes))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) checkHierarchy(ctx context.Context, actorID, spaceID uuid.UUID, targetPos int32) error {
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

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, targetID pgtype.UUID, meta []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID: spaceID, ActorID: pgUUID(actorID), Action: action, TargetID: targetID, Metadata: meta,
	})
}

func roleDTO(r db.Role) map[string]any {
	return map[string]any{
		"id":          uuid.UUID(r.ID.Bytes).String(),
		"space_id":    uuid.UUID(r.SpaceID.Bytes).String(),
		"name":        r.Name,
		"permissions": r.Permissions,
		"color":       r.Color,
		"position":    r.Position,
		"is_everyone": r.IsEveryone,
		"hoist":       r.Hoist,
		"mentionable": r.Mentionable,
		"icon_emoji":  r.IconEmoji,
	}
}

func writeHierarchyErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrHierarchy) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "hierarchy check failed")
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
