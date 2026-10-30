//go:build integration

package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/store"
	"github.com/fh/opshub/backend/internal/ticket"
)

// newAlertTicket 通过告警建一张事件单，返回其 id。
func newAlertTicket(t *testing.T, st *store.Store, fp string) int64 {
	t.Helper()
	ctx := context.Background()
	if res := NewAlerts(st, fixedClock{t0}).Handle(ctx, payload(firing(fp, "GOV-APPLY", "P2"))); res.Created != 1 {
		t.Fatalf("create alert ticket: %+v", res)
	}
	return must(st.GetOpenTicketByFingerprint(ctx, text(fp))).ID
}

// transitions 把流水整理成 "action:from→to"，便于整体断言。
func transitions(logs []store.TicketLog) []string {
	var out []string
	for _, l := range logs {
		out = append(out, l.Action+":"+l.FromStatus.String+"→"+l.ToStatus.String)
	}
	return out
}

func TestTicketFullLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	id := newAlertTicket(t, st, "fp-lifecycle")
	op := ActionInput{Operator: "张工"}

	step := func(at time.Duration, act ticket.Action) TicketDetail {
		t.Helper()
		svc := NewTickets(st, fixedClock{t0.Add(at)})
		return must(svc.Do(ctx, id, act, op))
	}

	d := step(10*time.Minute, ticket.Ack)
	if d.Status != "acknowledged" || d.Assignee != "张工" || !d.RespondedAt.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("after ack: status=%s assignee=%s responded_at=%v", d.Status, d.Assignee, d.RespondedAt)
	}

	// 非法跳转：已受理不能直接关单 → 409，且状态和流水都不变。
	_, err := NewTickets(st, fixedClock{t0}).Do(ctx, id, ticket.Close, op)
	wantErr(t, err, apperr.ErrInvalidTransition)
	if d := must(NewTickets(st, fixedClock{t0}).Get(ctx, id)); d.Status != "acknowledged" || len(d.Logs) != 2 {
		t.Fatalf("illegal close changed ticket: status=%s logs=%d", d.Status, len(d.Logs))
	}

	step(30*time.Minute, ticket.Resolve)
	d = step(40*time.Minute, ticket.Reopen)
	if d.Status != "reopened" || d.ResolvedAt != nil {
		t.Fatalf("after reopen: status=%s resolved_at=%v", d.Status, d.ResolvedAt)
	}
	step(90*time.Minute, ticket.Resolve)
	d = step(100*time.Minute, ticket.Close)

	if d.Status != "closed" || !d.ResolvedAt.Equal(t0.Add(90*time.Minute)) || !d.ClosedAt.Equal(t0.Add(100*time.Minute)) {
		t.Errorf("final: status=%s resolved_at=%v closed_at=%v", d.Status, d.ResolvedAt, d.ClosedAt)
	}
	want := []string{
		"created:→new",
		"ack:new→acknowledged",
		"resolve:acknowledged→resolved",
		"reopen:resolved→reopened",
		"resolve:reopened→resolved",
		"close:resolved→closed",
	}
	if got := transitions(d.Logs); !slices.Equal(got, want) {
		t.Errorf("logs:\n got  %v\n want %v", got, want)
	}

	// 关单释放了去重锁：同一告警再次触发会新建工单。
	if res := NewAlerts(st, fixedClock{t0.Add(2 * time.Hour)}).Handle(ctx, payload(firing("fp-lifecycle", "GOV-APPLY", "P2"))); res.Created != 1 {
		t.Errorf("re-fire after close: %+v", res)
	}
}

func TestTicketActionErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	svc := NewTickets(st, fixedClock{t0})
	id := newAlertTicket(t, st, "fp-errors")

	_, err := svc.Do(ctx, id, ticket.Ack, ActionInput{Operator: "  "})
	wantErr(t, err, apperr.ErrValidation)

	_, err = svc.Do(ctx, 999, ticket.Ack, ActionInput{Operator: "张工"})
	wantErr(t, err, apperr.ErrNotFound)
}

func TestManualTicket(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	svc := NewTickets(st, fixedClock{t0})

	d := must(svc.CreateManual(ctx, ManualInput{Title: "用户反馈无法提交办件", SystemCode: "GOV-APPLY", Severity: "P1", Operator: "值班员"}))
	// 一级系统 P1：15 分钟响应 / 2 小时恢复
	if d.Source != "manual" || d.Type != "incident" || d.VendorName != "星河软件" ||
		!d.ResponseDeadline.Equal(t0.Add(15*time.Minute)) || !d.ResolveDeadline.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("manual ticket = %+v", d.GetTicketViewRow)
	}
	if len(d.Logs) != 1 || d.Logs[0].Actor != "值班员" {
		t.Errorf("logs = %+v", d.Logs)
	}

	_, err := svc.CreateManual(ctx, ManualInput{Title: "x", SystemCode: "GOV-NOPE", Severity: "P2", Operator: "值班员"})
	wantErr(t, err, apperr.ErrValidation)

	_, err = svc.CreateManual(ctx, ManualInput{Title: "x", SystemCode: "GOV-APPLY", Severity: "P9", Operator: "值班员"})
	wantErr(t, err, apperr.ErrValidation)

	// 问题单可以建，但目前不支持状态流转。
	p := must(svc.CreateManual(ctx, ManualInput{Type: "problem", Title: "错误率反复升高的根因分析", SystemCode: "GOV-APPLY", Severity: "P3", Operator: "值班员"}))
	_, err = svc.Do(ctx, p.ID, ticket.Ack, ActionInput{Operator: "张工"})
	wantErr(t, err, apperr.ErrInvalidTransition)
}

func TestTicketListFilter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	setupGovApply(t, st)
	svc := NewTickets(st, fixedClock{t0})

	acked := newAlertTicket(t, st, "fp-list-1")
	newAlertTicket(t, st, "fp-list-2")
	NewAlerts(st, fixedClock{t0}).Handle(ctx, payload(firing("fp-list-3", "GOV-LEGACY", "P3")))
	must(svc.Do(ctx, acked, ticket.Ack, ActionInput{Operator: "张工"}))

	count := func(f ListFilter) int { return len(must(svc.List(ctx, f))) }
	if n := count(ListFilter{}); n != 3 {
		t.Errorf("all = %d", n)
	}
	if n := count(ListFilter{Status: "new"}); n != 2 {
		t.Errorf("status=new = %d", n)
	}
	if n := count(ListFilter{System: "GOV-APPLY", Status: "new"}); n != 1 {
		t.Errorf("GOV-APPLY + new = %d", n)
	}
	if n := count(ListFilter{System: "GOV-LEGACY"}); n != 1 {
		t.Errorf("unregistered system = %d", n)
	}
	_, err := svc.List(ctx, ListFilter{Status: "done"})
	wantErr(t, err, apperr.ErrValidation)
}

// 同一张已恢复的工单，同时有人点"关闭"、有人点"重开"：行锁让操作排队，恰好一个成功。
func TestTicketConcurrentActions(t *testing.T) {
	ctx := context.Background()
	st, pool := newTestDB(t)
	setupGovApply(t, st)
	svc := NewTickets(st, fixedClock{t0})
	id := newAlertTicket(t, st, "fp-race")
	must(svc.Do(ctx, id, ticket.Ack, ActionInput{Operator: "张工"}))
	must(svc.Do(ctx, id, ticket.Resolve, ActionInput{Operator: "张工"}))

	const n = 20
	warmPool(t, pool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		act := ticket.Close
		if i%2 == 1 {
			act = ticket.Reopen
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Do(ctx, id, act, ActionInput{Operator: "并发操作人"})
		}()
	}
	close(start)
	wg.Wait()

	var ok, conflict int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, apperr.ErrInvalidTransition):
			conflict++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	t.Logf("succeeded=%d conflicts=%d", ok, conflict)
	if ok != 1 || conflict != n-1 {
		t.Errorf("succeeded=%d conflicts=%d, want 1/%d", ok, conflict, n-1)
	}
	if logs := must(svc.Get(ctx, id)).Logs; len(logs) != 4 { // created, ack, resolve, + 1 次成功的操作
		t.Errorf("logs = %v", transitions(logs))
	}
}
