package bots

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestComponentFilename(t *testing.T) {
	require.Equal(t, "bot-abc12345.cfg.lua", componentFilename("bot-abc12345.krovara.local"))
	require.Equal(t, "bot-abc12345.cfg.lua", componentFilename("bot-abc12345"))
}

func TestWriteComponent_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewService(nil, "krovara.local").WithComponentsDir(dir)

	jid := "bot-deadbeef.krovara.local"
	require.NoError(t, s.writeComponent(jid, "s3cret"))

	body, err := os.ReadFile(filepath.Join(dir, "bot-deadbeef.cfg.lua"))
	require.NoError(t, err)
	require.Contains(t, string(body), `Component "bot-deadbeef.krovara.local"`)
	require.Contains(t, string(body), `component_secret = "s3cret"`)

	require.NoError(t, s.removeComponent(jid))
	_, err = os.Stat(filepath.Join(dir, "bot-deadbeef.cfg.lua"))
	require.True(t, os.IsNotExist(err))
}

func TestWriteComponent_DisabledWhenNoDir(t *testing.T) {
	s := NewService(nil, "krovara.local")
	require.NoError(t, s.writeComponent("bot-x.krovara.local", "x"))
	require.NoError(t, s.removeComponent("bot-x.krovara.local"))
}

func TestComponentJID_StableShape(t *testing.T) {
	s := NewService(nil, "example.com")
	id := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	jid := s.componentJID(id)
	require.True(t, strings.HasPrefix(jid, "bot-"))
	require.True(t, strings.HasSuffix(jid, ".example.com"))
	require.Equal(t, "bot-01234567.example.com", jid)
}

func TestComponentJID_FallsBackToDefaultDomain(t *testing.T) {
	s := NewService(nil, "")
	id := uuid.New()
	require.True(t, strings.HasSuffix(s.componentJID(id), "."+DefaultDomain))
}

func TestHashSecret_StableForSameInput(t *testing.T) {
	a := HashSecret("hunter2")
	b := HashSecret("hunter2")
	require.Equal(t, a, b)
}

func TestHashSecret_DifferentForDifferentInput(t *testing.T) {
	a := HashSecret("hunter2")
	b := HashSecret("hunter3")
	require.NotEqual(t, a, b)
}
