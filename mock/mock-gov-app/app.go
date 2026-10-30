package main

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 故障模式。用原子变量保存，chaos 接口写、业务接口读，无需加锁。
const (
	modeNormal int32 = iota
	modeSlow
	modeError
)

// errorRatio 是 error 模式下请求失败的比例。
const errorRatio = 0.3

// App 模拟一个政务业务系统：一个办件接口 + 健康检查 + 指标 + 故障注入开关。
type App struct {
	mode atomic.Int32

	reg      *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec

	// 以下两个函数可在测试中替换，避免真的 sleep、让随机结果可控。
	sleep     func(time.Duration)
	randFloat func() float64
}

func NewApp() *App {
	a := &App{
		reg:       prometheus.NewRegistry(),
		sleep:     time.Sleep,
		randFloat: rand.Float64,
	}

	// Counter：只增不减的累计值。Prometheus 用 rate() 把它换算成"每秒多少次"。
	a.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP 请求总数。",
	}, []string{"handler", "code"})

	// Histogram：把每次耗时落到预先划好的桶里计数，之后可以用
	// histogram_quantile() 估算 P95。桶要密集覆盖告警阈值 1s 附近，否则估算不准。
	a.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP 请求耗时（秒）。",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 5},
	}, []string{"handler"})

	// Gauge：可增可减的瞬时值。这里在每次被抓取时读取当前故障模式，
	// 方便在 Grafana 上看到"什么时候注入了故障"。
	chaosMode := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gov_app_chaos_mode",
		Help: "当前故障注入模式：0 正常，1 变慢，2 报错。",
	}, func() float64 { return float64(a.mode.Load()) })

	a.reg.MustRegister(
		a.requests, a.duration, chaosMode,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return a
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/apply", a.instrument("apply", http.HandlerFunc(a.handleApply)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(a.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("POST /chaos/slow", a.setMode(modeSlow))
	mux.HandleFunc("POST /chaos/error", a.setMode(modeError))
	mux.HandleFunc("POST /chaos/reset", a.setMode(modeNormal))
	return mux
}

// handleApply 模拟一次办件：正常耗时 20–100ms；slow 模式额外 1.5–2s；
// error 模式下 30% 的请求返回 500。
func (a *App) handleApply(w http.ResponseWriter, r *http.Request) {
	delay := 20*time.Millisecond + time.Duration(a.randFloat()*80)*time.Millisecond

	switch a.mode.Load() {
	case modeSlow:
		delay += 1500*time.Millisecond + time.Duration(a.randFloat()*500)*time.Millisecond
	case modeError:
		if a.randFloat() < errorRatio {
			a.sleep(delay)
			http.Error(w, "injected failure", http.StatusInternalServerError)
			return
		}
	}

	a.sleep(delay)
	writeJSON(w, http.StatusOK, map[string]string{"result": "accepted"})
}

func (a *App) setMode(m int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.mode.Store(m)
		writeJSON(w, http.StatusOK, map[string]int32{"chaos_mode": m})
	}
}

// instrument 是一个中间件：包住真正的 handler，记录状态码和耗时。
func (a *App) instrument(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		a.requests.WithLabelValues(name, strconv.Itoa(rec.status)).Inc()
		a.duration.WithLabelValues(name).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder 记下 handler 写出的状态码（标准库的 ResponseWriter 不提供读取接口）。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
