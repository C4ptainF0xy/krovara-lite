package messagepush

import (
	"testing"

	"github.com/google/uuid"
)

func TestResolveMentions(t *testing.T) {
	a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	got := resolveMentions("hey @"+a+" and @"+b+" check this", nil, nil)
	if _, ok := got[uuid.MustParse(a)]; !ok {
		t.Errorf("expected mention of %s", a)
	}
	if len(got) != 2 {
		t.Errorf("got %d uuid mentions, want 2", len(got))
	}

	alice := uuid.MustParse(a)
	bob := uuid.MustParse(b)
	carol := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	byUser := map[string]uuid.UUID{"alice": alice}
	roles := map[string][]uuid.UUID{"mods": {bob, carol}}

	got = resolveMentions("yo @Alice ping @mods now", byUser, roles)
	for _, want := range []uuid.UUID{alice, bob, carol} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected %s mentioned", want)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

func TestResolveMentions_noneOnPlainText(t *testing.T) {
	if len(resolveMentions("hello world", map[string]uuid.UUID{"x": uuid.New()}, nil)) != 0 {
		t.Error("plain text should produce no mentions")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("short string mangled: %q", got)
	}
	long := "01234567890123456789"
	if got := truncate(long, 10); got != "012345678…" {
		t.Errorf("truncate = %q", got)
	}
}
