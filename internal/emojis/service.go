package emojis

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type kindCfg struct {
	kind  string
	max   int
	re    *regexp.Regexp
	lower bool
}

var (
	emojiCfg = kindCfg{
		kind:  "emoji",
		max:   100,
		re:    regexp.MustCompile(`^[a-z0-9_]{2,32}$`),
		lower: true,
	}

	stickerCfg = kindCfg{
		kind:  "sticker",
		max:   50,
		re:    regexp.MustCompile(`^[\p{L}\p{N} _-]{1,40}$`),
		lower: false,
	}
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	s.mount(r, resolver, userIDFn, "emojis", emojiCfg)
	s.mount(r, resolver, userIDFn, "stickers", stickerCfg)

	r.Get("/me/emojis", s.handleListMine(userIDFn, emojiCfg))
	r.Get("/me/stickers", s.handleListMine(userIDFn, stickerCfg))
}

func (s *Service) mount(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc, path string, cfg kindCfg) {
	r.Route("/spaces/{spaceID}/"+path, func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList(cfg))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Post("/", s.handleCreate(cfg, userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Delete("/{assetID}", s.handleDelete(cfg))
	})
}

func (s *Service) handleList(cfg kindCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceEmojis(r.Context(), db.ListSpaceEmojisParams{
			SpaceID: pgUUID(spaceID),
			Kind:    cfg.kind,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, e := range rows {
			out = append(out, emojiDTO(e))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleListMine(uidFn permissions.UserIDFunc, cfg kindCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		rows, err := s.q.ListUserEmojis(r.Context(), db.ListUserEmojisParams{
			UserID: pgUUID(uid),
			Kind:   cfg.kind,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, e := range rows {
			out = append(out, emojiDTO(e))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type createReq struct {
	Name     string `json:"name"`
	FileKey  string `json:"file_key"`
	Animated bool   `json:"animated"`
}

func (s *Service) handleCreate(cfg kindCfg, uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		name := strings.TrimSpace(req.Name)
		if cfg.lower {
			name = strings.ToLower(name)
		}
		if !cfg.re.MatchString(name) {
			writeError(w, http.StatusBadRequest, "invalid name")
			return
		}
		if req.FileKey == "" {
			writeError(w, http.StatusBadRequest, "missing file_key")
			return
		}

		if _, err := uuid.Parse(req.FileKey); err != nil {
			writeError(w, http.StatusBadRequest, "invalid file_key")
			return
		}

		n, err := s.q.CountSpaceEmojis(r.Context(), db.CountSpaceEmojisParams{
			SpaceID: pgUUID(spaceID),
			Kind:    cfg.kind,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "count failed")
			return
		}
		if n >= int64(cfg.max) {
			writeError(w, http.StatusConflict, "limit reached")
			return
		}

		emoji, err := s.q.CreateCustomEmoji(r.Context(), db.CreateCustomEmojiParams{
			SpaceID:   pgUUID(spaceID),
			Name:      name,
			FileKey:   req.FileKey,
			Animated:  req.Animated,
			CreatedBy: pgUUID(uidFn(r.Context())),
			Kind:      cfg.kind,
		})
		if err != nil {

			writeError(w, http.StatusConflict, "a name like this already exists")
			return
		}
		writeJSON(w, http.StatusCreated, emojiDTO(emoji))
	}
}

func (s *Service) handleDelete(cfg kindCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		assetID, err := uuid.Parse(chi.URLParam(r, "assetID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		e, err := s.q.GetCustomEmoji(r.Context(), pgUUID(assetID))
		if errors.Is(err, pgx.ErrNoRows) ||
			(err == nil && (uuid.UUID(e.SpaceID.Bytes) != spaceID || e.Kind != cfg.kind)) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if err := s.q.DeleteCustomEmoji(r.Context(), pgUUID(assetID)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func emojiDTO(e db.CustomEmoji) map[string]any {
	return map[string]any{
		"id":       uuid.UUID(e.ID.Bytes).String(),
		"space_id": uuid.UUID(e.SpaceID.Bytes).String(),
		"name":     e.Name,
		"file_key": e.FileKey,
		"animated": e.Animated,
		"kind":     e.Kind,
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
