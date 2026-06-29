package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const IndexName = "messages"

type Message struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	SpaceID   string `json:"space_id"`
	AuthorID  string `json:"author_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	HasLink   bool   `json:"has_link"`
	HasMedia  bool   `json:"has_media"`
}

type Service struct {
	q  db.Querier
	ms meilisearch.ServiceManager
}

func NewService(q db.Querier, host, apiKey string) *Service {
	return &Service{
		q:  q,
		ms: meilisearch.New(host, meilisearch.WithAPIKey(apiKey)),
	}
}

func (s *Service) EnsureIndex(ctx context.Context) error {
	_, _ = s.ms.CreateIndex(&meilisearch.IndexConfig{Uid: IndexName, PrimaryKey: "id"})
	idx := s.ms.Index(IndexName)
	filterable := []any{"channel_id", "space_id", "author_id", "created_at", "has_link", "has_media"}
	if _, err := idx.UpdateFilterableAttributesWithContext(ctx, &filterable); err != nil {
		return fmt.Errorf("filterable: %w", err)
	}
	sortable := []string{"created_at"}
	if _, err := idx.UpdateSortableAttributesWithContext(ctx, &sortable); err != nil {
		return fmt.Errorf("sortable: %w", err)
	}
	return nil
}

func (s *Service) Index(ctx context.Context, m Message) error {
	_, err := s.ms.Index(IndexName).AddDocumentsWithContext(ctx, []Message{m}, nil)
	return err
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, uidFn permissions.UserIDFunc) {
	r.With(permissions.RequireSpace(resolver, uidFn, permissions.ViewChannel)).
		Get("/spaces/{spaceID}/search", s.handleSearch(uidFn))
	r.Get("/search", s.handleGlobalSearch(uidFn))
}

func (s *Service) handleGlobalSearch(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawQ := strings.TrimSpace(r.URL.Query().Get("q"))
		if rawQ == "" {
			writeError(w, http.StatusBadRequest, "missing q")
			return
		}
		uid := uidFn(r.Context())
		spacesList, err := s.q.ListUserSpaces(r.Context(), pgtype.UUID{Bytes: uid, Valid: true})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "space lookup failed")
			return
		}
		if len(spacesList) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"hits": []any{}, "total": 0})
			return
		}
		ids := make([]string, 0, len(spacesList))
		for _, sp := range spacesList {
			ids = append(ids, uuid.UUID(sp.ID.Bytes).String())
		}

		pq := ParseQuery(rawQ)
		filter := []string{"space_id IN [" + joinQuoted(ids) + "]"}
		if a := firstNonEmpty(r.URL.Query().Get("author"), pq.From); a != "" {
			if _, err := uuid.Parse(a); err != nil {
				writeError(w, http.StatusBadRequest, "invalid author filter")
				return
			}
			filter = append(filter, "author_id = "+quote(a))
		}
		if pq.After > 0 {
			filter = append(filter, "created_at >= "+itoa(pq.After))
		}
		if pq.Before > 0 {
			filter = append(filter, "created_at < "+itoa(pq.Before))
		}
		if pq.HasLink {
			filter = append(filter, "has_link = true")
		}
		if pq.HasMedia {
			filter = append(filter, "has_media = true")
		}

		req := &meilisearch.SearchRequest{Query: pq.Text, Filter: filter, Limit: 50, Sort: []string{"created_at:desc"}}
		resp, err := s.ms.Index(IndexName).SearchWithContext(r.Context(), pq.Text, req)
		if err != nil {
			writeError(w, http.StatusBadGateway, "search backend error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hits":  resp.Hits,
			"total": resp.EstimatedTotalHits,
			"took":  resp.ProcessingTimeMs,
		})
	}
}

func (s *Service) handleSearch(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rawQ := strings.TrimSpace(r.URL.Query().Get("q"))
		if rawQ == "" {
			writeError(w, http.StatusBadRequest, "missing q")
			return
		}
		uid := uidFn(r.Context())

		pq := ParseQuery(rawQ)
		channelHint := firstNonEmpty(r.URL.Query().Get("channel"), pq.In)
		authorHint := firstNonEmpty(r.URL.Query().Get("author"), pq.From)

		filter, err := s.allowedFilter(r.Context(), uid, spaceID, channelHint, authorHint)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}

		if pq.After > 0 {
			filter = append(filter, "created_at >= "+itoa(pq.After))
		}
		if pq.Before > 0 {
			filter = append(filter, "created_at < "+itoa(pq.Before))
		}
		if pq.HasLink {
			filter = append(filter, "has_link = true")
		}
		if pq.HasMedia {
			filter = append(filter, "has_media = true")
		}

		text := pq.Text
		req := &meilisearch.SearchRequest{
			Query:  text,
			Filter: filter,
			Limit:  50,
			Sort:   []string{"created_at:desc"},
		}
		resp, err := s.ms.Index(IndexName).SearchWithContext(r.Context(), text, req)
		if err != nil {
			writeError(w, http.StatusBadGateway, "search backend error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hits":  resp.Hits,
			"total": resp.EstimatedTotalHits,
			"took":  resp.ProcessingTimeMs,
		})
	}
}

func (s *Service) allowedFilter(ctx context.Context, userID, spaceID uuid.UUID, channelHint, authorHint string) ([]string, error) {

	_ = userID
	rows, err := s.q.ListSpaceChannels(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
	if err != nil {
		return nil, err
	}
	channelIDs := make([]string, 0, len(rows))
	for _, c := range rows {
		channelIDs = append(channelIDs, uuid.UUID(c.ID.Bytes).String())
	}
	if len(channelIDs) == 0 {
		return nil, errors.New("no visible channels")
	}

	out := []string{
		"space_id = " + quote(spaceID.String()),
		"channel_id IN [" + joinQuoted(channelIDs) + "]",
	}
	if channelHint != "" {
		if _, err := uuid.Parse(channelHint); err != nil {
			return nil, errors.New("invalid channel filter")
		}

		if !contains(channelIDs, channelHint) {
			return nil, errors.New("channel not visible")
		}
		out = append(out, "channel_id = "+quote(channelHint))
	}
	if authorHint != "" {
		if _, err := uuid.Parse(authorHint); err != nil {
			return nil, errors.New("invalid author filter")
		}
		out = append(out, "author_id = "+quote(authorHint))
	}
	return out, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func quote(s string) string          { return `"` + s + `"` }
func joinQuoted(ids []string) string { return quote(strings.Join(ids, `","`)) }
func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var Now = func() time.Time { return time.Now() }
