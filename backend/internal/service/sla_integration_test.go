//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/sla"
)

// 逐条核对迁移里的策略与设计文档 2.3 节的表格一致（单位：分钟）。
func TestSLAPolicySeed(t *testing.T) {
	ctx := context.Background()
	q := newTestStore(t).Queries

	want := map[int]map[sla.Severity][2]int{
		1: {sla.P1: {15, 120}, sla.P2: {30, 240}, sla.P3: {60, 480}, sla.P4: {240, 1440}},
		2: {sla.P1: {30, 240}, sla.P2: {60, 480}, sla.P3: {120, 1440}, sla.P4: {480, 2880}},
		3: {sla.P1: {60, 480}, sla.P2: {120, 1440}, sla.P3: {240, 4320}, sla.P4: {1440, 7200}},
	}
	for level, bySev := range want {
		for sev, minutes := range bySev {
			p, err := policyFor(ctx, q, level, sev)
			if err != nil {
				t.Fatalf("level=%d %s: %v", level, sev, err)
			}
			if p.Response != time.Duration(minutes[0])*time.Minute || p.Resolve != time.Duration(minutes[1])*time.Minute {
				t.Errorf("level=%d %s: got %v/%v, want %dm/%dm", level, sev, p.Response, p.Resolve, minutes[0], minutes[1])
			}
		}
	}
}

func TestSLADeadlinesForGovApply(t *testing.T) {
	ctx := context.Background()
	q := newTestStore(t).Queries

	// 一级系统出 P2 故障：30 分钟内响应，4 小时内恢复。
	p, err := policyFor(ctx, q, 1, sla.P2)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 30, 6, 0, 0, 0, time.UTC)
	d := p.DeadlinesFrom(created)
	if !d.Response.Equal(created.Add(30*time.Minute)) || !d.Resolve.Equal(created.Add(4*time.Hour)) {
		t.Errorf("got %v / %v", d.Response, d.Resolve)
	}

	// 未登记系统按二级计时。
	p, err = policyFor(ctx, q, sla.UnregisteredLevel, sla.P1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Response != 30*time.Minute {
		t.Errorf("unregistered P1 response = %v, want 30m", p.Response)
	}
}

func TestSLAPolicyMissing(t *testing.T) {
	_, err := policyFor(context.Background(), newTestStore(t).Queries, 4, sla.P1)
	wantErr(t, err, apperr.ErrNotFound)
}
