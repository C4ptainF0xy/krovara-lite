package permissions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitfield_Has(t *testing.T) {
	b := ViewChannel | SendMessages
	require.True(t, b.Has(ViewChannel))
	require.True(t, b.Has(SendMessages))
	require.True(t, b.Has(ViewChannel|SendMessages))
	require.False(t, b.Has(ManageMessages))
	require.False(t, b.Has(ViewChannel|ManageMessages))
}

func TestBitfield_Or(t *testing.T) {
	require.Equal(t, ViewChannel|SendMessages, ViewChannel.Or(SendMessages))
	require.Equal(t, ViewChannel, ViewChannel.Or(ViewChannel))
}

func TestBitfield_AndNot(t *testing.T) {
	b := ViewChannel | SendMessages | ManageMessages
	require.Equal(t, ViewChannel|ManageMessages, b.AndNot(SendMessages))
	require.Equal(t, Bitfield(0), b.AndNot(b))
	require.Equal(t, b, b.AndNot(BanMembers))
}

func TestBitfield_Int64RoundTrip(t *testing.T) {
	cases := []Bitfield{0, ViewChannel, All, Administrator}
	for _, c := range cases {
		require.Equal(t, c, FromInt64(c.ToInt64()))
	}
}

func TestAll_IncludesEveryConstant(t *testing.T) {
	for _, p := range []Bitfield{
		ViewChannel, SendMessages, ManageMessages, ManageChannels,
		ManageRoles, KickMembers, BanMembers, ManageSpace,
		CreateInvite, ConnectVoice, SpeakVoice, Administrator,
	} {
		require.True(t, All.Has(p), "All missing %b", p)
	}
}

func TestConstants_AreDistinctSingleBits(t *testing.T) {
	all := []Bitfield{
		ViewChannel, SendMessages, ManageMessages, ManageChannels,
		ManageRoles, KickMembers, BanMembers, ManageSpace,
		CreateInvite, ConnectVoice, SpeakVoice, Administrator,
	}
	seen := map[Bitfield]bool{}
	for _, p := range all {
		require.NotZero(t, p)
		require.Zero(t, p&(p-1), "%b has more than one bit set", p)
		require.False(t, seen[p], "duplicate bit %b", p)
		seen[p] = true
	}
}
