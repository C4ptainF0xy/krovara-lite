package jobs

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	clamd "github.com/lyimmi/go-clamd"
	"github.com/riverqueue/river"
)

type FileScanArgs struct {
	FileID string `json:"file_id"`
}

func (FileScanArgs) Kind() string { return "krovara.file_scan" }

func (FileScanArgs) InsertOpts() river.InsertOpts {

	return river.InsertOpts{MaxAttempts: 4}
}

type FileScanWorker struct {
	river.WorkerDefaults[FileScanArgs]
	Pool  *pgxpool.Pool
	Clamd *clamd.Clamd
}

func (w *FileScanWorker) Work(ctx context.Context, job *river.Job[FileScanArgs]) error {
	id, err := uuid.Parse(job.Args.FileID)
	if err != nil {
		return fmt.Errorf("bad file id %q: %w", job.Args.FileID, err)
	}

	var path string
	err = w.Pool.QueryRow(ctx, `SELECT path FROM files WHERE id = $1`, id).Scan(&path)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("load file %s: %w", id, err)
	}

	f, err := os.Open(path)
	if err != nil {

		_, _ = w.Pool.Exec(ctx, `UPDATE files SET scan_status = 'error' WHERE id = $1`, id)
		return nil
	}
	clean, scanErr := w.Clamd.ScanStream(ctx, f)
	f.Close()

	status := "clean"
	switch {
	case scanErr != nil:
		status = "error"
	case !clean:
		status = "infected"
		_ = os.Remove(path)
	}

	if _, err := w.Pool.Exec(ctx, `UPDATE files SET scan_status = $2 WHERE id = $1`, id, status); err != nil {
		return fmt.Errorf("update scan_status: %w", err)
	}

	if scanErr != nil {

		return fmt.Errorf("clamd scan %s: %w", id, scanErr)
	}
	return nil
}
