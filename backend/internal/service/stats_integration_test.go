//go:build integration

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/cmdb"
	"github.com/fh/opshub/backend/internal/sla"
	"github.com/fh/opshub/backend/internal/store"
	"github.com/fh/opshub/backend/internal/ticket"
)

// setupStatsScenario 走真实流程造 4 张 P2 事件单（都在 t0 创建），统计时刻为 t0+10h：
//
//	单  系统（等级）          厂商  时限(响应/恢复)  受理      恢复       结论
//	T1  GOV-APPLY（1级）      星河  30m / 4h        +10m ✓   +60m ✓    修复 60 分钟
//	T2  GOV-OA（2级）         星河  60m / 8h        +90m ✗   +120m ✓   修复 120 分钟
//	T3  GOV-PORTAL（3级）     云帆  120m / 24h      +30m ✓   未恢复    恢复待定（截止 +24h）
//	T4  GOV-LEGACY（未登记→2级）-   60m / 8h        从未受理  未恢复    响应、恢复都违约，当前超时
//
// 手算（按厂商）：
//
//	星河：2 单，MTTR=(60+120)/2=90，响应 1/2，恢复 2/2，超时 0
//	云帆：1 单，MTTR=无，响应 1/1，恢复 0/0（待定），超时 0
//	未关联厂商：1 单，MTTR=无，响应 0/1，恢复 0/1，超时 1
//	合计：4 单，已恢复 2，MTTR=90，响应 2/4，恢复 2/3，超时 1
func setupStatsScenario(t *testing.T) (*store.Store, time.Time) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	c := NewCMDB(st)
	xinghe := must(c.CreateVendor(ctx, cmdb.VendorInput{Name: "星河软件"}))
	yunfan := must(c.CreateVendor(ctx, cmdb.VendorInput{Name: "云帆网络"}))
	must(c.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-APPLY", Name: "一网通办", Level: 1, VendorID: &xinghe.ID}))
	must(c.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-OA", Name: "协同办公", Level: 2, VendorID: &xinghe.ID}))
	must(c.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-PORTAL", Name: "门户网站", Level: 3, VendorID: &yunfan.ID}))

	alerts := NewAlerts(st, fixedClock{t0})
	ids := map[string]int64{}
	for _, sys := range []string{"GOV-APPLY", "GOV-OA", "GOV-PORTAL", "GOV-LEGACY"} {
		fp := "fp-stats-" + sys
		alerts.Handle(ctx, payload(firing(fp, sys, "P2")))
		ids[sys] = must(st.GetOpenTicketByFingerprint(ctx, text(fp))).ID
	}
	do := func(sys string, after time.Duration, act ticket.Action) {
		t.Helper()
		must(NewTickets(st, fixedClock{t0.Add(after)}).Do(ctx, ids[sys], act, ActionInput{Operator: "厂商工程师"}))
	}
	do("GOV-APPLY", 10*time.Minute, ticket.Ack)
	do("GOV-APPLY", 60*time.Minute, ticket.Resolve)
	do("GOV-OA", 90*time.Minute, ticket.Ack)
	do("GOV-OA", 120*time.Minute, ticket.Resolve)
	do("GOV-PORTAL", 30*time.Minute, ticket.Ack)
	return st, t0.Add(10 * time.Hour)
}

type wantStats struct {
	key                  string
	tickets, resolved    int
	mttr                 float64 // -1 表示无
	respMet, respTotal   int
	resolveMet, resTotal int
	overdue              int
}

func checkStats(t *testing.T, g sla.GroupStats, w wantStats) {
	t.Helper()
	mttrOK := (w.mttr < 0 && g.MTTRMinutes == nil) || (g.MTTRMinutes != nil && math.Abs(*g.MTTRMinutes-w.mttr) < 1e-9)
	if g.Key != w.key || g.Tickets != w.tickets || g.Resolved != w.resolved || !mttrOK ||
		g.ResponseMet != w.respMet || g.ResponseTotal != w.respTotal ||
		g.ResolveMet != w.resolveMet || g.ResolveTotal != w.resTotal || g.OverdueNow != w.overdue {
		mttr := "nil"
		if g.MTTRMinutes != nil {
			mttr = time.Duration(*g.MTTRMinutes * float64(time.Minute)).String()
		}
		t.Errorf("%s:\n got  %+v (mttr=%s)\n want %+v", w.key, g, mttr, w)
	}
}

func TestStatsByVendorMatchesHandCalculation(t *testing.T) {
	st, now := setupStatsScenario(t)
	r := must(NewStats(st, fixedClock{now}).SLA(context.Background(), StatsQuery{GroupBy: GroupByVendor}))

	if len(r.Groups) != 3 {
		t.Fatalf("groups = %+v", r.Groups)
	}
	byKey := map[string]sla.GroupStats{}
	for _, g := range r.Groups {
		byKey[g.Key] = g
	}
	checkStats(t, byKey["星河软件"], wantStats{"星河软件", 2, 2, 90, 1, 2, 2, 2, 0})
	checkStats(t, byKey["云帆网络"], wantStats{"云帆网络", 1, 0, -1, 1, 1, 0, 0, 0})
	checkStats(t, byKey["未关联厂商"], wantStats{"未关联厂商", 1, 0, -1, 0, 1, 0, 1, 1})
	checkStats(t, r.Total, wantStats{"total", 4, 2, 90, 2, 4, 2, 3, 1})
	if math.Abs(*r.Total.ResolveRate-2.0/3) > 1e-9 {
		t.Errorf("total resolve rate = %v, want 2/3", *r.Total.ResolveRate)
	}
}

func TestStatsBySystem(t *testing.T) {
	st, now := setupStatsScenario(t)
	r := must(NewStats(st, fixedClock{now}).SLA(context.Background(), StatsQuery{}))
	if r.GroupBy != GroupBySystem {
		t.Errorf("default group_by = %s", r.GroupBy)
	}
	var keys []string
	for _, g := range r.Groups {
		keys = append(keys, g.Key)
		if g.Key == "GOV-LEGACY" && g.Name != "未登记系统" {
			t.Errorf("GOV-LEGACY name = %s", g.Name)
		}
	}
	if len(keys) != 4 || keys[0] != "GOV-APPLY" || keys[3] != "GOV-PORTAL" {
		t.Errorf("keys = %v", keys)
	}
}

func TestStatsWindowAndValidation(t *testing.T) {
	ctx := context.Background()
	st, now := setupStatsScenario(t)
	svc := NewStats(st, fixedClock{now})

	// 时间窗左闭右开：from 恰好等于创建时刻 → 计入；to 恰好等于创建时刻 → 不计入。
	if r := must(svc.SLA(ctx, StatsQuery{From: t0, To: now})); r.Total.Tickets != 4 {
		t.Errorf("[t0, now) tickets = %d, want 4", r.Total.Tickets)
	}
	r := must(svc.SLA(ctx, StatsQuery{From: t0.Add(-time.Hour), To: t0}))
	if r.Total.Tickets != 0 || r.Groups == nil || len(r.Groups) != 0 {
		t.Errorf("[t0-1h, t0) = %+v, want empty groups", r)
	}

	_, err := svc.SLA(ctx, StatsQuery{GroupBy: "department"})
	wantErr(t, err, apperr.ErrValidation)
	_, err = svc.SLA(ctx, StatsQuery{From: now, To: t0})
	wantErr(t, err, apperr.ErrValidation)
}

func TestTicketSLAInDetailAndOverdueFilter(t *testing.T) {
	ctx := context.Background()
	st, now := setupStatsScenario(t)
	svc := NewTickets(st, fixedClock{now})

	overdue := must(svc.List(ctx, ListFilter{Overdue: true}))
	if len(overdue) != 1 || overdue[0].SystemLabel != "GOV-LEGACY" {
		t.Fatalf("overdue = %+v", overdue)
	}
	d := must(svc.Get(ctx, overdue[0].ID))
	if !d.SLA.Overdue || !d.SLA.ResponseOverdue || !d.SLA.ResolveOverdue {
		t.Errorf("GOV-LEGACY sla = %+v", d.SLA)
	}

	for _, it := range must(svc.List(ctx, ListFilter{System: "GOV-PORTAL"})) {
		if it.SLA.ResponseMet == nil || !*it.SLA.ResponseMet || it.SLA.ResolveMet != nil || it.SLA.Overdue {
			t.Errorf("GOV-PORTAL sla = %+v（应为响应达标、恢复待定、未超时）", it.SLA)
		}
	}
}
