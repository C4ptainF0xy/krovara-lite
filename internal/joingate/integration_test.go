package joingate_test

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
	"github.com/krovara/krovara/internal/joingate"
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
	joinGateSvc := joingate.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			joinGateSvc.Routes(api, resolver, auth.UserID)
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

func seedRole(t *testing.T, e *env, spaceID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.pool.QueryRow(context.Background(),
		`INSERT INTO roles (space_id, name, is_everyone) VALUES ($1, $2, false) RETURNING id`,
		spaceID, name).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestJoinGate_FullApprovalFlow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, aliceID := register(t, e, "alice", "alice@example.com")

	spaceID := createSpace(t, e, ownerTok, "Gated Guild")
	roleID := seedRole(t, e, spaceID, "Verified")

	resp := e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/join-form", map[string]any{
		"enabled": true,
		"questions": []map[string]any{
			{"id": "why", "label": "Why do you want to join?", "required": true},
		},
		"auto_role_id": roleID.String(),
	}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/join-form", nil, aliceTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var form map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&form))
	_ = resp.Body.Close()
	require.Equal(t, true, form["enabled"])

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{}}, aliceTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{{"question_id": "why", "answer": "I love it"}}}, aliceTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var jr map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&jr))
	_ = resp.Body.Close()
	reqID := jr["id"].(string)

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{{"question_id": "why", "answer": "again"}}}, aliceTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/join-requests", nil, aliceTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/join-requests?status=pending", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var queue []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&queue))
	_ = resp.Body.Close()
	require.Len(t, queue, 1)
	require.Equal(t, "alice", queue[0]["username"])

	resp = e.do(t, http.MethodPost, "/api/join-requests/"+reqID+"/review",
		map[string]any{"action": "approve"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rev map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rev))
	_ = resp.Body.Close()
	require.Equal(t, "approved", rev["status"])
	require.Equal(t, true, rev["member_created"])

	mem, err := e.q.GetMemberByUser(context.Background(), db.GetMemberByUserParams{
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		UserID:  pgtype.UUID{Bytes: aliceID, Valid: true},
	})
	require.NoError(t, err)
	has, err := e.q.HasMemberRole(context.Background(), db.HasMemberRoleParams{
		MemberID: mem.ID, RoleID: pgtype.UUID{Bytes: roleID, Valid: true},
	})
	require.NoError(t, err)
	require.True(t, has)

	items, err := e.q.ListInbox(context.Background(), db.ListInboxParams{
		UserID: pgtype.UUID{Bytes: aliceID, Valid: true}, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "join_approved", items[0].Kind)
	require.NotNil(t, items[0].Preview)

	resp = e.do(t, http.MethodPost, "/api/join-requests/"+reqID+"/review",
		map[string]any{"action": "approve"}, ownerTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestJoinGate_SubmitGuards(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, _ := register(t, e, "alice", "alice@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")

	resp := e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{}}, aliceTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/join-form",
		map[string]any{"enabled": true, "questions": []map[string]any{}}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{}}, ownerTok)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestJoinGate_AutoRoleMustBelongToSpace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	otherTok, _ := register(t, e, "other", "other@example.com")

	spaceID := createSpace(t, e, ownerTok, "Guild")
	otherSpaceID := createSpace(t, e, otherTok, "Other")
	foreignRole := seedRole(t, e, otherSpaceID, "Foreign")

	resp := e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/join-form", map[string]any{
		"enabled": true, "questions": []map[string]any{}, "auto_role_id": foreignRole.String(),
	}, ownerTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestJoinGate_KarmaGate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "owner", "owner@example.com")
	aliceTok, aliceID := register(t, e, "alice", "alice@example.com")
	spaceID := createSpace(t, e, ownerTok, "Elite")

	resp := e.do(t, http.MethodPut, "/api/spaces/"+spaceID.String()+"/join-form", map[string]any{
		"enabled": true, "questions": []map[string]any{}, "min_karma": 5,
	}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{}}, aliceTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()

	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO karma (user_id, space_id, score) VALUES ($1, $2, 5)`, aliceID, spaceID)
	require.NoError(t, err)

	resp = e.do(t, http.MethodPost, "/api/spaces/"+spaceID.String()+"/join-requests",
		map[string]any{"answers": []map[string]any{}}, aliceTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()
}
