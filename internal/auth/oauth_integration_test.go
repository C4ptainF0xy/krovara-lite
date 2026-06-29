package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/cookiejar"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/oauth2"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
)

type mockProvider struct {
	srv             *httptest.Server
	expectedCode    string
	storedChallenge string
	userInfo        auth.UserInfo

	gotVerifier string
}

func newMockProvider(t *testing.T, ui auth.UserInfo) *mockProvider {
	t.Helper()
	m := &mockProvider{
		expectedCode: "the-auth-code",
		userInfo:     ui,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		m.storedChallenge = q.Get("code_challenge")

		redirect, _ := url.Parse(q.Get("redirect_uri"))
		rq := redirect.Query()
		rq.Set("code", m.expectedCode)
		rq.Set("state", q.Get("state"))
		redirect.RawQuery = rq.Encode()
		http.Redirect(w, r, redirect.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		m.gotVerifier = r.PostForm.Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.userInfo)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockProvider) provider(redirectURL string) *auth.ProviderConfig {
	cfg := &auth.ProviderConfig{
		Name: "mock",
		OAuth: &oauth2.Config{
			ClientID:     "mock-client",
			ClientSecret: "mock-secret",
			RedirectURL:  redirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:   m.srv.URL + "/authorize",
				TokenURL:  m.srv.URL + "/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
			Scopes: []string{"email"},
		},
		FetchUserInfo: func(ctx context.Context, client *http.Client) (auth.UserInfo, error) {
			var got auth.UserInfo
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, m.srv.URL+"/userinfo", nil)
			resp, err := client.Do(req)
			if err != nil {
				return auth.UserInfo{}, err
			}
			defer func() { _ = resp.Body.Close() }()
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				return auth.UserInfo{}, err
			}
			return got, nil
		},
	}
	return cfg
}

type oauthEnv struct {
	srv    *httptest.Server
	client *http.Client
	q      *db.Queries
	mock   *mockProvider
}

func setupOAuth(t *testing.T, ui auth.UserInfo) *oauthEnv {
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

	migrationsDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	migrateDSN := "pgx5://" + dsn[len("postgres://"):]
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsDir), migrateDSN)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	q := db.New(pool)
	signer := auth.NewJWTSigner([]byte("test-secret"), time.Hour)
	sessions := auth.NewSessionStore(q, 24*time.Hour)
	svc := auth.NewService(q, signer, sessions)

	srv := httptest.NewUnstartedServer(nil)
	t.Cleanup(srv.Close)
	srvURL := "http://" + srv.Listener.Addr().String()

	mock := newMockProvider(t, ui)
	prov := mock.provider(srvURL + "/api/auth/mock/callback")
	oauthSvc := auth.NewOAuthService(svc, false, prov)

	mux := chi.NewMux()
	mux.Route("/api/auth", func(r chi.Router) { svc.Routes(r, oauthSvc) })
	srv.Config.Handler = mux
	srv.Start()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &oauthEnv{srv: srv, client: client, q: q, mock: mock}
}

func runFlow(t *testing.T, env *oauthEnv) *http.Response {
	t.Helper()

	resp, err := env.client.Get(env.srv.URL + "/api/auth/mock")
	require.NoError(t, err)
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("start status=%d body=%s url=%s", resp.StatusCode, body, env.srv.URL+"/api/auth/mock")
	}
	authURL, err := resp.Location()
	require.NoError(t, err)
	_ = resp.Body.Close()

	resp, err = env.client.Get(authURL.String())
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	cbURL, err := resp.Location()
	require.NoError(t, err)
	_ = resp.Body.Close()

	cb, err := env.client.Get(cbURL.String())
	require.NoError(t, err)
	return cb
}

func TestOAuthNewUser(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{
		ProviderID: "mock-123",
		Email:      "newcomer@example.com",
		Username:   "newcomer",
	})

	resp := runFlow(t, env)

	require.Equal(t, http.StatusFound, resp.StatusCode)
	loc, err := resp.Location()
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Contains(t, loc.Path, "/oauth/mock/callback")
	frag, err := url.ParseQuery(loc.Fragment)
	require.NoError(t, err)
	require.Empty(t, frag.Get("access_token"), "new user should not get tokens before picking a username")
	signupToken := frag.Get("signup_token")
	require.NotEmpty(t, signupToken)
	require.Equal(t, "newcomer", frag.Get("suggested"))

	require.NotEmpty(t, env.mock.gotVerifier)
	require.NotEmpty(t, env.mock.storedChallenge)

	body := strings.NewReader(`{"signup_token":"` + signupToken + `","username":"chosenname"}`)
	cr, err := http.Post(env.srv.URL+"/api/auth/complete", "application/json", body)
	require.NoError(t, err)
	defer cr.Body.Close()
	require.Equal(t, http.StatusOK, cr.StatusCode)
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(cr.Body).Decode(&tok))
	require.NotEmpty(t, tok.AccessToken)
	require.NotEmpty(t, tok.RefreshToken)
}

func TestOAuthLinksByEmail(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{
		ProviderID: "mock-999",
		Email:      "linkme@example.com",
		Username:   "linkme",
	})

	regBody := strings.NewReader(`{"username":"original","email":"linkme@example.com","password":"strongpass"}`)
	r, err := http.Post(env.srv.URL+"/api/auth/register", "application/json", regBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, r.StatusCode)
	_ = r.Body.Close()

	resp := runFlow(t, env)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	_ = resp.Body.Close()

	user, err := env.q.GetUserByEmail(context.Background(), "linkme@example.com")
	require.NoError(t, err)
	require.Equal(t, "original", user.Username)
}

func TestOAuthStateMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{
		ProviderID: "mock-1",
		Email:      "x@example.com",
		Username:   "x",
	})

	resp, err := env.client.Get(env.srv.URL + "/api/auth/mock")
	require.NoError(t, err)
	_ = resp.Body.Close()

	resp, err = env.client.Get(env.srv.URL + "/api/auth/mock/callback?code=x&state=tampered")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestOAuthUnknownProvider(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{})

	resp, err := env.client.Get(env.srv.URL + "/api/auth/bogus")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = env.client.Get(env.srv.URL + "/api/auth/bogus/callback?code=x&state=y")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestOAuthCallbackMissingCookie(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{})

	resp, err := env.client.Get(env.srv.URL + "/api/auth/mock/callback?code=x&state=y")
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestOAuthCallbackMissingCode(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	env := setupOAuth(t, auth.UserInfo{})

	resp, err := env.client.Get(env.srv.URL + "/api/auth/mock")
	require.NoError(t, err)
	_ = resp.Body.Close()

	u, _ := url.Parse(env.srv.URL + "/api/auth/mock/callback")
	var state string
	for _, c := range env.client.Jar.Cookies(u) {
		if c.Name == "krovara_oauth_state" {
			state = c.Value
		}
	}
	require.NotEmpty(t, state)

	resp, err = env.client.Get(env.srv.URL + "/api/auth/mock/callback?state=" + state)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}
