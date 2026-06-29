package channels_test

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
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

	resolver := permissions.NewPGResolver(q)
	spacesSvc := spaces.NewService(pool)
	channelsSvc := channels.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.CategoryRoutes(api, resolver, auth.UserID)
			channelsSvc.OverwriteRoutes(api, resolver, auth.UserID)
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

func createChannel(t *testing.T, e *env, token string, spaceID uuid.UUID, name string) map[string]any {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/channels",
		map[string]any{"name": name}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ch map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	_ = resp.Body.Close()
	return ch
}

func TestChannelLock_Toggle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	ch := createChannel(t, e, ownerTok, spaceID, "general")
	channelID := ch["id"].(string)
	require.Equal(t, false, ch["locked"])

	resp := e.do(t, http.MethodPut, "/api/channels/"+channelID+"/lock",
		map[string]any{"locked": true}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var locked map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&locked))
	_ = resp.Body.Close()
	require.Equal(t, true, locked["locked"])
	require.Equal(t, ownerID.String(), locked["locked_by"])
	require.NotEmpty(t, locked["locked_at"])

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/channels", nil, ownerTok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	var found bool
	for _, c := range list {
		if c["id"] == channelID {
			require.Equal(t, true, c["locked"])
			found = true
		}
	}
	require.True(t, found, "locked channel should appear in the list")

	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE action = 'channel.lock' AND target_id = $1`,
		channelID).Scan(&n))
	require.Equal(t, 1, n)

	resp = e.do(t, http.MethodPut, "/api/channels/"+channelID+"/lock",
		map[string]any{"locked": false}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var unlocked map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&unlocked))
	_ = resp.Body.Close()
	require.Equal(t, false, unlocked["locked"])
	require.NotContains(t, unlocked, "locked_by")
	require.NotContains(t, unlocked, "locked_at")
}

func TestChannelLock_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	ch := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodPut, "/api/channels/"+ch["id"].(string)+"/lock",
		map[string]any{"locked": true}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestCategories_CRUDAndChannelMove(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	ch := createChannel(t, e, ownerTok, spaceID, "general")
	channelID := ch["id"].(string)
	require.Nil(t, ch["category_id"])

	mkCat := func(name string) map[string]any {
		resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/categories",
			map[string]any{"name": name}, ownerTok)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		var c map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&c))
		_ = resp.Body.Close()
		return c
	}
	cat1 := mkCat("Discussions")
	cat2 := mkCat("Vocal")
	require.Equal(t, float64(0), cat1["position"])
	require.Equal(t, float64(1), cat2["position"])

	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/categories", nil, ownerTok)
	var cats []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cats))
	_ = resp.Body.Close()
	require.Len(t, cats, 2)
	require.Equal(t, "Discussions", cats[0]["name"])

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID.String()+"/categories/"+cat1["id"].(string),
		map[string]any{"name": "Général"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/channels/"+channelID+"/move",
		map[string]any{"category_id": cat1["id"], "position": 0}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var moved map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&moved))
	_ = resp.Body.Close()
	require.Equal(t, cat1["id"], moved["category_id"])

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID.String()+"/categories/"+cat1["id"].(string), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/channels", nil, ownerTok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	for _, c := range list {
		if c["id"] == channelID {
			require.Nil(t, c["category_id"], "channel should fall back to root after category delete")
		}
	}
}

func TestCategoryMove_RejectsForeignCategory(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceA := createSpace(t, e, ownerTok, "A")
	spaceB := createSpace(t, e, ownerTok, "B")
	ch := createChannel(t, e, ownerTok, spaceA, "general")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceB.String()+"/categories",
		map[string]any{"name": "Other"}, ownerTok)
	var catB map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&catB))
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/channels/"+ch["id"].(string)+"/move",
		map[string]any{"category_id": catB["id"], "position": 0}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestOverwrites_RoleAndMember(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	ch := createChannel(t, e, ownerTok, spaceID, "general")
	channelID := ch["id"].(string)

	everyone, err := e.q.GetEveryoneRole(context.Background(), pgtype.UUID{Bytes: spaceID, Valid: true})
	require.NoError(t, err)
	roleID := uuid.UUID(everyone.ID.Bytes).String()

	var memberID string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT m.id FROM members m JOIN spaces s ON s.id = m.space_id
		 WHERE m.space_id = $1 AND m.user_id = s.owner_id`, spaceID).Scan(&memberID))

	resp := e.do(t, http.MethodPut, "/api/channels/"+channelID+"/overwrites/role/"+roleID,
		map[string]any{"allow": 0, "deny": 2}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ow map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ow))
	_ = resp.Body.Close()
	require.Equal(t, "role", ow["target_type"])
	require.Equal(t, float64(2), ow["deny"])

	resp = e.do(t, http.MethodPut, "/api/channels/"+channelID+"/overwrites/member/"+memberID,
		map[string]any{"allow": 1, "deny": 0}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/overwrites", nil, ownerTok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 2)

	resp = e.do(t, http.MethodPut, "/api/channels/"+channelID+"/overwrites/role/"+roleID,
		map[string]any{"allow": 1 << 40, "deny": 0}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/channels/"+channelID+"/overwrites/role/"+roleID, nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+channelID+"/overwrites", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, "member", list[0]["target_type"])
}

func TestOverwrites_RequiresManageRoles(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	ch := createChannel(t, e, ownerTok, spaceID, "general")

	resp := e.do(t, http.MethodGet, "/api/channels/"+ch["id"].(string)+"/overwrites", nil, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
