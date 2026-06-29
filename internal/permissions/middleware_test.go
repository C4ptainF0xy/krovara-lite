package permissions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type stubResolver struct {
	mc  MemberContext
	err error
}

func (s stubResolver) ResolveSpace(_ context.Context, _, _ uuid.UUID) (MemberContext, error) {
	return s.mc, s.err
}

func (s stubResolver) ResolveChannel(_ context.Context, _, _ uuid.UUID) (MemberContext, error) {
	return s.mc, s.err
}

func newRouter(resolver Resolver, uidFn UserIDFunc, need Bitfield) http.Handler {
	r := chi.NewRouter()
	r.With(RequireChannel(resolver, uidFn, need)).Get("/channels/{channelID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.With(RequireSpace(resolver, uidFn, need)).Get("/spaces/{spaceID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return r
}

func TestRequire_Allows(t *testing.T) {
	uid := uuid.New()
	cid := uuid.New()
	resolver := stubResolver{mc: MemberContext{
		EveryoneRole: everyone(ViewChannel),
	}}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ViewChannel)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/"+cid.String(), nil))
	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestRequire_Forbidden(t *testing.T) {
	uid := uuid.New()
	cid := uuid.New()
	resolver := stubResolver{mc: MemberContext{
		EveryoneRole: everyone(ViewChannel),
	}}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ManageSpace)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/"+cid.String(), nil))
	require.Equal(t, http.StatusForbidden, rr.Code)
}

func TestRequire_Unauthenticated(t *testing.T) {
	h := newRouter(stubResolver{}, func(context.Context) uuid.UUID { return uuid.Nil }, ViewChannel)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/"+uuid.New().String(), nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRequire_BadChannelID(t *testing.T) {
	uid := uuid.New()
	h := newRouter(stubResolver{}, func(context.Context) uuid.UUID { return uid }, ViewChannel)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/not-a-uuid", nil))
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRequire_ResolverError(t *testing.T) {
	uid := uuid.New()
	cid := uuid.New()
	resolver := stubResolver{err: errors.New("db down")}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ViewChannel)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/"+cid.String(), nil))
	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestRequire_OwnerAllowed(t *testing.T) {
	uid := uuid.New()
	cid := uuid.New()
	resolver := stubResolver{mc: MemberContext{
		IsOwner:      true,
		EveryoneRole: everyone(0),
	}}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ManageSpace)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/channels/"+cid.String(), nil))
	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestRequireSpace_Allows(t *testing.T) {
	uid := uuid.New()
	sid := uuid.New()
	resolver := stubResolver{mc: MemberContext{
		IsOwner:      true,
		EveryoneRole: everyone(0),
	}}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ManageSpace)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/spaces/"+sid.String(), nil))
	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestRequireSpace_Forbidden(t *testing.T) {
	uid := uuid.New()
	sid := uuid.New()
	resolver := stubResolver{mc: MemberContext{
		EveryoneRole: everyone(ViewChannel),
	}}
	h := newRouter(resolver, func(context.Context) uuid.UUID { return uid }, ManageSpace)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/spaces/"+sid.String(), nil))
	require.Equal(t, http.StatusForbidden, rr.Code)
}
