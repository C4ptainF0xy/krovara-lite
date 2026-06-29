package members_test

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
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/members"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/roles"
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
	rolesSvc := roles.NewService(pool)
	membersSvc := members.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			rolesSvc.Routes(api, resolver, auth.UserID)
			membersSvc.Routes(api, resolver, auth.UserID)
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

func createSpace(t *testing.T, e *env, token, name string) uuid.UUID {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": name}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	id, err := uuid.Parse(sp["id"].(string))
	require.NoError(t, err)
	return id
}

func addMember(t *testing.T, e *env, spaceID, userID uuid.UUID) uuid.UUID {
	t.Helper()
	mem, err := e.q.CreateMember(context.Background(), db.CreateMemberParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
	})
	require.NoError(t, err)
	return uuid.UUID(mem.ID.Bytes)
}

func TestRoles_CreateAssignRemove(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	bobMemID := addMember(t, e, spaceID, bobID)

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/roles", map[string]any{
		"name":        "Moderator",
		"permissions": int64(permissions.ManageMessages | permissions.KickMembers),
		"color":       "#ff0000",
		"position":    int32(5),
	}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var role map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	roleID, _ := uuid.Parse(role["id"].(string))

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/roles", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 2)

	resp = e.do(t, http.MethodPut, "/api/members/"+bobMemID.String()+"/roles/"+roleID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPut, "/api/members/"+bobMemID.String()+"/roles/"+roleID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/members/"+bobMemID.String()+"/roles/"+roleID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/roles/"+roleID.String(), map[string]any{"name": "Mod"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	require.Equal(t, "Mod", role["name"])

	resp = e.do(t, http.MethodDelete, "/api/roles/"+roleID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestRoles_BulkAssignAndCosmetics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	_, carolID := register(t, e, "carol", "carol@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	bobMem := addMember(t, e, spaceID, bobID)
	carolMem := addMember(t, e, spaceID, carolID)

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/roles", map[string]any{
		"name": "VIP", "position": int32(3),
	}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var role map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	roleID := role["id"].(string)

	resp = e.do(t, http.MethodPatch, "/api/roles/"+roleID, map[string]any{
		"hoist": true, "mentionable": true, "icon_emoji": "⭐",
	}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	require.Equal(t, true, role["hoist"])
	require.Equal(t, true, role["mentionable"])
	require.Equal(t, "⭐", role["icon_emoji"])

	resp = e.do(t, http.MethodPost, "/api/roles/"+roleID+"/members", map[string]any{
		"member_ids": []string{bobMem.String(), carolMem.String()}, "action": "add",
	}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var res map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&res))
	_ = resp.Body.Close()
	require.Equal(t, float64(2), res["affected"])

	resp = e.do(t, http.MethodGet, "/api/roles/"+roleID+"/members", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rm []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rm))
	_ = resp.Body.Close()
	require.Len(t, rm, 2)

	resp = e.do(t, http.MethodPost, "/api/roles/"+roleID+"/members", map[string]any{
		"member_ids": []string{carolMem.String()}, "action": "remove",
	}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/roles/"+roleID+"/members", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rm))
	_ = resp.Body.Close()
	require.Len(t, rm, 1)
}

func TestRoles_EveryoneCannotBeDeleted(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, tok, "Test")

	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/roles", nil, tok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	everyoneID := list[0]["id"].(string)

	resp = e.do(t, http.MethodDelete, "/api/roles/"+everyoneID, nil, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestRoles_NonOwnerHierarchyBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	bobMemID := addMember(t, e, spaceID, bobID)

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/roles", map[string]any{
		"name":        "Admin",
		"permissions": int64(permissions.ManageRoles),
		"position":    int32(10),
	}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var role map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	adminID, _ := uuid.Parse(role["id"].(string))

	resp = e.do(t, http.MethodPut, "/api/members/"+bobMemID.String()+"/roles/"+adminID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/roles/"+adminID.String(), nil, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/roles", map[string]any{
		"name": "Sneaky", "position": int32(10),
	}, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/roles", map[string]any{
		"name": "Helper", "position": int32(5),
	}, bobTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestMembers_ListAndNickname(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	addMember(t, e, spaceID, bobID)

	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/members", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 2)

	nick := "Boss"
	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID.String()+"/members/"+ownerID.String(),
		map[string]any{"nickname": nick}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID.String()+"/members/"+bobID.String(),
		map[string]any{"nickname": "Bobby"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestMembers_KickHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	addMember(t, e, spaceID, bobID)

	resp := e.do(t, http.MethodDelete, "/api/spaces/"+spaceID.String()+"/members/"+ownerID.String(), nil, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID.String()+"/members/"+bobID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/members", nil, ownerTok)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
}

func TestMembers_CrossSpaceRoleRejected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")

	spaceA := createSpace(t, e, ownerTok, "A")
	spaceB := createSpace(t, e, ownerTok, "B")
	bobMemA := addMember(t, e, spaceA, bobID)

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceB.String()+"/roles", map[string]any{
		"name": "X", "position": int32(5),
	}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var role map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&role))
	_ = resp.Body.Close()
	roleBID := role["id"].(string)

	resp = e.do(t, http.MethodPut, "/api/members/"+bobMemA.String()+"/roles/"+roleBID, nil, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestMembers_SpaceProfile(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, aliceID := register(t, e, "alice", "alice@example.com")
	outsiderTok, _ := register(t, e, "mallory", "mallory@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")
	addMember(t, e, spaceID, aliceID)

	resp := e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/members/me/profile",
		map[string]any{"nickname": "AliceInGuild", "bio": "résident de la guilde"}, aliceTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/members", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var roster []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&roster))
	_ = resp.Body.Close()
	var found bool
	for _, m := range roster {
		if m["user_id"] == aliceID.String() {
			found = true
			require.Equal(t, "AliceInGuild", m["nickname"])
			require.Equal(t, "résident de la guilde", m["bio"])
		}
	}
	require.True(t, found, "alice should be in the roster")

	resp = e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/members/me/profile",
		map[string]any{"nickname": "x"}, outsiderTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestMembers_RosterRoleColor(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, aliceID := register(t, e, "alice", "alice@example.com")
	_ = aliceTok

	spaceID := createSpace(t, e, ownerTok, "Guild")
	aliceMemID := addMember(t, e, spaceID, aliceID)

	var lowID, highID uuid.UUID
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`INSERT INTO roles (space_id, name, color, position, is_everyone) VALUES ($1,'Member','#888888',1,false) RETURNING id`,
		spaceID).Scan(&lowID))
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`INSERT INTO roles (space_id, name, color, icon_emoji, position, is_everyone) VALUES ($1,'Mod','#e84118','🛡',5,false) RETURNING id`,
		spaceID).Scan(&highID))
	for _, rid := range []uuid.UUID{lowID, highID} {
		_, err := e.pool.Exec(context.Background(),
			`INSERT INTO member_roles (member_id, role_id) VALUES ($1,$2)`, aliceMemID, rid)
		require.NoError(t, err)
	}

	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/members", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var roster []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&roster))
	_ = resp.Body.Close()
	var checked bool
	for _, m := range roster {
		if m["user_id"] == aliceID.String() {
			checked = true
			require.Equal(t, "#e84118", m["role_color"])
			require.Equal(t, "🛡", m["role_icon"])
		}
	}
	require.True(t, checked)
}
