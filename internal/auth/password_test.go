package auth

import (
	"strings"
	"testing"
)

var testParams = Argon2idParams{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPasswordWithParams("hunter2", testParams)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected prefix: %q", hash)
	}

	ok, err := VerifyPassword("hunter2", hash)
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v err=%v", ok, err)
	}

	ok, err = VerifyPassword("hunter3", hash)
	if err != nil {
		t.Fatalf("verify wrong: err=%v", err)
	}
	if ok {
		t.Fatal("verify wrong: expected mismatch")
	}
}

func TestHashDifferentEachCall(t *testing.T) {
	a, _ := HashPasswordWithParams("same", testParams)
	b, _ := HashPasswordWithParams("same", testParams)
	if a == b {
		t.Fatal("salt should make each hash unique")
	}
}

func TestVerifyMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$only-three-parts",
		"$argon2i$v=19$m=8,t=1,p=1$xxx$yyy",
		"$argon2id$v=99$m=8,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=19$m=bad,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=19$m=8,t=1,p=1$@@@@$BBBB",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}
