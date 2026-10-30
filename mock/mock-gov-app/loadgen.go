package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"
)

// runLoadgen 以 rps 的速率对 url 发 GET，直到 ctx 被取消。
// 没有流量时错误率的分母为 0，告警永远不会触发，所以 mock 自带流量。
//
// 每个请求放到独立的 goroutine 里发：慢故障期间单个请求要 2s，
// 如果串行发送，实际速率会掉下来，指标就失真了。
func runLoadgen(ctx context.Context, url string, rps int) {
	if rps <= 0 {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(time.Second / time.Duration(rps))
	defer ticker.Stop()

	log.Printf("loadgen: %d rps -> %s", rps, url)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			go func() {
				resp, err := client.Get(url)
				if err != nil {
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}
	}
}
