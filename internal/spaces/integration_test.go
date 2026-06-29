package spaces_test

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
	"github.com/krovara/krovara/internal/channels"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
)

type env struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
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

	spacesSvc.PublicRoutes(mux)
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			channelsSvc.Routes(api, resolver, auth.UserID)
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
		"username": username,
		"email":    email,
		"password": "correct horse battery",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	_ = resp.Body.Close()
	return body.AccessToken
}

func TestSpacesChannels_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	access := register(t, e, "alice", "alice@example.com")

	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": "Gaming"}, access)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var space map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&space))
	_ = resp.Body.Close()
	spaceID := space["id"].(string)
	require.Equal(t, "Gaming", space["name"])

	resp = e.do(t, http.MethodGet, "/api/spaces", nil, access)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID+"/channels", nil, access)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var chans []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chans))
	_ = resp.Body.Close()
	require.Len(t, chans, 1)
	require.Equal(t, "general", chans[0]["name"])

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID+"/channels",
		map[string]any{"name": "off-topic", "type": "text"}, access)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ch map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	_ = resp.Body.Close()
	chanID := ch["id"].(string)

	resp = e.do(t, http.MethodPatch, "/api/channels/"+chanID,
		map[string]any{"name": "memes"}, access)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	_ = resp.Body.Close()
	require.Equal(t, "memes", ch["name"])

	resp = e.do(t, http.MethodDelete, "/api/channels/"+chanID, nil, access)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID+"/channels", nil, access)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chans))
	_ = resp.Body.Close()
	require.Len(t, chans, 1)
	require.Equal(t, "general", chans[0]["name"])

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID,
		map[string]any{"name": "Gaming Pro"}, access)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&space))
	_ = resp.Body.Close()
	require.Equal(t, "Gaming Pro", space["name"])

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID,
		map[string]any{"password": "correct horse battery"}, access)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID, nil, access)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSpacesChannels_NonOwnerCannotMutate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	aliceTok := register(t, e, "alice", "alice@example.com")
	bobTok := register(t, e, "bob", "bob@example.com")

	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": "Alice's"}, aliceTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var space map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&space))
	_ = resp.Body.Close()
	spaceID := space["id"].(string)

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID,
		map[string]any{"name": "hijacked"}, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID, nil, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID+"/channels",
		map[string]any{"name": "x"}, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSpacesChannels_UnauthenticatedRejected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": "x"}, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
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

func TestSpaceSettings_UpdateAndVanity(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "alice", "alice@example.com")
	id := createSpace(t, e, tok, "Test")

	resp := e.do(t, http.MethodPatch, "/api/spaces/"+id, map[string]any{
		"description": "Notre QG",
		"tags":        []string{"gaming", "fr"},
		"language":    "fr",
		"rules":       "Sois sympa.",
	}, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	require.Equal(t, "Notre QG", sp["description"])
	require.Equal(t, "fr", sp["language"])
	require.ElementsMatch(t, []any{"gaming", "fr"}, sp["tags"].([]any))

	resp = e.do(t, http.MethodPut, "/api/spaces/"+id+"/vanity", map[string]any{"slug": "Sloth"}, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	require.Equal(t, "sloth", sp["vanity_slug"])

	resp = e.do(t, http.MethodGet, "/api/spaces/by-vanity/SLOTH", nil, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPut, "/api/spaces/"+id+"/vanity", map[string]any{"slug": "admin"}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	id2 := createSpace(t, e, tok, "Other")
	resp = e.do(t, http.MethodPut, "/api/spaces/"+id2+"/vanity", map[string]any{"slug": "sloth"}, tok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSpaceDelete_RequiresPasswordStepUp(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "alice", "alice@example.com")
	id := createSpace(t, e, tok, "Test")

	resp := e.do(t, http.MethodDelete, "/api/spaces/"+id, map[string]any{"password": "nope"}, tok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+id, nil, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+id,
		map[string]any{"password": "correct horse battery"}, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+id, nil, tok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSpaceTransfer_OwnershipToMember(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	register(t, e, "bob", "bob@example.com")
	id := createSpace(t, e, ownerTok, "Test")

	ctx := context.Background()
	var bobID string
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, "bob@example.com").Scan(&bobID))

	resp := e.do(t, http.MethodPost, "/api/spaces/"+id+"/transfer",
		map[string]any{"new_owner_id": bobID}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	_, err := e.pool.Exec(ctx,
		`INSERT INTO members (space_id, user_id) VALUES ($1, $2)`, id, bobID)
	require.NoError(t, err)

	resp = e.do(t, http.MethodPost, "/api/spaces/"+id+"/transfer",
		map[string]any{"new_owner_id": bobID}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	require.Equal(t, bobID, sp["owner_id"])

	resp = e.do(t, http.MethodPost, "/api/spaces/"+id+"/transfer",
		map[string]any{"new_owner_id": bobID}, ownerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
