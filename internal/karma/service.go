package karma

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const (
	vouchWeight = 1

	DefaultMaxVouchesPerDay = 10

	DefaultMinAccountAge = 7 * 24 * time.Hour
)

type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool

	MaxVouchesPerDay int64
	MinAccountAge    time.Duration
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		q:                db.New(pool),
		pool:             pool,
		MaxVouchesPerDay: DefaultMaxVouchesPerDay,
		MinAccountAge:    DefaultMinAccountAge,
	}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/karma", func(rr chi.Router) {
		rr.Get("/{userID}", s.handleGet())
		rr.Post("/{userID}", s.handleVouch(userIDFn))
	})
}

func (s *Service) handleGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, targetID, ok := spaceAndUser(w, r)
		if !ok {
			return
		}
		score, err := s.q.GetKarma(r.Context(), db.GetKarmaParams{
			UserID:  pgUUID(targetID),
			SpaceID: pgUUID(spaceID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			score = 0
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":  targetID.String(),
			"space_id": spaceID.String(),
			"score":    score,
		})
	}
}

func (s *Service) handleVouch(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, targetID, ok := spaceAndUser(w, r)
		if !ok {
			return
		}
		source := uidFn(r.Context())
		if source == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if source == targetID {
			writeError(w, http.StatusBadRequest, "cannot vouch yourself")
			return
		}

		_, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(source),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusForbidden, "not a member of this space")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "member lookup failed")
			return
		}
		if _, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(targetID),
		}); errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "target is not a member of this space")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "member lookup failed")
			return
		}

		srcUser, err := s.q.GetUserByID(r.Context(), pgUUID(source))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user lookup failed")
			return
		}

		if s.MinAccountAge > 0 && srcUser.CreatedAt.Valid && time.Since(srcUser.CreatedAt.Time) < s.MinAccountAge {
			writeError(w, http.StatusForbidden, "account too new to vouch")
			return
		}

		since := pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true}
		count, err := s.q.CountKarmaEventsBySourceSince(r.Context(), db.CountKarmaEventsBySourceSinceParams{
			SourceUser: pgUUID(source), CreatedAt: since,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cap check failed")
			return
		}
		if count >= s.MaxVouchesPerDay {
			writeError(w, http.StatusTooManyRequests, "daily vouch limit reached")
			return
		}

		var score int32
		err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			if _, err := qtx.InsertKarmaEvent(r.Context(), db.InsertKarmaEventParams{
				SpaceID:    pgUUID(spaceID),
				TargetUser: pgUUID(targetID),
				SourceUser: pgUUID(source),
				Delta:      vouchWeight,
				Reason:     "vouch",
			}); err != nil {
				return err
			}
			sc, err := qtx.AddKarma(r.Context(), db.AddKarmaParams{
				UserID:  pgUUID(targetID),
				SpaceID: pgUUID(spaceID),
				Score:   vouchWeight,
			})
			if err != nil {
				return err
			}
			score = sc
			return nil
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "already vouched for this member")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "vouch failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":  targetID.String(),
			"space_id": spaceID.String(),
			"score":    score,
		})
	}
}

func spaceAndUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space id")
		return uuid.Nil, uuid.Nil, false
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return uuid.Nil, uuid.Nil, false
	}
	return spaceID, userID, true
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
