package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/sla"
	"github.com/fh/opshub/backend/internal/store"
	"github.com/fh/opshub/backend/internal/ticket"
)

// Tickets 提供工单查询、手工建单和状态流转。
type Tickets struct {
	st    *store.Store
	clock Clock
}

func NewTickets(st *store.Store, clock Clock) *Tickets {
	return &Tickets{st: st, clock: clock}
}

// TicketDetail 是工单详情：工单 + 关联系统/厂商名称 + 全部流水。
type TicketDetail struct {
	store.GetTicketViewRow
	Logs []store.TicketLog `json:"logs"`
}

// ListFilter 的字段为空表示不按该项筛选。
type ListFilter struct {
	Status string
	System string
}

func (s *Tickets) List(ctx context.Context, f ListFilter) ([]store.ListTicketsRow, error) {
	var p store.ListTicketsParams
	if f.Status != "" {
		if !validStatus(f.Status) {
			return nil, fmt.Errorf("%w: status %q 不合法", apperr.ErrValidation, f.Status)
		}
		p.Status = pgtype.Text{String: f.Status, Valid: true}
	}
	if f.System != "" {
		p.System = pgtype.Text{String: f.System, Valid: true}
	}
	return s.st.ListTickets(ctx, p)
}

func (s *Tickets) Get(ctx context.Context, id int64) (TicketDetail, error) {
	t, err := s.st.GetTicketView(ctx, id)
	if err != nil {
		return TicketDetail{}, store.MapError(err, fmt.Sprintf("ticket %d", id))
	}
	logs, err := s.st.ListTicketLogs(ctx, id)
	if err != nil {
		return TicketDetail{}, err
	}
	return TicketDetail{GetTicketViewRow: t, Logs: logs}, nil
}

// ActionInput 是状态操作的请求体。没有登录体系，操作人由调用方填写。
type ActionInput struct {
	Operator string `json:"operator"`
	Note     string `json:"note"`
}

// Do 对工单执行一个状态操作：锁行 → 状态机校验并计算新字段 → 写回 → 记流水，全部在一个事务里。
func (s *Tickets) Do(ctx context.Context, id int64, act ticket.Action, in ActionInput) (TicketDetail, error) {
	operator := strings.TrimSpace(in.Operator)
	if operator == "" {
		return TicketDetail{}, fmt.Errorf("%w: operator 不能为空", apperr.ErrValidation)
	}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetTicketForUpdate(ctx, id)
		if err != nil {
			return store.MapError(err, fmt.Sprintf("ticket %d", id))
		}
		now := s.clock.Now()
		next, err := ticket.Apply(snapshotOf(cur), act, operator, now)
		if err != nil {
			return err
		}
		if err := q.UpdateTicketState(ctx, store.UpdateTicketStateParams{
			ID: id, Status: string(next.Status), Assignee: next.Assignee,
			RespondedAt: next.RespondedAt, ResolvedAt: next.ResolvedAt, ClosedAt: next.ClosedAt,
		}); err != nil {
			return err
		}
		return q.InsertTicketLog(ctx, store.InsertTicketLogParams{
			TicketID: id, Actor: operator, Action: string(act),
			FromStatus: pgtype.Text{String: cur.Status, Valid: true},
			ToStatus:   pgtype.Text{String: string(next.Status), Valid: true},
			Note:       strings.TrimSpace(in.Note), CreatedAt: now,
		})
	})
	if err != nil {
		return TicketDetail{}, err
	}
	return s.Get(ctx, id)
}

// ManualInput 是手工建单的请求体。手工单必须关联已登记的系统，否则无法确定 SLA 和责任厂商。
type ManualInput struct {
	Type       string `json:"type"` // 默认 incident
	Title      string `json:"title"`
	SystemCode string `json:"system_code"`
	Severity   string `json:"severity"`
	Operator   string `json:"operator"`
	Note       string `json:"note"`
}

func (s *Tickets) CreateManual(ctx context.Context, in ManualInput) (TicketDetail, error) {
	if in.Type == "" {
		in.Type = ticket.TypeIncident
	}
	in.Title, in.Operator = strings.TrimSpace(in.Title), strings.TrimSpace(in.Operator)
	switch {
	case !ticket.ValidType(in.Type):
		return TicketDetail{}, fmt.Errorf("%w: type 必须是 incident / problem / change", apperr.ErrValidation)
	case in.Title == "":
		return TicketDetail{}, fmt.Errorf("%w: title 不能为空", apperr.ErrValidation)
	case in.Operator == "":
		return TicketDetail{}, fmt.Errorf("%w: operator 不能为空", apperr.ErrValidation)
	}
	sev, err := sla.ParseSeverity(in.Severity)
	if err != nil {
		return TicketDetail{}, err
	}

	var id int64
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		sys, err := q.GetSystemByCode(ctx, in.SystemCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: 系统 %q 未在 CMDB 登记", apperr.ErrValidation, in.SystemCode)
		}
		if err != nil {
			return err
		}
		policy, err := policyFor(ctx, q, int(sys.Level), sev)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		dl := policy.DeadlinesFrom(now)
		t, err := q.CreateManualTicket(ctx, store.CreateManualTicketParams{
			Type: in.Type, Title: in.Title, SystemID: pgtype.Int8{Int64: sys.ID, Valid: true},
			SystemLabel: sys.Code, Severity: string(sev),
			CreatedAt: now, ResponseDeadline: dl.Response, ResolveDeadline: dl.Resolve,
		})
		if err != nil {
			return err
		}
		id = t.ID
		note := "手工建单"
		if n := strings.TrimSpace(in.Note); n != "" {
			note += "：" + n
		}
		return q.InsertTicketLog(ctx, store.InsertTicketLogParams{
			TicketID: t.ID, Actor: in.Operator, Action: "created",
			ToStatus: pgtype.Text{String: t.Status, Valid: true}, Note: note, CreatedAt: now,
		})
	})
	if err != nil {
		return TicketDetail{}, err
	}
	return s.Get(ctx, id)
}

func snapshotOf(t store.Ticket) ticket.Snapshot {
	return ticket.Snapshot{
		Type: t.Type, Status: ticket.Status(t.Status), Assignee: t.Assignee,
		RespondedAt: t.RespondedAt, ResolvedAt: t.ResolvedAt, ClosedAt: t.ClosedAt,
	}
}

func validStatus(s string) bool {
	for _, st := range ticket.AllStatuses {
		if string(st) == s {
			return true
		}
	}
	return false
}
