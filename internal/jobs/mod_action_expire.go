package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/krovara/krovara/internal/db"
)

type ModActionExpireArgs struct{}

func (ModActionExpireArgs) Kind() string { return "krovara.mod_action_expire" }

func (ModActionExpireArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

type ModActionExpireWorker struct {
	river.WorkerDefaults[ModActionExpireArgs]
	Pool *pgxpool.Pool
}

func (w *ModActionExpireWorker) Work(ctx context.Context, _ *river.Job[ModActionExpireArgs]) error {
	return db.New(w.Pool).DeactivateExpiredTimeouts(ctx)
}
