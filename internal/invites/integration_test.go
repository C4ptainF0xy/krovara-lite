package invites_test

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
	"github.com/krovara/krovara/internal/bans"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/invites"
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
	invitesSvc := invites.NewService(pool)
	bansSvc := bans.NewService(pool, "conference.krovara.local")

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })

	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			invitesSvc.Routes(api, resolver, auth.UserID)
			bansSvc.Routes(api, resolver, auth.UserID)
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
	id, _ := uuid.Parse(sp["id"].(string))
	return id
}

func TestInviteFlow_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/invites",
		map[string]any{"max_uses": int32(5)}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var inv map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	_ = resp.Body.Close()
	code := inv["code"].(string)
	require.Len(t, code, 8)

	resp = e.do(t, http.MethodGet, "/api/invites/"+code, nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	mem, err := e.q.GetMemberByUser(context.Background(), db.GetMemberByUserParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: bobID, Valid: true},
	})
	require.NoError(t, err)
	require.True(t, mem.SpaceID.Valid)

	resp = e.do(t, http.MethodGet, "/api/invites/"+code, nil, bobTok)
	var preview map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&preview))
	_ = resp.Body.Close()
	require.Equal(t, float64(1), preview["uses"])

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestInviteFlow_Expired(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, _ := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/invites",
		map[string]any{"ttl_seconds": int32(60)}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var inv map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	_ = resp.Body.Close()
	code := inv["code"].(string)

	_, err := e.pool.Exec(context.Background(),
		`UPDATE invites SET expires_at = NOW() - INTERVAL '1 hour' WHERE code = $1`, code)
	require.NoError(t, err)

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusGone, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestInviteFlow_MaxUsesExhausted(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, _ := register(t, e, "bob", "bob@example.com")
	carolTok, _ := register(t, e, "carol", "carol@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/invites",
		map[string]any{"max_uses": int32(1)}, ownerTok)
	var inv map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	_ = resp.Body.Close()
	code := inv["code"].(string)

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, carolTok)
	require.Equal(t, http.StatusGone, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestInviteFlow_BannedUserRejected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, bobID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/invites",
		map[string]any{"max_uses": int32(5)}, ownerTok)
	var inv map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	_ = resp.Body.Close()
	code := inv["code"].(string)
	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/bans",
		map[string]any{"user_id": bobID.String(), "reason": "spam"}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	_, err := e.q.GetMemberByUser(context.Background(), db.GetMemberByUserParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: bobID, Valid: true},
	})
	require.Error(t, err)

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/bans", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)

	resp = e.do(t, http.MethodDelete, "/api/spaces/"+spaceID.String()+"/bans/"+bobID.String(), nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/invites/"+code+"/accept", nil, bobTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestInviteFlow_CannotBanOwnerOrSelf(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, ownerID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/bans",
		map[string]any{"user_id": ownerID.String()}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestInvite_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/invites", map[string]any{}, ownerTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var inv map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	_ = resp.Body.Close()
	code := inv["code"].(string)

	resp = e.do(t, http.MethodDelete, "/api/invites/"+code, nil, ownerTok)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/invites/"+code, nil, ownerTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}
