package reports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/reports"
	"github.com/krovara/krovara/internal/spaces"
)

type env struct {
	srv *httptest.Server
	q   *db.Queries
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
	reportsSvc := reports.NewService(pool)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) {
			spacesSvc.Routes(api, resolver, auth.UserID)
			reportsSvc.Routes(api, resolver, auth.UserID)
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, q: q}
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

func TestReportFlow_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	reporterTok, reporterID := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	msgID := uuid.New()
	resp := e.do(t, http.MethodPost, "/api/reports", map[string]any{
		"target_type": "message",
		"target_id":   msgID.String(),
		"reason":      "spam",
		"space_id":    spaceID.String(),
	}, reporterTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()
	reportID := created["id"].(string)
	require.Equal(t, "pending", *strPtr(created["status"]))
	require.Equal(t, reporterID.String(), created["reporter_id"])

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/reports?status=pending", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, reportID, list[0]["id"])

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceID.String()+"/reports/"+reportID,
		map[string]any{"status": "resolved"}, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var resolved map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&resolved))
	_ = resp.Body.Close()
	require.Equal(t, "resolved", *strPtr(resolved["status"]))
	require.NotNil(t, resolved["resolved_at"])

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/reports?status=pending", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Empty(t, list)
}

func TestReportFlow_NonMemberForbidden(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	strangerTok, _ := register(t, e, "eve", "eve@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	resp := e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/reports", nil, strangerTok)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestReportFlow_CrossSpaceResolveRejected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	reporterTok, _ := register(t, e, "bob", "bob@example.com")
	spaceA := createSpace(t, e, ownerTok, "A")
	spaceB := createSpace(t, e, ownerTok, "B")

	resp := e.do(t, http.MethodPost, "/api/reports", map[string]any{
		"target_type": "message",
		"target_id":   uuid.New().String(),
		"reason":      "spam",
		"space_id":    spaceA.String(),
	}, reporterTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()
	reportID := created["id"].(string)

	resp = e.do(t, http.MethodPatch, "/api/spaces/"+spaceB.String()+"/reports/"+reportID,
		map[string]any{"status": "resolved"}, ownerTok)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestReportFlow_CategoryAndContext(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	reporterTok, _ := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")

	ctxPayload := []map[string]any{
		{"author": "carol", "body": "message précédent", "ts": 1},
		{"author": "dave", "body": "message signalé", "ts": 2},
		{"author": "erin", "body": "message suivant", "ts": 3},
	}
	resp := e.do(t, http.MethodPost, "/api/reports", map[string]any{
		"target_type": "message",
		"target_id":   uuid.New().String(),
		"reason":      "propos haineux répétés",
		"category":    "harassment",
		"context":     ctxPayload,
		"space_id":    spaceID.String(),
	}, reporterTok)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()
	require.Equal(t, "harassment", created["category"])
	ctxOut, ok := created["context"].([]any)
	require.True(t, ok, "context should echo back as an array")
	require.Len(t, ctxOut, 3)

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/reports?category=harassment", nil, ownerTok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list, 1)
	require.Equal(t, "harassment", list[0]["category"])

	resp = e.do(t, http.MethodGet, "/api/spaces/"+spaceID.String()+"/reports?category=spam", nil, ownerTok)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Empty(t, list)
}

func TestReport_RejectsBadCategoryAndOversizeContext(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	ownerTok, _ := register(t, e, "alice", "alice@example.com")
	reporterTok, _ := register(t, e, "bob", "bob@example.com")
	spaceID := createSpace(t, e, ownerTok, "Test")
	_ = ownerTok

	resp := e.do(t, http.MethodPost, "/api/reports", map[string]any{
		"target_type": "message",
		"target_id":   uuid.New().String(),
		"reason":      "x",
		"category":    "bogus",
		"space_id":    spaceID.String(),
	}, reporterTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()

	big := make([]map[string]any, 0, 400)
	for range 400 {
		big = append(big, map[string]any{"body": strings.Repeat("x", 64)})
	}
	resp = e.do(t, http.MethodPost, "/api/reports", map[string]any{
		"target_type": "message",
		"target_id":   uuid.New().String(),
		"reason":      "x",
		"context":     big,
		"space_id":    spaceID.String(),
	}, reporterTok)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func strPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}
