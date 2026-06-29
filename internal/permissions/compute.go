package permissions

import "sort"

type Role struct {
	ID          string
	Permissions Bitfield
	Position    int
	IsEveryone  bool
}

type ChannelOverwrite struct {
	RoleID string
	Allow  Bitfield
	Deny   Bitfield
}

type MemberContext struct {
	IsOwner      bool
	EveryoneRole Role
	Roles        []Role
	Overwrites   []ChannelOverwrite

	MemberOverwrite *ChannelOverwrite
}

func Compute(mc MemberContext) Bitfield {
	if mc.IsOwner {
		return All
	}

	base := mc.EveryoneRole.Permissions
	for _, r := range mc.Roles {
		base = base.Or(r.Permissions)
	}

	if base.Has(Administrator) {
		return All
	}

	applicable := make([]Role, 0, len(mc.Roles)+1)
	applicable = append(applicable, mc.EveryoneRole)
	applicable = append(applicable, mc.Roles...)
	sort.SliceStable(applicable, func(i, j int) bool {
		return applicable[i].Position < applicable[j].Position
	})

	owByRole := make(map[string]ChannelOverwrite, len(mc.Overwrites))
	for _, ow := range mc.Overwrites {
		owByRole[ow.RoleID] = ow
	}

	for _, r := range applicable {
		ow, ok := owByRole[r.ID]
		if !ok {
			continue
		}
		base = base.AndNot(ow.Deny).Or(ow.Allow)
	}

	if mc.MemberOverwrite != nil {
		base = base.AndNot(mc.MemberOverwrite.Deny).Or(mc.MemberOverwrite.Allow)
	}

	return base
}
