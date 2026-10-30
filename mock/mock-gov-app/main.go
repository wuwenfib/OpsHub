// mock-gov-app 模拟一个被管的政务业务系统，暴露 /metrics 并支持故障注入。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	addr := envOr("LISTEN_ADDR", ":8081")
	rps, err := strconv.Atoi(envOr("LOADGEN_RPS", "5"))
	if err != nil {
		log.Fatalf("invalid LOADGEN_RPS: %v", err)
	}

	// 收到 SIGINT/SIGTERM 时 ctx 被取消，用于优雅退出（docker stop 发的就是 SIGTERM）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := NewApp()
	srv := &http.Server{Addr: addr, Handler: app.Routes()}

	go func() {
		log.Printf("mock-gov-app listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()
	go runLoadgen(ctx, "http://127.0.0.1"+portOf(addr)+"/api/apply", rps)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// portOf 从 ":8081" 或 "0.0.0.0:8081" 里取出 ":8081"。
func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i:]
		}
	}
	return addr
}
