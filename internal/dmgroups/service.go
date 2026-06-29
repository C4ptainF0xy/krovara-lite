package dmgroups

import (
	"context"
	"crypto/rand"
	"encoding/base32"
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
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/permissions"
)

const maxMembers = 10

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, uidFn permissions.UserIDFunc) {
	r.Route("/dm-groups", func(rr chi.Router) {
		rr.Get("/", s.handleList(uidFn))
		rr.Post("/", s.handleCreate(uidFn))
		rr.Post("/join/{code}", s.handleJoin(uidFn))
		rr.Get("/{id}", s.handleGet(uidFn))
		rr.Patch("/{id}", s.handleUpdate(uidFn))
		rr.Post("/{id}/leave", s.handleLeave(uidFn))
		rr.Post("/{id}/transfer", s.handleTransfer(uidFn))
		rr.Get("/{id}/messages", s.handleListMessages(uidFn))
		rr.Post("/{id}/messages", s.handleSendMessage(uidFn))
		rr.Delete("/{id}/members/{userID}", s.handleKick(uidFn))
		rr.Get("/{id}/invites", s.handleListInvites(uidFn))
		rr.Post("/{id}/invites", s.handleCreateInvite(uidFn))
		rr.Delete("/{id}/invites/{code}", s.handleRevokeInvite(uidFn))
	})
}

func (s *Service) notifyMembers(ctx context.Context, groupID uuid.UUID, evt string, data map[string]any) {
	members, err := s.q.ListDMGroupMembers(ctx, pgUUID(groupID))
	if err != nil {
		return
	}
	for _, m := range members {
		_ = eventsfeed.Emit(ctx, s.pool, uuid.UUID(m.ID.Bytes), evt, data)
	}
}

func (s *Service) isMember(ctx context.Context, groupID, userID uuid.UUID) bool {
	ok, _ := s.q.IsDMGroupMember(ctx, db.IsDMGroupMemberParams{GroupID: pgUUID(groupID), UserID: pgUUID(userID)})
	return ok
}

func groupDTO(g db.DmGroup) map[string]any {
	return map[string]any{
		"id":       uuid.UUID(g.ID.Bytes).String(),
		"owner_id": uuid.UUID(g.OwnerID.Bytes).String(),
		"name":     g.Name,
		"icon_key": g.IconKey,
	}
}

type createReq struct {
	Name      *string  `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		ids := map[uuid.UUID]struct{}{me: {}}
		for _, raw := range req.MemberIDs {
			if id, err := uuid.Parse(raw); err == nil {
				ids[id] = struct{}{}
			}
		}
		if len(ids) > maxMembers {
			writeError(w, http.StatusBadRequest, "un groupe est limité à 10 membres")
			return
		}

		var g db.DmGroup
		err := pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			var cerr error
			g, cerr = qtx.CreateDMGroup(r.Context(), db.CreateDMGroupParams{OwnerID: pgUUID(me), Name: req.Name})
			if cerr != nil {
				return cerr
			}
			for id := range ids {
				if aerr := qtx.AddDMGroupMember(r.Context(), db.AddDMGroupMemberParams{GroupID: g.ID, UserID: pgUUID(id)}); aerr != nil {
					return aerr
				}
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		s.notifyMembers(r.Context(), uuid.UUID(g.ID.Bytes), "group_update", map[string]any{"group_id": uuid.UUID(g.ID.Bytes).String(), "kind": "created"})
		writeJSON(w, http.StatusCreated, groupDTO(g))
	}
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		rows, err := s.q.ListMyDMGroups(r.Context(), pgUUID(me))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, g := range rows {
			out = append(out, map[string]any{
				"id":           uuid.UUID(g.ID.Bytes).String(),
				"owner_id":     uuid.UUID(g.OwnerID.Bytes).String(),
				"name":         g.Name,
				"icon_key":     g.IconKey,
				"member_count": g.MemberCount,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleGet(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if !s.isMember(r.Context(), gid, me) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		members, _ := s.q.ListDMGroupMembers(r.Context(), pgUUID(gid))
		mout := make([]map[string]any, 0, len(members))
		for _, m := range members {
			mout = append(mout, map[string]any{
				"id":           uuid.UUID(m.ID.Bytes).String(),
				"username":     m.Username,
				"display_name": m.DisplayName,
				"avatar_key":   m.AvatarKey,
			})
		}
		out := groupDTO(g)
		out["members"] = mout
		writeJSON(w, http.StatusOK, out)
	}
}

type updateReq struct {
	Name    *string `json:"name"`
	IconKey *string `json:"icon_key"`
}

func (s *Service) handleUpdate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		if !s.isMember(r.Context(), gid, me) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		var req updateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		updated, err := s.q.UpdateDMGroup(r.Context(), db.UpdateDMGroupParams{ID: g.ID, Name: req.Name, IconKey: req.IconKey})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.notifyMembers(r.Context(), gid, "group_update", map[string]any{"group_id": gid.String(), "kind": "meta"})
		writeJSON(w, http.StatusOK, groupDTO(updated))
	}
}

func (s *Service) handleLeave(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		_ = s.q.RemoveDMGroupMember(r.Context(), db.RemoveDMGroupMemberParams{GroupID: pgUUID(gid), UserID: pgUUID(me)})

		if uuid.UUID(g.OwnerID.Bytes) == me {
			members, _ := s.q.ListDMGroupMembers(r.Context(), pgUUID(gid))
			if len(members) == 0 {
				_ = s.q.DeleteDMGroup(r.Context(), pgUUID(gid))
			} else {
				_ = s.q.TransferDMGroup(r.Context(), db.TransferDMGroupParams{ID: pgUUID(gid), OwnerID: members[0].ID})
			}
		}
		s.notifyMembers(r.Context(), gid, "group_update", map[string]any{"group_id": gid.String(), "kind": "members"})
		_ = eventsfeed.Emit(r.Context(), s.pool, me, "group_update", map[string]any{"group_id": gid.String(), "kind": "left"})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleKick(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		target, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if uuid.UUID(g.OwnerID.Bytes) != me {
			writeError(w, http.StatusForbidden, "owner only")
			return
		}
		if target == me {
			writeError(w, http.StatusBadRequest, "use leave")
			return
		}
		_ = s.q.RemoveDMGroupMember(r.Context(), db.RemoveDMGroupMemberParams{GroupID: pgUUID(gid), UserID: pgUUID(target)})
		s.notifyMembers(r.Context(), gid, "group_update", map[string]any{"group_id": gid.String(), "kind": "members"})
		_ = eventsfeed.Emit(r.Context(), s.pool, target, "group_update", map[string]any{"group_id": gid.String(), "kind": "kicked"})
		w.WriteHeader(http.StatusNoContent)
	}
}

type transferReq struct {
	UserID string `json:"user_id"`
}

func (s *Service) handleTransfer(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if uuid.UUID(g.OwnerID.Bytes) != me {
			writeError(w, http.StatusForbidden, "owner only")
			return
		}
		var req transferReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		target, err := uuid.Parse(req.UserID)
		if err != nil || !s.isMember(r.Context(), gid, target) {
			writeError(w, http.StatusBadRequest, "target not a member")
			return
		}
		_ = s.q.TransferDMGroup(r.Context(), db.TransferDMGroupParams{ID: pgUUID(gid), OwnerID: pgUUID(target)})
		s.notifyMembers(r.Context(), gid, "group_update", map[string]any{"group_id": gid.String(), "kind": "meta"})
		w.WriteHeader(http.StatusNoContent)
	}
}

type sendReq struct {
	Body string `json:"body"`
}

func (s *Service) handleSendMessage(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if !s.isMember(r.Context(), gid, me) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		var req sendReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.Body == "" || len(req.Body) > 4000 {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		m, err := s.q.CreateDMGroupMessage(r.Context(), db.CreateDMGroupMessageParams{GroupID: pgUUID(gid), AuthorID: pgUUID(me), Body: req.Body})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "send failed")
			return
		}
		payload := map[string]any{
			"group_id":  gid.String(),
			"id":        uuid.UUID(m.ID.Bytes).String(),
			"author_id": me.String(),
			"body":      req.Body,
			"at":        m.CreatedAt.Time,
		}
		s.notifyMembers(r.Context(), gid, "group_message", payload)
		writeJSON(w, http.StatusCreated, payload)
	}
}

func (s *Service) handleListMessages(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if !s.isMember(r.Context(), gid, me) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		params := db.ListDMGroupMessagesParams{GroupID: pgUUID(gid), Limit: 50}
		if b := r.URL.Query().Get("before"); b != "" {
			if t, perr := time.Parse(time.RFC3339, b); perr == nil {
				params.Before = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
		rows, err := s.q.ListDMGroupMessages(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))

		for i := len(rows) - 1; i >= 0; i-- {
			m := rows[i]
			out = append(out, map[string]any{
				"id":         uuid.UUID(m.ID.Bytes).String(),
				"author_id":  uuid.UUID(m.AuthorID.Bytes).String(),
				"username":   m.Username,
				"avatar_key": m.AvatarKey,
				"body":       m.Body,
				"at":         m.CreatedAt.Time,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleCreateInvite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if uuid.UUID(g.OwnerID.Bytes) != me {
			writeError(w, http.StatusForbidden, "owner only")
			return
		}
		code := newCode()
		if err := s.q.CreateDMGroupInvite(r.Context(), db.CreateDMGroupInviteParams{Code: code, GroupID: pgUUID(gid), CreatedBy: pgUUID(me)}); err != nil {
			writeError(w, http.StatusInternalServerError, "invite failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"code": code})
	}
}

func (s *Service) handleListInvites(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if !s.isMember(r.Context(), gid, me) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		rows, _ := s.q.ListDMGroupInvites(r.Context(), pgUUID(gid))
		out := make([]map[string]any, 0, len(rows))
		for _, inv := range rows {
			out = append(out, map[string]any{"code": inv.Code})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleRevokeInvite(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		gid, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		g, err := s.q.GetDMGroup(r.Context(), pgUUID(gid))
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if uuid.UUID(g.OwnerID.Bytes) != me {
			writeError(w, http.StatusForbidden, "owner only")
			return
		}
		_ = s.q.DeleteDMGroupInvite(r.Context(), db.DeleteDMGroupInviteParams{Code: chi.URLParam(r, "code"), GroupID: pgUUID(gid)})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleJoin(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		inv, err := s.q.GetDMGroupInvite(r.Context(), chi.URLParam(r, "code"))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invitation invalide")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		gid := uuid.UUID(inv.GroupID.Bytes)
		if n, _ := s.q.CountDMGroupMembers(r.Context(), inv.GroupID); n >= maxMembers {
			writeError(w, http.StatusForbidden, "groupe complet (10 max)")
			return
		}
		_ = s.q.AddDMGroupMember(r.Context(), db.AddDMGroupMemberParams{GroupID: inv.GroupID, UserID: pgUUID(me)})
		s.notifyMembers(r.Context(), gid, "group_update", map[string]any{"group_id": gid.String(), "kind": "members"})
		writeJSON(w, http.StatusOK, map[string]any{"group_id": gid.String()})
	}
}

func newCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
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

var _ = context.Background
