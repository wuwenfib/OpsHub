// Package sla 定义严重级别、SLA 策略和截止时间计算。纯逻辑，不访问数据库。
package sla

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

// Severity 是故障严重级别，P1 最严重。
type Severity string

const (
	P1 Severity = "P1"
	P2 Severity = "P2"
	P3 Severity = "P3"
	P4 Severity = "P4"
)

const (
	// DefaultSeverity：告警没带 severity 标签或标签值非法时使用。
	DefaultSeverity = P3
	// UnregisteredLevel：告警关联不到 CMDB 时按二级系统计时——不知道它多重要，取中间档。
	UnregisteredLevel = 2
)

// ParseSeverity 解析 "P1"～"P4"（忽略首尾空白和大小写）。
func ParseSeverity(s string) (Severity, error) {
	switch v := Severity(strings.ToUpper(strings.TrimSpace(s))); v {
	case P1, P2, P3, P4:
		return v, nil
	default:
		return "", fmt.Errorf("%w: severity %q 不合法，应为 P1～P4", apperr.ErrValidation, s)
	}
}

// Policy 是某个"系统等级 × 严重级别"组合下的时限。
type Policy struct {
	Response time.Duration // 从创建到受理的时限
	Resolve  time.Duration // 从创建到恢复的时限
}

// Deadlines 是一张工单的两个截止时间，统一为 UTC。
type Deadlines struct {
	Response time.Time
	Resolve  time.Time
}

// DeadlinesFrom 以工单创建时间为起点算出截止时间（自然时间，不扣除节假日）。
func (p Policy) DeadlinesFrom(createdAt time.Time) Deadlines {
	start := createdAt.UTC()
	return Deadlines{
		Response: start.Add(p.Response),
		Resolve:  start.Add(p.Resolve),
	}
}

// Times 是评估一张工单 SLA 所需的时间字段。
type Times struct {
	CreatedAt        time.Time
	ResponseDeadline time.Time
	ResolveDeadline  time.Time
	RespondedAt      *time.Time
	ResolvedAt       *time.Time // 重开会清空，所以这里总是"最后一次恢复"的时间
	Closed           bool
}

// Evaluation 是一张工单在某个时刻的 SLA 状态。
// *Met 为 nil 表示"还没有结论"：尚未响应/恢复，但也还没到截止时间。
type Evaluation struct {
	ResponseMet     *bool `json:"response_met"`
	ResolveMet      *bool `json:"resolve_met"`
	ResponseOverdue bool  `json:"response_overdue"` // 现在仍未响应且已过响应截止
	ResolveOverdue  bool  `json:"resolve_overdue"`  // 现在仍未恢复且已过恢复截止
	Overdue         bool  `json:"overdue"`          // 二者之一，前端据此标红
}

// Evaluate 是"是否达标、是否超时"的唯一定义；统计、工单详情、超时筛选都用它。
func Evaluate(t Times, now time.Time) Evaluation {
	var e Evaluation
	e.ResponseMet, e.ResponseOverdue = judge(t.RespondedAt, t.ResponseDeadline, now)
	e.ResolveMet, e.ResolveOverdue = judge(t.ResolvedAt, t.ResolveDeadline, now)
	if t.Closed {
		e.ResolveOverdue = false // 已关闭的单不再算"当前超时"（关单前必然已恢复，这里只是防御）
	}
	e.Overdue = e.ResponseOverdue || e.ResolveOverdue
	return e
}

func judge(doneAt *time.Time, deadline, now time.Time) (met *bool, overdue bool) {
	switch {
	case doneAt != nil:
		ok := !doneAt.After(deadline)
		return &ok, false
	case now.After(deadline):
		no := false
		return &no, true // 还没做完但已过期：确定违约
	default:
		return nil, false // 还没做完也没过期：待定
	}
}

// Record 是参与统计的一张工单及其分组。
type Record struct {
	GroupKey  string
	GroupName string
	Times
}

// GroupStats 是一个分组（某系统或某厂商）的 SLA 统计。比率在分母为 0 时为 null。
type GroupStats struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Tickets       int      `json:"tickets"`
	Resolved      int      `json:"resolved"`
	MTTRMinutes   *float64 `json:"mttr_minutes"`
	ResponseMet   int      `json:"response_met"`
	ResponseTotal int      `json:"response_total"` // 已有结论的工单数（待定的不计入）
	ResponseRate  *float64 `json:"response_rate"`
	ResolveMet    int      `json:"resolve_met"`
	ResolveTotal  int      `json:"resolve_total"`
	ResolveRate   *float64 `json:"resolve_rate"`
	OverdueNow    int      `json:"overdue_now"`

	totalRepair time.Duration
}

// Aggregate 按 GroupKey 分组汇总，并给出全部工单的合计。分组按 Key 排序。
func Aggregate(records []Record, now time.Time) (groups []GroupStats, total GroupStats) {
	byKey := map[string]*GroupStats{}
	var keys []string
	total = GroupStats{Key: "total", Name: "合计"}
	for _, r := range records {
		g, ok := byKey[r.GroupKey]
		if !ok {
			g = &GroupStats{Key: r.GroupKey, Name: r.GroupName}
			byKey[r.GroupKey] = g
			keys = append(keys, r.GroupKey)
		}
		e := Evaluate(r.Times, now)
		g.add(r.Times, e)
		total.add(r.Times, e)
	}
	slices.Sort(keys)
	for _, k := range keys {
		g := byKey[k]
		g.finish()
		groups = append(groups, *g)
	}
	total.finish()
	return groups, total
}

func (g *GroupStats) add(t Times, e Evaluation) {
	g.Tickets++
	if t.ResolvedAt != nil {
		g.Resolved++
		g.totalRepair += t.ResolvedAt.Sub(t.CreatedAt)
	}
	if e.ResponseMet != nil {
		g.ResponseTotal++
		if *e.ResponseMet {
			g.ResponseMet++
		}
	}
	if e.ResolveMet != nil {
		g.ResolveTotal++
		if *e.ResolveMet {
			g.ResolveMet++
		}
	}
	if e.Overdue {
		g.OverdueNow++
	}
}

func (g *GroupStats) finish() {
	if g.Resolved > 0 {
		m := g.totalRepair.Minutes() / float64(g.Resolved)
		g.MTTRMinutes = &m
	}
	g.ResponseRate = ratio(g.ResponseMet, g.ResponseTotal)
	g.ResolveRate = ratio(g.ResolveMet, g.ResolveTotal)
}

func ratio(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	r := float64(a) / float64(b)
	return &r
}
