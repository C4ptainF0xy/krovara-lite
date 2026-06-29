package links

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/krovara/krovara/internal/db"
)

type fakeLookup struct{ bad map[string]string }

func (f fakeLookup) MaliciousURLHashes(_ context.Context, hashes []string) ([]db.MaliciousURLHashesRow, error) {
	var out []db.MaliciousURLHashesRow
	for _, h := range hashes {
		if threat, ok := f.bad[h]; ok {
			t := threat
			out = append(out, db.MaliciousURLHashesRow{UrlHash: h, Threat: &t})
		}
	}
	return out, nil
}

func hashOf(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:])
}

func TestHandleCheck_FlagsKnownBadOnly(t *testing.T) {
	const badURL = "http://evil.example/malware.bin"
	const safeURL = "https://example.com/ok"
	svc := NewService(fakeLookup{bad: map[string]string{hashOf(badURL): "malware_download"}})

	body, _ := json.Marshal(map[string]any{"urls": []string{badURL, safeURL}})
	req := httptest.NewRequest(http.MethodPost, "/links/check", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	svc.handleCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Malicious []flagged `json:"malicious"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Malicious) != 1 {
		t.Fatalf("got %d flagged, want 1: %+v", len(out.Malicious), out.Malicious)
	}
	if out.Malicious[0].URL != badURL || out.Malicious[0].Threat != "malware_download" {
		t.Errorf("flagged = %+v", out.Malicious[0])
	}
}

func TestHandleCheck_EmptyWhenNoneBad(t *testing.T) {
	svc := NewService(fakeLookup{bad: map[string]string{}})
	body, _ := json.Marshal(map[string]any{"urls": []string{"https://example.com"}})
	req := httptest.NewRequest(http.MethodPost, "/links/check", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	svc.handleCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"malicious":[]`) {
		t.Errorf("expected empty malicious list, got %s", rec.Body.String())
	}
}
