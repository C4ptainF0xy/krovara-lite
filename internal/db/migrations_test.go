package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func migrationsURL(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	require.NoError(t, err)
	return "file://" + filepath.ToSlash(abs)
}

func TestMigrationsUpDownUp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pg, err := tcpg.Run(ctx,
		"postgres:16-alpine",
		tcpg.WithDatabase("krovara"),
		tcpg.WithUsername("krovara"),
		tcpg.WithPassword("krovara"),
		tcpg.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(pg)
	})

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	migrateDSN := "pgx5://" + dsn[len("postgres://"):]

	m, err := migrate.New(migrationsURL(t), migrateDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = m.Close()
	})

	require.NoError(t, m.Up(), "initial up")
	require.NoError(t, m.Down(), "down to zero")
	require.NoError(t, m.Up(), "second up")
}
