package polls_test

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
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/polls"
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
	pollsSvc := polls.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			pollsSvc.Routes(api, resolver, auth.UserID)
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

func seedChannel(t *testing.T, e *env, spaceID string) string {
	t.Helper()
	var id string
	err := e.pool.QueryRow(context.Background(),
		`INSERT INTO channels (space_id, name, type, position) VALUES ($1,'general','text',0) RETURNING id`,
		spaceID).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestPolls_CreateVoteResults(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok := register(t, e, "alice", "alice@example.com")
	sid := createSpace(t, e, tok, "Guild")
	cid := seedChannel(t, e, sid)

	resp := e.do(t, http.MethodPost, "/api/channels/"+cid+"/polls",
		map[string]any{"question": "Pizza ou sushi ?", "options": []string{"Pizza", "Sushi"}}, tok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var poll map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&poll))
	_ = resp.Body.Close()
	pollID := poll["id"].(string)
	options := poll["options"].([]any)
	require.Len(t, options, 2)
	pizza := options[0].(map[string]any)["id"].(string)
	sushi := options[1].(map[string]any)["id"].(string)

	resp = e.do(t, http.MethodPost, "/api/channels/"+cid+"/polls",
		map[string]any{"question": "x", "options": []string{"only one"}}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/polls/"+pollID+"/vote", map[string]any{"option_id": pizza}, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/polls/"+pollID+"/vote", map[string]any{"option_id": sushi}, tok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/channels/"+cid+"/polls", nil, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, sushi, list[0]["my_option"])
	for _, o := range list[0]["options"].([]any) {
		om := o.(map[string]any)
		if om["id"] == sushi {
			require.EqualValues(t, 1, om["votes"])
		} else {
			require.EqualValues(t, 0, om["votes"])
		}
	}

	resp = e.do(t, http.MethodPost, "/api/polls/"+pollID+"/vote",
		map[string]any{"option_id": "11111111-1111-1111-1111-111111111111"}, tok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/polls/"+pollID+"/close", nil, tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	resp = e.do(t, http.MethodPost, "/api/polls/"+pollID+"/vote", map[string]any{"option_id": pizza}, tok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}
