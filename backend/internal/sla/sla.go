// Package sla 定义严重级别、SLA 策略和截止时间计算。纯逻辑，不访问数据库。
package sla

import (
	"fmt"
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
