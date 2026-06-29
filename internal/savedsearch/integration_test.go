package savedsearch_test

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
	"github.com/krovara/krovara/internal/savedsearch"
)

type env struct {
	srv *httptest.Server
	q   *db.Queries
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
	svc := savedsearch.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) { svc.Routes(api, auth.UserID) })
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

func TestSavedSearch_CRUDAndIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	aliceTok := register(t, e, "alice", "alice@example.com")
	bobTok := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodPost, "/api/me/saved-searches", map[string]any{"name": "", "query": "x"}, aliceTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/me/saved-searches",
		map[string]any{"name": "Bugs", "query": "from:dev crash"}, aliceTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()
	id := created["id"].(string)
	require.Equal(t, "Bugs", created["name"])

	resp = e.do(t, http.MethodGet, "/api/me/saved-searches", nil, aliceTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)

	resp = e.do(t, http.MethodGet, "/api/me/saved-searches", nil, bobTok)
	var bobList []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&bobList))
	_ = resp.Body.Close()
	require.Len(t, bobList, 0)

	resp = e.do(t, http.MethodDelete, "/api/me/saved-searches/"+id, nil, bobTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/me/saved-searches/"+id, nil, aliceTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/me/saved-searches", nil, aliceTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 0)
}
