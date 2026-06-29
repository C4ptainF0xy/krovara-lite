package karma_test

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
	"github.com/krovara/krovara/internal/karma"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/spaces"
)

type env struct {
	srv   *httptest.Server
	pool  *pgxpool.Pool
	q     *db.Queries
	karma *karma.Service
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
	karmaSvc := karma.NewService(pool)

	karmaSvc.MinAccountAge = 0

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			karmaSvc.Routes(api, auth.UserID)
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, pool: pool, q: q, karma: karmaSvc}
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

func addMember(t *testing.T, e *env, spaceID, userID uuid.UUID) {
	t.Helper()
	_, err := e.q.CreateMember(context.Background(), db.CreateMemberParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
	})
	require.NoError(t, err)
}

func (e *env) vouch(t *testing.T, spaceID, targetID uuid.UUID, bearer string) *http.Response {
	return e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/karma/"+targetID.String(), nil, bearer)
}

func (e *env) score(t *testing.T, spaceID, userID uuid.UUID, bearer string) int {
	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/karma/"+userID.String(), nil, bearer)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Score int `json:"score"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	_ = resp.Body.Close()
	return out.Score
}

func TestKarma_VouchHappyPathAndGuards(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, aliceID := register(t, e, "alice", "alice@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")
	carolTok, carolID := register(t, e, "carol", "carol@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")
	addMember(t, e, spaceID, aliceID)
	addMember(t, e, spaceID, bobID)

	resp := e.vouch(t, spaceID, aliceID, aliceTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.vouch(t, spaceID, bobID, carolTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.vouch(t, spaceID, carolID, aliceTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.vouch(t, spaceID, bobID, aliceTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, 1, e.score(t, spaceID, bobID, aliceTok))

	resp = e.vouch(t, spaceID, bobID, aliceTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, 1, e.score(t, spaceID, bobID, aliceTok))

	resp = e.vouch(t, spaceID, bobID, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, 2, e.score(t, spaceID, bobID, aliceTok))
}

func TestKarma_DailyCap(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	e.karma.MaxVouchesPerDay = 1

	ownerTok, ownerID := register(t, e, "owner", "owner@example.com")
	capperTok, capperID := register(t, e, "capper", "capper@example.com")
	_, bobID := register(t, e, "bob", "bob@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")
	addMember(t, e, spaceID, capperID)
	addMember(t, e, spaceID, bobID)

	resp := e.vouch(t, spaceID, bobID, capperTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.vouch(t, spaceID, ownerID, capperTok)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestKarma_AccountAgeGate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	e.karma.MinAccountAge = time.Hour

	ownerTok, ownerID := register(t, e, "owner", "owner@example.com")
	newbieTok, newbieID := register(t, e, "newbie", "newbie@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")
	addMember(t, e, spaceID, newbieID)

	resp := e.vouch(t, spaceID, ownerID, newbieTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}
