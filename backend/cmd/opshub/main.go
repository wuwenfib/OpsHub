// opshub 是 OpsHub 的后端单体服务。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fh/opshub/backend/internal/api"
	"github.com/fh/opshub/backend/internal/service"
	"github.com/fh/opshub/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("opshub exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	addr := envOr("LISTEN_ADDR", ":8080")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 先连库（带重试，等数据库就绪），再迁移，最后才对外提供服务。
	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(dbURL); err != nil {
		return err
	}

	st := store.NewStore(pool)
	router := api.NewRouter(api.Deps{
		DB:      pool,
		CMDB:    service.NewCMDB(st),
		Alerts:  service.NewAlerts(st, service.RealClock{}),
		Tickets: service.NewTickets(st, service.RealClock{}),
		Stats:   service.NewStats(st, service.RealClock{}),
	})
	srv := &http.Server{Addr: addr, Handler: router}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("opshub listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
