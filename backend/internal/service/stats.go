package service

import (
	"context"
	"fmt"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/sla"
	"github.com/fh/opshub/backend/internal/store"
)

const (
	GroupBySystem = "system"
	GroupByVendor = "vendor"

	defaultStatsWindow = 30 * 24 * time.Hour
	unregisteredName   = "未登记系统"
	noVendorName       = "未关联厂商"
)

// Stats 提供 SLA 统计，用于按系统、按厂商考核。
type Stats struct {
	st    *store.Store
	clock Clock
}

func NewStats(st *store.Store, clock Clock) *Stats {
	return &Stats{st: st, clock: clock}
}

// StatsQuery 的零值字段使用默认值：按系统分组、最近 30 天。
type StatsQuery struct {
	GroupBy string
	From    time.Time
	To      time.Time
}

type SLAReport struct {
	GroupBy     string           `json:"group_by"`
	From        time.Time        `json:"from"`
	To          time.Time        `json:"to"`
	GeneratedAt time.Time        `json:"generated_at"` // 当前超时数、待定判断都以这个时刻为准
	Groups      []sla.GroupStats `json:"groups"`
	Total       sla.GroupStats   `json:"total"`
}

func (s *Stats) SLA(ctx context.Context, q StatsQuery) (SLAReport, error) {
	if q.GroupBy == "" {
		q.GroupBy = GroupBySystem
	}
	if q.GroupBy != GroupBySystem && q.GroupBy != GroupByVendor {
		return SLAReport{}, fmt.Errorf("%w: group_by 必须是 system 或 vendor", apperr.ErrValidation)
	}
	now := s.clock.Now()
	if q.To.IsZero() {
		q.To = now
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-defaultStatsWindow)
	}
	if !q.From.Before(q.To) {
		return SLAReport{}, fmt.Errorf("%w: from 必须早于 to", apperr.ErrValidation)
	}

	rows, err := s.st.ListTicketsForStats(ctx, store.ListTicketsForStatsParams{FromTime: q.From, ToTime: q.To})
	if err != nil {
		return SLAReport{}, err
	}
	records := make([]sla.Record, len(rows))
	for i, r := range rows {
		key, name := groupOf(q.GroupBy, r)
		records[i] = sla.Record{GroupKey: key, GroupName: name, Times: slaTimes(
			r.CreatedAt, r.ResponseDeadline, r.ResolveDeadline, r.RespondedAt, r.ResolvedAt, r.Status)}
	}
	groups, total := sla.Aggregate(records, now)
	if groups == nil {
		groups = []sla.GroupStats{} // JSON 里输出 [] 而不是 null
	}
	return SLAReport{
		GroupBy: q.GroupBy, From: q.From.UTC(), To: q.To.UTC(), GeneratedAt: now,
		Groups: groups, Total: total,
	}, nil
}

func groupOf(groupBy string, r store.ListTicketsForStatsRow) (key, name string) {
	if groupBy == GroupByVendor {
		if r.VendorName == "" {
			return noVendorName, noVendorName
		}
		return r.VendorName, r.VendorName
	}
	if r.SystemName == "" {
		return r.SystemLabel, unregisteredName
	}
	return r.SystemLabel, r.SystemName
}

func slaTimes(created, respDL, resolveDL time.Time, responded, resolved *time.Time, status string) sla.Times {
	return sla.Times{
		CreatedAt: created, ResponseDeadline: respDL, ResolveDeadline: resolveDL,
		RespondedAt: responded, ResolvedAt: resolved, Closed: status == "closed",
	}
}
