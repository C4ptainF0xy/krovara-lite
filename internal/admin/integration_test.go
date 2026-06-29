package admin_test

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/admin"
	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
)

type env struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
	q    *db.Queries
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
	adminSvc := admin.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			adminSvc.Routes(api, auth.UserID)
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, pool: pool, q: q}
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

func register(t *testing.T, e *env, username, email string) (token string, userID uuid.UUID) {
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
	user, err := e.q.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	return body.AccessToken, uuid.UUID(user.ID.Bytes)
}

func (e *env) makeAdmin(t *testing.T, userID uuid.UUID) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(),
		`UPDATE users SET is_admin = true WHERE id = $1`, userID)
	require.NoError(t, err)
}

func (e *env) login(t *testing.T, email string) int {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": "correct horse battery",
	}, "")
	code := resp.StatusCode
	_ = resp.Body.Close()
	return code
}

func TestAdmin_NonAdminForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	userTok, _ := register(t, e, "bob", "bob@example.com")
	resp := e.do(t, http.MethodGet, "/api/admin/users", nil, userTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestAdmin_DisableUserBlocksLogin(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	adminTok, adminID := register(t, e, "root", "root@example.com")
	e.makeAdmin(t, adminID)
	_, bobID := register(t, e, "bob", "bob@example.com")

	require.Equal(t, http.StatusOK, e.login(t, "bob@example.com"))

	resp := e.do(t, http.MethodPatch, "/api/admin/users/"+bobID.String(),
		map[string]any{"disabled": true}, adminTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	require.Equal(t, http.StatusForbidden, e.login(t, "bob@example.com"))

	resp = e.do(t, http.MethodPatch, "/api/admin/users/"+bobID.String(),
		map[string]any{"disabled": false}, adminTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, e.login(t, "bob@example.com"))
}

func TestAdmin_DeleteUser(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	adminTok, adminID := register(t, e, "root", "root@example.com")
	e.makeAdmin(t, adminID)
	_, bobID := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodDelete, "/api/admin/users/"+bobID.String(), nil, adminTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	_, err := e.q.GetUserByEmail(context.Background(), "bob@example.com")
	require.Error(t, err)
}

func TestAdmin_CannotDisableSelf(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	adminTok, adminID := register(t, e, "root", "root@example.com")
	e.makeAdmin(t, adminID)

	resp := e.do(t, http.MethodPatch, "/api/admin/users/"+adminID.String(),
		map[string]any{"disabled": true}, adminTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestAdmin_UserSignals(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	adminTok, adminID := register(t, e, "root", "root@example.com")
	e.makeAdmin(t, adminID)
	_, aliceID := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	_, eveID := register(t, e, "eve", "eve@example.com")

	for _, id := range []uuid.UUID{aliceID, bobID} {
		_, err := e.pool.Exec(context.Background(),
			`UPDATE users SET signup_ip_hash = $2 WHERE id = $1`, id, "deadbeefcafe")
		require.NoError(t, err)
	}

	got := e.signals(t, adminTok, aliceID)
	require.Equal(t, true, got["signup_ip_known"])
	require.EqualValues(t, 1, got["signup_ip_sibling_acc"])

	got = e.signals(t, adminTok, eveID)
	require.Equal(t, false, got["signup_ip_known"])
	require.EqualValues(t, 0, got["signup_ip_sibling_acc"])

	userTok, _ := register(t, e, "mallory", "mallory@example.com")
	resp := e.do(t, http.MethodGet, "/api/admin/users/"+aliceID.String()+"/signals", nil, userTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func (e *env) signals(t *testing.T, bearer string, userID uuid.UUID) map[string]any {
	t.Helper()
	resp := e.do(t, http.MethodGet, "/api/admin/users/"+userID.String()+"/signals", nil, bearer)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	_ = resp.Body.Close()
	return out
}
