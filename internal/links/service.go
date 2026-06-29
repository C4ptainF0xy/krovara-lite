package links

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/krovara/krovara/internal/db"
)

const maxURLs = 50

type lookup interface {
	MaliciousURLHashes(ctx context.Context, hashes []string) ([]db.MaliciousURLHashesRow, error)
}

type Service struct {
	q  lookup
	sb *SafeBrowsing
}

func NewService(q lookup) *Service {
	return &Service{q: q}
}

func (s *Service) WithSafeBrowsing(sb *SafeBrowsing) *Service {
	s.sb = sb
	return s
}

func (s *Service) Routes(r chi.Router) {
	r.Post("/links/check", s.handleCheck)
}

type checkReq struct {
	URLs []string `json:"urls"`
}

type flagged struct {
	URL    string `json:"url"`
	Threat string `json:"threat"`
}

func (s *Service) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.URLs) > maxURLs {
		req.URLs = req.URLs[:maxURLs]
	}

	byHash := make(map[string]string, len(req.URLs))
	hashes := make([]string, 0, len(req.URLs))
	for _, u := range req.URLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		sum := sha256.Sum256([]byte(u))
		h := hex.EncodeToString(sum[:])
		if _, ok := byHash[h]; !ok {
			hashes = append(hashes, h)
		}
		byHash[h] = u
	}

	out := []flagged{}
	flaggedURLs := make(map[string]bool)
	if len(hashes) > 0 {
		rows, err := s.q.MaliciousURLHashes(r.Context(), hashes)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "check failed")
			return
		}
		for _, row := range rows {
			threat := ""
			if row.Threat != nil {
				threat = *row.Threat
			}
			u := byHash[row.UrlHash]
			out = append(out, flagged{URL: u, Threat: threat})
			flaggedURLs[u] = true
		}
	}

	if s.sb != nil {
		remaining := make([]string, 0, len(byHash))
		for _, u := range byHash {
			if !flaggedURLs[u] {
				remaining = append(remaining, u)
			}
		}
		if len(remaining) > 0 {
			for u, threat := range s.sb.Lookup(r.Context(), remaining) {
				if threat != "" && !flaggedURLs[u] {
					out = append(out, flagged{URL: u, Threat: threat})
					flaggedURLs[u] = true
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"malicious": out})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
