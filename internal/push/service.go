package push

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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

type Enqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type Service struct {
	q     *db.Queries
	v     *validator.Validate
	river Enqueuer
}

func NewService(pool *pgxpool.Pool, enq Enqueuer) *Service {
	return &Service{
		q:     db.New(pool),
		v:     validator.New(validator.WithRequiredStructEnabled()),
		river: enq,
	}
}

func (s *Service) Routes(r chi.Router, uidFn permissions.UserIDFunc) {
	r.Route("/me/devices", func(rr chi.Router) {
		rr.Get("/", s.handleListDevices(uidFn))
		rr.Post("/", s.handleCreateDevice(uidFn))
		rr.Delete("/{deviceID}", s.handleDeleteDevice(uidFn))
	})
	r.Get("/me/push-prefs", s.handleListPrefs(uidFn))
	r.Put("/spaces/{spaceID}/push-pref", s.handlePutPref(uidFn))
}

type deviceReq struct {
	Name string `json:"name" validate:"required,min=1,max=64"`
}

func (s *Service) handleCreateDevice(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req deviceReq
		if !s.decode(w, r, &req) {
			return
		}
		topic, err := randomTopic()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "topic gen failed")
			return
		}
		row, err := s.q.CreateDevice(r.Context(), db.CreateDeviceParams{
			UserID:    pgUUID(uid),
			Name:      req.Name,
			NtfyTopic: topic,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, deviceDTO(row))
	}
}

func (s *Service) handleListDevices(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		rows, err := s.q.ListUserDevices(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, d := range rows {
			out = append(out, deviceDTO(d))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleDeleteDevice(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "deviceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid device id")
			return
		}
		if err := s.q.DeleteDevice(r.Context(), db.DeleteDeviceParams{
			ID:     pgUUID(id),
			UserID: pgUUID(uidFn(r.Context())),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type prefReq struct {
	Scope string `json:"scope" validate:"required,oneof=all mentions none"`
}

func (s *Service) handlePutPref(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req prefReq
		if !s.decode(w, r, &req) {
			return
		}
		row, err := s.q.UpsertPushPref(r.Context(), db.UpsertPushPrefParams{
			UserID:  pgUUID(uidFn(r.Context())),
			SpaceID: pgUUID(spaceID),
			Scope:   req.Scope,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upsert failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"space_id": uuid.UUID(row.SpaceID.Bytes).String(),
			"scope":    row.Scope,
		})
	}
}

func (s *Service) handleListPrefs(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListUserPushPrefs(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, p := range rows {
			out = append(out, map[string]any{
				"space_id": uuid.UUID(p.SpaceID.Bytes).String(),
				"scope":    p.Scope,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) Emit(ctx context.Context, userID, spaceID uuid.UUID, isMention bool, title, body string) {
	if s.river == nil {
		return
	}
	pref, err := s.q.GetPushPref(ctx, db.GetPushPrefParams{
		UserID: pgUUID(userID), SpaceID: pgUUID(spaceID),
	})
	scope := "all"
	if err == nil {
		scope = pref.Scope
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return
	}
	switch scope {
	case "none":
		return
	case "mentions":
		if !isMention {
			return
		}
	}
	devices, err := s.q.ListUserDevices(ctx, pgUUID(userID))
	if err != nil {
		return
	}
	for _, d := range devices {
		_, _ = s.river.Insert(ctx, PushNotifyArgs{
			Topic: d.NtfyTopic,
			Title: title,
			Body:  body,
		}, nil)
	}
}

type PushNotifyArgs struct {
	Topic string `json:"topic"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (PushNotifyArgs) Kind() string { return "krovara.push_notify" }

func (PushNotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type PushNotifyWorker struct {
	river.WorkerDefaults[PushNotifyArgs]
	BaseURL string
	Client  *http.Client
}

func (w *PushNotifyWorker) Work(ctx context.Context, job *river.Job[PushNotifyArgs]) error {
	if w.BaseURL == "" {
		return errors.New("push: ntfy base URL not configured")
	}
	c := w.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	url := strings.TrimRight(w.BaseURL, "/") + "/" + job.Args.Topic
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(job.Args.Body)))
	if err != nil {
		return err
	}
	if job.Args.Title != "" {
		req.Header.Set("Title", job.Args.Title)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: HTTP %d", resp.StatusCode)
	}
	return nil
}

func randomTopic() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}

	return "krv-" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func deviceDTO(d db.Device) map[string]any {
	return map[string]any{
		"id":         uuid.UUID(d.ID.Bytes).String(),
		"name":       d.Name,
		"ntfy_topic": d.NtfyTopic,
		"created_at": d.CreatedAt.Time,
	}
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
