package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestSigner(t *testing.T, ttl time.Duration, now func() time.Time) *JWTSigner {
	t.Helper()
	s := NewJWTSigner([]byte("test-secret-do-not-use"), ttl)
	if now != nil {
		s.now = now
	}
	return s
}

func TestJWTSignAndParseRoundTrip(t *testing.T) {
	s := newTestSigner(t, time.Hour, nil)
	uid := uuid.New()
	tok, err := s.Sign(uid)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := s.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != uid {
		t.Fatalf("user mismatch: got %s want %s", got, uid)
	}
}

func TestJWTExpired(t *testing.T) {
	now := time.Now()
	signer := newTestSigner(t, time.Minute, func() time.Time { return now })
	tok, _ := signer.Sign(uuid.New())

	signer.now = func() time.Time { return now.Add(2 * time.Minute) }
	_, err := signer.Parse(tok)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTWrongSecret(t *testing.T) {
	a := newTestSigner(t, time.Hour, nil)
	b := NewJWTSigner([]byte("different-secret"), time.Hour)
	tok, _ := a.Sign(uuid.New())
	if _, err := b.Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTGarbage(t *testing.T) {
	s := newTestSigner(t, time.Hour, nil)
	for _, bad := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := s.Parse(bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("expected ErrInvalidToken for %q, got %v", bad, err)
		}
	}
}
