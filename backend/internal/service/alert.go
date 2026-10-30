package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fh/opshub/backend/internal/alert"
	"github.com/fh/opshub/backend/internal/sla"
	"github.com/fh/opshub/backend/internal/store"
)

const actorAlertmanager = "alertmanager"

// Alerts 处理 Alertmanager 推送的告警：关联 CMDB、去重、建事件单或追加流水。
type Alerts struct {
	st    *store.Store
	clock Clock
}

func NewAlerts(st *store.Store, clock Clock) *Alerts {
	return &Alerts{st: st, clock: clock}
}

// WebhookResult 统计一次推送里各条告警的处理结果，作为 webhook 的响应体返回，便于排查。
type WebhookResult struct {
	Received   int `json:"received"`
	Created    int `json:"created"`
	Duplicates int `json:"duplicates"`
	Recovered  int `json:"recovered"`
	Ignored    int `json:"ignored"`
	Failed     int `json:"failed"`
}

// Handle 逐条处理告警，每条一个独立事务：一条失败只记日志，不影响同批其他告警，
// 也不让整个 webhook 失败（否则 Alertmanager 会重试整批，造成重复处理）。
func (s *Alerts) Handle(ctx context.Context, p alert.Payload) WebhookResult {
	res := WebhookResult{Received: len(p.Alerts)}
	for _, a := range p.Alerts {
		act, err := s.handleOne(ctx, a)
		if err != nil {
			res.Failed++
			slog.Error("handle alert failed", "alertname", a.Name(), "system", a.System(), "fingerprint", a.Fingerprint, "err", err)
			continue
		}
		switch act {
		case alert.CreateTicket:
			res.Created++
		case alert.AppendDuplicate:
			res.Duplicates++
		case alert.AppendRecovered:
			res.Recovered++
		case alert.Ignore:
			res.Ignored++
		}
	}
	return res
}

func (s *Alerts) handleOne(ctx context.Context, a alert.Alert) (alert.Action, error) {
	var act alert.Action
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		now := s.clock.Now()
		open, err := q.GetOpenTicketByFingerprint(ctx, pgtype.Text{String: a.Fingerprint, Valid: true})
		hasOpen := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		act = alert.Decide(a, hasOpen)
		switch act {
		case alert.CreateTicket:
			created, err := s.createTicket(ctx, q, a, now)
			if err != nil {
				return err
			}
			if created {
				return nil
			}
			// 并发冲突：另一个请求刚为同一指纹建了单。按"重复告警"处理。
			slog.Info("alert ticket already created by concurrent request", "fingerprint", a.Fingerprint)
			act = alert.AppendDuplicate
			if open, err = q.GetOpenTicketByFingerprint(ctx, pgtype.Text{String: a.Fingerprint, Valid: true}); err != nil {
				return err
			}
			return appendLog(ctx, q, open.ID, "duplicate_alert", duplicateNote(a), now)
		case alert.AppendDuplicate:
			return appendLog(ctx, q, open.ID, "duplicate_alert", duplicateNote(a), now)
		case alert.AppendRecovered:
			return appendLog(ctx, q, open.ID, "alert_resolved",
				fmt.Sprintf("告警 %s 已恢复（%s），请人工确认后关单", a.Name(), a.EndsAt.UTC().Format(time.RFC3339)), now)
		}
		return nil
	})
	return act, err
}

// createTicket 建事件单；返回 created=false 表示被并发请求抢先（唯一索引冲突）。
func (s *Alerts) createTicket(ctx context.Context, q *store.Queries, a alert.Alert, now time.Time) (created bool, err error) {
	var notes []string
	if d := a.Annotations["description"]; d != "" {
		notes = append(notes, d)
	}

	// 1. 关联 CMDB：拿到系统等级；关联不上则按未登记系统处理。
	level := sla.UnregisteredLevel
	var systemID pgtype.Int8
	sys, err := q.GetSystemByCode(ctx, a.System())
	switch {
	case err == nil:
		level, systemID = int(sys.Level), pgtype.Int8{Int64: sys.ID, Valid: true}
	case errors.Is(err, pgx.ErrNoRows):
		notes = append(notes, fmt.Sprintf("system=%q 未在 CMDB 登记，按 %d 级系统计算 SLA", a.System(), level))
	default:
		return false, err
	}

	// 2. 严重级别 + SLA 策略 → 两个截止时间。
	sev, sevNote := a.Severity()
	if sevNote != "" {
		notes = append(notes, sevNote)
	}
	policy, err := policyFor(ctx, q, level, sev)
	if err != nil {
		return false, err
	}
	dl := policy.DeadlinesFrom(now)

	// 3. 插入；ON CONFLICT DO NOTHING 冲突时不返回行。
	t, err := q.CreateAlertTicket(ctx, store.CreateAlertTicketParams{
		Title:            a.Title(),
		SystemID:         systemID,
		SystemLabel:      a.System(),
		Severity:         string(sev),
		AlertFingerprint: pgtype.Text{String: a.Fingerprint, Valid: true},
		CreatedAt:        now,
		ResponseDeadline: dl.Response,
		ResolveDeadline:  dl.Resolve,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	note := fmt.Sprintf("告警 %s 触发，自动建单", a.Name())
	if len(notes) > 0 {
		note += "：" + strings.Join(notes, "；")
	}
	return true, q.InsertTicketLog(ctx, store.InsertTicketLogParams{
		TicketID: t.ID, Actor: actorAlertmanager, Action: "created",
		ToStatus: pgtype.Text{String: t.Status, Valid: true}, Note: note, CreatedAt: now,
	})
}

func appendLog(ctx context.Context, q *store.Queries, ticketID int64, action, note string, now time.Time) error {
	return q.InsertTicketLog(ctx, store.InsertTicketLogParams{
		TicketID: ticketID, Actor: actorAlertmanager, Action: action, Note: note, CreatedAt: now,
	})
}

func duplicateNote(a alert.Alert) string {
	note := fmt.Sprintf("告警 %s 再次推送，未新建工单", a.Name())
	if d := a.Annotations["description"]; d != "" {
		note += "：" + d
	}
	return note
}
