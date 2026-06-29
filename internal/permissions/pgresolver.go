package permissions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
)

var ErrNotMember = errors.New("permissions: user is not a member of this space")

type PGResolver struct {
	q db.Querier
}

func NewPGResolver(q db.Querier) *PGResolver {
	return &PGResolver{q: q}
}

func (r *PGResolver) ResolveSpace(ctx context.Context, userID, spaceID uuid.UUID) (MemberContext, error) {
	space, err := r.q.GetSpace(ctx, pgUUID(spaceID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberContext{}, ErrNotMember
		}
		return MemberContext{}, fmt.Errorf("get space: %w", err)
	}
	return r.buildContext(ctx, userID, space, nil)
}

func (r *PGResolver) ResolveChannel(ctx context.Context, userID, channelID uuid.UUID) (MemberContext, error) {
	ch, err := r.q.GetChannel(ctx, pgUUID(channelID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberContext{}, ErrNotMember
		}
		return MemberContext{}, fmt.Errorf("get channel: %w", err)
	}
	space, err := r.q.GetSpace(ctx, ch.SpaceID)
	if err != nil {
		return MemberContext{}, fmt.Errorf("get space: %w", err)
	}
	return r.buildContext(ctx, userID, space, &ch.ID)
}

func (r *PGResolver) buildContext(ctx context.Context, userID uuid.UUID, space db.Space, channelID *pgtype.UUID) (MemberContext, error) {
	mc := MemberContext{}
	if space.OwnerID.Valid && uuid.UUID(space.OwnerID.Bytes) == userID {
		mc.IsOwner = true
	}

	everyoneRow, err := r.q.GetEveryoneRole(ctx, space.ID)
	if err != nil {
		return MemberContext{}, fmt.Errorf("get everyone role: %w", err)
	}
	mc.EveryoneRole = roleFromDB(everyoneRow)

	member, err := r.q.GetMemberByUser(ctx, db.GetMemberByUserParams{
		SpaceID: space.ID,
		UserID:  pgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if mc.IsOwner {
				return mc, nil
			}
			return MemberContext{}, ErrNotMember
		}
		return MemberContext{}, fmt.Errorf("get member: %w", err)
	}

	roles, err := r.q.ListMemberRoles(ctx, member.ID)
	if err != nil {
		return MemberContext{}, fmt.Errorf("list member roles: %w", err)
	}
	roleIDs := make([]pgtype.UUID, 0, len(roles)+1)
	roleIDs = append(roleIDs, everyoneRow.ID)
	for _, role := range roles {
		mc.Roles = append(mc.Roles, roleFromDB(role))
		roleIDs = append(roleIDs, role.ID)
	}

	if channelID != nil {
		ows, err := r.q.ListChannelOverwritesForRoles(ctx, db.ListChannelOverwritesForRolesParams{
			ChannelID: *channelID,
			RoleIds:   roleIDs,
		})
		if err != nil {
			return MemberContext{}, fmt.Errorf("list overwrites: %w", err)
		}
		for _, ow := range ows {
			mc.Overwrites = append(mc.Overwrites, ChannelOverwrite{
				RoleID: uuid.UUID(ow.RoleID.Bytes).String(),
				Allow:  int64PtrToBits(ow.Allow),
				Deny:   int64PtrToBits(ow.Deny),
			})
		}

		memberOw, err := r.q.GetMemberChannelOverwrite(ctx, db.GetMemberChannelOverwriteParams{
			ChannelID: *channelID,
			MemberID:  member.ID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return MemberContext{}, fmt.Errorf("get member overwrite: %w", err)
		}
		if err == nil {
			mc.MemberOverwrite = &ChannelOverwrite{
				Allow: int64PtrToBits(memberOw.Allow),
				Deny:  int64PtrToBits(memberOw.Deny),
			}
		}
	}

	return mc, nil
}

func roleFromDB(r db.Role) Role {
	perms := Bitfield(0)
	if r.Permissions != nil {
		perms = FromInt64(*r.Permissions)
	}
	pos := 0
	if r.Position != nil {
		pos = int(*r.Position)
	}
	isEveryone := r.IsEveryone != nil && *r.IsEveryone
	return Role{
		ID:          uuid.UUID(r.ID.Bytes).String(),
		Permissions: perms,
		Position:    pos,
		IsEveryone:  isEveryone,
	}
}

func int64PtrToBits(p *int64) Bitfield {
	if p == nil {
		return 0
	}
	return FromInt64(*p)
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
