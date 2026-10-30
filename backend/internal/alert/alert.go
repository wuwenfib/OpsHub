// Package alert 解析 Alertmanager 的 webhook 推送，并决定每条告警该怎么处理。纯逻辑，不访问数据库。
package alert

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/sla"
)

// Payload 是 Alertmanager webhook 的请求体（version 4），只保留用到的字段。
// 一次推送对应 Alertmanager 的一个分组，里面可能有多条告警。
type Payload struct {
	Version  string  `json:"version"`
	Status   string  `json:"status"`
	Receiver string  `json:"receiver"`
	GroupKey string  `json:"groupKey"`
	Alerts   []Alert `json:"alerts"`
}

type Alert struct {
	Status      string            `json:"status"` // firing | resolved
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"` // 由全部标签算出的哈希，同一告警每次推送都相同
}

const (
	StatusFiring   = "firing"
	StatusResolved = "resolved"
)

// Parse 解析并校验请求体。整个请求体不合法时返回 ErrValidation（对应 HTTP 400）。
func Parse(body []byte) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Payload{}, fmt.Errorf("%w: webhook 请求体不是合法 JSON: %v", apperr.ErrValidation, err)
	}
	for i, a := range p.Alerts {
		if a.Fingerprint == "" {
			return Payload{}, fmt.Errorf("%w: 第 %d 条告警缺少 fingerprint", apperr.ErrValidation, i)
		}
		if a.Status != StatusFiring && a.Status != StatusResolved {
			return Payload{}, fmt.Errorf("%w: 第 %d 条告警 status=%q 不合法", apperr.ErrValidation, i, a.Status)
		}
	}
	return p, nil
}

func (a Alert) Name() string   { return a.Labels["alertname"] }
func (a Alert) System() string { return a.Labels["system"] }

// Severity 取 severity 标签；缺失或非法时返回默认级别，并在 note 里说明原因（写进工单流水）。
func (a Alert) Severity() (sev sla.Severity, note string) {
	raw, ok := a.Labels["severity"]
	if !ok {
		return sla.DefaultSeverity, fmt.Sprintf("告警未带 severity 标签，按 %s 处理", sla.DefaultSeverity)
	}
	sev, err := sla.ParseSeverity(raw)
	if err != nil {
		return sla.DefaultSeverity, fmt.Sprintf("告警 severity=%q 不合法，按 %s 处理", raw, sla.DefaultSeverity)
	}
	return sev, ""
}

// Title 作为工单标题：优先用规则里的 summary 注解，没有就用"告警名 + 系统"。
func (a Alert) Title() string {
	if s := strings.TrimSpace(a.Annotations["summary"]); s != "" {
		return s
	}
	if sys := a.System(); sys != "" {
		return fmt.Sprintf("%s [%s]", a.Name(), sys)
	}
	return a.Name()
}

// Action 是一条告警的处理方式。
type Action int

const (
	Ignore          Action = iota // 恢复通知但没有对应工单（例如工单已关闭）
	CreateTicket                  // 新故障：建事件单
	AppendDuplicate               // 已有未关闭工单：只记一条"重复告警"
	AppendRecovered               // 告警恢复：只记一条"告警已恢复"，不自动关单
)

func (a Action) String() string {
	return [...]string{"ignore", "create_ticket", "append_duplicate", "append_recovered"}[a]
}

// Decide 根据告警状态和"该指纹是否已有未关闭工单"决定动作。
func Decide(a Alert, hasOpenTicket bool) Action {
	switch {
	case a.Status == StatusFiring && !hasOpenTicket:
		return CreateTicket
	case a.Status == StatusFiring:
		return AppendDuplicate
	case hasOpenTicket:
		return AppendRecovered
	default:
		return Ignore
	}
}
