package channels

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
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/permissions"
)

func emitSpaceUpdate(ctx context.Context, pool *pgxpool.Pool, spaceID uuid.UUID, what string) {
	_ = eventsfeed.Emit(ctx, pool, uuid.Nil, "space_update", map[string]any{
		"space_id": spaceID.String(),
		"what":     what,
	})
}

var ErrNotFound = errors.New("channels: not found")

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

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/channels", func(rr chi.Router) {

		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageChannels)).
			Post("/", s.handleCreate(userIDFn))
	})
	r.Route("/channels/{channelID}", func(rr chi.Router) {
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageChannels)).
			Patch("/", s.handleUpdate(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageChannels)).
			Delete("/", s.handleDelete(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageChannels)).
			Put("/lock", s.handleSetLock(userIDFn))
	})
}

type createReq struct {
	Name       string  `json:"name"        validate:"required,min=1,max=64"`
	Topic      *string `json:"topic"       validate:"omitempty,max=512"`
	Type       *string `json:"type"        validate:"omitempty,oneof=text voice"`
	Position   *int32  `json:"position"`
	IsPrivate  *bool   `json:"is_private"`
	CategoryID *string `json:"category_id" validate:"omitempty,uuid4"`
}

type updateReq struct {
	Name            *string `json:"name"             validate:"omitempty,min=1,max=64"`
	Topic           *string `json:"topic"            validate:"omitempty,max=512"`
	Position        *int32  `json:"position"`
	IsPrivate       *bool   `json:"is_private"`
	SlowmodeSeconds *int32  `json:"slowmode_seconds" validate:"omitempty,min=0,max=21600"`
	NSFW            *bool   `json:"nsfw"`
	ReadOnly        *bool   `json:"read_only"`

	IconEmoji *string `json:"icon_emoji"       validate:"omitempty,max=34"`
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceChannels(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, ch := range rows {
			out = append(out, channelDTO(ch))
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
		typ := "text"
		if req.Type != nil {
			typ = *req.Type
		}
		var catID pgtype.UUID
		if req.CategoryID != nil && *req.CategoryID != "" {
			parsed, err := uuid.Parse(*req.CategoryID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid category id")
				return
			}
			catID = pgUUID(parsed)
		}
		ch, err := s.q.CreateChannel(r.Context(), db.CreateChannelParams{
			SpaceID:    pgUUID(spaceID),
			Name:       req.Name,
			Topic:      req.Topic,
			Type:       &typ,
			Position:   req.Position,
			IsPrivate:  req.IsPrivate,
			CategoryID: catID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "channel.create", ch.ID, nil)
		emitSpaceUpdate(r.Context(), s.pool, spaceID, "channels")
		writeJSON(w, http.StatusCreated, channelDTO(ch))
	}
}

func (s *Service) handleUpdate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		var req updateReq
		if !s.decode(w, r, &req) {
			return
		}
		ch, err := s.q.UpdateChannel(r.Context(), db.UpdateChannelParams{
			ID:              pgUUID(id),
			Name:            req.Name,
			Topic:           req.Topic,
			Position:        req.Position,
			IsPrivate:       req.IsPrivate,
			SlowmodeSeconds: req.SlowmodeSeconds,
			Nsfw:            req.NSFW,
			ReadOnly:        req.ReadOnly,
			IconEmoji:       req.IconEmoji,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.logAudit(r.Context(), ch.SpaceID, uidFn(r.Context()), "channel.update", ch.ID, nil)
		emitSpaceUpdate(r.Context(), s.pool, uuid.UUID(ch.SpaceID.Bytes), "channels")
		writeJSON(w, http.StatusOK, channelDTO(ch))
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		ch, err := s.q.GetChannel(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if err := s.q.DeleteChannel(r.Context(), pgUUID(id)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.logAudit(r.Context(), ch.SpaceID, uidFn(r.Context()), "channel.delete", ch.ID, nil)
		emitSpaceUpdate(r.Context(), s.pool, uuid.UUID(ch.SpaceID.Bytes), "channels")
		w.WriteHeader(http.StatusNoContent)
	}
}

type lockReq struct {
	Locked *bool `json:"locked" validate:"required"`
}

func (s *Service) handleSetLock(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		var req lockReq
		if !s.decode(w, r, &req) {
			return
		}
		actor := uidFn(r.Context())
		ch, err := s.q.SetChannelLock(r.Context(), db.SetChannelLockParams{
			ID:       pgUUID(id),
			Locked:   *req.Locked,
			LockedBy: pgUUID(actor),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lock failed")
			return
		}
		action := "channel.unlock"
		if *req.Locked {
			action = "channel.lock"
		}
		s.logAudit(r.Context(), ch.SpaceID, actor, action, ch.ID, nil)
		writeJSON(w, http.StatusOK, channelDTO(ch))
	}
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, targetID pgtype.UUID, metadata []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID:  spaceID,
		ActorID:  pgUUID(actorID),
		Action:   action,
		TargetID: targetID,
		Metadata: metadata,
	})
}

func channelDTO(ch db.Channel) map[string]any {
	out := map[string]any{
		"id":               uuid.UUID(ch.ID.Bytes).String(),
		"space_id":         uuid.UUID(ch.SpaceID.Bytes).String(),
		"name":             ch.Name,
		"topic":            ch.Topic,
		"type":             ch.Type,
		"position":         ch.Position,
		"is_private":       ch.IsPrivate,
		"locked":           ch.Locked,
		"slowmode_seconds": ch.SlowmodeSeconds,
		"nsfw":             ch.Nsfw,
		"read_only":        ch.ReadOnly,
		"icon_emoji":       ch.IconEmoji,
		"created_at":       ch.CreatedAt.Time,
	}
	if ch.CategoryID.Valid {
		out["category_id"] = uuid.UUID(ch.CategoryID.Bytes).String()
	} else {
		out["category_id"] = nil
	}
	if ch.LockedBy.Valid {
		out["locked_by"] = uuid.UUID(ch.LockedBy.Bytes).String()
	}
	if ch.LockedAt.Valid {
		out["locked_at"] = ch.LockedAt.Time
	}
	return out
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
