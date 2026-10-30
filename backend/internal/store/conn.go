// Package store 负责数据库连接、迁移和查询。
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // 注册 pgx5:// 驱动
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fh/opshub/backend/migrations"
)

// Connect 建立连接池，数据库还没就绪时每秒重试一次，直到 ctx 取消或超过 30 次。
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	for attempt := 1; ; attempt++ {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		if attempt >= 30 {
			pool.Close()
			return nil, fmt.Errorf("database not reachable after %d attempts: %w", attempt, err)
		}
		slog.Warn("database not ready, retrying", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Migrate 把数据库升级到 migrations 目录里的最新版本；已是最新时什么也不做。
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	version, dirty, err := m.Version()
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	slog.Info("database migrated", "version", version, "dirty", dirty)
	return nil
}

// pgx5URL 把 postgres:// 换成 golang-migrate 的 pgx/v5 驱动认的 pgx5://。
func pgx5URL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(databaseURL, prefix) {
			return "pgx5://" + strings.TrimPrefix(databaseURL, prefix)
		}
	}
	return databaseURL
}
