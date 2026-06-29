package moderation_test

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/moderation"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
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
	moderationSvc := moderation.NewService(pool, mucHost)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
			moderationSvc.Routes(api, resolver, auth.UserID)
			moderationSvc.TimeoutRoutes(api, resolver, auth.UserID)
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

func userOf(t *testing.T, e *env, email string) uuid.UUID {
	t.Helper()
	u, err := e.q.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	return uuid.UUID(u.ID.Bytes)
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

func createChannel(t *testing.T, e *env, token string, spaceID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/channels",
		map[string]any{"name": name}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ch map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	_ = resp.Body.Close()
	id, _ := uuid.Parse(ch["id"].(string))
	return id
}

func (e *env) seedMessage(t *testing.T, channelID uuid.UUID, key string) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", type, value)
VALUES ($1, $2, 'muc_log', $3, 'xml', '<message><body>hi</body></message>')`,
		mucHost, channelID.String(), key)
	require.NoError(t, err)
}

func (e *env) seedAuthored(t *testing.T, channelID, authorID uuid.UUID, key, body string) {
	t.Helper()
	e.seedAuthoredAt(t, channelID, authorID, key, body, time.Now().Unix())
}

func (e *env) seedAuthoredAt(t *testing.T, channelID, authorID uuid.UUID, key, body string, when int64) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body>%s</body></message>`,
		channelID.String(), mucHost, authorID.String(), body)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, when, value)
	require.NoError(t, err)
}

func addMember(t *testing.T, e *env, spaceID, userID uuid.UUID) {
	t.Helper()
	_, err := e.q.CreateMember(context.Background(), db.CreateMemberParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
	})
	require.NoError(t, err)
}

func (e *env) bulkDelete(t *testing.T, channelID uuid.UUID, ids []string, token string) (int, int64) {
	t.Helper()
	resp := e.do(t, http.MethodPost,
		"/api/channels/"+channelID.String()+"/messages/bulk-delete",
		map[string]any{"archive_ids": ids}, token)
	status := resp.StatusCode
	if status != http.StatusOK {
		_ = resp.Body.Close()
		return status, 0
	}
	var out struct {
		Deleted int64 `json:"deleted"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	_ = resp.Body.Close()
	return status, out.Deleted
}

func (e *env) purge(t *testing.T, channelID, authorID uuid.UUID, withinHours int, token string) (int, int64) {
	t.Helper()
	status, deleted, _ := e.purgeFull(t, channelID, authorID, withinHours, token)
	return status, deleted
}

func (e *env) purgeFull(t *testing.T, channelID, authorID uuid.UUID, withinHours int, token string) (int, int64, []string) {
	t.Helper()
	body := map[string]any{"author_id": authorID.String()}
	if withinHours > 0 {
		body["within_hours"] = withinHours
	}
	resp := e.do(t, http.MethodPost,
		"/api/channels/"+channelID.String()+"/messages/purge", body, token)
	status := resp.StatusCode
	if status != http.StatusOK {
		_ = resp.Body.Close()
		return status, 0, nil
	}
	var out struct {
		Deleted    int64    `json:"deleted"`
		ArchiveIDs []string `json:"archive_ids"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	_ = resp.Body.Close()
	return status, out.Deleted, out.ArchiveIDs
}

func (e *env) messageCount(t *testing.T, channelID uuid.UUID, key string) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM prosodyarchive WHERE "user" = $1 AND "key" = $2`,
		channelID.String(), key).Scan(&n))
	return n
}

func TestDeleteMessage_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, "msg-1")

	resp := e.do(t, http.MethodDelete,
		"/api/channels/"+channelID.String()+"/messages/msg-1", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, 0, e.messageCount(t, channelID, "msg-1"))

	resp = e.do(t, http.MethodDelete,
		"/api/channels/"+channelID.String()+"/messages/msg-1", nil, ownerTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestDeleteMessage_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, "msg-1")

	resp := e.do(t, http.MethodDelete,
		"/api/channels/"+channelID.String()+"/messages/msg-1", nil, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	require.Equal(t, 1, e.messageCount(t, channelID, "msg-1"))
}

func TestBulkDelete_ModeratorDeletesAny(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	ownerID := userOf(t, e, "alice@example.com")
	other := uuid.New()
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedAuthored(t, channelID, ownerID, "m1", "a")
	e.seedAuthored(t, channelID, other, "m2", "b")
	e.seedAuthored(t, channelID, other, "m3", "c")

	status, deleted := e.bulkDelete(t, channelID, []string{"m1", "m2", "m3"}, ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 3, deleted)
	require.Equal(t, 0, e.messageCount(t, channelID, "m1"))
	require.Equal(t, 0, e.messageCount(t, channelID, "m2"))
	require.Equal(t, 0, e.messageCount(t, channelID, "m3"))
}

func TestBulkDelete_NonModOwnOnly(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	ownerID := userOf(t, e, "alice@example.com")
	bobTok := register(t, e, "bob", "bob@example.com")
	bobID := userOf(t, e, "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	addMember(t, e, spaceID, bobID)
	e.seedAuthored(t, channelID, bobID, "bob-1", "à moi")
	e.seedAuthored(t, channelID, ownerID, "alice-1", "à elle")

	status, deleted := e.bulkDelete(t, channelID, []string{"bob-1"}, bobTok)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, deleted)
	require.Equal(t, 0, e.messageCount(t, channelID, "bob-1"))

	status, _ = e.bulkDelete(t, channelID, []string{"alice-1"}, bobTok)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, 1, e.messageCount(t, channelID, "alice-1"))

	e.seedAuthored(t, channelID, bobID, "bob-2", "à moi 2")
	status, _ = e.bulkDelete(t, channelID, []string{"bob-2", "alice-1"}, bobTok)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, 1, e.messageCount(t, channelID, "bob-2"))
	require.Equal(t, 1, e.messageCount(t, channelID, "alice-1"))
}

func TestBulkDelete_Validation(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	status, _ := e.bulkDelete(t, channelID, []string{}, ownerTok)
	require.Equal(t, http.StatusBadRequest, status)

	big := make([]string, 101)
	for i := range big {
		big[i] = fmt.Sprintf("k%d", i)
	}
	status, _ = e.bulkDelete(t, channelID, big, ownerTok)
	require.Equal(t, http.StatusBadRequest, status)
}

func TestBulkDelete_IdempotentCount(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	ownerID := userOf(t, e, "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedAuthored(t, channelID, ownerID, "m1", "présent")

	status, deleted := e.bulkDelete(t, channelID, []string{"m1", "ghost"}, ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, deleted)
	require.Equal(t, 0, e.messageCount(t, channelID, "m1"))
}

func TestBulkDelete_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	ownerID := userOf(t, e, "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedAuthored(t, channelID, ownerID, "m1", "secret")

	status, _ := e.bulkDelete(t, channelID, []string{"m1"}, strangerTok)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, 1, e.messageCount(t, channelID, "m1"))
}

func TestPurge_ByAuthor(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	ownerID := userOf(t, e, "alice@example.com")
	bob := uuid.New()
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedAuthored(t, channelID, bob, "b1", "x")
	e.seedAuthored(t, channelID, bob, "b2", "y")
	e.seedAuthored(t, channelID, bob, "b3", "z")
	e.seedAuthored(t, channelID, ownerID, "a1", "garde")
	e.seedAuthored(t, channelID, ownerID, "a2", "garde2")

	status, deleted, ids := e.purgeFull(t, channelID, bob, 0, ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 3, deleted)

	require.ElementsMatch(t, []string{"b1", "b2", "b3"}, ids)
	require.Equal(t, 0, e.messageCount(t, channelID, "b1"))
	require.Equal(t, 0, e.messageCount(t, channelID, "b2"))
	require.Equal(t, 0, e.messageCount(t, channelID, "b3"))
	require.Equal(t, 1, e.messageCount(t, channelID, "a1"))
	require.Equal(t, 1, e.messageCount(t, channelID, "a2"))
}

func TestPurge_WithinHours(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	bob := uuid.New()
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	now := time.Now().Unix()
	e.seedAuthoredAt(t, channelID, bob, "old", "vieux", now-10*3600)
	e.seedAuthored(t, channelID, bob, "fresh", "récent")

	status, deleted := e.purge(t, channelID, bob, 1, ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, deleted)
	require.Equal(t, 1, e.messageCount(t, channelID, "old"))
	require.Equal(t, 0, e.messageCount(t, channelID, "fresh"))
}

func TestPurge_NonModForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	bobTok := register(t, e, "bob", "bob@example.com")
	bobID := userOf(t, e, "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	addMember(t, e, spaceID, bobID)
	e.seedAuthored(t, channelID, bobID, "b1", "x")

	status, _ := e.purge(t, channelID, bobID, 0, bobTok)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, 1, e.messageCount(t, channelID, "b1"))
}

func TestPurge_InvalidAuthor(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPost,
		"/api/channels/"+channelID.String()+"/messages/purge",
		map[string]any{"author_id": "not-a-uuid"}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestTimeout_CreateGateRevokeExpire(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	bobTok := register(t, e, "bob", "bob@example.com")
	bobID := userOf(t, e, "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	addMember(t, e, spaceID, bobID)

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/timeouts",
		map[string]any{"user_id": bobID.String(), "minutes": 30, "reason": "spam"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/timeouts/me", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var me map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	_ = resp.Body.Close()
	require.Equal(t, true, me["active"])

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/mod-actions", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var hist []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&hist))
	_ = resp.Body.Close()
	require.Len(t, hist, 1)

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/mod-actions", nil, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID.String()+"/timeouts/"+bobID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/timeouts/me", nil, bobTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	_ = resp.Body.Close()
	require.Equal(t, false, me["active"])
}

func TestTimeout_AutoExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	_ = register(t, e, "bob", "bob@example.com")
	bobID := userOf(t, e, "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	addMember(t, e, spaceID, bobID)

	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO mod_actions (space_id, target_user, action, expires_at, active)
		 VALUES ($1, $2, 'timeout', NOW() - INTERVAL '1 minute', TRUE)`,
		spaceID, bobID)
	require.NoError(t, err)

	q := db.New(e.pool)
	require.NoError(t, q.DeactivateExpiredTimeouts(context.Background()))

	got, err := q.GetActiveTimeout(context.Background(), db.GetActiveTimeoutParams{
		SpaceID:    pgtype.UUID{Bytes: spaceID, Valid: true},
		TargetUser: pgtype.UUID{Bytes: bobID, Valid: true},
	})
	require.ErrorIs(t, err, pgx.ErrNoRows, "expired timeout should no longer be active, got %v", got)
}
