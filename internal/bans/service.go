package bans

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
	"github.com/krovara/krovara/internal/permissions"
)

var (
	ErrSelfBan   = errors.New("bans: cannot ban yourself")
	ErrOwnerBan  = errors.New("bans: cannot ban the space owner")
	ErrHierarchy = errors.New("bans: target outranks caller")
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
	r.Route("/spaces/{spaceID}/bans", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.BanMembers)).
			Post("/", s.handleBan(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.BanMembers)).
			Get("/", s.handleList())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.BanMembers)).
			Delete("/{userID}", s.handleUnban(userIDFn))
	})
}

type banReq struct {
	UserID       string  `json:"user_id" validate:"required,uuid"`
	Reason       *string `json:"reason"  validate:"omitempty,max=512"`
	WipeMessages bool    `json:"wipe_messages"`
}

func (s *Service) handleBan(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req banReq
		if !s.decode(w, r, &req) {
			return
		}
		targetID, err := uuid.Parse(req.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		actor := uidFn(r.Context())
		if actor == targetID {
			writeError(w, http.StatusBadRequest, ErrSelfBan.Error())
			return
		}

		sp, err := s.q.GetSpace(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "space lookup failed")
			return
		}
		if sp.OwnerID.Valid && uuid.UUID(sp.OwnerID.Bytes) == targetID {
			writeError(w, http.StatusForbidden, ErrOwnerBan.Error())
			return
		}

		if targetMem, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID),
			UserID:  pgUUID(targetID),
		}); err == nil {
			if err := s.checkHierarchy(r.Context(), actor, spaceID, targetMem.ID); err != nil {
				writeError(w, http.StatusForbidden, err.Error())
				return
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "member lookup failed")
			return
		}

		var ban db.Ban
		err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			if mem, err := qtx.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
				SpaceID: pgUUID(spaceID),
				UserID:  pgUUID(targetID),
			}); err == nil {
				if err := qtx.DeleteMember(r.Context(), mem.ID); err != nil {
					return fmt.Errorf("delete membership: %w", err)
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("lookup membership: %w", err)
			}
			b, err := qtx.CreateBan(r.Context(), db.CreateBanParams{
				SpaceID:     pgUUID(spaceID),
				UserID:      pgUUID(targetID),
				ModeratorID: pgUUID(actor),
				Reason:      req.Reason,
			})
			if err != nil {
				return fmt.Errorf("create ban: %w", err)
			}

			if req.WipeMessages {
				if _, err := tx.Exec(r.Context(), `
DELETE FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log'
   AND "user" IN (SELECT id::text FROM channels WHERE space_id = $2)
   AND value LIKE '%/' || $3 || chr(39) || '%'
`, s.mucHost, pgUUID(spaceID), targetID.String()); err != nil {
					return fmt.Errorf("wipe messages: %w", err)
				}
			}
			meta, _ := json.Marshal(map[string]any{"reason": req.Reason, "wiped": req.WipeMessages})
			_, _ = qtx.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
				SpaceID:  pgUUID(spaceID),
				ActorID:  pgUUID(actor),
				Action:   "member.ban",
				TargetID: pgUUID(targetID),
				Metadata: meta,
			})
			ban = b
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ban failed")
			return
		}
		writeJSON(w, http.StatusCreated, banDTO(ban))
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceBans(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, b := range rows {
			out = append(out, banDTO(b))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleUnban(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		userID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if err := s.q.DeleteBan(r.Context(), db.DeleteBanParams{
			SpaceID: pgUUID(spaceID),
			UserID:  pgUUID(userID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "unban failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "member.unban", pgUUID(userID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) checkHierarchy(ctx context.Context, actorID, spaceID uuid.UUID, targetMemberID pgtype.UUID) error {
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
	actorMax, err := s.q.GetMemberMaxRolePosition(ctx, mem.ID)
	if err != nil {
		return err
	}
	targetMax, err := s.q.GetMemberMaxRolePosition(ctx, targetMemberID)
	if err != nil {
		return err
	}
	if targetMax >= actorMax {
		return ErrHierarchy
	}
	return nil
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, targetID pgtype.UUID, meta []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID: spaceID, ActorID: pgUUID(actorID), Action: action, TargetID: targetID, Metadata: meta,
	})
}

func banDTO(b db.Ban) map[string]any {
	return map[string]any{
		"id":           uuid.UUID(b.ID.Bytes).String(),
		"space_id":     uuid.UUID(b.SpaceID.Bytes).String(),
		"user_id":      uuid.UUID(b.UserID.Bytes).String(),
		"moderator_id": uuid.UUID(b.ModeratorID.Bytes).String(),
		"reason":       b.Reason,
		"created_at":   b.CreatedAt.Time,
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
