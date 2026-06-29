package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Verifier interface {
	Verify(ctx context.Context, token, ip string) (bool, error)
}

type Turnstile struct {
	Secret   string
	Endpoint string
	Client   *http.Client
}

const defaultEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

func NewTurnstile(secret string) *Turnstile {
	if secret == "" {
		return nil
	}
	return &Turnstile{
		Secret:   secret,
		Endpoint: defaultEndpoint,
		Client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type siteverifyResp struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func (t *Turnstile) Verify(ctx context.Context, token, ip string) (bool, error) {
	if token == "" {
		return false, nil
	}
	form := url.Values{"secret": {t.Secret}, "response": {token}}
	if ip != "" {
		form.Set("remoteip", ip)
	}
	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out siteverifyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}
