package files_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/krovara/krovara/internal/files"
)

type env struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
	q    *db.Queries
	root string
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

	root := t.TempDir()
	store, err := files.NewLocalStore(root)
	require.NoError(t, err)

	q := db.New(pool)
	signer := auth.NewJWTSigner([]byte("test-secret"), time.Hour)
	sessions := auth.NewSessionStore(q, 24*time.Hour)
	authSvc := auth.NewService(q, signer, sessions)
	filesSvc := files.NewService(q, store, nil)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { authSvc.Routes(r, nil) })
	mux.Group(func(g chi.Router) {
		g.Use(auth.RequireAuth(signer))
		g.Route("/api", func(api chi.Router) { filesSvc.Routes(api, auth.UserID) })
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{srv: srv, pool: pool, q: q, root: root}
}

func register(t *testing.T, e *env, username, email string) (token string, userID uuid.UUID) {
	t.Helper()
	body := bytes.NewReader([]byte(`{"username":"` + username + `","email":"` + email + `","password":"correct horse battery"}`))
	resp, err := http.Post(e.srv.URL+"/api/auth/register", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	u, err := e.q.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	return out.AccessToken, uuid.UUID(u.ID.Bytes)
}

func uploadFile(t *testing.T, e *env, token, path, filename, mime string, content []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="` + filename + `"`}
	h["Content-Type"] = []string{mime}
	part, err := mw.CreatePart(h)
	require.NoError(t, err)
	_, _ = part.Write(content)
	require.NoError(t, mw.Close())

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

func TestFiles_UploadAndServe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, _ := register(t, e, "alice", "alice@example.com")

	resp := uploadFile(t, e, tok, "/api/files?kind=attachment", "pic.png", "image/png", pngBytes)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var f map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&f))
	require.Equal(t, "image/png", f["mimetype"])
	require.Equal(t, "attachment", f["kind"])
	require.NotEmpty(t, f["sha256"])
	id := f["id"].(string)

	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/files/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	srvResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer srvResp.Body.Close()
	require.Equal(t, http.StatusOK, srvResp.StatusCode)
	require.Equal(t, "image/png", srvResp.Header.Get("Content-Type"))
	require.Contains(t, srvResp.Header.Get("Cache-Control"), "immutable")
	got, _ := io.ReadAll(srvResp.Body)
	require.Equal(t, pngBytes, got)

	etag := srvResp.Header.Get("ETag")
	require.NotEmpty(t, etag)
	req2, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/files/"+id, nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	req2.Header.Set("If-None-Match", etag)
	cond, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	cond.Body.Close()
	require.Equal(t, http.StatusNotModified, cond.StatusCode)
}

func TestFiles_Dedup(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, _ := register(t, e, "alice", "alice@example.com")

	r1 := uploadFile(t, e, tok, "/api/files", "a.png", "image/png", pngBytes)
	defer r1.Body.Close()
	require.Equal(t, http.StatusCreated, r1.StatusCode)
	var f1 map[string]any
	require.NoError(t, json.NewDecoder(r1.Body).Decode(&f1))

	r2 := uploadFile(t, e, tok, "/api/files", "b.png", "image/png", pngBytes)
	defer r2.Body.Close()
	require.Equal(t, http.StatusOK, r2.StatusCode)
	var f2 map[string]any
	require.NoError(t, json.NewDecoder(r2.Body).Decode(&f2))
	require.Equal(t, f1["id"], f2["id"], "dedup should return existing row")

	entries, err := os.ReadDir(filepath.Join(e.root, "attachment"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestFiles_OversizeRejected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, _ := register(t, e, "alice", "alice@example.com")

	big := bytes.Repeat([]byte{0x00}, files.DefaultMaxBytes+1)
	resp := uploadFile(t, e, tok, "/api/files", "huge.pdf", "application/pdf", big)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestFiles_MimeNotAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, _ := register(t, e, "alice", "alice@example.com")

	resp := uploadFile(t, e, tok, "/api/files", "evil.exe", "application/x-msdownload", []byte{0x4D, 0x5A})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
}

func TestFiles_AvatarReplace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	tok, uid := register(t, e, "alice", "alice@example.com")

	r1 := uploadFile(t, e, tok, "/api/me/avatar", "a.png", "image/png", pngBytes)
	defer r1.Body.Close()
	require.Equal(t, http.StatusOK, r1.StatusCode)
	var f1 map[string]any
	require.NoError(t, json.NewDecoder(r1.Body).Decode(&f1))
	firstID := f1["id"].(string)

	user, err := e.q.GetUserByID(context.Background(), pgUUIDLocal(uid))
	require.NoError(t, err)
	require.NotNil(t, user.AvatarKey)
	require.Equal(t, firstID, *user.AvatarKey)

	other := append([]byte{}, pngBytes...)
	other[len(other)-1] ^= 0xFF
	r2 := uploadFile(t, e, tok, "/api/me/avatar", "b.png", "image/png", other)
	defer r2.Body.Close()
	require.Equal(t, http.StatusOK, r2.StatusCode)
	var f2 map[string]any
	require.NoError(t, json.NewDecoder(r2.Body).Decode(&f2))
	require.NotEqual(t, firstID, f2["id"])

	_, err = e.q.GetFile(context.Background(), pgUUIDFromStr(firstID))
	require.Error(t, err)
}

func TestFiles_OwnerOnlyDelete(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	e := setup(t)
	aliceTok, _ := register(t, e, "alice", "alice@example.com")
	bobTok, _ := register(t, e, "bob", "bob@example.com")

	resp := uploadFile(t, e, aliceTok, "/api/files", "a.png", "image/png", pngBytes)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var f map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&f))
	id := f["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/files/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+bobTok)
	r2, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	r2.Body.Close()
	require.Equal(t, http.StatusForbidden, r2.StatusCode)

	req2, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/files/"+id, nil)
	req2.Header.Set("Authorization", "Bearer "+aliceTok)
	r3, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	r3.Body.Close()
	require.Equal(t, http.StatusNoContent, r3.StatusCode)
}

func pgUUIDLocal(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
func pgUUIDFromStr(s string) pgtype.UUID {
	id, _ := uuid.Parse(s)
	return pgUUIDLocal(id)
}
