package permissions

type Bitfield uint64

const (
	ViewChannel    Bitfield = 1 << 0
	SendMessages   Bitfield = 1 << 1
	ManageMessages Bitfield = 1 << 2
	ManageChannels Bitfield = 1 << 3
	ManageRoles    Bitfield = 1 << 4
	KickMembers    Bitfield = 1 << 5
	BanMembers     Bitfield = 1 << 6
	ManageSpace    Bitfield = 1 << 7
	CreateInvite   Bitfield = 1 << 8
	ConnectVoice   Bitfield = 1 << 9
	SpeakVoice     Bitfield = 1 << 10
	Administrator  Bitfield = 1 << 11
)

const All = ViewChannel | SendMessages | ManageMessages | ManageChannels |
	ManageRoles | KickMembers | BanMembers | ManageSpace |
	CreateInvite | ConnectVoice | SpeakVoice | Administrator

func (b Bitfield) Has(want Bitfield) bool {
	return b&want == want
}

func (b Bitfield) Or(other Bitfield) Bitfield {
	return b | other
}

func (b Bitfield) AndNot(mask Bitfield) Bitfield {
	return b &^ mask
}

func FromInt64(v int64) Bitfield {
	return Bitfield(uint64(v))
}

func (b Bitfield) ToInt64() int64 {
	return int64(b)
}
