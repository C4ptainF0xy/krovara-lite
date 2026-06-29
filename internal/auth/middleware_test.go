package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestUserIDEmptyContext(t *testing.T) {
	if got := UserID(context.Background()); got != uuid.Nil {
		t.Fatalf("expected uuid.Nil, got %s", got)
	}
}

func TestUserIDWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "not-a-uuid")
	if got := UserID(ctx); got != uuid.Nil {
		t.Fatalf("expected uuid.Nil for wrong type, got %s", got)
	}
}
