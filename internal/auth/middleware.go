package auth

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/krovara/krovara/internal/db"
)

type ctxKey struct{}

type disabledCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[uuid.UUID]disabledEntry
}

type disabledEntry struct {
	disabled bool
	at       time.Time
}

func (c *disabledCache) isDisabled(ctx context.Context, q db.Querier, uid uuid.UUID) bool {
	c.mu.RLock()
	e, ok := c.m[uid]
	c.mu.RUnlock()
	if ok && time.Since(e.at) < c.ttl {
		return e.disabled
	}
	u, err := q.GetUserByID(ctx, pgUUID(uid))
	if err != nil {

		return false
	}
	c.mu.Lock()
	c.m[uid] = disabledEntry{disabled: u.Disabled, at: time.Now()}
	c.mu.Unlock()
	return u.Disabled
}

func RequireAuth(signer *JWTSigner, q ...db.Querier) func(http.Handler) http.Handler {
	var querier db.Querier
	if len(q) > 0 {
		querier = q[0]
	}
	cache := &disabledCache{ttl: 15 * time.Second, m: make(map[uuid.UUID]disabledEntry)}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			uid, err := signer.Parse(token)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if querier != nil && cache.isDisabled(r.Context(), querier, uid) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(ctxKey{}).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}
