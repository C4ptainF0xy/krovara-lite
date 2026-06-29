package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	googleoauth "golang.org/x/oauth2/google"
)

type UserInfo struct {
	ProviderID string
	Email      string
	Username   string
}

type UserInfoFetcher func(ctx context.Context, client *http.Client) (UserInfo, error)

type ProviderConfig struct {
	Name          string
	OAuth         *oauth2.Config
	FetchUserInfo UserInfoFetcher
}

const (
	googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	githubUserURL     = "https://api.github.com/user"
	githubEmailsURL   = "https://api.github.com/user/emails"
)

func GoogleProvider(clientID, clientSecret, redirectURL string) *ProviderConfig {
	return &ProviderConfig{
		Name: "google",
		OAuth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     googleoauth.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		},
		FetchUserInfo: googleFetcher(googleUserInfoURL),
	}
}

func GitHubProvider(clientID, clientSecret, redirectURL string) *ProviderConfig {
	return &ProviderConfig{
		Name: "github",
		OAuth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     github.Endpoint,
			Scopes:       []string{"read:user", "user:email"},
		},
		FetchUserInfo: githubFetcher(githubUserURL, githubEmailsURL),
	}
}

func googleFetcher(userInfoURL string) UserInfoFetcher {
	return func(ctx context.Context, client *http.Client) (UserInfo, error) {
		var body struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := fetchJSON(ctx, client, userInfoURL, &body); err != nil {
			return UserInfo{}, err
		}
		if body.Sub == "" || body.Email == "" {
			return UserInfo{}, errors.New("google userinfo missing sub or email")
		}
		username := body.Name
		if username == "" {
			username = body.Email
		}
		return UserInfo{ProviderID: body.Sub, Email: body.Email, Username: username}, nil
	}
}

func githubFetcher(userURL, emailsURL string) UserInfoFetcher {
	return func(ctx context.Context, client *http.Client) (UserInfo, error) {
		var u struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Email string `json:"email"`
		}
		if err := fetchJSON(ctx, client, userURL, &u); err != nil {
			return UserInfo{}, err
		}
		if u.ID == 0 || u.Login == "" {
			return UserInfo{}, errors.New("github user missing id or login")
		}
		email := u.Email
		if email == "" {
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := fetchJSON(ctx, client, emailsURL, &emails); err != nil {
				return UserInfo{}, err
			}
			for _, e := range emails {
				if e.Primary && e.Verified {
					email = e.Email
					break
				}
			}
		}
		if email == "" {
			return UserInfo{}, errors.New("github user has no usable email")
		}
		return UserInfo{ProviderID: fmt.Sprintf("%d", u.ID), Email: email, Username: u.Login}, nil
	}
}

func fetchJSON(ctx context.Context, client *http.Client, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s returned %d: %s", url, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkcePair() (verifier, challenge string, err error) {
	verifier, err = randomURLSafe(48)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
