package messagehooks

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/searchingest"
	"github.com/krovara/krovara/internal/webhooks"
)

type Emitter interface {
	Emit(ctx context.Context, spaceID uuid.UUID, event string, payload any)
}

func Listen(ctx context.Context, pool *pgxpool.Pool, emitter Emitter, log *slog.Logger) error {
	const channel = "search_ingest"
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		err := listenOnce(ctx, pool, emitter, channel, log)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warn("message hooks listener disconnected", "err", err, "retry_in", backoff)
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

func listenOnce(ctx context.Context, pool *pgxpool.Pool, emitter Emitter, channel string, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	log.Info("message hooks listener online", "channel", channel)
	q := db.New(pool)

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		sortID, perr := strconv.ParseInt(n.Payload, 10, 64)
		if perr != nil {
			continue
		}
		if err := dispatch(ctx, pool, q, emitter, sortID); err != nil {
			log.Warn("message hook dispatch failed", "sort_id", sortID, "err", err)
		}
	}
}

func dispatch(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, emitter Emitter, sortID int64) error {
	var user, value string
	var when int64
	err := pool.QueryRow(ctx, `
SELECT "user", value, "when" FROM prosodyarchive
WHERE sort_id = $1 AND host = 'conference.krovara.local' AND store = 'muc_log' AND type = 'xml'
`, sortID).Scan(&user, &value, &when)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	parsed, err := searchingest.ParseArchiveRow(user, value)
	if err != nil {
		return nil
	}
	channelUUID, err := uuid.Parse(parsed.ChannelID)
	if err != nil {
		return nil
	}
	channel, err := q.GetChannel(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {
		return nil
	}

	payload := map[string]any{
		"id":         fmt.Sprintf("p%d", sortID),
		"channel_id": parsed.ChannelID,
		"space_id":   uuid.UUID(channel.SpaceID.Bytes).String(),
		"author_id":  parsed.AuthorID,
		"content":    parsed.Body,
		"created_at": when,
	}
	emitter.Emit(ctx, uuid.UUID(channel.SpaceID.Bytes), webhooks.EventMessageCreate, payload)
	return nil
}
