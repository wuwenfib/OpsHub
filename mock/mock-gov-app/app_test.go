package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestApp 返回一个不真正 sleep、随机数固定的 App，并记录最后一次 sleep 的时长。
func newTestApp(r float64) (*App, *time.Duration) {
	a := NewApp()
	var slept time.Duration
	a.sleep = func(d time.Duration) { slept = d }
	a.randFloat = func() float64 { return r }
	return a, &slept
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestApplyByMode(t *testing.T) {
	tests := []struct {
		name       string
		chaos      string  // 先调用的 chaos 接口，空表示不注入
		rand       float64 // 固定的随机数
		wantStatus int
		minDelay   time.Duration
		maxDelay   time.Duration
	}{
		{"normal", "", 0.5, 200, 20 * time.Millisecond, 100 * time.Millisecond},
		{"slow", "/chaos/slow", 0.5, 200, 1500 * time.Millisecond, 2100 * time.Millisecond},
		{"error hit", "/chaos/error", 0.1, 500, 0, 100 * time.Millisecond},
		{"error miss", "/chaos/error", 0.9, 200, 0, 100 * time.Millisecond},
		{"reset after slow", "/chaos/reset", 0.5, 200, 20 * time.Millisecond, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, slept := newTestApp(tt.rand)
			h := a.Routes()
			if tt.name == "reset after slow" {
				do(t, h, "POST", "/chaos/slow")
			}
			if tt.chaos != "" {
				if rec := do(t, h, "POST", tt.chaos); rec.Code != 200 {
					t.Fatalf("chaos %s: status %d", tt.chaos, rec.Code)
				}
			}
			rec := do(t, h, "GET", "/api/apply")
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if *slept < tt.minDelay || *slept > tt.maxDelay {
				t.Errorf("delay = %v, want in [%v, %v]", *slept, tt.minDelay, tt.maxDelay)
			}
		})
	}
}

func TestMetricsExposed(t *testing.T) {
	a, _ := newTestApp(0.1)
	h := a.Routes()
	do(t, h, "POST", "/chaos/error")
	do(t, h, "GET", "/api/apply")

	body := do(t, h, "GET", "/metrics").Body.String()
	for _, want := range []string{
		`http_requests_total{code="500",handler="apply"} 1`,
		`http_request_duration_seconds_bucket{handler="apply",le="1"}`,
		`gov_app_chaos_mode 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestChaosRequiresPost(t *testing.T) {
	a, _ := newTestApp(0.5)
	if rec := do(t, a.Routes(), "GET", "/chaos/slow"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /chaos/slow status = %d, want 405", rec.Code)
	}
}
