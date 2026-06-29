package voip_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/krovara/krovara/internal/voip"
)

func TestIssue_HMACRoundTrips(t *testing.T) {
	s := voip.NewService("super-secret", []string{"turn:turn.example.com:3478"})
	uid := uuid.New()

	cred := s.Issue(uid)
	require.NotEmpty(t, cred.Username)
	require.NotEmpty(t, cred.Credential)
	require.True(t, strings.HasSuffix(cred.Username, uid.String()))
	require.Equal(t, []string{"turn:turn.example.com:3478"}, cred.URIs)
	require.NoError(t, s.VerifyCredential(cred.Username, cred.Credential))
}

func TestVerifyCredential_RejectsTamper(t *testing.T) {
	s := voip.NewService("super-secret", []string{"turn:turn.example.com:3478"})
	cred := s.Issue(uuid.New())

	require.Error(t, s.VerifyCredential(cred.Username, cred.Credential+"x"))

	require.Error(t, s.VerifyCredential("9999999999:other", cred.Credential))
}

func TestVerifyCredential_RejectsExpired(t *testing.T) {
	s := voip.NewService("super-secret", []string{"turn:turn.example.com:3478"})

	uid := uuid.New()
	past := time.Now().Add(-time.Hour).Unix()
	username := timestampUser(past, uid.String())
	cred := hmacFor("super-secret", username)
	require.ErrorContains(t, s.VerifyCredential(username, cred), "expired")
}

func TestHandleCredentials_UnauthReturns401(t *testing.T) {
	s := voip.NewService("super-secret", []string{"turn:turn.example.com:3478"})
	r := chi.NewMux()
	s.Routes(r, func(context.Context) uuid.UUID { return uuid.Nil })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/voip/turn-credentials", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleCredentials_ReturnsJSON(t *testing.T) {
	s := voip.NewService("super-secret", []string{"turn:turn.example.com:3478"})
	uid := uuid.New()
	r := chi.NewMux()
	s.Routes(r, func(context.Context) uuid.UUID { return uid })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/voip/turn-credentials", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	var body voip.Credential
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	require.Contains(t, body.Username, uid.String())
	require.NoError(t, s.VerifyCredential(body.Username, body.Credential))
}

func TestHandleCredentials_UnconfiguredReturns503(t *testing.T) {
	s := voip.NewService("", nil)
	uid := uuid.New()
	r := chi.NewMux()
	s.Routes(r, func(context.Context) uuid.UUID { return uid })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/voip/turn-credentials", nil))
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func timestampUser(ts int64, user string) string {
	return strconv.FormatInt(ts, 10) + ":" + user
}

func hmacFor(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
