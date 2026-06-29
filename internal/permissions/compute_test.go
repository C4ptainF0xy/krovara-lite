package permissions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func everyone(perms Bitfield) Role {
	return Role{ID: "everyone", Permissions: perms, Position: 0, IsEveryone: true}
}

func role(id string, position int, perms Bitfield) Role {
	return Role{ID: id, Permissions: perms, Position: position}
}

func TestCompute_OwnerShortCircuits(t *testing.T) {
	got := Compute(MemberContext{
		IsOwner:      true,
		EveryoneRole: everyone(0),
	})
	require.Equal(t, All, got)
}

func TestCompute_OwnerBeatsExplicitDeny(t *testing.T) {
	got := Compute(MemberContext{
		IsOwner:      true,
		EveryoneRole: everyone(0),
		Overwrites:   []ChannelOverwrite{{RoleID: "everyone", Deny: All}},
	})
	require.Equal(t, All, got)
}

func TestCompute_EveryoneBase(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel | SendMessages),
	})
	require.Equal(t, ViewChannel|SendMessages, got)
}

func TestCompute_RoleOrEveryone(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles:        []Role{role("r1", 1, SendMessages)},
	})
	require.Equal(t, ViewChannel|SendMessages, got)
}

func TestCompute_MultipleRolesUnion(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles: []Role{
			role("r1", 1, SendMessages),
			role("r2", 2, ManageMessages),
			role("r3", 3, KickMembers),
		},
	})
	require.Equal(t, ViewChannel|SendMessages|ManageMessages|KickMembers, got)
}

func TestCompute_AdministratorGrantsAll(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(0),
		Roles:        []Role{role("admin", 5, Administrator)},
	})
	require.Equal(t, All, got)
}

func TestCompute_AdministratorBeatsChannelDeny(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(0),
		Roles:        []Role{role("admin", 5, Administrator)},
		Overwrites:   []ChannelOverwrite{{RoleID: "admin", Deny: All}},
	})
	require.Equal(t, All, got)
}

func TestCompute_EveryoneDenyRemovesBase(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel | SendMessages),
		Overwrites:   []ChannelOverwrite{{RoleID: "everyone", Deny: SendMessages}},
	})
	require.Equal(t, ViewChannel, got)
}

func TestCompute_EveryoneAllowAddsBit(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Overwrites:   []ChannelOverwrite{{RoleID: "everyone", Allow: CreateInvite}},
	})
	require.Equal(t, ViewChannel|CreateInvite, got)
}

func TestCompute_RoleAllowBeatsEveryoneDeny(t *testing.T) {

	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles:        []Role{role("mod", 1, 0)},
		Overwrites: []ChannelOverwrite{
			{RoleID: "everyone", Deny: SendMessages},
			{RoleID: "mod", Allow: SendMessages},
		},
	})
	require.True(t, got.Has(SendMessages))
	require.True(t, got.Has(ViewChannel))
}

func TestCompute_HigherRoleDenyOverridesLowerAllow(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(0),
		Roles: []Role{
			role("low", 1, 0),
			role("high", 2, 0),
		},
		Overwrites: []ChannelOverwrite{
			{RoleID: "low", Allow: SendMessages},
			{RoleID: "high", Deny: SendMessages},
		},
	})
	require.False(t, got.Has(SendMessages))
}

func TestCompute_SameRoleAllowBeatsItsOwnDeny(t *testing.T) {

	got := Compute(MemberContext{
		EveryoneRole: everyone(0),
		Roles:        []Role{role("r", 1, 0)},
		Overwrites: []ChannelOverwrite{
			{RoleID: "r", Deny: SendMessages, Allow: SendMessages},
		},
	})
	require.True(t, got.Has(SendMessages))
}

func TestCompute_OverwriteForUnheldRoleIgnored(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles:        []Role{role("mine", 1, 0)},
		Overwrites: []ChannelOverwrite{
			{RoleID: "stranger", Deny: ViewChannel},
		},
	})
	require.Equal(t, ViewChannel, got)
}

func TestCompute_NoRolesNoOverwrites(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
	})
	require.Equal(t, ViewChannel, got)
}

func TestCompute_UnsortedRolesStillRespectPosition(t *testing.T) {

	mc := MemberContext{
		EveryoneRole: everyone(0),
		Roles: []Role{
			role("high", 5, 0),
			role("low", 1, 0),
		},
		Overwrites: []ChannelOverwrite{
			{RoleID: "low", Allow: SendMessages},
			{RoleID: "high", Deny: SendMessages},
		},
	}
	require.False(t, Compute(mc).Has(SendMessages))
}

func TestCompute_MemberOverwriteAllowBeatsRoleDeny(t *testing.T) {

	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles:        []Role{role("high", 5, 0)},
		Overwrites: []ChannelOverwrite{
			{RoleID: "high", Deny: SendMessages},
		},
		MemberOverwrite: &ChannelOverwrite{Allow: SendMessages},
	})
	require.True(t, got.Has(SendMessages))
	require.True(t, got.Has(ViewChannel))
}

func TestCompute_MemberOverwriteDenyBeatsRoleAllow(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel | SendMessages),
		Roles:        []Role{role("r", 1, 0)},
		Overwrites: []ChannelOverwrite{
			{RoleID: "r", Allow: SendMessages},
		},
		MemberOverwrite: &ChannelOverwrite{Deny: SendMessages},
	})
	require.False(t, got.Has(SendMessages))
	require.True(t, got.Has(ViewChannel))
}

func TestCompute_MemberOverwriteWithinRowAllowBeatsDeny(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole:    everyone(0),
		MemberOverwrite: &ChannelOverwrite{Deny: SendMessages, Allow: SendMessages},
	})
	require.True(t, got.Has(SendMessages))
}

func TestCompute_MemberOverwriteIgnoredForOwner(t *testing.T) {
	got := Compute(MemberContext{
		IsOwner:         true,
		EveryoneRole:    everyone(0),
		MemberOverwrite: &ChannelOverwrite{Deny: All},
	})
	require.Equal(t, All, got)
}

func TestCompute_MemberOverwriteIgnoredForAdmin(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole:    everyone(0),
		Roles:           []Role{role("admin", 5, Administrator)},
		MemberOverwrite: &ChannelOverwrite{Deny: All},
	})
	require.Equal(t, All, got)
}

func TestCompute_StackedAllowsAccumulate(t *testing.T) {
	got := Compute(MemberContext{
		EveryoneRole: everyone(ViewChannel),
		Roles: []Role{
			role("a", 1, 0),
			role("b", 2, 0),
		},
		Overwrites: []ChannelOverwrite{
			{RoleID: "a", Allow: SendMessages},
			{RoleID: "b", Allow: CreateInvite},
		},
	})
	require.Equal(t, ViewChannel|SendMessages|CreateInvite, got)
}
