package jobs

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type FileCleanupArgs struct{}

func (FileCleanupArgs) Kind() string { return "krovara.file_cleanup" }

func (FileCleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 2}
}

type FileCleanupWorker struct {
	river.WorkerDefaults[FileCleanupArgs]
	Pool *pgxpool.Pool
}

const orphanQuery = `
SELECT id, path FROM files
 WHERE created_at < NOW() - INTERVAL '24 hours'
   AND kind IN ('avatar', 'icon')
   AND id::text NOT IN (SELECT avatar_key FROM users WHERE avatar_key IS NOT NULL)
   AND id::text NOT IN (SELECT icon_key   FROM spaces WHERE icon_key   IS NOT NULL)
 LIMIT 500
`

func (w *FileCleanupWorker) Work(ctx context.Context, _ *river.Job[FileCleanupArgs]) error {
	rows, err := w.Pool.Query(ctx, orphanQuery)
	if err != nil {
		return fmt.Errorf("scan orphans: %w", err)
	}
	type orphan struct {
		id   string
		path string
	}
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.path); err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, o := range orphans {

		if err := os.Remove(o.path); err != nil && !os.IsNotExist(err) {

			continue
		}
		if _, err := w.Pool.Exec(ctx, `DELETE FROM files WHERE id = $1`, o.id); err != nil {
			return fmt.Errorf("delete row %s: %w", o.id, err)
		}
	}
	return nil
}
