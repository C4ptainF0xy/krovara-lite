package games_test

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
	"github.com/krovara/krovara/internal/games"
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
	gamesSvc := games.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			gamesSvc.Routes(api, auth.UserID)
			gamesSvc.AdminRoutes(api, auth.UserID)
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

func makeAdmin(t *testing.T, e *env, email string) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `UPDATE users SET is_admin = true WHERE email = $1`, email)
	require.NoError(t, err)
}

func TestGames_SubmitReviewCatalogue(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	userTok := register(t, e, "alice", "alice@example.com")
	adminTok := register(t, e, "staff", "staff@example.com")
	makeAdmin(t, e, "staff@example.com")

	resp := e.do(t, http.MethodPost, "/api/games", map[string]any{"name": "Valorant"}, userTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var g map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&g))
	_ = resp.Body.Close()
	require.Equal(t, "pending", g["status"])
	gid := g["id"].(string)

	resp = e.do(t, http.MethodPost, "/api/games", map[string]any{"name": "valorant"}, userTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/games", nil, userTok)
	var cat []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cat))
	_ = resp.Body.Close()
	require.Len(t, cat, 0)

	resp = e.do(t, http.MethodGet, "/api/admin/games/pending", nil, userTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/admin/games/pending", nil, adminTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cat))
	_ = resp.Body.Close()
	require.Len(t, cat, 1)

	resp = e.do(t, http.MethodPost, "/api/admin/games/"+gid+"/review", map[string]any{"approve": true}, adminTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/games?q=valor", nil, userTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cat))
	_ = resp.Body.Close()
	require.Len(t, cat, 1)
	require.Equal(t, "approved", cat[0]["status"])
}

func TestGames_Reject(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	userTok := register(t, e, "alice", "alice@example.com")
	adminTok := register(t, e, "staff", "staff@example.com")
	makeAdmin(t, e, "staff@example.com")

	resp := e.do(t, http.MethodPost, "/api/games", map[string]any{"name": "Troll Game"}, userTok)
	var g map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&g))
	_ = resp.Body.Close()
	gid := g["id"].(string)

	resp = e.do(t, http.MethodPost, "/api/admin/games/"+gid+"/review",
		map[string]any{"approve": false, "reason": "image invalide"}, adminTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&g))
	_ = resp.Body.Close()
	require.Equal(t, "rejected", g["status"])
	require.Equal(t, "image invalide", g["reject_reason"])

	resp = e.do(t, http.MethodGet, "/api/games", nil, userTok)
	var cat []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cat))
	_ = resp.Body.Close()
	require.Len(t, cat, 0)
}
