package permissions

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Resolver interface {
	ResolveSpace(ctx context.Context, userID, spaceID uuid.UUID) (MemberContext, error)
	ResolveChannel(ctx context.Context, userID, channelID uuid.UUID) (MemberContext, error)
}

type UserIDFunc func(context.Context) uuid.UUID

func RequireSpace(resolver Resolver, userID UserIDFunc, need Bitfield) func(http.Handler) http.Handler {
	return guard(need, userID, "spaceID", resolver.ResolveSpace)
}

func RequireChannel(resolver Resolver, userID UserIDFunc, need Bitfield) func(http.Handler) http.Handler {
	return guard(need, userID, "channelID", resolver.ResolveChannel)
}

type resolveFunc func(context.Context, uuid.UUID, uuid.UUID) (MemberContext, error)

func guard(need Bitfield, userID UserIDFunc, param string, resolve resolveFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := userID(r.Context())
			if uid == uuid.Nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			target, err := uuid.Parse(chi.URLParam(r, param))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mc, err := resolve(r.Context(), uid, target)
			if errors.Is(err, ErrNotMember) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if !Compute(mc).Has(need) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
