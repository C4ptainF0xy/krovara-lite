package tasks_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
	"github.com/krovara/krovara/internal/tasks"
)

type env struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	pg, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("krovara"), tcpg.WithUsername("krovara"), tcpg.WithPassword("krovara"),
		tcpg.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pg) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	migDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	m, err := migrate.New("file://"+filepath.ToSlash(migDir), "pgx5://"+dsn[len("postgres://"):])
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	q := db.New(pool)
	signer := auth.NewJWTSigner([]byte("test-secret"), time.Hour)
	sessions := auth.NewSessionStore(q, 24*time.Hour)
	authSvc := auth.NewService(q, signer, sessions)
	resolver := permissions.NewPGResolver(q)
	spacesSvc := spaces.NewService(pool)
	tasksSvc := tasks.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			tasksSvc.Routes(api, resolver, auth.UserID)
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, pool: pool}
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

func createSpace(t *testing.T, e *env, tok, name string) string {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": name}, tok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	return sp["id"].(string)
}

func TestTasks_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "alice", "alice@example.com")
	sid := createSpace(t, e, tok, "Work")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+sid+"/tasks",
		map[string]any{"title": "Préparer la démo", "source_archive_id": "stanza-1"}, tok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var task map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&task))
	_ = resp.Body.Close()
	require.Equal(t, "open", task["status"])
	require.Equal(t, "stanza-1", task["source_archive_id"])
	tid := task["id"].(string)

	resp = e.do(t, http.MethodGet, "/api/spaces/"+sid+"/tasks", nil, tok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)

	resp = e.do(t, http.MethodPatch, "/api/tasks/"+tid, map[string]any{"status": "done"}, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&task))
	_ = resp.Body.Close()
	require.Equal(t, "done", task["status"])

	resp = e.do(t, http.MethodPatch, "/api/tasks/"+tid, map[string]any{"status": "bogus"}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/tasks/"+tid, nil, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+sid+"/tasks", nil, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 0)
}

func TestTasks_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	sid := createSpace(t, e, ownerTok, "Work")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+sid+"/tasks", map[string]any{"title": "x"}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
