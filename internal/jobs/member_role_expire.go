package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/krovara/krovara/internal/db"
)

type MemberRoleExpireArgs struct{}

func (MemberRoleExpireArgs) Kind() string { return "krovara.member_role_expire" }

func (MemberRoleExpireArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

type MemberRoleExpireWorker struct {
	river.WorkerDefaults[MemberRoleExpireArgs]
	Pool *pgxpool.Pool
}

func (w *MemberRoleExpireWorker) Work(ctx context.Context, _ *river.Job[MemberRoleExpireArgs]) error {
	_, err := db.New(w.Pool).DeleteExpiredMemberRoles(ctx)
	return err
}
