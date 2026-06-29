package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

const URLhausFeedURL = "https://urlhaus.abuse.ch/downloads/csv_recent/"

type LinkFeedArgs struct{}

func (LinkFeedArgs) Kind() string { return "krovara.link_feed" }

func (LinkFeedArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type LinkFeedWorker struct {
	river.WorkerDefaults[LinkFeedArgs]
	Pool    *pgxpool.Pool
	FeedURL string
	HTTP    *http.Client
}

func (w *LinkFeedWorker) Work(ctx context.Context, _ *river.Job[LinkFeedArgs]) error {
	feed := w.FeedURL
	if feed == "" {
		feed = URLhausFeedURL
	}
	hc := w.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed, nil)
	if err != nil {
		return err
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("urlhaus fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("urlhaus status %d", res.StatusCode)
	}

	r := csv.NewReader(res.Body)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	type row struct{ hash, threat string }
	var rows []row
	seen := make(map[string]struct{})
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(rec) < 3 {
			continue
		}
		u := strings.TrimSpace(rec[2])
		if u == "" {
			continue
		}
		sum := sha256.Sum256([]byte(u))
		h := hex.EncodeToString(sum[:])
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		threat := ""
		if len(rec) >= 6 {
			threat = rec[5]
		}
		rows = append(rows, row{h, threat})
	}
	if len(rows) == 0 {
		return fmt.Errorf("urlhaus feed produced no rows")
	}

	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE malicious_urls`); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{"malicious_urls"},
		[]string{"url_hash", "threat"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			return []any{rows[i].hash, rows[i].threat}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return tx.Commit(ctx)
}
