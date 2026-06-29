package messages_test

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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/messages"
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
	messagesSvc := messages.NewService(pool, mucHost)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
			messagesSvc.Routes(api, resolver, auth.UserID)
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

func (e *env) seedMessage(t *testing.T, channelID, authorID uuid.UUID, key, body string) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body>%s</body></message>`,
		channelID.String(), mucHost, authorID.String(), body)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, time.Now().Unix(), value)
	require.NoError(t, err)
}

func (e *env) seedOriginalMessage(t *testing.T, channelID, authorID uuid.UUID, key, originID, body string) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body>%s</body><origin-id xmlns="urn:xmpp:sid:0" id="%s"/></message>`,
		channelID.String(), mucHost, authorID.String(), body, originID)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, time.Now().Unix(), value)
	require.NoError(t, err)
}

func (e *env) seedCorrection(t *testing.T, channelID, authorID uuid.UUID, key, originID, replaceID, body string) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body>%s</body><origin-id xmlns="urn:xmpp:sid:0" id="%s"/><replace xmlns="urn:xmpp:message-correct:0" id="%s"/></message>`,
		channelID.String(), mucHost, authorID.String(), body, originID, replaceID)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, time.Now().Unix(), value)
	require.NoError(t, err)
}

func decodeList(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	return list
}

func TestPinFlow_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "épingle moi")

	resp := e.do(t, http.MethodPost, "/api/channels/"+channelID.String()+"/pins",
		map[string]any{"archive_id": "msg-1", "note": "à lire"}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var pin map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&pin))
	_ = resp.Body.Close()
	require.Equal(t, "msg-1", pin["archive_id"])
	require.Equal(t, "à lire", pin["note"])
	require.Equal(t, "épingle moi", pin["body"])
	require.Equal(t, ownerID.String(), pin["author_id"])
	require.Equal(t, false, pin["missing"])

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID.String()+"/pins", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	list := decodeList(t, resp)
	require.Len(t, list, 1)
	require.Equal(t, "épingle moi", list[0]["body"])

	resp = e.do(t, http.MethodDelete, "/api/channels/"+channelID.String()+"/pins/msg-1", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID.String()+"/pins", nil, ownerTok)
	require.Empty(t, decodeList(t, resp))
}

func TestPin_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "secret")

	resp := e.do(t, http.MethodPost, "/api/channels/"+channelID.String()+"/pins",
		map[string]any{"archive_id": "msg-1"}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestPin_UnknownMessage(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPost, "/api/channels/"+channelID.String()+"/pins",
		map[string]any{"archive_id": "ghost"}, ownerTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSaveFlow_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "garde ça")

	resp := e.do(t, http.MethodPost, "/api/me/saves", map[string]any{
		"channel_id": channelID.String(), "archive_id": "msg-1", "folder": "idées",
	}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var save map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&save))
	_ = resp.Body.Close()
	require.Equal(t, "msg-1", save["archive_id"])
	require.Equal(t, "idées", save["folder"])
	require.Equal(t, "garde ça", save["body"])
	require.Equal(t, channelID.String(), save["channel_id"])
	require.Equal(t, spaceID.String(), save["space_id"])

	resp = e.do(t, http.MethodGet, "/api/me/saves", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	list := decodeList(t, resp)
	require.Len(t, list, 1)
	require.Equal(t, "garde ça", list[0]["body"])

	resp = e.do(t, http.MethodDelete, "/api/me/saves/msg-1", nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/me/saves", nil, ownerTok)
	require.Empty(t, decodeList(t, resp))
}

func TestSave_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "privé")

	resp := e.do(t, http.MethodPost, "/api/me/saves", map[string]any{
		"channel_id": channelID.String(), "archive_id": "msg-1",
	}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func (e *env) seedReaction(t *testing.T, channelID, authorID uuid.UUID, key, targetKey string) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><reactions xmlns="urn:xmpp:reactions:0" id="%s"><reaction>🔥</reaction></reactions></message>`,
		channelID.String(), mucHost, authorID.String(), targetKey)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, time.Now().Unix(), value)
	require.NoError(t, err)
}

func (e *env) seedAttributedMessage(t *testing.T, channelID, authorID uuid.UUID, key, body string) {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body xml:lang="en">%s</body></message>`,
		channelID.String(), mucHost, authorID.String(), body)
	_, err := e.pool.Exec(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5)`,
		mucHost, channelID.String(), key, time.Now().Unix(), value)
	require.NoError(t, err)
}

func (e *env) readState(t *testing.T, channelID uuid.UUID, archiveID, mode, token string) map[string]any {
	t.Helper()
	body := map[string]any{"archive_id": archiveID}
	if mode != "" {
		body["mode"] = mode
	}
	resp := e.do(t, http.MethodPut, "/api/channels/"+channelID.String()+"/read-state", body, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	_ = resp.Body.Close()
	return out
}

func TestReadState_MarkReadAndUnread(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "un")
	e.seedMessage(t, channelID, ownerID, "msg-2", "deux")
	e.seedMessage(t, channelID, ownerID, "msg-3", "trois")

	rs := e.readState(t, channelID, "msg-3", "", ownerTok)
	require.Equal(t, "msg-3", rs["last_read_archive_id"])
	require.EqualValues(t, 0, rs["unread_count"])

	resp := e.do(t, http.MethodGet, "/api/me/read-state", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	list := decodeList(t, resp)
	require.Len(t, list, 1)
	require.Equal(t, channelID.String(), list[0]["channel_id"])
	require.EqualValues(t, 0, list[0]["unread_count"])

	e.seedMessage(t, channelID, ownerID, "msg-4", "quatre")
	resp = e.do(t, http.MethodGet, "/api/me/read-state", nil, ownerTok)
	list = decodeList(t, resp)
	require.EqualValues(t, 1, list[0]["unread_count"])

	rs = e.readState(t, channelID, "msg-3", "unread", ownerTok)
	require.Equal(t, "msg-2", rs["last_read_archive_id"])
	require.EqualValues(t, 2, rs["unread_count"])
}

func TestReadState_ReadDoesNotRegress(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "un")
	e.seedMessage(t, channelID, ownerID, "msg-2", "deux")
	e.seedMessage(t, channelID, ownerID, "msg-3", "trois")

	rs := e.readState(t, channelID, "msg-3", "", ownerTok)
	require.Equal(t, "msg-3", rs["last_read_archive_id"])
	require.EqualValues(t, 0, rs["unread_count"])

	rs = e.readState(t, channelID, "msg-1", "", ownerTok)
	require.Equal(t, "msg-3", rs["last_read_archive_id"])
	require.EqualValues(t, 0, rs["unread_count"])

	rs = e.readState(t, channelID, "msg-2", "unread", ownerTok)
	require.Equal(t, "msg-1", rs["last_read_archive_id"])
	require.EqualValues(t, 2, rs["unread_count"])
}

func TestReadState_ReactionsDoNotCountAsUnread(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "salut")

	rs := e.readState(t, channelID, "msg-1", "", ownerTok)
	require.EqualValues(t, 0, rs["unread_count"])

	e.seedReaction(t, channelID, ownerID, "react-1", "msg-1")
	resp := e.do(t, http.MethodGet, "/api/me/read-state", nil, ownerTok)
	list := decodeList(t, resp)
	require.Len(t, list, 1)
	require.EqualValues(t, 0, list[0]["unread_count"])

	e.seedAttributedMessage(t, channelID, ownerID, "msg-2", "hola")
	resp = e.do(t, http.MethodGet, "/api/me/read-state", nil, ownerTok)
	list = decodeList(t, resp)
	require.EqualValues(t, 1, list[0]["unread_count"])
}

func TestReadState_MarkUnreadFirstMessage(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "premier")

	rs := e.readState(t, channelID, "msg-1", "unread", ownerTok)
	require.Equal(t, "", rs["last_read_archive_id"])
	require.EqualValues(t, 0, rs["last_read_sort_id"])
	require.EqualValues(t, 1, rs["unread_count"])
}

func (e *env) editHistory(t *testing.T, channelID uuid.UUID, archiveID, token string) ([]map[string]any, int) {
	t.Helper()
	resp := e.do(t, http.MethodGet,
		"/api/channels/"+channelID.String()+"/messages/"+archiveID+"/history", nil, token)
	status := resp.StatusCode
	if status != http.StatusOK {
		_ = resp.Body.Close()
		return nil, status
	}
	return decodeList(t, resp), status
}

func TestEditHistory_ReconstructsRevisions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	e.seedOriginalMessage(t, channelID, ownerID, "msg-1", "o1", "v1")
	e.seedCorrection(t, channelID, ownerID, "edit-a", "o2", "o1", "v2")
	e.seedCorrection(t, channelID, ownerID, "edit-b", "o3", "o1", "v3")

	e.seedOriginalMessage(t, channelID, ownerID, "msg-2", "x1", "autre")
	e.seedCorrection(t, channelID, ownerID, "edit-c", "x2", "x1", "autre v2")

	hist, status := e.editHistory(t, channelID, "msg-1", ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, hist, 3)
	require.Equal(t, "v1", hist[0]["body"])
	require.Equal(t, true, hist[0]["original"])
	require.Equal(t, "v2", hist[1]["body"])
	require.Equal(t, false, hist[1]["original"])
	require.Equal(t, "v3", hist[2]["body"])
}

func TestEditHistory_RejectsForgedCrossAuthorCorrection(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	e.seedOriginalMessage(t, channelID, ownerID, "msg-1", "o1", "v1")
	e.seedCorrection(t, channelID, ownerID, "edit-a", "o2", "o1", "v2")

	eve := uuid.New()
	e.seedCorrection(t, channelID, eve, "edit-evil", "o3", "o1", "PROPAGANDE")

	hist, status := e.editHistory(t, channelID, "msg-1", ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, hist, 2)
	require.Equal(t, "v1", hist[0]["body"])
	require.Equal(t, "v2", hist[1]["body"])
	for _, rev := range hist {
		require.NotEqual(t, "PROPAGANDE", rev["body"])
	}
}

func TestEditHistory_KeyAnchorFallback(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	e.seedMessage(t, channelID, ownerID, "key-1", "v1")
	e.seedCorrection(t, channelID, ownerID, "edit-a", "o2", "key-1", "v2")

	hist, status := e.editHistory(t, channelID, "key-1", ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, hist, 2)
	require.Equal(t, "v1", hist[0]["body"])
	require.Equal(t, "v2", hist[1]["body"])
}

func TestEditHistory_NoEditsReturnsOriginalOnly(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedOriginalMessage(t, channelID, ownerID, "msg-1", "o1", "jamais édité")

	hist, status := e.editHistory(t, channelID, "msg-1", ownerTok)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, hist, 1)
	require.Equal(t, "jamais édité", hist[0]["body"])
	require.Equal(t, true, hist[0]["original"])
}

func TestEditHistory_UnknownMessage(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	_, status := e.editHistory(t, channelID, "ghost", ownerTok)
	require.Equal(t, http.StatusNotFound, status)
}

func TestEditHistory_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedOriginalMessage(t, channelID, ownerID, "msg-1", "o1", "privé")

	_, status := e.editHistory(t, channelID, "msg-1", strangerTok)
	require.Equal(t, http.StatusForbidden, status)
}

func TestReadState_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")
	e.seedMessage(t, channelID, ownerID, "msg-1", "privé")

	resp := e.do(t, http.MethodPut, "/api/channels/"+channelID.String()+"/read-state",
		map[string]any{"archive_id": "msg-1"}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestReadState_UnknownMessage(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	channelID := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPut, "/api/channels/"+channelID.String()+"/read-state",
		map[string]any{"archive_id": "ghost"}, ownerTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}
