package links

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SafeBrowsing struct {
	apiKey string
	http   *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	threat string
	at     time.Time
}

const sbCacheTTL = 30 * time.Minute

func NewSafeBrowsing(apiKey string) *SafeBrowsing {
	if apiKey == "" {
		return nil
	}
	return &SafeBrowsing{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 5 * time.Second},
		cache:  make(map[string]cacheEntry),
	}
}

func (s *SafeBrowsing) Lookup(ctx context.Context, urls []string) map[string]string {
	out := make(map[string]string, len(urls))
	var toQuery []string

	s.mu.Lock()
	for _, u := range urls {
		if e, ok := s.cache[u]; ok && time.Since(e.at) < sbCacheTTL {
			out[u] = e.threat
		} else {
			toQuery = append(toQuery, u)
		}
	}
	s.mu.Unlock()

	if len(toQuery) == 0 {
		return out
	}

	entries := make([]map[string]string, len(toQuery))
	for i, u := range toQuery {
		entries[i] = map[string]string{"url": u}
	}
	body, _ := json.Marshal(map[string]any{
		"client": map[string]string{"clientId": "krovara", "clientVersion": "1.0"},
		"threatInfo": map[string]any{
			"threatTypes":      []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE", "POTENTIALLY_HARMFUL_APPLICATION"},
			"platformTypes":    []string{"ANY_PLATFORM"},
			"threatEntryTypes": []string{"URL"},
			"threatEntries":    entries,
		},
	})

	url := "https://safebrowsing.googleapis.com/v4/threatMatches:find?key=" + s.apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out
	}

	var parsed struct {
		Matches []struct {
			ThreatType string `json:"threatType"`
			Threat     struct {
				URL string `json:"url"`
			} `json:"threat"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return out
	}

	hits := make(map[string]string)
	for _, m := range parsed.Matches {
		hits[m.Threat.URL] = friendlyThreat(m.ThreatType)
	}

	now := time.Now()
	s.mu.Lock()
	for _, u := range toQuery {
		t := hits[u]
		s.cache[u] = cacheEntry{threat: t, at: now}
		out[u] = t
	}
	s.mu.Unlock()
	return out
}

func friendlyThreat(t string) string {
	switch t {
	case "MALWARE":
		return "malware"
	case "SOCIAL_ENGINEERING":
		return "phishing"
	case "UNWANTED_SOFTWARE":
		return "logiciel indésirable"
	case "POTENTIALLY_HARMFUL_APPLICATION":
		return "application dangereuse"
	default:
		return "menace"
	}
}

var _ = fmt.Sprintf
