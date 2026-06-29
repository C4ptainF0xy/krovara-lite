package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleAndGitHubProviderDefaults(t *testing.T) {
	g := GoogleProvider("id", "secret", "https://example.com/cb")
	if g.Name != "google" || g.OAuth.ClientID != "id" || g.OAuth.ClientSecret != "secret" {
		t.Fatalf("google config: %+v", g.OAuth)
	}
	if g.OAuth.RedirectURL != "https://example.com/cb" || g.FetchUserInfo == nil {
		t.Fatalf("google: unexpected fields")
	}

	gh := GitHubProvider("a", "b", "c")
	if gh.Name != "github" || gh.OAuth.ClientID != "a" || gh.FetchUserInfo == nil {
		t.Fatalf("github config: %+v", gh.OAuth)
	}
}

func TestGoogleFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"123","email":"a@b.c","name":"Alice"}`))
	}))
	t.Cleanup(srv.Close)

	info, err := googleFetcher(srv.URL)(context.Background(), http.DefaultClient)
	if err != nil || info.ProviderID != "123" || info.Email != "a@b.c" || info.Username != "Alice" {
		t.Fatalf("got info=%+v err=%v", info, err)
	}
}

func TestGoogleFetcherMissingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sub":"","email":""}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := googleFetcher(srv.URL)(context.Background(), http.DefaultClient); err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestGoogleFetcherEmailFallbackUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sub":"1","email":"x@y.z"}`))
	}))
	t.Cleanup(srv.Close)
	info, err := googleFetcher(srv.URL)(context.Background(), http.DefaultClient)
	if err != nil || info.Username != "x@y.z" {
		t.Fatalf("expected email fallback, got %+v", info)
	}
}

func TestGitHubFetcherPublicEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":42,"login":"bob","email":"bob@x.y"}`))
	}))
	t.Cleanup(srv.Close)
	info, err := githubFetcher(srv.URL, "unused")(context.Background(), http.DefaultClient)
	if err != nil || info.ProviderID != "42" || info.Email != "bob@x.y" {
		t.Fatalf("got %+v err=%v", info, err)
	}
}

func TestGitHubFetcherPrivateEmail(t *testing.T) {
	user := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"login":"carol","email":null}`))
	}))
	t.Cleanup(user.Close)
	emails := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"email":"old@x.y","primary":false,"verified":true},
			{"email":"carol@x.y","primary":true,"verified":true}
		]`))
	}))
	t.Cleanup(emails.Close)

	info, err := githubFetcher(user.URL, emails.URL)(context.Background(), http.DefaultClient)
	if err != nil || info.Email != "carol@x.y" {
		t.Fatalf("got %+v err=%v", info, err)
	}
}

func TestGitHubFetcherNoUsableEmail(t *testing.T) {
	user := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"login":"x","email":null}`))
	}))
	t.Cleanup(user.Close)
	emails := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"email":"unverified@x.y","primary":true,"verified":false}]`))
	}))
	t.Cleanup(emails.Close)

	if _, err := githubFetcher(user.URL, emails.URL)(context.Background(), http.DefaultClient); err == nil {
		t.Fatal("expected error for no verified primary email")
	}
}

func TestFetchJSONNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)
	var out any
	if err := fetchJSON(context.Background(), http.DefaultClient, srv.URL, &out); err == nil {
		t.Fatal("expected non-2xx error")
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 9: "9", 10: "10", 99: "99", 1234: "1234"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPKCEPair(t *testing.T) {
	v, c, err := pkcePair()
	if err != nil || v == "" || c == "" || v == c {
		t.Fatalf("pkce v=%q c=%q err=%v", v, c, err)
	}
}
