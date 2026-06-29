package discovery_test

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
	"github.com/krovara/krovara/internal/discovery"
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
	resolver := permissions.NewPGResolver(q)
	spacesSvc := spaces.NewService(pool)
	discSvc := discovery.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			discSvc.Routes(api, resolver, auth.UserID)
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

func createSpace(t *testing.T, e *env, tok, name string) string {
	t.Helper()
	resp := e.do(t, http.MethodPost, "/api/spaces", map[string]any{"name": name}, tok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	_ = resp.Body.Close()
	return sp["id"].(string)
}

func TestDiscovery_ListExploreDelist(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "alice", "alice@example.com")
	sid := createSpace(t, e, tok, "Krovara Gamers")

	resp := e.do(t, http.MethodGet, "/api/discover", nil, tok)
	var ex []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ex))
	_ = resp.Body.Close()
	require.Len(t, ex, 0)

	resp = e.do(t, http.MethodPut, "/api/spaces/"+sid+"/listing", map[string]any{"category": "gaming"}, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var l map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&l))
	_ = resp.Body.Close()
	require.Equal(t, true, l["listed"])
	require.EqualValues(t, 1, l["member_count"])

	resp = e.do(t, http.MethodGet, "/api/discover?category=gaming&q=gamers", nil, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ex))
	_ = resp.Body.Close()
	require.Len(t, ex, 1)
	require.Equal(t, "Krovara Gamers", ex[0]["name"])

	resp = e.do(t, http.MethodGet, "/api/discover?category=tech", nil, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ex))
	_ = resp.Body.Close()
	require.Len(t, ex, 0)

	resp = e.do(t, http.MethodPut, "/api/spaces/"+sid+"/listing", map[string]any{"category": "bogus"}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+sid+"/listing", nil, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/discover", nil, tok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ex))
	_ = resp.Body.Close()
	require.Len(t, ex, 0)
}

func parseUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func TestDiscovery_OpenJoin(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	sid := createSpace(t, e, ownerTok, "Open Space")

	resp := e.do(t, http.MethodPost, "/api/discover/"+sid+"/join", nil, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPut, "/api/spaces/"+sid+"/listing", map[string]any{"category": "community"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/discover/"+sid+"/join", nil, strangerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var jr map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&jr))
	_ = resp.Body.Close()
	require.Equal(t, sid, jr["space_id"])
	require.NotEmpty(t, jr["member_id"])

	resp = e.do(t, http.MethodPost, "/api/discover/"+sid+"/join", nil, strangerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestDiscovery_OpenJoinGatedReturnsForm(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	sid := createSpace(t, e, ownerTok, "Gated Space")

	resp := e.do(t, http.MethodPut, "/api/spaces/"+sid+"/listing", map[string]any{"category": "community"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	q := db.New(e.pool)
	spaceUUID, err := parseUUID(sid)
	require.NoError(t, err)
	_, err = q.UpsertJoinForm(context.Background(), db.UpsertJoinFormParams{
		SpaceID: spaceUUID, Enabled: true, Questions: []byte("[]"),
	})
	require.NoError(t, err)

	resp = e.do(t, http.MethodPost, "/api/discover/"+sid+"/join", nil, strangerTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	_ = resp.Body.Close()
	require.Equal(t, true, body["requires_form"])
}

func TestDiscovery_NonOwnerCannotList(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok := register(t, e, "alice", "alice@example.com")
	strangerTok := register(t, e, "eve", "eve@example.com")
	sid := createSpace(t, e, ownerTok, "Private")

	resp := e.do(t, http.MethodPut, "/api/spaces/"+sid+"/listing", map[string]any{"category": "tech"}, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
