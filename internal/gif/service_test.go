package gif

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDenied(t *testing.T) {
	cases := map[string]bool{
		"cat":           false,
		"happy dog":     false,
		"PORN":          true,
		"some nude pic": true,
		"sexy":          true,
		"banjo":         false,
	}
	for in, want := range cases {
		if got := denied(in); got != want {
			t.Errorf("denied(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHandleSearch_MapsAndFilters(t *testing.T) {
	var gotURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		_, _ = w.Write([]byte(`{
			"result": true,
			"data": {
				"has_next": true,
				"data": [
					{"id": 1, "slug": "happy-cat", "title": "Happy Cat",
					 "file": {"md": {"gif": {"url": "https://cdn/md.gif", "width": 220, "height": 200}},
					          "sm": {"webp": {"url": "https://cdn/sm.webp", "width": 100, "height": 90}}}},
					{"id": 2, "slug": "porn-clip", "title": "x",
					 "file": {"md": {"gif": {"url": "https://cdn/2.gif", "width": 1, "height": 1}},
					          "sm": {"webp": {"url": "https://cdn/2.webp"}}}}
				]
			}
		}`))
	}))
	defer upstream.Close()

	s := &Service{apiKey: "k", base: upstream.URL, http: upstream.Client()}

	req := httptest.NewRequest(http.MethodGet, "/gif/search?q=cat", nil)
	rec := httptest.NewRecorder()
	s.handleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(gotURL, "rating=g") {
		t.Errorf("upstream request missing rating=g: %s", gotURL)
	}

	var out struct {
		Items []item `json:"items"`
		Next  bool   `json:"has_next"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Next {
		t.Error("has_next not propagated")
	}
	if len(out.Items) != 1 {
		t.Fatalf("got %d items, want 1 (denylisted slug filtered)", len(out.Items))
	}
	it := out.Items[0]
	if it.URL != "https://cdn/md.gif" || it.Preview != "https://cdn/sm.webp" {
		t.Errorf("urls = %q / %q", it.URL, it.Preview)
	}
	if it.Width != 220 || it.Height != 200 {
		t.Errorf("dims = %dx%d, want 220x200", it.Width, it.Height)
	}
}

func TestHandleSearch_DeniedQueryShortCircuits(t *testing.T) {
	hit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	s := &Service{apiKey: "k", base: upstream.URL, http: upstream.Client()}
	req := httptest.NewRequest(http.MethodGet, "/gif/search?q=porn", nil)
	rec := httptest.NewRecorder()
	s.handleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if hit {
		t.Error("denylisted query reached upstream — should short-circuit")
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("expected empty items, got %s", rec.Body.String())
	}
}
