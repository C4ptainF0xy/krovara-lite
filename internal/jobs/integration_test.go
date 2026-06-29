package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/jobs"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	pg, err := tcpg.Run(ctx,
		"postgres:16-alpine",
		tcpg.WithDatabase("krovara"),
		tcpg.WithUsername("krovara"),
		tcpg.WithPassword("krovara"),
		tcpg.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pg) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	migDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	m, err := migrate.New("file://"+filepath.ToSlash(migDir), "pgx5://"+dsn[len("postgres://"):])
	require.NoError(t, err)
	require.NoError(t, m.Up())
	srcErr, dbErr := m.Close()
	require.NoError(t, srcErr)
	require.NoError(t, dbErr)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

type recordingSender struct {
	mu        sync.Mutex
	calls     []string
	failsLeft int32
}

func (s *recordingSender) Send(_ context.Context, to, subject, _ string) error {
	if atomic.AddInt32(&s.failsLeft, -1) >= 0 {
		return errors.New("transient")
	}
	s.mu.Lock()
	s.calls = append(s.calls, to+":"+subject)
	s.mu.Unlock()
	return nil
}

func startClient(t *testing.T, pool *pgxpool.Pool, sender jobs.EmailSender) *river.Client[pgx.Tx] {
	t.Helper()
	workers := river.NewWorkers()
	river.AddWorker(workers, &jobs.EmailWorker{Sender: sender})
	river.AddWorker(workers, &jobs.InvitePurgeWorker{Pool: pool})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: workers,

		RetryPolicy: &river.DefaultClientRetryPolicy{},
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background()))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	})
	return client
}

func TestEmailJob_Runs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	pool := setupPool(t)
	sender := &recordingSender{}
	client := startClient(t, pool, sender)

	_, err := client.Insert(context.Background(), jobs.EmailArgs{
		To: "alice@example.com", Subject: "hello", HTML: "<p>hi</p>",
	}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		return len(sender.calls) == 1
	}, 10*time.Second, 50*time.Millisecond)
	require.Equal(t, "alice@example.com:hello", sender.calls[0])
}

func TestEmailJob_RetriesThenSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	pool := setupPool(t)
	sender := &recordingSender{failsLeft: 2}
	client := startClient(t, pool, sender)

	_, err := client.Insert(context.Background(), jobs.EmailArgs{
		To: "bob@example.com", Subject: "retry-me",
	}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		return len(sender.calls) == 1
	}, 60*time.Second, 200*time.Millisecond)
}

func TestInvitePurge_DeletesExpired(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	pool := setupPool(t)
	ctx := context.Background()

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (username,email,password_hash) VALUES ('alice','a@b.c','x') RETURNING id::text`).Scan(&userID))
	var spaceID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO spaces (owner_id,name) VALUES ($1,'S') RETURNING id::text`, userID).Scan(&spaceID))

	_, err := pool.Exec(ctx,
		`INSERT INTO invites (space_id,creator_id,code,expires_at) VALUES ($1,$2,'expired',NOW() - INTERVAL '1 hour')`, spaceID, userID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO invites (space_id,creator_id,code,expires_at) VALUES ($1,$2,'fresh',NOW() + INTERVAL '1 hour')`, spaceID, userID)
	require.NoError(t, err)

	client := startClient(t, pool, &recordingSender{})
	_, err = client.Insert(ctx, jobs.InvitePurgeArgs{}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var n int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM invites`).Scan(&n)
		return n == 1
	}, 10*time.Second, 100*time.Millisecond)

	var remaining string
	require.NoError(t, pool.QueryRow(ctx, `SELECT code FROM invites`).Scan(&remaining))
	require.Equal(t, "fresh", remaining)
}
