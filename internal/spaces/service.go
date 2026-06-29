package spaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

var defaultEveryonePerms = permissions.ViewChannel |
	permissions.SendMessages |
	permissions.ConnectVoice |
	permissions.SpeakVoice

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

func (s *Service) CreateSpace(ctx context.Context, ownerID uuid.UUID, name string, iconKey *string) (db.Space, error) {
	var out db.Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.q.WithTx(tx)

		space, err := qtx.CreateSpace(ctx, db.CreateSpaceParams{
			OwnerID: pgUUID(ownerID),
			Name:    name,
			IconKey: iconKey,
		})
		if err != nil {
			return fmt.Errorf("create space: %w", err)
		}

		perms := defaultEveryonePerms.ToInt64()
		pos := int32(0)
		isEveryone := true
		if _, err := qtx.CreateRole(ctx, db.CreateRoleParams{
			SpaceID:     space.ID,
			Name:        "@everyone",
			Permissions: &perms,
			Position:    &pos,
			IsEveryone:  &isEveryone,
		}); err != nil {
			return fmt.Errorf("create everyone role: %w", err)
		}

		chType := "text"
		chPos := int32(0)
		if _, err := qtx.CreateChannel(ctx, db.CreateChannelParams{
			SpaceID:  space.ID,
			Name:     "general",
			Type:     &chType,
			Position: &chPos,
		}); err != nil {
			return fmt.Errorf("create general channel: %w", err)
		}

		if _, err := qtx.CreateMember(ctx, db.CreateMemberParams{
			SpaceID: space.ID,
			UserID:  pgUUID(ownerID),
		}); err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}

		meta, _ := json.Marshal(map[string]any{"name": name})
		if _, err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
			SpaceID:  space.ID,
			ActorID:  pgUUID(ownerID),
			Action:   "space.create",
			TargetID: space.ID,
			Metadata: meta,
		}); err != nil {
			return fmt.Errorf("create audit log: %w", err)
		}

		out = space
		return nil
	})
	return out, err
}

var ErrNotFound = errors.New("spaces: not found")

func (s *Service) GetSpace(ctx context.Context, id uuid.UUID) (db.Space, error) {
	sp, err := s.q.GetSpace(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Space{}, ErrNotFound
	}
	return sp, err
}

type SpaceSettings struct {
	Name        *string
	IconKey     *string
	Description *string
	Rules       *string
	BannerKey   *string
	Tags        []string
	Language    *string
}

func (s *Service) UpdateSpace(ctx context.Context, actorID, id uuid.UUID, in SpaceSettings) (db.Space, error) {
	sp, err := s.q.UpdateSpace(ctx, db.UpdateSpaceParams{
		ID:          pgUUID(id),
		Name:        in.Name,
		IconKey:     in.IconKey,
		Description: in.Description,
		Rules:       in.Rules,
		BannerKey:   in.BannerKey,
		Tags:        in.Tags,
		Language:    in.Language,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Space{}, ErrNotFound
	}
	if err != nil {
		return db.Space{}, err
	}
	s.logAudit(ctx, sp.ID, actorID, "space.update", sp.ID, nil)
	return sp, nil
}

var ErrVanityTaken = errors.New("spaces: vanity slug already taken")

var ErrVanityReserved = errors.New("spaces: vanity slug is reserved")

func (s *Service) SetVanity(ctx context.Context, actorID, id uuid.UUID, slug *string) (db.Space, error) {
	var slugArg *string
	if slug != nil {
		norm := strings.ToLower(strings.TrimSpace(*slug))
		if !validVanity(norm) {
			return db.Space{}, fmt.Errorf("invalid vanity slug")
		}
		if reservedVanity[norm] {
			return db.Space{}, ErrVanityReserved
		}
		slugArg = &norm
	}
	sp, err := s.q.SetSpaceVanity(ctx, db.SetSpaceVanityParams{ID: pgUUID(id), VanitySlug: slugArg})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Space{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return db.Space{}, ErrVanityTaken
	}
	if err != nil {
		return db.Space{}, err
	}
	s.logAudit(ctx, sp.ID, actorID, "space.vanity", sp.ID, nil)
	return sp, nil
}

func (s *Service) GetByVanity(ctx context.Context, slug string) (db.Space, error) {
	sp, err := s.q.GetSpaceByVanity(ctx, strings.TrimSpace(slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Space{}, ErrNotFound
	}
	return sp, err
}

var ErrNotOwner = errors.New("spaces: caller is not the space owner")

var ErrTargetNotMember = errors.New("spaces: target user is not a member")

func (s *Service) TransferOwnership(ctx context.Context, actorID, id, newOwner uuid.UUID) (db.Space, error) {
	sp, err := s.GetSpace(ctx, id)
	if err != nil {
		return db.Space{}, err
	}
	if uuid.UUID(sp.OwnerID.Bytes) != actorID {
		return db.Space{}, ErrNotOwner
	}
	if _, err := s.q.GetMemberByUser(ctx, db.GetMemberByUserParams{
		SpaceID: pgUUID(id),
		UserID:  pgUUID(newOwner),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Space{}, ErrTargetNotMember
		}
		return db.Space{}, err
	}
	out, err := s.q.TransferSpaceOwnership(ctx, db.TransferSpaceOwnershipParams{
		ID: pgUUID(id), OwnerID: pgUUID(newOwner),
	})
	if err != nil {
		return db.Space{}, err
	}
	meta, _ := json.Marshal(map[string]any{"new_owner": newOwner.String()})
	s.logAudit(ctx, out.ID, actorID, "space.transfer", pgUUID(newOwner), meta)
	return out, nil
}

func (s *Service) verifyStepUp(ctx context.Context, userID uuid.UUID, password string) (bool, error) {
	user, err := s.q.GetUserByID(ctx, pgUUID(userID))
	if err != nil {
		return false, fmt.Errorf("get user: %w", err)
	}
	if user.PasswordHash == nil || *user.PasswordHash == "" {
		return false, nil
	}
	ok, err := auth.VerifyPassword(password, *user.PasswordHash)
	if err != nil {
		return false, nil
	}
	return ok, nil
}

func (s *Service) DeleteSpace(ctx context.Context, actorID, id uuid.UUID) error {

	s.logAudit(ctx, pgUUID(id), actorID, "space.delete", pgUUID(id), nil)
	return s.q.DeleteSpace(ctx, pgUUID(id))
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

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
