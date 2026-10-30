// Package ticket 是工单的领域逻辑：事件单状态机，以及每个动作对工单字段的影响。纯逻辑，不访问数据库。
package ticket

import (
	"fmt"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

type Status string

const (
	New          Status = "new"
	Acknowledged Status = "acknowledged"
	Resolved     Status = "resolved"
	Closed       Status = "closed"
	Reopened     Status = "reopened"
)

// AllStatuses 用于测试遍历全部组合。
var AllStatuses = []Status{New, Acknowledged, Resolved, Closed, Reopened}

type Action string

const (
	Ack     Action = "ack"     // 受理：厂商确认接单
	Resolve Action = "resolve" // 标记恢复：业务已恢复
	Close   Action = "close"   // 关闭：甲方确认后关单
	Reopen  Action = "reopen"  // 重开：确认没修好
)

var AllActions = []Action{Ack, Resolve, Close, Reopen}

func ParseAction(s string) (Action, error) {
	for _, a := range AllActions {
		if string(a) == s {
			return a, nil
		}
	}
	return "", fmt.Errorf("%w: 未知操作 %q", apperr.ErrValidation, s)
}

const (
	TypeIncident = "incident"
	TypeProblem  = "problem"
	TypeChange   = "change"
)

func ValidType(t string) bool {
	return t == TypeIncident || t == TypeProblem || t == TypeChange
}

// transitions 是事件单状态机：当前状态 → 允许的动作 → 下一状态。表里没有的组合都是非法跳转。
//
//	new ──ack──▶ acknowledged ──resolve──▶ resolved ──close──▶ closed
//	                                         │   ▲
//	                                   reopen│   │resolve
//	                                         ▼   │
//	                                        reopened
var transitions = map[Status]map[Action]Status{
	New:          {Ack: Acknowledged},
	Acknowledged: {Resolve: Resolved},
	Reopened:     {Resolve: Resolved},
	Resolved:     {Close: Closed, Reopen: Reopened},
}

// Transition 返回执行动作后的状态；非法跳转返回 ErrInvalidTransition（HTTP 409）。
func Transition(cur Status, act Action) (Status, error) {
	if next, ok := transitions[cur][act]; ok {
		return next, nil
	}
	return "", fmt.Errorf("%w: 工单状态为 %s，不能执行 %s", apperr.ErrInvalidTransition, cur, act)
}

// Snapshot 是状态流转涉及的工单字段。
type Snapshot struct {
	Type        string
	Status      Status
	Assignee    string
	RespondedAt *time.Time
	ResolvedAt  *time.Time
	ClosedAt    *time.Time
}

// Apply 计算执行动作后的工单字段。operator 是操作人，now 是操作时间。
func Apply(s Snapshot, act Action, operator string, now time.Time) (Snapshot, error) {
	if s.Type != TypeIncident {
		return Snapshot{}, fmt.Errorf("%w: 目前只有事件单（incident）支持状态流转，当前类型 %s", apperr.ErrInvalidTransition, s.Type)
	}
	next, err := Transition(s.Status, act)
	if err != nil {
		return Snapshot{}, err
	}
	out := s
	out.Status = next
	switch act {
	case Ack:
		out.RespondedAt = &now // 响应时间 = 受理时间，用于考核响应 SLA
		out.Assignee = operator
	case Resolve:
		out.ResolvedAt = &now
	case Close:
		out.ClosedAt = &now
	case Reopen:
		out.ResolvedAt = nil // 没修好：清空恢复时间，最终以最后一次恢复计算恢复 SLA
	}
	return out, nil
}
