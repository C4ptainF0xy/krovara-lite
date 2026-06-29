package voip

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/krovara/krovara/internal/permissions"
)

const DefaultTTL = 1 * time.Hour

type Service struct {
	secret []byte
	uris   []string
	ttl    time.Duration
	now    func() time.Time
}

func NewService(secret string, uris []string) *Service {
	return &Service{
		secret: []byte(secret),
		uris:   uris,
		ttl:    DefaultTTL,
		now:    time.Now,
	}
}

func (s *Service) Routes(r chi.Router, uidFn permissions.UserIDFunc) {
	r.Get("/voip/turn-credentials", s.handleCredentials(uidFn))
}

type Credential struct {
	URIs       []string  `json:"uris"`
	Username   string    `json:"username"`
	Credential string    `json:"credential"`
	TTLSeconds int64     `json:"ttl_seconds"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (s *Service) Issue(userID uuid.UUID) Credential {
	expires := s.now().Add(s.ttl)

	username := strconv.FormatInt(expires.Unix(), 10) + ":" + userID.String()
	mac := hmac.New(sha1.New, s.secret)
	mac.Write([]byte(username))
	cred := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return Credential{
		URIs:       s.uris,
		Username:   username,
		Credential: cred,
		TTLSeconds: int64(s.ttl.Seconds()),
		ExpiresAt:  expires,
	}
}

func (s *Service) handleCredentials(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if len(s.secret) == 0 || len(s.uris) == 0 {
			http.Error(w, "voip not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Issue(uid))
	}
}

func (s *Service) VerifyCredential(username, credential string) error {
	mac := hmac.New(sha1.New, s.secret)
	mac.Write([]byte(username))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(credential)) {
		return fmt.Errorf("voip: hmac mismatch")
	}
	parts := strings.SplitN(username, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("voip: malformed username")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return fmt.Errorf("voip: bad timestamp")
	}
	if time.Unix(ts, 0).Before(s.now()) {
		return fmt.Errorf("voip: credential expired")
	}
	return nil
}
