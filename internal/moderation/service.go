package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
	"github.com/krovara/krovara/internal/searchingest"
)

type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	mucHost string
}

func NewService(pool *pgxpool.Pool, mucHost string) *Service {
	return &Service{pool: pool, q: db.New(pool), mucHost: mucHost}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/channels/{channelID}/messages", func(rr chi.Router) {
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Delete("/{archiveID}", s.handleDelete(resolver, userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Post("/bulk-delete", s.handleBulkDelete(resolver, userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageMessages)).
			Post("/purge", s.handlePurge(userIDFn))
	})
}

func (s *Service) handleDelete(resolver permissions.Resolver, uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		archiveID := chi.URLParam(r, "archiveID")
		if archiveID == "" || len(archiveID) > 255 {
			writeError(w, http.StatusBadRequest, "invalid archive id")
			return
		}
		uid := uidFn(r.Context())

		var value string
		err = s.pool.QueryRow(r.Context(), `
SELECT value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = $3
`, s.mucHost, channelID.String(), archiveID).Scan(&value)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		isAuthor := false
		if parsed, perr := searchingest.ParseArchiveRow(channelID.String(), value); perr == nil {
			isAuthor = parsed.AuthorID == uid.String()
		}
		if !isAuthor {
			mc, perr := resolver.ResolveChannel(r.Context(), uid, channelID)
			if perr != nil || !permissions.Compute(mc).Has(permissions.ManageMessages) {
				writeError(w, http.StatusForbidden, "not allowed")
				return
			}
		}

		tag, err := s.pool.Exec(r.Context(), `
DELETE FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = $3
`, s.mucHost, channelID.String(), archiveID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}

		ch, err := s.q.GetChannel(r.Context(), pgUUID(channelID))
		if err == nil {
			meta, _ := json.Marshal(map[string]any{
				"archive_id": archiveID, "channel_id": channelID.String(), "self": isAuthor,
			})
			s.logAudit(r.Context(), ch.SpaceID, uid, "message.delete", meta)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

const (
	maxBulk       = 100
	maxPurge      = 500
	maxPurgeHours = 24 * 30
)

type bulkDeleteReq struct {
	ArchiveIDs []string `json:"archive_ids"`
}

func (s *Service) handleBulkDelete(resolver permissions.Resolver, uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		var req bulkDeleteReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ids := dedupNonEmpty(req.ArchiveIDs)
		if len(ids) == 0 {
			writeError(w, http.StatusBadRequest, "no messages")
			return
		}
		if len(ids) > maxBulk {
			writeError(w, http.StatusBadRequest, "too many messages")
			return
		}
		for _, id := range ids {
			if len(id) > 255 {
				writeError(w, http.StatusBadRequest, "invalid archive id")
				return
			}
		}
		uid := uidFn(r.Context())

		hasManage := false
		if mc, perr := resolver.ResolveChannel(r.Context(), uid, channelID); perr == nil {
			hasManage = permissions.Compute(mc).Has(permissions.ManageMessages)
		}

		rows, err := s.pool.Query(r.Context(), `
SELECT "key", value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = ANY($3)
`, s.mucHost, channelID.String(), ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		type arow struct{ key, value string }
		var found []arow
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				continue
			}
			found = append(found, arow{k, v})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if !hasManage {
			for _, rw := range found {
				parsed, perr := searchingest.ParseArchiveRow(channelID.String(), rw.value)
				if perr != nil || parsed.AuthorID != uid.String() {
					writeError(w, http.StatusForbidden, "not allowed")
					return
				}
			}
		}

		keys := make([]string, 0, len(found))
		for _, rw := range found {
			keys = append(keys, rw.key)
		}
		var deleted int64
		if len(keys) > 0 {
			tag, err := s.pool.Exec(r.Context(), `
DELETE FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = ANY($3)
`, s.mucHost, channelID.String(), keys)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "delete failed")
				return
			}
			deleted = tag.RowsAffected()
		}

		if ch, err := s.q.GetChannel(r.Context(), pgUUID(channelID)); err == nil {
			meta, _ := json.Marshal(map[string]any{
				"channel_id": channelID.String(), "count": deleted,
				"self": !hasManage, "archive_ids": keys,
			})
			s.logAudit(r.Context(), ch.SpaceID, uid, "message.bulk_delete", meta)
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
	}
}

type purgeReq struct {
	AuthorID    string `json:"author_id"`
	WithinHours int    `json:"within_hours"`
}

func (s *Service) handlePurge(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		var req purgeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		author, err := uuid.Parse(req.AuthorID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid author id")
			return
		}
		if req.WithinHours < 0 || req.WithinHours > maxPurgeHours {
			writeError(w, http.StatusBadRequest, "invalid window")
			return
		}
		threshold := int64(0)
		if req.WithinHours > 0 {
			threshold = time.Now().Unix() - int64(req.WithinHours)*3600
		}

		rows, err := s.pool.Query(r.Context(), `
SELECT "key", value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2
   AND value LIKE '%' || $3 || '%'
   AND ($4 = 0 OR "when" >= $4)
 ORDER BY sort_id DESC
 LIMIT $5
`, s.mucHost, channelID.String(), author.String(), threshold, maxPurge)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var keys []string
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				continue
			}

			if parsed, perr := searchingest.ParseArchiveRow(channelID.String(), v); perr == nil && parsed.AuthorID == author.String() {
				keys = append(keys, k)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		var deleted int64
		if len(keys) > 0 {
			tag, err := s.pool.Exec(r.Context(), `
DELETE FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = ANY($3)
`, s.mucHost, channelID.String(), keys)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "delete failed")
				return
			}
			deleted = tag.RowsAffected()
		}

		uid := uidFn(r.Context())
		if ch, err := s.q.GetChannel(r.Context(), pgUUID(channelID)); err == nil {
			meta, _ := json.Marshal(map[string]any{
				"channel_id": channelID.String(), "author_id": author.String(),
				"count": deleted, "within_hours": req.WithinHours, "archive_ids": keys,
			})
			s.logAudit(r.Context(), ch.SpaceID, uid, "message.purge_author", meta)
		}

		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "archive_ids": keysOrEmpty(keys)})
	}
}

func keysOrEmpty(keys []string) []string {
	if keys == nil {
		return []string{}
	}
	return keys
}

func dedupNonEmpty(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actorID uuid.UUID, action string, metadata []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID:  spaceID,
		ActorID:  pgUUID(actorID),
		Action:   action,
		Metadata: metadata,
	})
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
