package gif

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const klipyBase = "https://api.klipy.com/api/v1"

const perPage = 24

var denylist = []string{
	"porn", "pornhub", "nsfw", "xxx", "hentai", "nude", "naked", "boob",
	"tits", "pussy", "dick", "cock", "penis", "vagina", "cum", "sex",
	"sexy", "fuck", "blowjob", "anal", "creampie", "onlyfans", "escort",
}

type Service struct {
	apiKey string
	base   string
	http   *http.Client
}

func NewService(apiKey string) *Service {
	return &Service{
		apiKey: apiKey,
		base:   klipyBase,
		http:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/gif/search", s.handleSearch)
}

type item struct {
	ID      int64  `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Preview string `json:"preview"`
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing q")
		return
	}
	if denied(q) {

		writeJSON(w, http.StatusOK, map[string]any{"items": []item{}, "has_next": false})
		return
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}

	resp, err := s.search(r.Context(), q, page)
	if err != nil {
		writeError(w, http.StatusBadGateway, "gif backend error")
		return
	}

	items := make([]item, 0, len(resp.Data.Data))
	for _, k := range resp.Data.Data {
		if denied(k.Slug) || denied(k.Title) {
			continue
		}
		md := k.File["md"]
		sm := k.File["sm"]
		send := firstURL(md.Gif, k.File["hd"].Gif)
		preview := firstURL(sm.Webp, sm.Gif, md.Gif)
		if send.URL == "" || preview.URL == "" {
			continue
		}
		items = append(items, item{
			ID:      k.ID,
			Slug:    k.Slug,
			Title:   k.Title,
			Preview: preview.URL,
			URL:     send.URL,
			Width:   send.Width,
			Height:  send.Height,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "has_next": resp.Data.HasNext})
}

func (s *Service) search(ctx context.Context, q string, page int) (*klipyResp, error) {
	u := fmt.Sprintf("%s/%s/gifs/search?q=%s&per_page=%d&page=%d&rating=g",
		s.base, url.PathEscape(s.apiKey), url.QueryEscape(q), perPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("klipy status %d", res.StatusCode)
	}
	var out klipyResp
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func denied(s string) bool {
	l := strings.ToLower(s)
	for _, term := range denylist {
		if strings.Contains(l, term) {
			return true
		}
	}
	return false
}

func firstURL(media ...klipyMedia) klipyMedia {
	for _, m := range media {
		if m.URL != "" {
			return m
		}
	}
	return klipyMedia{}
}

type klipyResp struct {
	Result bool `json:"result"`
	Data   struct {
		Data    []klipyItem `json:"data"`
		HasNext bool        `json:"has_next"`
	} `json:"data"`
}

type klipyItem struct {
	ID    int64                `json:"id"`
	Slug  string               `json:"slug"`
	Title string               `json:"title"`
	File  map[string]klipyFile `json:"file"`
}

type klipyFile struct {
	Gif  klipyMedia `json:"gif"`
	Webp klipyMedia `json:"webp"`
}

type klipyMedia struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
