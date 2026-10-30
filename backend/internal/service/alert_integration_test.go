//go:build integration

package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fh/opshub/backend/internal/alert"
	"github.com/fh/opshub/backend/internal/cmdb"
	"github.com/fh/opshub/backend/internal/store"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var t0 = time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)

func firing(fp, system, severity string) alert.Alert {
	labels := map[string]string{"alertname": "GovAppHighErrorRate", "system": system}
	if severity != "" {
		labels["severity"] = severity
	}
	return alert.Alert{
		Status: alert.StatusFiring, Fingerprint: fp, Labels: labels,
		Annotations: map[string]string{"summary": system + " 接口错误率超过 5%", "description": "当前错误率 28%"},
	}
}

func resolved(a alert.Alert) alert.Alert {
	a.Status = alert.StatusResolved
	a.EndsAt = t0.Add(10 * time.Minute)
	return a
}

func payload(alerts ...alert.Alert) alert.Payload { return alert.Payload{Alerts: alerts} }

// setupGovApply 建一个一级系统 GOV-APPLY，归属厂商"星河软件"。
func setupGovApply(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	c := NewCMDB(st)
	v := must(c.CreateVendor(ctx, cmdb.VendorInput{Name: "星河软件"}))
	must(c.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-APPLY", Name: "一网通办", Level: 1, VendorID: &v.ID}))
}

func logActions(t *testing.T, st *store.Store, ticketID int64) []string {
	t.Helper()
	logs := must(st.ListTicketLogs(context.Background(), ticketID))
	out := make([]string, len(logs))
	for i, l := range logs {
		out[i] = l.Action
	}
	return out
}

func TestAlertLifecycle(t *testing.T) {
	ctx := context.Background()
	st, pool := newTestDB(t)
	setupGovApply(t, st)
	svc := NewAlerts(st, fixedClock{t0})
	a := firing("fp-error-rate", "GOV-APPLY", "P2")

	// 1. 首次 firing → 建单
	if res := svc.Handle(ctx, payload(a)); res.Created != 1 {
		t.Fatalf("first firing: %+v", res)
	}
	tickets := must(st.ListTickets(ctx))
	if len(tickets) != 1 {
		t.Fatalf("tickets = %d", len(tickets))
	}
	tk := tickets[0]
	if tk.Status != "new" || tk.Severity != "P2" || tk.SystemCode != "GOV-APPLY" || tk.VendorName != "星河软件" || tk.Type != "incident" || tk.Source != "alert" {
		t.Errorf("ticket = %+v", tk)
	}
	// 一级系统 P2：30 分钟响应 / 4 小时恢复
	if !tk.ResponseDeadline.Equal(t0.Add(30*time.Minute)) || !tk.ResolveDeadline.Equal(t0.Add(4*time.Hour)) {
		t.Errorf("deadlines = %v / %v", tk.ResponseDeadline, tk.ResolveDeadline)
	}

	// 2. 再次 firing → 不建单，只追加流水
	if res := svc.Handle(ctx, payload(a)); res.Duplicates != 1 || res.Created != 0 {
		t.Fatalf("second firing: %+v", res)
	}
	// 3. resolved → 只追加流水，不关单
	if res := svc.Handle(ctx, payload(resolved(a))); res.Recovered != 1 {
		t.Fatalf("resolved: %+v", res)
	}
	detail := must(NewTickets(st).Get(ctx, tk.ID))
	if detail.Status != "new" {
		t.Errorf("status after resolved = %s, want new（不应自动关单）", detail.Status)
	}
	got := logActions(t, st, tk.ID)
	if want := []string{"created", "duplicate_alert", "alert_resolved"}; !slices.Equal(got, want) {
		t.Errorf("logs = %v, want %v", got, want)
	}

	// 4. 工单关闭后同一告警再 firing → 新建工单（状态机在 2.5，这里直接改库模拟关单）
	if _, err := pool.Exec(ctx, "UPDATE ticket SET status = 'closed' WHERE id = $1", tk.ID); err != nil {
		t.Fatal(err)
	}
	if res := svc.Handle(ctx, payload(a)); res.Created != 1 {
		t.Fatalf("firing after close: %+v", res)
	}
	if n := len(must(st.ListTickets(ctx))); n != 2 {
		t.Errorf("tickets after re-fire = %d, want 2", n)
	}
}

func TestAlertEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	svc := NewAlerts(st, fixedClock{t0})

	t.Run("unregistered system uses level-2 SLA", func(t *testing.T) {
		svc.Handle(ctx, payload(firing("fp-unknown", "GOV-LEGACY", "P1")))
		tk := findByLabel(t, st, "GOV-LEGACY")
		if tk.SystemID.Valid || tk.SystemCode != "" {
			t.Errorf("system should be NULL, got %+v", tk.SystemID)
		}
		// 二级 P1：30 分钟响应 / 4 小时恢复
		if !tk.ResponseDeadline.Equal(t0.Add(30 * time.Minute)) {
			t.Errorf("response deadline = %v", tk.ResponseDeadline)
		}
	})

	t.Run("missing severity defaults to P3", func(t *testing.T) {
		svc.Handle(ctx, payload(firing("fp-nosev", "GOV-APPLY", "")))
		tk := must(st.GetOpenTicketByFingerprint(ctx, text("fp-nosev")))
		if tk.Severity != "P3" {
			t.Errorf("severity = %s", tk.Severity)
		}
		logs := must(st.ListTicketLogs(ctx, tk.ID))
		if len(logs) != 1 || !strings.Contains(logs[0].Note, "未带 severity 标签") {
			t.Errorf("created log note = %q", logs[0].Note)
		}
	})

	t.Run("resolved without ticket is ignored", func(t *testing.T) {
		res := svc.Handle(ctx, payload(resolved(firing("fp-never-fired", "GOV-APPLY", "P2"))))
		if res.Ignored != 1 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("one payload with mixed alerts", func(t *testing.T) {
		res := svc.Handle(ctx, payload(
			firing("fp-mixed-1", "GOV-APPLY", "P2"),
			firing("fp-mixed-1", "GOV-APPLY", "P2"), // 同批重复
			resolved(firing("fp-mixed-2", "GOV-APPLY", "P2")),
		))
		if res.Received != 3 || res.Created != 1 || res.Duplicates != 1 || res.Ignored != 1 {
			t.Errorf("result = %+v", res)
		}
	})
}

// 20 个并发请求推送同一告警：唯一索引保证只建一张工单，其余都记为重复。
func TestAlertConcurrentDedup(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	svc := NewAlerts(st, fixedClock{t0})
	a := firing("fp-storm", "GOV-APPLY", "P2")

	const n = 20
	results := make([]WebhookResult, n)
	var wg sync.WaitGroup
	start := make(chan struct{}) // 起跑线：所有 goroutine 就位后同时放行，确保真的并发
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = svc.Handle(ctx, payload(a))
		}()
	}
	close(start)
	wg.Wait()

	var created, dup, failed int
	for _, r := range results {
		created += r.Created
		dup += r.Duplicates
		failed += r.Failed
	}
	if created != 1 || dup != n-1 || failed != 0 {
		t.Errorf("created=%d duplicates=%d failed=%d, want 1/%d/0", created, dup, failed, n-1)
	}
	tickets := must(st.ListTickets(ctx))
	if len(tickets) != 1 {
		t.Fatalf("tickets = %d, want 1", len(tickets))
	}
	if logs := logActions(t, st, tickets[0].ID); len(logs) != n {
		t.Errorf("logs = %d, want %d", len(logs), n)
	}
}

func findByLabel(t *testing.T, st *store.Store, label string) store.ListTicketsRow {
	t.Helper()
	for _, tk := range must(st.ListTickets(context.Background())) {
		if tk.SystemLabel == label {
			return tk
		}
	}
	t.Fatalf("no ticket with system_label %s", label)
	return store.ListTicketsRow{}
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
