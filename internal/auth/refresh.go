package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
)

const DefaultRefreshTTL = 30 * 24 * time.Hour

const refreshGrace = 15 * time.Second

var ErrRefreshInvalid = errors.New("auth: refresh token invalid")

var ErrRefreshReuse = errors.New("auth: refresh token reuse detected")

type SessionStore struct {
	q     db.Querier
	ttl   time.Duration
	grace time.Duration
	now   func() time.Time
}

func NewSessionStore(q db.Querier, ttl time.Duration) *SessionStore {
	if ttl == 0 {
		ttl = DefaultRefreshTTL
	}
	return &SessionStore{q: q, ttl: ttl, grace: refreshGrace, now: time.Now}
}

func (s *SessionStore) SetGrace(d time.Duration) { s.grace = d }

func (s *SessionStore) Create(ctx context.Context, userID uuid.UUID) (token string, expiresAt time.Time, err error) {
	token, err = newRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = s.now().Add(s.ttl)

	_, err = s.q.CreateSession(ctx, db.CreateSessionParams{
		UserID:       pgUUID(userID),
		RefreshToken: token,
		ExpiresAt:    pgTime(expiresAt),
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *SessionStore) Rotate(ctx context.Context, oldToken string) (newToken string, userID uuid.UUID, expiresAt time.Time, err error) {
	existing, err := s.q.GetSessionByRefreshToken(ctx, oldToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", uuid.Nil, time.Time{}, ErrRefreshInvalid
		}
		return "", uuid.Nil, time.Time{}, fmt.Errorf("looking up session: %w", err)
	}

	if existing.ReplacedBy.Valid {
		if tok, uid, exp, ok := s.graceSuccessor(ctx, existing); ok {
			return tok, uid, exp, nil
		}
		_ = s.q.DeleteSessionFamily(ctx, existing.FamilyID)
		return "", uuid.Nil, time.Time{}, ErrRefreshReuse
	}

	if existing.ExpiresAt.Time.Before(s.now()) {
		_ = s.q.DeleteSession(ctx, existing.ID)
		return "", uuid.Nil, time.Time{}, ErrRefreshInvalid
	}

	if u, uerr := s.q.GetUserByID(ctx, existing.UserID); uerr == nil && u.Disabled {
		_ = s.q.DeleteSessionFamily(ctx, existing.FamilyID)
		return "", uuid.Nil, time.Time{}, ErrRefreshInvalid
	}

	newToken, err = newRefreshToken()
	if err != nil {
		return "", uuid.Nil, time.Time{}, err
	}
	expiresAt = s.now().Add(s.ttl)
	uid := uuid.UUID(existing.UserID.Bytes)

	successor, err := s.q.CreateSessionInFamily(ctx, db.CreateSessionInFamilyParams{
		UserID:       existing.UserID,
		RefreshToken: newToken,
		ExpiresAt:    pgTime(expiresAt),
		FamilyID:     existing.FamilyID,
	})
	if err != nil {
		return "", uuid.Nil, time.Time{}, fmt.Errorf("creating successor session: %w", err)
	}

	if _, err := s.q.ConsumeSession(ctx, db.ConsumeSessionParams{
		ReplacedBy:      successor.ID,
		OldRefreshToken: oldToken,
	}); err != nil {

		_ = s.q.DeleteSession(ctx, successor.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			if reread, rerr := s.q.GetSessionByRefreshToken(ctx, oldToken); rerr == nil && reread.ReplacedBy.Valid {
				if tok, uid, exp, ok := s.graceSuccessor(ctx, reread); ok {
					return tok, uid, exp, nil
				}
			}
			_ = s.q.DeleteSessionFamily(ctx, existing.FamilyID)
			return "", uuid.Nil, time.Time{}, ErrRefreshReuse
		}
		return "", uuid.Nil, time.Time{}, fmt.Errorf("consuming session: %w", err)
	}

	return newToken, uid, expiresAt, nil
}

func (s *SessionStore) graceSuccessor(ctx context.Context, consumed db.Session) (token string, userID uuid.UUID, expiresAt time.Time, ok bool) {
	if s.grace <= 0 || !consumed.UsedAt.Valid || s.now().Sub(consumed.UsedAt.Time) > s.grace {
		return "", uuid.Nil, time.Time{}, false
	}
	succ, err := s.q.GetSessionByID(ctx, consumed.ReplacedBy)
	if err != nil || succ.ReplacedBy.Valid || !succ.ExpiresAt.Time.After(s.now()) {

		return "", uuid.Nil, time.Time{}, false
	}
	return succ.RefreshToken, uuid.UUID(succ.UserID.Bytes), succ.ExpiresAt.Time, true
}

func (s *SessionStore) Revoke(ctx context.Context, token string) error {
	if err := s.q.DeleteSessionByRefreshToken(ctx, token); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

func newRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
