package searchingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/search"
)

type SearchIndexArgs struct {
	SortID int64 `json:"sort_id"`
}

func (SearchIndexArgs) Kind() string { return "krovara.search_index" }

func (SearchIndexArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type Indexer interface {
	Index(ctx context.Context, m search.Message) error
}

type SearchIndexWorker struct {
	river.WorkerDefaults[SearchIndexArgs]
	Pool    *pgxpool.Pool
	Indexer Indexer
}

type archiveRow struct {
	User  string
	When  int64
	Value string
}

func (w *SearchIndexWorker) Work(ctx context.Context, j *river.Job[SearchIndexArgs]) error {
	return w.IndexBySortID(ctx, j.Args.SortID)
}

func (w *SearchIndexWorker) IndexBySortID(ctx context.Context, sortID int64) error {
	row, err := w.fetchRow(ctx, sortID)
	if err != nil {
		return fmt.Errorf("fetch row %d: %w", sortID, err)
	}
	parsed, err := ParseArchiveRow(row.User, row.Value)
	if errors.Is(err, ErrNoBody) {

		return nil
	}
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	q := db.New(w.Pool)
	channelUUID, ok := parseUUID(parsed.ChannelID)
	if !ok {

		return nil
	}
	channel, err := q.GetChannel(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {

		return nil
	}
	spaceID := uuidString(channel.SpaceID.Bytes)

	doc := search.Message{
		ID:        fmt.Sprintf("p%d", sortID),
		ChannelID: parsed.ChannelID,
		SpaceID:   spaceID,
		AuthorID:  parsed.AuthorID,
		Content:   parsed.Body,
		CreatedAt: row.When,
		HasLink:   parsed.HasLink,
		HasMedia:  parsed.HasMedia,
	}
	if err := w.Indexer.Index(ctx, doc); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	return nil
}

func (w *SearchIndexWorker) fetchRow(ctx context.Context, sortID int64) (archiveRow, error) {
	var r archiveRow
	err := w.Pool.QueryRow(ctx, `
SELECT "user", "when", value
FROM prosodyarchive
WHERE sort_id = $1
  AND host = 'conference.krovara.local'
  AND store = 'muc_log'
  AND type = 'xml'
`, sortID).Scan(&r.User, &r.When, &r.Value)
	return r, err
}
