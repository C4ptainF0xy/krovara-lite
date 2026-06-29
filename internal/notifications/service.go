package notifications

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Get("/me/inbox", s.handleInbox(userIDFn))
	r.Put("/me/inbox/read", s.handleReadAll(userIDFn))
	r.Put("/me/inbox/{id}/read", s.handleReadOne(userIDFn))
	r.Get("/me/notif-settings", s.handleListSettings(userIDFn))
	r.Put("/me/notif-settings", s.handleUpsertSetting(userIDFn))
}

func (s *Service) handleInbox(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := pgUUID(uidFn(r.Context()))
		rows, err := s.q.ListInbox(r.Context(), db.ListInboxParams{UserID: self, Limit: 100})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		unread, _ := s.q.CountInboxUnread(r.Context(), self)
		items := make([]map[string]any, 0, len(rows))
		for _, it := range rows {
			m := map[string]any{
				"id":         uuid.UUID(it.ID.Bytes).String(),
				"kind":       it.Kind,
				"archive_id": it.ArchiveID,
				"preview":    it.Preview,
				"read":       it.Read,
				"created_at": it.CreatedAt.Time,
			}
			if it.SpaceID.Valid {
				m["space_id"] = uuid.UUID(it.SpaceID.Bytes).String()
			}
			if it.ChannelID.Valid {
				m["channel_id"] = uuid.UUID(it.ChannelID.Bytes).String()
			}
			if it.AuthorID.Valid {
				m["author_id"] = uuid.UUID(it.AuthorID.Bytes).String()
			}
			items = append(items, m)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
	}
}

func (s *Service) handleReadAll(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.q.MarkAllInboxRead(r.Context(), pgUUID(uidFn(r.Context()))); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleReadOne(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		if err := s.q.MarkInboxItemRead(r.Context(), db.MarkInboxItemReadParams{
			ID: pgUUID(id), UserID: pgUUID(uidFn(r.Context())),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleListSettings(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListNotifSettings(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, st := range rows {
			m := map[string]any{
				"scope_type":        st.ScopeType,
				"scope_id":          uuid.UUID(st.ScopeID.Bytes).String(),
				"level":             st.Level,
				"suppress_everyone": st.SuppressEveryone,
			}
			if st.MutedUntil.Valid {
				m["muted_until"] = st.MutedUntil.Time
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type settingReq struct {
	ScopeType        string `json:"scope_type"`
	ScopeID          string `json:"scope_id"`
	Level            string `json:"level"`
	MuteMinutes      int    `json:"mute_minutes"`
	SuppressEveryone bool   `json:"suppress_everyone"`
}

func (s *Service) handleUpsertSetting(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req settingReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.ScopeType != "space" && req.ScopeType != "channel" {
			writeError(w, http.StatusBadRequest, "invalid scope_type")
			return
		}
		switch req.Level {
		case "all", "mentions", "nothing":
		default:
			writeError(w, http.StatusBadRequest, "invalid level")
			return
		}
		scopeID, err := uuid.Parse(req.ScopeID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid scope_id")
			return
		}
		var muted pgtype.Timestamptz
		if req.MuteMinutes > 0 {
			muted = pgtype.Timestamptz{Time: time.Now().Add(time.Duration(req.MuteMinutes) * time.Minute), Valid: true}
		}
		st, err := s.q.UpsertNotifSetting(r.Context(), db.UpsertNotifSettingParams{
			UserID:           pgUUID(uidFn(r.Context())),
			ScopeType:        req.ScopeType,
			ScopeID:          pgUUID(scopeID),
			Level:            req.Level,
			MutedUntil:       muted,
			SuppressEveryone: req.SuppressEveryone,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "save failed")
			return
		}
		out := map[string]any{
			"scope_type": st.ScopeType, "scope_id": uuid.UUID(st.ScopeID.Bytes).String(),
			"level": st.Level, "suppress_everyone": st.SuppressEveryone,
		}
		if st.MutedUntil.Valid {
			out["muted_until"] = st.MutedUntil.Time
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
