package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/envfile"
	"github.com/krovara/krovara/internal/search"
	"github.com/krovara/krovara/internal/searchingest"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := envfile.LoadFirst(".env.dev", ".env"); err != nil {
		log.Warn("envfile", "err", err)
	}

	fromSortID := flag.Int64("from-sort-id", 0, "skip rows with sort_id <= this value")
	batchSize := flag.Int("batch", 500, "rows per fetch")
	flag.Parse()

	dsn := envOr("KROVARA_DB_DSN", "")
	host := os.Getenv("KROVARA_MEILI_HOST")
	key := os.Getenv("MEILI_MASTER_KEY")
	if dsn == "" || host == "" || key == "" {
		log.Error("missing KROVARA_DB_DSN / KROVARA_MEILI_HOST / MEILI_MASTER_KEY")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Error("pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	indexer := search.NewService(db.New(pool), host, key)
	if err := indexer.EnsureIndex(ctx); err != nil {
		log.Warn("ensure index", "err", err)
	}

	worker := &searchingest.SearchIndexWorker{Pool: pool, Indexer: indexer}

	cursor := *fromSortID
	indexed, skipped := 0, 0
	for ctx.Err() == nil {
		rows, err := pool.Query(ctx, `
SELECT sort_id FROM prosodyarchive
WHERE sort_id > $1
  AND host = 'conference.krovara.local'
  AND store = 'muc_log'
  AND type = 'xml'
ORDER BY sort_id ASC
LIMIT $2
`, cursor, *batchSize)
		if err != nil {
			log.Error("query", "err", err)
			os.Exit(1)
		}
		ids, err := scanInt64s(rows)
		if err != nil {
			log.Error("scan", "err", err)
			os.Exit(1)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if err := worker.IndexBySortID(ctx, id); err != nil {
				log.Warn("skip", "sort_id", id, "err", err)
				skipped++
				continue
			}
			indexed++
			cursor = id
		}
		log.Info("progress", "indexed", indexed, "skipped", skipped, "cursor", cursor)
	}
	log.Info("done", "indexed", indexed, "skipped", skipped)
}

func scanInt64s(rows interface {
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}) ([]int64, error) {
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
