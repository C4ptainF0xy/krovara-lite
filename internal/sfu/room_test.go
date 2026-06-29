package sfu

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoomJoinLeave(t *testing.T) {
	hub := NewHub(nil)
	r := hub.GetOrCreate("room-1")

	a := &Peer{id: "a"}
	b := &Peer{id: "b"}

	require.Empty(t, r.Join(a), "first peer sees no others")

	existing := r.Join(b)
	require.Len(t, existing, 1)
	require.Equal(t, "a", existing[0].id)

	others := r.Others("a")
	require.Len(t, others, 1)
	require.Equal(t, "b", others[0].id)

	require.ElementsMatch(t, []string{"room-1"}, hub.Rooms())

	r.Leave("a")
	require.ElementsMatch(t, []string{"room-1"}, hub.Rooms(), "room still alive while b is in")

	r.Leave("b")
	require.Empty(t, hub.Rooms(), "empty room dropped from hub")
}

func TestHubGetOrCreateIsStable(t *testing.T) {
	hub := NewHub(nil)
	r1 := hub.GetOrCreate("x")
	r2 := hub.GetOrCreate("x")
	require.Same(t, r1, r2)
}
