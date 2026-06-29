package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/krovara/krovara/internal/captcha"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/email"
)

type Service struct {
	Queries  db.Querier
	Signer   *JWTSigner
	Sessions *SessionStore

	Captcha captcha.Verifier

	SignupIPKey []byte

	Email  email.Sender
	AppURL string
	v      *validator.Validate
}

func NewService(q db.Querier, signer *JWTSigner, sessions *SessionStore) *Service {
	return &Service{
		Queries:  q,
		Signer:   signer,
		Sessions: sessions,
		v:        validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (s *Service) Routes(r chi.Router, oauth *OAuthService) {
	rl := NewRateLimitMiddleware(5, time.Minute)
	r.Group(func(g chi.Router) {
		g.Use(rl)
		g.Post("/register", s.handleRegister)
		g.Post("/login", s.handleLogin)
		g.Post("/login/2fa", s.handleLogin2FA)
		g.Post("/verify-email", s.handleVerifyEmail)
		g.Post("/resend-verification", s.handleResendVerification)
	})
	r.Post("/refresh", s.handleRefresh)
	r.Post("/logout", s.handleLogout)
	if oauth != nil {
		oauth.Routes(r)
	}
}

type registerReq struct {
	Username     string `json:"username"      validate:"required,min=3,max=32"`
	Email        string `json:"email"         validate:"required,email"`
	Password     string `json:"password"      validate:"required,min=8,max=128"`
	CaptchaToken string `json:"captcha_token"`
}

type loginReq struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type tokenResp struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type loginResp struct {
	Requires2FA bool   `json:"requires_2fa,omitempty"`
	TempToken   string `json:"temp_token,omitempty"`
	*tokenResp
}

type login2FAReq struct {
	TempToken string `json:"temp_token" validate:"required"`
	Code      string `json:"code"       validate:"required"`
}

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !s.decode(w, r, &req) {
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if !usernameRE.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "pseudo invalide : lettres, chiffres, . _ - uniquement (sans espaces)")
		return
	}
	req.Username = strings.ToLower(req.Username)

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if banned, _ := s.Queries.IsIdentifierBanned(r.Context(), db.IsIdentifierBannedParams{
		Lower: req.Email, Lower_2: req.Username,
	}); banned {
		writeError(w, http.StatusForbidden, "cet identifiant est banni")
		return
	}

	if s.Captcha != nil {
		ok, verr := s.Captcha.Verify(r.Context(), req.CaptchaToken, clientIP(r))
		if verr != nil {
			writeError(w, http.StatusBadGateway, "captcha verification unavailable")
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "captcha failed")
			return
		}
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing failed")
		return
	}
	pwd := hash
	user, err := s.Queries.CreateUser(r.Context(), db.CreateUserParams{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: &pwd,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "username or email already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	s.sendVerificationCode(r.Context(), uuid.UUID(user.ID.Bytes), req.Email)

	if h := s.signupIPHash(clientIP(r)); h != "" {
		_ = s.Queries.SetUserSignupIPHash(r.Context(), db.SetUserSignupIPHashParams{
			ID:           user.ID,
			SignupIpHash: &h,
		})
	}

	s.issueTokens(r.Context(), w, user.ID.Bytes)
}

func (s *Service) signupIPHash(ip string) string {
	if len(s.SignupIPKey) == 0 || ip == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.SignupIPKey)
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !s.decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.Queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if user.PasswordHash == nil {

		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	ok, err := VerifyPassword(req.Password, *user.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if user.Disabled {
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}

	if user.TotpEnabled {
		tempToken, err := s.Signer.Sign2FA(user.ID.Bytes)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "temp token issue failed")
			return
		}
		writeJSON(w, http.StatusOK, loginResp{
			Requires2FA: true,
			TempToken:   tempToken,
		})
		return
	}

	s.issueTokens(r.Context(), w, user.ID.Bytes)
}

func (s *Service) handleLogin2FA(w http.ResponseWriter, r *http.Request) {
	var req login2FAReq
	if !s.decode(w, r, &req) {
		return
	}

	uid, err := s.Signer.Parse2FA(req.TempToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired 2fa token")
		return
	}

	u, err := s.Queries.GetUserByID(r.Context(), pgtype.UUID{Bytes: uid, Valid: true})
	if err != nil || u.Disabled || !u.TotpEnabled || u.TotpSecret == nil {
		writeError(w, http.StatusUnauthorized, "invalid user or 2fa disabled")
		return
	}

	req.Code = strings.TrimSpace(req.Code)

	valid, _ := totp.ValidateCustom(req.Code, *u.TotpSecret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      2,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if !valid && u.BackupCodes != nil {

		var codes []string
		if err := json.Unmarshal(u.BackupCodes, &codes); err == nil {
			for i, c := range codes {
				if c == req.Code {
					valid = true

					codes = append(codes[:i], codes[i+1:]...)
					newCodes, _ := json.Marshal(codes)
					_ = s.Queries.UpdateBackupCodes(r.Context(), db.UpdateBackupCodesParams{
						ID:          u.ID,
						BackupCodes: newCodes,
					})
					break
				}
			}
		}
	}

	if !valid {
		writeError(w, http.StatusUnauthorized, "code 2FA invalide")
		return
	}

	s.issueTokens(r.Context(), w, uid)
}

const refreshCookieName = "krovara_refresh"

func (s *Service) setRefreshCookie(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/api/auth",
		Expires:  exp,
		HttpOnly: true,
		Secure:   trustProxy,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   trustProxy,
		SameSite: http.SameSiteLaxMode,
	})
}

func refreshTokenFrom(r *http.Request, body string) string {
	if c, err := r.Cookie(refreshCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return body
}

func (s *Service) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	token := refreshTokenFrom(r, req.RefreshToken)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	newToken, uid, refreshExp, err := s.Sessions.Rotate(r.Context(), token)
	if err != nil {

		if errors.Is(err, ErrRefreshInvalid) || errors.Is(err, ErrRefreshReuse) {
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		writeError(w, http.StatusInternalServerError, "rotation failed")
		return
	}

	access, err := s.Signer.Sign(uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing failed")
		return
	}

	s.setRefreshCookie(w, newToken, refreshExp)
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:      access,
		RefreshToken:     newToken,
		AccessExpiresAt:  s.Signer.now().Add(s.Signer.ttl),
		RefreshExpiresAt: refreshExp,
	})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	token := refreshTokenFrom(r, req.RefreshToken)
	s.clearRefreshCookie(w)
	if token == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.Sessions.Revoke(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) issueTokens(ctx context.Context, w http.ResponseWriter, uidBytes [16]byte) {
	access, refresh, accessExp, refreshExp, err := s.mintTokens(ctx, uuidFromBytes(uidBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	s.setRefreshCookie(w, refresh, refreshExp)
	writeJSON(w, http.StatusOK, tokenResp{
		AccessToken:      access,
		RefreshToken:     refresh,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
	})
}

func (s *Service) mintTokens(ctx context.Context, uid uuid.UUID) (access, refresh string, accessExp, refreshExp time.Time, err error) {
	access, err = s.Signer.Sign(uid)
	if err != nil {
		return
	}
	refresh, refreshExp, err = s.Sessions.Create(ctx, uid)
	if err != nil {
		return
	}
	accessExp = s.Signer.now().Add(s.Signer.ttl)
	return
}

func (s *Service) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	if err := s.v.Struct(dst); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
