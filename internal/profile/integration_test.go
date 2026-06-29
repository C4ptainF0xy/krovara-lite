package profile_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	"github.com/krovara/krovara/internal/profile"
)

type env struct {
	srv *httptest.Server
	q   *db.Queries
}

func setup(t *testing.T) *env {
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

	migDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	m, err := migrate.New("file://"+filepath.ToSlash(migDir), "pgx5://"+dsn[len("postgres://"):])
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
	authSvc := auth.NewService(q, signer, sessions)
	profileSvc := profile.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			profileSvc.Routes(api, auth.UserID)
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, q: q}
}

func (e *env) do(t *testing.T, method, path string, body any, bearer string) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func register(t *testing.T, e *env, username, email string) string {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/auth/register", map[string]any{
		"username": username, "email": email, "password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	_ = resp.Body.Close()
	return body.AccessToken
}

func TestProfile_UpdateAndFetch(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodGet, "/api/me", nil, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var me map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	_ = resp.Body.Close()
	require.Equal(t, "bob", me["username"])
	require.Nil(t, me["display_name"])

	resp = e.do(t, http.MethodPatch, "/api/me", map[string]any{
		"display_name": "Bobby",
		"status":       "en jeu",
	}, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	_ = resp.Body.Close()
	require.Equal(t, "Bobby", updated["display_name"])
	require.Equal(t, "en jeu", updated["status"])

	resp = e.do(t, http.MethodGet, "/api/me", nil, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	_ = resp.Body.Close()
	require.Equal(t, "Bobby", me["display_name"])

	resp = e.do(t, http.MethodPatch, "/api/me", map[string]any{"status": "dispo"}, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	_ = resp.Body.Close()
	require.Equal(t, "Bobby", updated["display_name"])
	require.Equal(t, "dispo", updated["status"])
}

func TestProfile_ValidationRejectsLongDisplayName(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodPatch, "/api/me", map[string]any{
		"display_name": strings.Repeat("x", 65),
	}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestProfile_ChangePassword(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodPatch, "/api/me/password", map[string]any{
		"current_password": "nope",
		"new_password":     "a brand new secret",
	}, tok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/me/password", map[string]any{
		"current_password": "correct horse battery",
		"new_password":     "short",
	}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/me/password", map[string]any{
		"current_password": "correct horse battery",
		"new_password":     "a brand new secret",
	}, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email": "bob@example.com", "password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email": "bob@example.com", "password": "a brand new secret",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestProfile_RequiresAuth(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	resp := e.do(t, http.MethodGet, "/api/me", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}
