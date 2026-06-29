package messages

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
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
	v       *validator.Validate
	mucHost string
}

func NewService(pool *pgxpool.Pool, mucHost string) *Service {
	return &Service{
		pool:    pool,
		q:       db.New(pool),
		v:       validator.New(validator.WithRequiredStructEnabled()),
		mucHost: mucHost,
	}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/channels/{channelID}/pins", func(rr chi.Router) {
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleListPins())
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageMessages)).
			Post("/", s.handleCreatePin(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageMessages)).
			Delete("/{archiveID}", s.handleDeletePin())
	})
	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
		Put("/channels/{channelID}/read-state", s.handleSetReadState(userIDFn))
	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
		Get("/channels/{channelID}/messages/{archiveID}/history", s.handleEditHistory())

	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
		Get("/channels/{channelID}/messages/{archiveID}/follow", s.handleIsFollowing(userIDFn))
	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
		Put("/channels/{channelID}/messages/{archiveID}/follow", s.handleFollow(userIDFn))
	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
		Delete("/channels/{channelID}/messages/{archiveID}/follow", s.handleUnfollow(userIDFn))
	r.Route("/me/saves", func(rr chi.Router) {
		rr.Get("/", s.handleListSaves(userIDFn))
		rr.Post("/", s.handleCreateSave(resolver, userIDFn))
		rr.Delete("/{archiveID}", s.handleDeleteSave(userIDFn))
	})
	r.Get("/me/read-state", s.handleReadStateSummary(userIDFn))
}

func (s *Service) followParams(r *http.Request, uidFn permissions.UserIDFunc) (db.FollowMessageParams, bool) {
	channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		return db.FollowMessageParams{}, false
	}
	archiveID := chi.URLParam(r, "archiveID")
	if archiveID == "" || len(archiveID) > 255 {
		return db.FollowMessageParams{}, false
	}
	return db.FollowMessageParams{
		UserID:    pgUUID(uidFn(r.Context())),
		ChannelID: pgUUID(channelID),
		ArchiveID: archiveID,
	}, true
}

func (s *Service) handleIsFollowing(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.followParams(r, uidFn)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid params")
			return
		}
		following, err := s.q.IsFollowingMessage(r.Context(), db.IsFollowingMessageParams(p))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"following": following})
	}
}

func (s *Service) handleFollow(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.followParams(r, uidFn)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid params")
			return
		}
		if err := s.q.FollowMessage(r.Context(), p); err != nil {
			writeError(w, http.StatusInternalServerError, "follow failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"following": true})
	}
}

func (s *Service) handleUnfollow(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.followParams(r, uidFn)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid params")
			return
		}
		if err := s.q.UnfollowMessage(r.Context(), db.UnfollowMessageParams(p)); err != nil {
			writeError(w, http.StatusInternalServerError, "unfollow failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type createPinReq struct {
	ArchiveID string `json:"archive_id" validate:"required,max=255"`
	Note      string `json:"note"       validate:"max=500"`
}

func (s *Service) handleCreatePin(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		var req createPinReq
		if !s.decode(w, r, &req) {
			return
		}

		author, body, when, found := s.fetchArchive(r.Context(), channelID.String(), req.ArchiveID)
		if !found {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		pin, err := s.q.CreatePin(r.Context(), db.CreatePinParams{
			ChannelID: pgUUID(channelID),
			ArchiveID: req.ArchiveID,
			PinnedBy:  pgUUID(uidFn(r.Context())),
			Note:      req.Note,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "pin failed")
			return
		}
		writeJSON(w, http.StatusCreated, pinDTO(pin, author, body, when, true))
	}
}

func (s *Service) handleListPins() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		pins, err := s.q.ListChannelPins(r.Context(), pgUUID(channelID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(pins))
		for _, p := range pins {
			author, body, when, found := s.fetchArchive(r.Context(), channelID.String(), p.ArchiveID)
			out = append(out, pinDTO(p, author, body, when, found))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleDeletePin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		archiveID := chi.URLParam(r, "archiveID")
		if archiveID == "" || len(archiveID) > 255 {
			writeError(w, http.StatusBadRequest, "invalid archive id")
			return
		}
		n, err := s.q.DeletePin(r.Context(), db.DeletePinParams{
			ChannelID: pgUUID(channelID),
			ArchiveID: archiveID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unpin failed")
			return
		}
		if n == 0 {
			writeError(w, http.StatusNotFound, "pin not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func pinDTO(p db.MessagePin, authorID, body string, when int64, found bool) map[string]any {
	out := map[string]any{
		"archive_id": p.ArchiveID,
		"channel_id": uuid.UUID(p.ChannelID.Bytes).String(),
		"pinned_by":  uuid.UUID(p.PinnedBy.Bytes).String(),
		"pinned_at":  p.CreatedAt.Time,
		"note":       p.Note,
		"missing":    !found,
		"author_id":  authorID,
		"body":       body,
	}
	if when > 0 {
		out["at"] = time.Unix(when, 0).UTC()
	}
	return out
}

type createSaveReq struct {
	ChannelID string `json:"channel_id" validate:"required,uuid"`
	ArchiveID string `json:"archive_id" validate:"required,max=255"`
	Folder    string `json:"folder"     validate:"max=64"`
}

func (s *Service) handleCreateSave(resolver permissions.Resolver, uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSaveReq
		if !s.decode(w, r, &req) {
			return
		}
		channelID, err := uuid.Parse(req.ChannelID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		uid := uidFn(r.Context())

		mc, err := resolver.ResolveChannel(r.Context(), uid, channelID)
		if errors.Is(err, permissions.ErrNotMember) {
			writeError(w, http.StatusForbidden, "not a member")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "resolve failed")
			return
		}
		if !permissions.Compute(mc).Has(permissions.ViewChannel) {
			writeError(w, http.StatusForbidden, "not allowed")
			return
		}
		author, body, when, found := s.fetchArchive(r.Context(), channelID.String(), req.ArchiveID)
		if !found {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		save, err := s.q.CreateSavedMessage(r.Context(), db.CreateSavedMessageParams{
			UserID:    pgUUID(uid),
			ChannelID: pgUUID(channelID),
			ArchiveID: req.ArchiveID,
			Folder:    req.Folder,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "save failed")
			return
		}
		writeJSON(w, http.StatusCreated, saveDTO(save, s.spaceOf(r.Context(), channelID), author, body, when, true))
	}
}

func (s *Service) handleListSaves(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		saves, err := s.q.ListSavedMessages(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(saves))
		for _, sv := range saves {
			channelID := uuid.UUID(sv.ChannelID.Bytes)
			author, body, when, found := s.fetchArchive(r.Context(), channelID.String(), sv.ArchiveID)
			out = append(out, saveDTO(sv, s.spaceOf(r.Context(), channelID), author, body, when, found))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleDeleteSave(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		archiveID := chi.URLParam(r, "archiveID")
		if archiveID == "" || len(archiveID) > 255 {
			writeError(w, http.StatusBadRequest, "invalid archive id")
			return
		}
		n, err := s.q.DeleteSavedMessage(r.Context(), db.DeleteSavedMessageParams{
			UserID:    pgUUID(uidFn(r.Context())),
			ArchiveID: archiveID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unsave failed")
			return
		}
		if n == 0 {
			writeError(w, http.StatusNotFound, "save not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func saveDTO(sv db.SavedMessage, spaceID, authorID, body string, when int64, found bool) map[string]any {
	out := map[string]any{
		"archive_id": sv.ArchiveID,
		"channel_id": uuid.UUID(sv.ChannelID.Bytes).String(),
		"space_id":   spaceID,
		"folder":     sv.Folder,
		"saved_at":   sv.CreatedAt.Time,
		"missing":    !found,
		"author_id":  authorID,
		"body":       body,
	}
	if when > 0 {
		out["at"] = time.Unix(when, 0).UTC()
	}
	return out
}

func (s *Service) spaceOf(ctx context.Context, channelID uuid.UUID) string {
	ch, err := s.q.GetChannel(ctx, pgUUID(channelID))
	if err != nil {
		return ""
	}
	return uuid.UUID(ch.SpaceID.Bytes).String()
}

type setReadStateReq struct {
	ArchiveID string `json:"archive_id" validate:"required,max=255"`
	Mode      string `json:"mode"       validate:"omitempty,oneof=read unread"`
}

func (s *Service) handleSetReadState(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		var req setReadStateReq
		if !s.decode(w, r, &req) {
			return
		}

		sortID, found := s.archiveSortID(r.Context(), channelID.String(), req.ArchiveID)
		if !found {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		lastSort, lastArchive := sortID, req.ArchiveID
		if req.Mode == "unread" {

			if pSort, pKey, hasPrev := s.archivePrev(r.Context(), channelID.String(), sortID); hasPrev {
				lastSort, lastArchive = pSort, pKey
			} else {
				lastSort, lastArchive = 0, ""
			}
		}
		rs, err := s.q.UpsertReadState(r.Context(), db.UpsertReadStateParams{
			UserID:            pgUUID(uidFn(r.Context())),
			ChannelID:         pgUUID(channelID),
			LastReadSortID:    lastSort,
			LastReadArchiveID: lastArchive,

			AdvanceOnly: req.Mode != "unread",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read-state failed")
			return
		}

		writeJSON(w, http.StatusOK, readStateDTO(rs, s.unreadCount(r.Context(), channelID.String(), rs.LastReadSortID)))
	}
}

func (s *Service) handleReadStateSummary(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListReadState(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, rs := range rows {
			channelID := uuid.UUID(rs.ChannelID.Bytes).String()
			out = append(out, readStateDTO(rs, s.unreadCount(r.Context(), channelID, rs.LastReadSortID)))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func readStateDTO(rs db.ChannelReadState, unread int64) map[string]any {
	return map[string]any{
		"channel_id":           uuid.UUID(rs.ChannelID.Bytes).String(),
		"last_read_archive_id": rs.LastReadArchiveID,
		"last_read_sort_id":    rs.LastReadSortID,
		"unread_count":         unread,
		"updated_at":           rs.UpdatedAt.Time,
	}
}

func (s *Service) archiveSortID(ctx context.Context, channelID, archiveID string) (int64, bool) {
	var sortID int64
	err := s.pool.QueryRow(ctx, `
SELECT sort_id FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = $3
`, s.mucHost, channelID, archiveID).Scan(&sortID)
	if err != nil {
		return 0, false
	}
	return sortID, true
}

const bodyPredicate = `(value LIKE '%<body>%' OR value LIKE '%<body %')`

func (s *Service) archivePrev(ctx context.Context, channelID string, sortID int64) (int64, string, bool) {
	var pSort int64
	var key string
	err := s.pool.QueryRow(ctx, `
SELECT sort_id, "key" FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND sort_id < $3 AND `+bodyPredicate+`
 ORDER BY sort_id DESC
 LIMIT 1
`, s.mucHost, channelID, sortID).Scan(&pSort, &key)
	if err != nil {
		return 0, "", false
	}
	return pSort, key, true
}

func (s *Service) unreadCount(ctx context.Context, channelID string, lastSort int64) int64 {
	var n int64
	err := s.pool.QueryRow(ctx, `
SELECT count(*) FROM (
    SELECT 1 FROM prosodyarchive
     WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND sort_id > $3 AND `+bodyPredicate+`
     LIMIT 100
) t
`, s.mucHost, channelID, lastSort).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func (s *Service) handleEditHistory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := uuidParam(w, r, "channelID")
		if !ok {
			return
		}
		archiveID := chi.URLParam(r, "archiveID")
		if archiveID == "" || len(archiveID) > 255 {
			writeError(w, http.StatusBadRequest, "invalid archive id")
			return
		}
		revs, found, err := s.editRevisions(r.Context(), channelID.String(), archiveID)
		if err != nil {

			writeError(w, http.StatusInternalServerError, "history failed")
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}
		out := make([]map[string]any, 0, len(revs))
		for _, rv := range revs {
			m := map[string]any{"body": rv.body, "original": rv.original}
			if rv.at > 0 {
				m["at"] = time.Unix(rv.at, 0).UTC()
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type revision struct {
	body     string
	at       int64
	original bool
}

const maxRevisions = 100

func (s *Service) editRevisions(ctx context.Context, channelID, archiveID string) ([]revision, bool, error) {
	var origValue string
	var origWhen int64
	err := s.pool.QueryRow(ctx, `
SELECT "when", value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = $3
`, s.mucHost, channelID, archiveID).Scan(&origWhen, &origValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	originID := ""
	origBody := ""
	origAuthor := ""
	if parsed, perr := searchingest.ParseArchiveRow(channelID, origValue); perr == nil {
		origBody, originID, origAuthor = parsed.Body, parsed.OriginID, parsed.AuthorID
	}
	revs := []revision{{body: origBody, at: origWhen, original: true}}

	if origAuthor == "" {
		return revs, true, nil
	}

	anchors := map[string]bool{archiveID: true}
	if originID != "" {
		anchors[originID] = true
	}

	a1 := archiveID
	if originID != "" {
		a1 = originID
	}
	rows, err := s.pool.Query(ctx, `
SELECT "when", value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2
   AND value LIKE '%message-correct%'
   AND (value LIKE '%' || $3 || '%' OR value LIKE '%' || $4 || '%')
 ORDER BY sort_id ASC
 LIMIT $5
`, s.mucHost, channelID, a1, archiveID, maxRevisions)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var when int64
		var value string
		if err := rows.Scan(&when, &value); err != nil {
			continue
		}
		parsed, perr := searchingest.ParseArchiveRow(channelID, value)

		if perr != nil || !anchors[parsed.ReplaceID] || parsed.AuthorID != origAuthor {
			continue
		}
		revs = append(revs, revision{body: parsed.Body, at: when})
		if len(revs) >= maxRevisions {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return revs, true, nil
}

func (s *Service) fetchArchive(ctx context.Context, channelID, archiveID string) (authorID, body string, when int64, found bool) {
	var value string
	var w int64
	err := s.pool.QueryRow(ctx, `
SELECT "when", value FROM prosodyarchive
 WHERE host = $1 AND store = 'muc_log' AND "user" = $2 AND "key" = $3
`, s.mucHost, channelID, archiveID).Scan(&w, &value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, false
	}
	if err != nil {
		return "", "", 0, false
	}
	parsed, perr := searchingest.ParseArchiveRow(channelID, value)
	if perr != nil {
		return "", "", w, true
	}
	return parsed.AuthorID, parsed.Body, w, true
}

func uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
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

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
