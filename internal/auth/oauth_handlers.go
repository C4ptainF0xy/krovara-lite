package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/oauth2"

	"github.com/krovara/krovara/internal/db"
)

type OAuthService struct {
	Providers map[string]*ProviderConfig
	Secure    bool
	Parent    *Service
}

func NewOAuthService(parent *Service, secure bool, providers ...*ProviderConfig) *OAuthService {
	m := make(map[string]*ProviderConfig, len(providers))
	for _, p := range providers {
		m[p.Name] = p
	}
	return &OAuthService{Providers: m, Secure: secure, Parent: parent}
}

func (o *OAuthService) Routes(r chi.Router) {
	r.Get("/{provider}", o.handleStart)
	r.Get("/{provider}/callback", o.handleCallback)
	r.Post("/complete", o.handleComplete)
}

const (
	cookieState    = "krovara_oauth_state"
	cookieVerifier = "krovara_oauth_verifier"
	cookieNative   = "krovara_oauth_native"
	cookieLifetime = 10 * time.Minute

	nativeRedirectScheme = "krovara"
)

func (o *OAuthService) handleStart(w http.ResponseWriter, r *http.Request) {
	prov, ok := o.Providers[chi.URLParam(r, "provider")]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown provider")
		return
	}

	state, err := randomURLSafe(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state generation failed")
		return
	}
	verifier, challenge, err := pkcePair()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pkce generation failed")
		return
	}

	o.setCookie(w, cookieState, state)
	o.setCookie(w, cookieVerifier, verifier)

	if r.URL.Query().Get("platform") == "app" {
		o.setCookie(w, cookieNative, "1")
	}

	url := prov.OAuth.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (o *OAuthService) handleCallback(w http.ResponseWriter, r *http.Request) {
	prov, ok := o.Providers[chi.URLParam(r, "provider")]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown provider")
		return
	}

	stateCookie, err := r.Cookie(cookieState)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "state mismatch")
		return
	}
	verifierCookie, err := r.Cookie(cookieVerifier)
	if err != nil || verifierCookie.Value == "" {
		writeError(w, http.StatusBadRequest, "missing pkce verifier")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}

	tok, err := prov.OAuth.Exchange(r.Context(), code,
		oauth2.SetAuthURLParam("code_verifier", verifierCookie.Value),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "code exchange failed")
		return
	}

	info, err := prov.FetchUserInfo(r.Context(), prov.OAuth.Client(r.Context(), tok))
	if err != nil {
		writeError(w, http.StatusBadGateway, "userinfo fetch failed")
		return
	}

	info.Email = strings.ToLower(strings.TrimSpace(info.Email))

	o.clearCookie(w, cookieState)
	o.clearCookie(w, cookieVerifier)

	user, found, err := o.findOAuthUser(r.Context(), prov.Name, info)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user provisioning failed")
		return
	}

	if found && user.Disabled {
		writeError(w, http.StatusForbidden, "compte désactivé")
		return
	}

	if !found {
		signupTok, err := o.Parent.Signer.SignOAuthSignup(prov.Name, info.ProviderID, info.Email, sanitizeUsername(info.Username))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "signup token issue failed")
			return
		}
		frag := url.Values{}
		frag.Set("signup_token", signupTok)
		frag.Set("suggested", sanitizeUsername(info.Username))
		o.redirectFragment(w, r, prov.Name, frag)
		return
	}

	if user.TotpEnabled {
		tempToken, err := o.Parent.Signer.Sign2FA(uuid.UUID(user.ID.Bytes))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "temp token issue failed")
			return
		}
		frag := url.Values{}
		frag.Set("requires_2fa", "1")
		frag.Set("temp_token", tempToken)
		o.redirectFragment(w, r, prov.Name, frag)
		return
	}

	access, refresh, accessExp, _, err := o.Parent.mintTokens(r.Context(), uuid.UUID(user.ID.Bytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	frag := url.Values{}
	frag.Set("access_token", access)
	frag.Set("refresh_token", refresh)
	frag.Set("access_expires_at", accessExp.Format(time.RFC3339))
	o.redirectFragment(w, r, prov.Name, frag)
}

func (o *OAuthService) redirectFragment(w http.ResponseWriter, r *http.Request, provider string, frag url.Values) {
	dest := "/oauth/" + provider + "/callback#" + frag.Encode()
	if c, err := r.Cookie(cookieNative); err == nil && c.Value == "1" {
		o.clearCookie(w, cookieNative)

		dest = nativeRedirectScheme + "://oauth/" + provider + "/callback?" + frag.Encode()
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

type oauthCompleteReq struct {
	SignupToken string `json:"signup_token"`
	Username    string `json:"username"`
}

func (o *OAuthService) handleComplete(w http.ResponseWriter, r *http.Request) {
	var req oauthCompleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	claims, err := o.Parent.Signer.ParseOAuthSignup(req.SignupToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired signup token")
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !usernameRE.MatchString(username) || len(username) < 3 || len(username) > 32 {
		writeError(w, http.StatusBadRequest, "pseudo invalide : 3-32 caractères, lettres/chiffres/._- (sans espaces)")
		return
	}
	if _, err := o.Parent.Queries.GetUserByUsername(r.Context(), username); err == nil {
		writeError(w, http.StatusConflict, "pseudo déjà pris")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	if _, err := o.Parent.Queries.GetOAuthAccount(r.Context(), db.GetOAuthAccountParams{
		Provider:   claims.Provider,
		ProviderID: claims.ProviderID,
	}); err == nil {
		writeError(w, http.StatusConflict, "compte déjà créé")
		return
	}

	user, err := o.Parent.Queries.CreateUser(r.Context(), db.CreateUserParams{
		Username:     username,
		Email:        claims.Email,
		PasswordHash: nil,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "pseudo ou email déjà pris")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}
	if _, err := o.Parent.Queries.CreateOAuthAccount(r.Context(), db.CreateOAuthAccountParams{
		UserID:     user.ID,
		Provider:   claims.Provider,
		ProviderID: claims.ProviderID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not link provider")
		return
	}

	access, refresh, accessExp, refreshExp, err := o.Parent.mintTokens(r.Context(), uuid.UUID(user.ID.Bytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	o.Parent.setRefreshCookie(w, refresh, refreshExp)
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpiresAt: accessExp,
	})
}

func (o *OAuthService) findOAuthUser(ctx context.Context, provider string, info UserInfo) (db.User, bool, error) {
	link, err := o.Parent.Queries.GetOAuthAccount(ctx, db.GetOAuthAccountParams{
		Provider:   provider,
		ProviderID: info.ProviderID,
	})
	if err == nil {
		u, err := o.Parent.Queries.GetUserByID(ctx, link.UserID)
		return u, err == nil, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, false, err
	}

	if existing, err := o.Parent.Queries.GetUserByEmail(ctx, info.Email); err == nil {
		if _, err := o.Parent.Queries.CreateOAuthAccount(ctx, db.CreateOAuthAccountParams{
			UserID:     existing.ID,
			Provider:   provider,
			ProviderID: info.ProviderID,
		}); err != nil {
			return db.User{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, false, err
	}

	return db.User{}, false, nil
}

func sanitizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "user"
	}
	return out
}

func uniqueUsername(ctx context.Context, q db.Querier, desired string) string {
	candidate := desired
	for i := 1; i < 100; i++ {
		if _, err := q.GetUserByUsername(ctx, candidate); errors.Is(err, pgx.ErrNoRows) {
			return candidate
		}
		candidate = desired + "-" + itoa(i)
	}

	return desired
}

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

func (o *OAuthService) setCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   o.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cookieLifetime.Seconds()),
	})
}

func (o *OAuthService) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   o.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
