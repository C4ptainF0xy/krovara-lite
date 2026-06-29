package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/krovara/krovara/internal/db"
)

type InvitePurgeArgs struct{}

func (InvitePurgeArgs) Kind() string { return "krovara.invite_purge" }

func (InvitePurgeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

type InvitePurgeWorker struct {
	river.WorkerDefaults[InvitePurgeArgs]
	Pool *pgxpool.Pool
}

func (w *InvitePurgeWorker) Work(ctx context.Context, _ *river.Job[InvitePurgeArgs]) error {
	q := db.New(w.Pool)
	if err := q.DeleteExpiredInvites(ctx); err != nil {
		return err
	}

	return q.DeleteExpiredXMPPTokens(ctx)
}
