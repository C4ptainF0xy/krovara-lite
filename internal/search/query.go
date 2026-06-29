package search

import (
	"strings"
	"time"
)

type ParsedQuery struct {
	Text     string
	From     string
	In       string
	Before   int64
	After    int64
	HasLink  bool
	HasMedia bool
}

func ParseQuery(raw string) ParsedQuery {
	var pq ParsedQuery
	var terms []string
	for _, tok := range strings.Fields(raw) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok || val == "" {
			terms = append(terms, tok)
			continue
		}
		switch strings.ToLower(key) {
		case "from":
			pq.From = val
		case "in":
			pq.In = val
		case "before":
			if t, ok := parseDate(val); ok {
				pq.Before = t.Unix()
			}
		case "after":
			if t, ok := parseDate(val); ok {
				pq.After = t.Unix()
			}
		case "has":
			switch strings.ToLower(val) {
			case "link", "url":
				pq.HasLink = true
			case "file", "image", "media", "attachment", "img":
				pq.HasMedia = true
			default:
				terms = append(terms, tok)
			}
		default:
			terms = append(terms, tok)
		}
	}
	pq.Text = strings.Join(terms, " ")
	return pq
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
