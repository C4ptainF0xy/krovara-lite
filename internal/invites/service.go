package invites

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/permissions"
)

var (
	ErrInviteNotFound = errors.New("invites: not found")
	ErrInviteExpired  = errors.New("invites: expired")
	ErrInviteFull     = errors.New("invites: max uses reached")
	ErrUserBanned     = errors.New("invites: user is banned from this space")
)

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

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
	r.Route("/spaces/{spaceID}/invites", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.CreateInvite)).
			Post("/", s.handleCreate(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.CreateInvite)).
			Get("/", s.handleList())
	})
	r.Route("/invites/{code}", func(rr chi.Router) {
		rr.Get("/", s.handlePreview())
		rr.Post("/accept", s.handleAccept(userIDFn))
		rr.Delete("/", s.handleDelete(userIDFn, resolver))
	})
}

type createReq struct {
	MaxUses *int32 `json:"max_uses" validate:"omitempty,min=1,max=1000"`

	TTLSeconds *int32 `json:"ttl_seconds" validate:"omitempty,min=60,max=2592000"`
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
		var expiresAt pgtype.Timestamptz
		if req.TTLSeconds != nil {
			expiresAt = pgtype.Timestamptz{Time: time.Now().Add(time.Duration(*req.TTLSeconds) * time.Second), Valid: true}
		}
		actor := uidFn(r.Context())

		if existing, err := s.q.FindReusableInvite(r.Context(), db.FindReusableInviteParams{
			SpaceID: pgUUID(spaceID), MaxUses: req.MaxUses,
		}); err == nil {
			writeJSON(w, http.StatusOK, inviteDTO(existing))
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "invite lookup failed")
			return
		}

		inv, err := s.createWithRetry(r.Context(), db.CreateInviteParams{
			SpaceID:   pgUUID(spaceID),
			CreatorID: pgUUID(actor),
			MaxUses:   req.MaxUses,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create invite")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), actor, "invite.create", inv.ID, nil)
		writeJSON(w, http.StatusCreated, inviteDTO(inv))
	}
}

func (s *Service) createWithRetry(ctx context.Context, params db.CreateInviteParams) (db.Invite, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode()
		if err != nil {
			return db.Invite{}, fmt.Errorf("generate code: %w", err)
		}
		params.Code = code
		inv, err := s.q.CreateInvite(ctx, params)
		if err == nil {
			return inv, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return db.Invite{}, err
	}
	return db.Invite{}, errors.New("invites: could not generate unique code")
}

func generateCode() (string, error) {
	var buf [5]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return strings.ToLower(codeEncoding.EncodeToString(buf[:])), nil
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceInvites(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, inv := range rows {
			out = append(out, inviteDTO(inv))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handlePreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		inv, sp, err := s.lookup(r.Context(), code)
		if err != nil {
			s.writeInviteErr(w, err)
			return
		}
		memberCount, _ := s.q.CountSpaceMembers(r.Context(), sp.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"code":              inv.Code,
			"space_id":          uuid.UUID(sp.ID.Bytes).String(),
			"space_name":        sp.Name,
			"space_icon":        sp.IconKey,
			"space_banner":      sp.BannerKey,
			"space_description": sp.Description,
			"member_count":      memberCount,
			"created_at":        sp.CreatedAt.Time,
			"expires_at":        inv.ExpiresAt.Time,
			"max_uses":          inv.MaxUses,
			"uses":              inv.Uses,
		})
	}
}

func (s *Service) handleAccept(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}

		var spaceID uuid.UUID
		var memberID pgtype.UUID
		var created bool
		err := pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			inv, err := qtx.GetInviteByCode(r.Context(), code)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInviteNotFound
			}
			if err != nil {
				return err
			}
			if inv.ExpiresAt.Valid && inv.ExpiresAt.Time.Before(time.Now()) {
				return ErrInviteExpired
			}
			if inv.MaxUses != nil {
				used := int32(0)
				if inv.Uses != nil {
					used = *inv.Uses
				}
				if used >= *inv.MaxUses {
					return ErrInviteFull
				}
			}

			banned, err := qtx.IsUserBanned(r.Context(), db.IsUserBannedParams{
				SpaceID: inv.SpaceID,
				UserID:  pgUUID(actor),
			})
			if err != nil {
				return err
			}
			if banned {
				return ErrUserBanned
			}

			spaceID = uuid.UUID(inv.SpaceID.Bytes)

			existing, err := qtx.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
				SpaceID: inv.SpaceID,
				UserID:  pgUUID(actor),
			})
			if err == nil {
				memberID = existing.ID
			} else if errors.Is(err, pgx.ErrNoRows) {
				mem, err := qtx.CreateMember(r.Context(), db.CreateMemberParams{
					SpaceID: inv.SpaceID,
					UserID:  pgUUID(actor),
				})
				if err != nil {
					return err
				}
				memberID = mem.ID
				created = true
			} else {
				return err
			}

			if _, err := qtx.IncrementInviteUses(r.Context(), inv.ID); err != nil {
				return err
			}
			meta, _ := json.Marshal(map[string]any{"code": code})
			_, _ = qtx.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
				SpaceID:  inv.SpaceID,
				ActorID:  pgUUID(actor),
				Action:   "invite.accept",
				TargetID: memberID,
				Metadata: meta,
			})
			return nil
		})
		if err != nil {
			s.writeInviteErr(w, err)
			return
		}

		if created {
			_ = eventsfeed.Emit(r.Context(), s.pool, uuid.Nil, "member_join", map[string]any{
				"space_id": spaceID.String(),
				"user_id":  actor.String(),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"space_id":  spaceID.String(),
			"member_id": uuid.UUID(memberID.Bytes).String(),
		})
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc, resolver permissions.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		inv, _, err := s.lookup(r.Context(), code)
		if err != nil {
			s.writeInviteErr(w, err)
			return
		}
		mc, err := resolver.ResolveSpace(r.Context(), actor, uuid.UUID(inv.SpaceID.Bytes))
		if err != nil || !permissions.Compute(mc).Has(permissions.ManageSpace) {
			writeError(w, http.StatusForbidden, "missing ManageSpace")
			return
		}
		if err := s.q.DeleteInviteByCode(r.Context(), code); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.logAudit(r.Context(), inv.SpaceID, actor, "invite.delete", inv.ID, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) lookup(ctx context.Context, code string) (db.Invite, db.Space, error) {
	inv, err := s.q.GetInviteByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Invite{}, db.Space{}, ErrInviteNotFound
	}
	if err != nil {
		return db.Invite{}, db.Space{}, err
	}
	sp, err := s.q.GetSpace(ctx, inv.SpaceID)
	if err != nil {
		return db.Invite{}, db.Space{}, err
	}
	return inv, sp, nil
}

func (s *Service) writeInviteErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInviteNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInviteExpired), errors.Is(err, ErrInviteFull):
		writeError(w, http.StatusGone, err.Error())
	case errors.Is(err, ErrUserBanned):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "invite error")
	}
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, targetID pgtype.UUID, meta []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID: spaceID, ActorID: pgUUID(actorID), Action: action, TargetID: targetID, Metadata: meta,
	})
}

func inviteDTO(inv db.Invite) map[string]any {
	return map[string]any{
		"id":         uuid.UUID(inv.ID.Bytes).String(),
		"space_id":   uuid.UUID(inv.SpaceID.Bytes).String(),
		"creator_id": uuid.UUID(inv.CreatorID.Bytes).String(),
		"code":       inv.Code,
		"max_uses":   inv.MaxUses,
		"uses":       inv.Uses,
		"expires_at": inv.ExpiresAt.Time,
		"created_at": inv.CreatedAt.Time,
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
