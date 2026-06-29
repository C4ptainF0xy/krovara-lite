package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const (
	EventMessageCreate = "message.create"
	EventMemberJoin    = "member.join"
	EventMemberLeave   = "member.leave"
)

type Enqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type Service struct {
	q     *db.Queries
	pool  *pgxpool.Pool
	river Enqueuer
	v     *validator.Validate
}

func NewService(pool *pgxpool.Pool, enq Enqueuer) *Service {
	return &Service{
		q:     db.New(pool),
		pool:  pool,
		river: enq,
		v:     validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, uidFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/webhooks", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Post("/", s.handleCreate(uidFn))
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Get("/", s.handleList())
	})
	r.Route("/webhooks/{webhookID}", func(rr chi.Router) {
		rr.Use(s.attachWebhookSpace)
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Patch("/", s.handleUpdate(uidFn))
		rr.With(permissions.RequireSpace(resolver, uidFn, permissions.ManageSpace)).
			Delete("/", s.handleDelete(uidFn))
	})
}

func (s *Service) PublicRoutes(r chi.Router) {
	r.Post("/inbound/webhooks/{webhookID}", s.handleIncoming())
}

func (s *Service) attachWebhookSpace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid webhook id")
			return
		}
		row, err := s.q.GetWebhook(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("spaceID", uuid.UUID(row.SpaceID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Name      string   `json:"name"       validate:"required,min=1,max=64"`
	URL       string   `json:"url"        validate:"required,url"`
	ChannelID *string  `json:"channel_id" validate:"omitempty,uuid"`
	Events    []string `json:"events"     validate:"required,min=1,dive,oneof=message.create member.join member.leave"`
}

type updateReq struct {
	Name      *string   `json:"name"       validate:"omitempty,min=1,max=64"`
	URL       *string   `json:"url"        validate:"omitempty,url"`
	ChannelID *string   `json:"channel_id" validate:"omitempty,uuid"`
	Events    *[]string `json:"events"     validate:"omitempty,min=1,dive,oneof=message.create member.join member.leave"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req createReq
		if !s.decode(w, r, &req) {
			return
		}
		secret, err := randomSecret()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "secret gen failed")
			return
		}
		var chID pgtype.UUID
		if req.ChannelID != nil {
			id, _ := uuid.Parse(*req.ChannelID)
			chID = pgUUID(id)
		}
		row, err := s.q.CreateWebhook(r.Context(), db.CreateWebhookParams{
			SpaceID:   pgUUID(spaceID),
			ChannelID: chID,
			Name:      req.Name,
			Url:       req.URL,
			Secret:    &secret,
			Events:    req.Events,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		_ = uidFn

		writeJSON(w, http.StatusCreated, webhookDTO(row, &secret))
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceWebhooks(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, w := range rows {
			out = append(out, webhookDTO(w, nil))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleUpdate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = uidFn
		id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid webhook id")
			return
		}
		var req updateReq
		if !s.decode(w, r, &req) {
			return
		}
		var chID pgtype.UUID
		if req.ChannelID != nil {
			id, _ := uuid.Parse(*req.ChannelID)
			chID = pgUUID(id)
		}
		var events []string
		if req.Events != nil {
			events = *req.Events
		}
		row, err := s.q.UpdateWebhook(r.Context(), db.UpdateWebhookParams{
			ID:        pgUUID(id),
			Name:      req.Name,
			Url:       req.URL,
			ChannelID: chID,
			Events:    events,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, webhookDTO(row, nil))
	}
}

func (s *Service) handleDelete(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = uidFn
		id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid webhook id")
			return
		}
		if err := s.q.DeleteWebhook(r.Context(), pgUUID(id)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type incomingPayload struct {
	Content string `json:"content" validate:"required,min=1,max=4000"`
}

var incomingLimiter = newRateLimiter(60, time.Minute)

func (s *Service) handleIncoming() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid webhook id")
			return
		}
		if !incomingLimiter.allow(id.String()) {
			writeError(w, http.StatusTooManyRequests, "rate limited")
			return
		}
		hook, err := s.q.GetWebhook(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var payload incomingPayload
		if !s.decode(w, r, &payload) {
			return
		}

		meta, _ := json.Marshal(map[string]any{"content": payload.Content})
		_, _ = s.q.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
			SpaceID:  hook.SpaceID,
			ActorID:  pgtype.UUID{},
			Action:   "webhook.incoming",
			TargetID: hook.ID,
			Metadata: meta,
		})
		w.WriteHeader(http.StatusAccepted)
	}
}

func (s *Service) Emit(ctx context.Context, spaceID uuid.UUID, event string, payload any) {
	if s.river == nil {
		return
	}
	rows, err := s.q.ListWebhooksForEvent(ctx, db.ListWebhooksForEventParams{
		SpaceID: pgUUID(spaceID),
		Event:   event,
	})
	if err != nil {
		return
	}
	body, _ := json.Marshal(payload)
	for _, h := range rows {
		_, _ = s.river.Insert(ctx, WebhookDeliverArgs{
			WebhookID: uuid.UUID(h.ID.Bytes).String(),
			URL:       h.Url,
			Secret:    deref(h.Secret),
			Event:     event,
			Body:      body,
		}, nil)
	}
}

type WebhookDeliverArgs struct {
	WebhookID string `json:"webhook_id"`
	URL       string `json:"url"`
	Secret    string `json:"secret"`
	Event     string `json:"event"`
	Body      []byte `json:"body"`
}

func (WebhookDeliverArgs) Kind() string { return "krovara.webhook_deliver" }
func (WebhookDeliverArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

type WebhookDeliverWorker struct {
	river.WorkerDefaults[WebhookDeliverArgs]
	Client *http.Client
}

func (w *WebhookDeliverWorker) Work(ctx context.Context, job *river.Job[WebhookDeliverArgs]) error {
	c := w.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	sig := SignPayload([]byte(job.Args.Secret), job.Args.Body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.Args.URL, bytes.NewReader(job.Args.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Krovara-Event", job.Args.Event)
	req.Header.Set("X-Krovara-Signature", "sha256="+sig)
	req.Header.Set("X-Krovara-Webhook-Id", job.Args.WebhookID)

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s: HTTP %d", job.Args.WebhookID, resp.StatusCode)
	}
	return nil
}

func SignPayload(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func randomSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func webhookDTO(h db.Webhook, fullSecret *string) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(h.ID.Bytes).String(),
		"space_id":   uuid.UUID(h.SpaceID.Bytes).String(),
		"channel_id": uuid.UUID(h.ChannelID.Bytes).String(),
		"name":       h.Name,
		"url":        h.Url,
		"events":     h.Events,
		"created_at": h.CreatedAt.Time,
	}
	if fullSecret != nil {
		out["secret"] = *fullSecret
	} else if h.Secret != nil && len(*h.Secret) >= 4 {
		out["secret_hint"] = "…" + (*h.Secret)[len(*h.Secret)-4:]
	}
	return out
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

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type rateLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	buckets map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, buckets: map[string][]time.Time{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	bucket := l.buckets[key]
	cutoff := now.Add(-l.window)

	keep := bucket[:0]
	for _, t := range bucket {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= l.max {
		l.buckets[key] = keep
		return false
	}
	l.buckets[key] = append(keep, now)
	return true
}
