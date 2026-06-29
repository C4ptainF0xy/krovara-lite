package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
)

type testEnv struct {
	srv      *httptest.Server
	mux      *chi.Mux
	svc      *auth.Service
	sessions *auth.SessionStore
}

type stubCaptcha struct{ accept string }

func (s stubCaptcha) Verify(_ context.Context, token, _ string) (bool, error) {
	return token == s.accept, nil
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	pg, err := tcpg.Run(ctx,
		"postgres:16-alpine",
		tcpg.WithDatabase("krovara"),
		tcpg.WithUsername("krovara"),
		tcpg.WithPassword("krovara"),
		tcpg.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pg) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	migrationsDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	migrateDSN := "pgx5://" + dsn[len("postgres://"):]
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsDir), migrateDSN)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	srcErr, dbErr := m.Close()
	require.NoError(t, srcErr)
	require.NoError(t, dbErr)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	q := db.New(pool)
	signer := auth.NewJWTSigner([]byte("test-secret"), time.Hour)
	sessions := auth.NewSessionStore(q, 24*time.Hour)

	sessions.SetGrace(0)
	svc := auth.NewService(q, signer, sessions)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { svc.Routes(r, nil) })
	mux.With(auth.RequireAuth(signer)).Get("/api/me", func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r.Context())
		_, _ = fmt.Fprint(w, uid.String())
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, mux: mux, svc: svc, sessions: sessions}
}

func (e *testEnv) post(t *testing.T, path string, body any, bearer string) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func (e *testEnv) get(t *testing.T, path string, bearer string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeTokens(t *testing.T, resp *http.Response) (access, refresh string) {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	_ = resp.Body.Close()
	return body.AccessToken, body.RefreshToken
}

func TestAuthFullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)

	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "correct horse battery",
	}, "")
	access, refresh := decodeTokens(t, resp)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)

	r := env.get(t, "/api/me", access)
	require.Equal(t, http.StatusOK, r.StatusCode)
	_ = r.Body.Close()

	resp = env.post(t, "/api/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": "correct horse battery",
	}, "")
	loginAccess, loginRefresh := decodeTokens(t, resp)
	require.NotEmpty(t, loginAccess)
	require.NotEqual(t, refresh, loginRefresh, "refresh tokens must be unique per session")

	resp = env.post(t, "/api/auth/refresh", map[string]any{
		"refresh_token": loginRefresh,
	}, "")
	_, rotated := decodeTokens(t, resp)
	require.NotEqual(t, loginRefresh, rotated)

	resp = env.post(t, "/api/auth/refresh", map[string]any{
		"refresh_token": loginRefresh,
	}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/logout", map[string]any{
		"refresh_token": rotated,
	}, "")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/refresh", map[string]any{
		"refresh_token": rotated,
	}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestProtectedRouteUnauthorized(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)

	r := env.get(t, "/api/me", "")
	require.Equal(t, http.StatusUnauthorized, r.StatusCode)
	_ = r.Body.Close()

	r = env.get(t, "/api/me", "not-a-jwt")
	require.Equal(t, http.StatusUnauthorized, r.StatusCode)
	_ = r.Body.Close()
}

func TestLoginWrongPassword(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
		"password": "rightpassword",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/login", map[string]any{
		"email":    "bob@example.com",
		"password": "wrongpassword",
	}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestRefreshUnknownToken(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	resp := env.post(t, "/api/auth/refresh", map[string]any{
		"refresh_token": "totally-fake-token",
	}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "mallory", "email": "m@example.com", "password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, refresh := decodeTokens(t, resp)

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": refresh}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, r2 := decodeTokens(t, resp)

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": refresh}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": r2}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "family must be revoked after reuse")
	_ = resp.Body.Close()
}

func TestRefreshBenignRaceTolerated(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	env.sessions.SetGrace(15 * time.Second)

	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "raceuser", "email": "race@example.com", "password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, r1 := decodeTokens(t, resp)

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": r1}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, r2 := decodeTokens(t, resp)

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": r1}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "benign concurrent refresh must be tolerated")
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/refresh", map[string]any{"refresh_token": r2}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "family must survive a benign race")
	_ = resp.Body.Close()
}

func TestRegisterCaptchaGate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	env.svc.Captcha = stubCaptcha{accept: "good-token"}

	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "alice", "email": "alice@example.com", "password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/register", map[string]any{
		"username": "alice", "email": "alice@example.com", "password": "correct horse battery",
		"captcha_token": "nope",
	}, "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/register", map[string]any{
		"username": "alice", "email": "alice@example.com", "password": "correct horse battery",
		"captcha_token": "good-token",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestRegisterValidation(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)

	resp := env.post(t, "/api/auth/register", map[string]any{
		"username": "dan",
		"password": "danpassword",
	}, "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/auth/register", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	r, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
	_ = r.Body.Close()
}

func TestRegisterDuplicate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setup(t)
	body := map[string]any{
		"username": "carol",
		"email":    "carol@example.com",
		"password": "carolspassword",
	}
	resp := env.post(t, "/api/auth/register", body, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = env.post(t, "/api/auth/register", body, "")
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}
