package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	DefaultAccessTTL = 15 * time.Minute
)

var ErrInvalidToken = errors.New("auth: invalid token")

type AccessClaims struct {
	UserID uuid.UUID `json:"sub"`
	jwt.RegisteredClaims
}

type OAuthSignupClaims struct {
	Provider   string `json:"provider"`
	ProviderID string `json:"pid"`
	Email      string `json:"email"`
	Suggested  string `json:"suggested"`
	jwt.RegisteredClaims
}

type JWTSigner struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewJWTSigner(secret []byte, ttl time.Duration) *JWTSigner {
	if ttl == 0 {
		ttl = DefaultAccessTTL
	}
	return &JWTSigner{secret: secret, ttl: ttl, now: time.Now}
}

func (s *JWTSigner) Sign(userID uuid.UUID) (string, error) {
	now := s.now()
	claims := AccessClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing jwt: %w", err)
	}
	return signed, nil
}

func (s *JWTSigner) Parse(token string) (uuid.UUID, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(s.now),
	)

	parsed, err := parser.ParseWithClaims(token, &AccessClaims{}, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return uuid.Nil, ErrInvalidToken
	}
	if claims.UserID == uuid.Nil || claims.Issuer == "2fa" {
		return uuid.Nil, ErrInvalidToken
	}
	return claims.UserID, nil
}

func (s *JWTSigner) Sign2FA(userID uuid.UUID) (string, error) {
	now := s.now()
	claims := AccessClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    "2fa",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing 2fa jwt: %w", err)
	}
	return signed, nil
}

func (s *JWTSigner) SignOAuthSignup(provider, providerID, email, suggested string) (string, error) {
	now := s.now()
	claims := OAuthSignupClaims{
		Provider:   provider,
		ProviderID: providerID,
		Email:      email,
		Suggested:  suggested,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "oauth-signup",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing oauth-signup jwt: %w", err)
	}
	return signed, nil
}

func (s *JWTSigner) ParseOAuthSignup(token string) (*OAuthSignupClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(s.now),
	)
	parsed, err := parser.ParseWithClaims(token, &OAuthSignupClaims{}, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	claims, ok := parsed.Claims.(*OAuthSignupClaims)
	if !ok || !parsed.Valid || claims.Issuer != "oauth-signup" || claims.ProviderID == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *JWTSigner) Parse2FA(token string) (uuid.UUID, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(s.now),
	)

	parsed, err := parser.ParseWithClaims(token, &AccessClaims{}, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return uuid.Nil, ErrInvalidToken
	}
	if claims.UserID == uuid.Nil || claims.Issuer != "2fa" {
		return uuid.Nil, ErrInvalidToken
	}
	return claims.UserID, nil
}
