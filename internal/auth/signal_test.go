package auth

import "testing"

func TestSignupIPHash(t *testing.T) {
	disabled := &Service{}
	if got := disabled.signupIPHash("203.0.113.7"); got != "" {
		t.Fatalf("no key configured → want empty, got %q", got)
	}

	s := &Service{SignupIPKey: []byte("test-hmac-key")}
	if got := s.signupIPHash(""); got != "" {
		t.Fatalf("empty IP → want empty, got %q", got)
	}

	a := s.signupIPHash("203.0.113.7")
	again := s.signupIPHash("203.0.113.7")
	other := s.signupIPHash("203.0.113.8")
	if a == "" {
		t.Fatal("keyed hash of a real IP must not be empty")
	}
	if a != again {
		t.Fatalf("hash must be deterministic: %q != %q", a, again)
	}
	if a == other {
		t.Fatal("different IPs must not collide")
	}

	s2 := &Service{SignupIPKey: []byte("other-key")}
	if s2.signupIPHash("203.0.113.7") == a {
		t.Fatal("digest must depend on the HMAC key")
	}
}
