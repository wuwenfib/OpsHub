package sla

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		in      string
		want    Severity
		wantErr bool
	}{
		{"P1", P1, false},
		{"P4", P4, false},
		{" p2 ", P2, false},
		{"P0", "", true},
		{"P5", "", true},
		{"critical", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSeverity(tt.in)
			if tt.wantErr {
				if !errors.Is(err, apperr.ErrValidation) {
					t.Fatalf("want ErrValidation, got %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got (%q, %v), want %q", got, err, tt.want)
			}
		})
	}
}

func TestDeadlinesFrom(t *testing.T) {
	shanghai := time.FixedZone("CST", 8*3600)
	tests := []struct {
		name         string
		policy       Policy
		createdAt    time.Time
		wantResponse time.Time
		wantResolve  time.Time
	}{
		{
			name:         "一级 P1：15 分钟响应 / 2 小时恢复",
			policy:       Policy{Response: 15 * time.Minute, Resolve: 2 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 6, 0, 0, 0, time.UTC),
			wantResponse: time.Date(2026, 10, 30, 6, 15, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC),
		},
		{
			name:         "跨天跨月：三级 P3 恢复 72 小时",
			policy:       Policy{Response: 4 * time.Hour, Resolve: 72 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 22, 30, 0, 0, time.UTC),
			wantResponse: time.Date(2026, 10, 31, 2, 30, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 11, 2, 22, 30, 0, 0, time.UTC),
		},
		{
			name:         "北京时间输入，结果统一为 UTC 且是同一时刻",
			policy:       Policy{Response: 30 * time.Minute, Resolve: 4 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 14, 0, 0, 0, shanghai), // = 06:00 UTC
			wantResponse: time.Date(2026, 10, 30, 6, 30, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.policy.DeadlinesFrom(tt.createdAt)
			if !d.Response.Equal(tt.wantResponse) || !d.Resolve.Equal(tt.wantResolve) {
				t.Errorf("got (%v, %v), want (%v, %v)", d.Response, d.Resolve, tt.wantResponse, tt.wantResolve)
			}
			if d.Response.Location() != time.UTC || d.Resolve.Location() != time.UTC {
				t.Errorf("deadlines should be UTC, got %v / %v", d.Response.Location(), d.Resolve.Location())
			}
		})
	}
}

// at 返回 2026-10-30 当天 hh:mm 的 UTC 时间。
func at(hh, mm int) time.Time { return time.Date(2026, 10, 30, hh, mm, 0, 0, time.UTC) }
func atp(hh, mm int) *time.Time {
	t := at(hh, mm)
	return &t
}

func TestEvaluate(t *testing.T) {
	now := at(12, 0)
	base := Times{CreatedAt: at(11, 0), ResponseDeadline: at(11, 30), ResolveDeadline: at(15, 0)}
	yes, no := true, false
	tests := []struct {
		name        string
		mutate      func(*Times)
		wantRespMet *bool
		wantOverdue bool
	}{
		{"按时响应", func(t *Times) { t.RespondedAt = atp(11, 10) }, &yes, false},
		{"恰好在截止时刻响应算达标", func(t *Times) { t.RespondedAt = atp(11, 30) }, &yes, false},
		{"晚于截止响应", func(t *Times) { t.RespondedAt = atp(11, 31) }, &no, false},
		{"未响应且已过截止：违约 + 当前超时", func(*Times) {}, &no, true},
		{"未响应但未到截止：待定", func(t *Times) { t.ResponseDeadline = at(12, 30) }, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := base
			tt.mutate(&tm)
			e := Evaluate(tm, now)
			if !eqBoolPtr(e.ResponseMet, tt.wantRespMet) || e.Overdue != tt.wantOverdue {
				t.Errorf("response_met=%v overdue=%v, want %v / %v", ptrStr(e.ResponseMet), e.Overdue, ptrStr(tt.wantRespMet), tt.wantOverdue)
			}
		})
	}

	// 恢复维度：未恢复且已过恢复截止 → 当前超时
	e := Evaluate(Times{CreatedAt: at(8, 0), ResponseDeadline: at(8, 15), ResolveDeadline: at(10, 0), RespondedAt: atp(8, 5)}, now)
	if !e.ResolveOverdue || e.ResponseOverdue || e.ResolveMet == nil || *e.ResolveMet {
		t.Errorf("resolve overdue case: %+v", e)
	}
}

// TestAggregateHandCalculated 用一组手算好的数据核对统计结果。now = 12:00。
//
//	工单 厂商  创建   响应截止 恢复截止 响应   恢复   说明
//	T1   A    08:00  08:15   10:00   08:10  09:30  响应达标，恢复达标，修复 90 分钟
//	T2   A    09:00  09:30   13:00   09:45  13:30  响应超时，恢复超时，修复 270 分钟
//	T3   B    10:00  10:30   14:00   10:40  -      响应超时；恢复未到截止 → 待定
//	T4   B    11:00  11:15   15:00   -      -      未响应且已过 11:15 → 响应违约，当前超时
//	T5   B    11:50  12:20   15:50   -      -      都未到截止 → 全部待定
//
// 手算：
//
//	A：2 单，已恢复 2，MTTR=(90+270)/2=180 分钟；响应 1/2=50%；恢复 1/2=50%；当前超时 0
//	B：3 单，已恢复 0，MTTR=无；响应 0/2=0%（T5 待定不计）；恢复 0/0=无；当前超时 1（T4）
//	合计：5 单，已恢复 2，MTTR=180；响应 1/4=25%；恢复 1/2=50%；当前超时 1
func TestAggregateHandCalculated(t *testing.T) {
	rec := func(vendor string, created, respDL, resolveDL time.Time, responded, resolved *time.Time) Record {
		return Record{GroupKey: vendor, GroupName: "厂商" + vendor, Times: Times{
			CreatedAt: created, ResponseDeadline: respDL, ResolveDeadline: resolveDL,
			RespondedAt: responded, ResolvedAt: resolved,
		}}
	}
	records := []Record{
		rec("B", at(10, 0), at(10, 30), at(14, 0), atp(10, 40), nil),
		rec("A", at(8, 0), at(8, 15), at(10, 0), atp(8, 10), atp(9, 30)),
		rec("A", at(9, 0), at(9, 30), at(13, 0), atp(9, 45), atp(13, 30)),
		rec("B", at(11, 0), at(11, 15), at(15, 0), nil, nil),
		rec("B", at(11, 50), at(12, 20), at(15, 50), nil, nil),
	}
	groups, total := Aggregate(records, at(12, 0))

	if len(groups) != 2 || groups[0].Key != "A" || groups[1].Key != "B" {
		t.Fatalf("groups = %+v", groups)
	}
	check := func(name string, g GroupStats, tickets, resolved int, mttr *float64, respMet, respTotal, resMet, resTotal, overdue int) {
		t.Helper()
		if g.Tickets != tickets || g.Resolved != resolved || !eqFloatPtr(g.MTTRMinutes, mttr) ||
			g.ResponseMet != respMet || g.ResponseTotal != respTotal ||
			g.ResolveMet != resMet || g.ResolveTotal != resTotal || g.OverdueNow != overdue {
			t.Errorf("%s = %+v", name, g)
		}
	}
	f := func(v float64) *float64 { return &v }
	check("A", groups[0], 2, 2, f(180), 1, 2, 1, 2, 0)
	check("B", groups[1], 3, 0, nil, 0, 2, 0, 0, 1)
	check("合计", total, 5, 2, f(180), 1, 4, 1, 2, 1)

	if !eqFloatPtr(groups[0].ResponseRate, f(0.5)) || !eqFloatPtr(groups[1].ResponseRate, f(0)) ||
		groups[1].ResolveRate != nil || !eqFloatPtr(total.ResponseRate, f(0.25)) || !eqFloatPtr(total.ResolveRate, f(0.5)) {
		t.Errorf("rates: A=%v B=%v/%v total=%v/%v", ptrStr(groups[0].ResponseRate), ptrStr(groups[1].ResponseRate),
			ptrStr(groups[1].ResolveRate), ptrStr(total.ResponseRate), ptrStr(total.ResolveRate))
	}
}

func TestAggregateEmpty(t *testing.T) {
	groups, total := Aggregate(nil, at(12, 0))
	if len(groups) != 0 || total.Tickets != 0 || total.MTTRMinutes != nil || total.ResponseRate != nil {
		t.Errorf("groups=%v total=%+v", groups, total)
	}
}

func eqBoolPtr(a, b *bool) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func eqFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	d := *a - *b
	return d < 1e-9 && d > -1e-9
}

func ptrStr[T any](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}
