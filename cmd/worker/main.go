package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	clamd "github.com/lyimmi/go-clamd"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/envfile"
	"github.com/krovara/krovara/internal/jobs"
	"github.com/krovara/krovara/internal/messagehooks"
	"github.com/krovara/krovara/internal/messagepush"
	"github.com/krovara/krovara/internal/push"
	"github.com/krovara/krovara/internal/search"
	"github.com/krovara/krovara/internal/searchingest"
	"github.com/krovara/krovara/internal/webhooks"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := envfile.LoadFirst(".env.dev", ".env"); err != nil {
		log.Warn("envfile", "err", err)
	}

	dsn := envOr("KROVARA_DB_DSN", "postgres://krovara:krovara@localhost:5432/krovara?sslmode=disable")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Error("pool open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	client, err := newClient(pool, log)
	if err != nil {
		log.Error("river client", "err", err)
		os.Exit(1)
	}

	if err := client.Start(ctx); err != nil {
		log.Error("river start", "err", err)
		os.Exit(1)
	}
	log.Info("worker online", "dsn_host", maskDSN(dsn))

	if host, key := os.Getenv("KROVARA_MEILI_HOST"), os.Getenv("MEILI_MASTER_KEY"); host != "" && key != "" {
		go func() {
			if err := searchingest.Listen(ctx, pool, client, log); err != nil && ctx.Err() == nil {
				log.Error("search ingest listener died", "err", err)
			}
		}()
	}

	pushSvc := push.NewService(pool, client)
	go func() {
		if err := messagepush.Listen(ctx, pool, pushSvc, log); err != nil && ctx.Err() == nil {
			log.Error("message push listener died", "err", err)
		}
	}()

	webhooksSvc := webhooks.NewService(pool, client)
	go func() {
		if err := messagehooks.Listen(ctx, pool, webhooksSvc, log); err != nil && ctx.Err() == nil {
			log.Error("message hooks listener died", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := client.Stop(shutdownCtx); err != nil {
		log.Warn("river stop", "err", err)
	}
}

func newClient(pool *pgxpool.Pool, log *slog.Logger) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &jobs.EmailWorker{Sender: newEmailSender(log)})
	river.AddWorker(workers, &jobs.InvitePurgeWorker{Pool: pool})
	river.AddWorker(workers, &jobs.FileCleanupWorker{Pool: pool})
	river.AddWorker(workers, &jobs.LinkFeedWorker{Pool: pool})
	river.AddWorker(workers, &jobs.ModActionExpireWorker{Pool: pool})
	river.AddWorker(workers, &jobs.MemberRoleExpireWorker{Pool: pool})
	river.AddWorker(workers, &webhooks.WebhookDeliverWorker{})
	river.AddWorker(workers, &push.PushNotifyWorker{BaseURL: envOr("KROVARA_NTFY_INTERNAL_URL", "http://ntfy:80")})

	if host, key := os.Getenv("KROVARA_MEILI_HOST"), os.Getenv("MEILI_MASTER_KEY"); host != "" && key != "" {
		river.AddWorker(workers, &searchingest.SearchIndexWorker{
			Pool:    pool,
			Indexer: search.NewService(db.New(pool), host, key),
		})
	}

	if c := newClamd(os.Getenv("KROVARA_CLAMAV_ADDR")); c != nil {
		river.AddWorker(workers, &jobs.FileScanWorker{Pool: pool, Clamd: c})
	}

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.InvitePurgeArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),

		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.FileCleanupArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),

		river.NewPeriodicJob(
			river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.LinkFeedArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),

		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.ModActionExpireArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),

		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.MemberRoleExpireArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
		Workers:      workers,
		PeriodicJobs: periodic,
		Logger:       log,
	})
}

func newEmailSender(log *slog.Logger) jobs.EmailSender {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		log.Warn("RESEND_API_KEY unset — emails will be dropped")
		return logSender{log: log}
	}
	_ = key
	return logSender{log: log}
}

type logSender struct{ log *slog.Logger }

func (l logSender) Send(_ context.Context, to, subject, _ string) error {
	l.log.Info("email (dropped)", "to", to, "subject", subject)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newClamd(addr string) *clamd.Clamd {
	if addr == "" {
		return nil
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}

	return clamd.NewClamd(clamd.WithTimeout(30*time.Second), clamd.WithTCP(host, port))
}

func maskDSN(dsn string) string {
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '@' {
			return "***@" + dsn[i+1:]
		}
	}
	return dsn
}
