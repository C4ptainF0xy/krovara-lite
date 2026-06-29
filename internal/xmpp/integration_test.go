package xmpp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/xmpp"
)

type env struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
	q    *db.Queries
	svc  *xmpp.Service
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
	xmppSvc := xmpp.NewService(q)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) { xmppSvc.PublicRoutes(api, auth.UserID) })
	})
	mux.Route("/internal", func(r chi.Router) { xmppSvc.InternalRoutes(r) })

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, pool: pool, q: q, svc: xmppSvc}
}

func registerAndGetID(t *testing.T, e *env, username, email string) (token string, userID uuid.UUID) {
	t.Helper()
	body := strings.NewReader(`{"username":"` + username + `","email":"` + email + `","password":"correct horse battery"}`)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	user, err := e.q.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	return out.AccessToken, uuid.UUID(user.ID.Bytes)
}

func TestBridge_IssueAndConsume(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	access, userID := registerAndGetID(t, e, "alice", "alice@example.com")

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/xmpp/token", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var tok struct {
		Token     string    `json:"token"`
		JID       string    `json:"jid"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tok))
	require.NotEmpty(t, tok.Token)
	require.Equal(t, userID.String()+"@krovara.local", tok.JID)
	require.WithinDuration(t, time.Now().Add(xmpp.DefaultTokenTTL), tok.ExpiresAt, 5*time.Second)

	q := url.Values{}
	q.Set("user", userID.String())
	q.Set("pass", tok.Token)
	resp, err = http.Get(e.srv.URL + "/internal/xmpp/check_password?" + q.Encode())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(e.srv.URL + "/internal/xmpp/check_password?" + q.Encode())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBridge_RejectsMismatchedUser(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	_, aliceID := registerAndGetID(t, e, "alice", "alice@example.com")
	_, bobID := registerAndGetID(t, e, "bob", "bob@example.com")

	token, _, err := e.svc.IssueToken(context.Background(), aliceID)
	require.NoError(t, err)

	form := url.Values{}
	form.Set("user", bobID.String())
	form.Set("pass", token)
	resp, err := http.Get(e.srv.URL + "/internal/xmpp/check_password?" + form.Encode())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	form.Set("user", aliceID.String())
	resp, err = http.Get(e.srv.URL + "/internal/xmpp/check_password?" + form.Encode())
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBridge_RejectsExpiredToken(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	_, userID := registerAndGetID(t, e, "alice", "alice@example.com")

	token, _, err := e.svc.IssueToken(context.Background(), userID)
	require.NoError(t, err)

	_, err = e.pool.Exec(context.Background(),
		`UPDATE xmpp_tokens SET expires_at = NOW() - INTERVAL '1 minute' WHERE token = $1`, token)
	require.NoError(t, err)

	form := url.Values{}
	form.Set("user", userID.String())
	form.Set("pass", token)
	resp, err := http.Get(e.srv.URL + "/internal/xmpp/check_password?" + form.Encode())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBridge_TokenRequiresAuth(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	resp, err := http.Post(e.srv.URL+"/api/xmpp/token", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBridge_ProsodyAuthMalformed(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)

	resp, err := http.Get(e.srv.URL + "/internal/xmpp/check_password")
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	form := url.Values{}
	form.Set("user", "not-a-uuid")
	form.Set("pass", "whatever")
	resp, err = http.Get(e.srv.URL + "/internal/xmpp/check_password?" + form.Encode())
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
