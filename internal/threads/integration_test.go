package threads_test

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

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
	"github.com/krovara/krovara/internal/threads"
)

const mucHost = "conference.krovara.local"

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

	_, err = pool.Exec(ctx, `
CREATE TABLE prosodyarchive (
    sort_id BIGSERIAL PRIMARY KEY,
    host    TEXT NOT NULL,
    "user"  TEXT NOT NULL,
    store   TEXT NOT NULL,
    "key"   TEXT NOT NULL,
    "when"  BIGINT,
    "with"  TEXT,
    type    TEXT,
    value   TEXT
)`)
	require.NoError(t, err)

	q := db.New(pool)
	signer := auth.NewJWTSigner([]byte("test-secret"), time.Hour)
	sessions := auth.NewSessionStore(q, 24*time.Hour)
	authSvc := auth.NewService(q, signer, sessions)

	resolver := permissions.NewPGResolver(q)
	spacesSvc := spaces.NewService(pool)
	channelsSvc := channels.NewService(pool)
	threadsSvc := threads.NewService(pool, mucHost)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
			threadsSvc.Routes(api, resolver, auth.UserID)
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
	u, err := e.q.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	return body.AccessToken, uuid.UUID(u.ID.Bytes)
}

func createSpace(t *testing.T, e *env, token, name string) uuid.UUID {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": name}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	id, _ := uuid.Parse(sp["id"].(string))
	return id
}

func createChannel(t *testing.T, e *env, token string, spaceID uuid.UUID, name string) string {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/channels",
		map[string]any{"name": name}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ch map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	_ = resp.Body.Close()
	return ch["id"].(string)
}

func TestThread_CreateListSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPost, "/api/channels/"+channelID+"/threads",
		map[string]any{"root_archive_id": "stanza-1", "title": "Bug triage"}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var thread map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&thread))
	_ = resp.Body.Close()
	threadID := thread["id"].(string)
	require.Equal(t, "Bug triage", thread["title"])
	require.Equal(t, channelID, thread["channel_id"])
	require.Equal(t, ownerID.String(), thread["created_by"])

	require.Equal(t, true, thread["is_subscribed"])

	room := "thread-" + threadID
	for i := 0; i < 3; i++ {
		_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, 0, 'message', '<message><body>hi</body></message>')`,
			mucHost, room, "k"+uuid.NewString())
		require.NoError(t, err)
	}

	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, 0, 'message', '<message><reactions/></message>')`,
		mucHost, room, "react-"+uuid.NewString())
	require.NoError(t, err)

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/threads", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, threadID, list[0]["id"])
	require.Equal(t, float64(3), list[0]["reply_count"])
	require.Equal(t, true, list[0]["is_subscribed"])

	memberTok, _ := register(t, e, "bob", "bob@example.com")

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/threads", nil, memberTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/threads/"+threadID+"/subscribe", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/threads", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Equal(t, false, list[0]["is_subscribed"])

	resp = e.do(t, http.MethodPost, "/api/threads/"+threadID+"/subscribe", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()
	resp = e.do(t, http.MethodPost, "/api/threads/"+threadID+"/subscribe", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/threads", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Equal(t, true, list[0]["is_subscribed"])
}

func TestThread_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPost, "/api/channels/"+channelID+"/threads",
		map[string]any{"root_archive_id": "s1", "title": "nope"}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
