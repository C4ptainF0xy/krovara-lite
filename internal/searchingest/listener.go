package searchingest

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type Enqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

func Listen(ctx context.Context, pool *pgxpool.Pool, client Enqueuer, log *slog.Logger) error {
	const channel = "search_ingest"
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		err := listenOnce(ctx, pool, client, channel, log)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warn("search ingest listener disconnected", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return ctx.Err()
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, client Enqueuer, channel string, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	log.Info("search ingest listener online", "channel", channel)

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		sortID, perr := strconv.ParseInt(n.Payload, 10, 64)
		if perr != nil {
			log.Warn("search ingest bad payload", "payload", n.Payload)
			continue
		}
		if _, err := client.Insert(ctx, SearchIndexArgs{SortID: sortID}, nil); err != nil {
			log.Warn("search ingest enqueue failed", "sort_id", sortID, "err", err)
		}
	}
}
