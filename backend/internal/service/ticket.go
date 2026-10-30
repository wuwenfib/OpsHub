package service

import (
	"context"
	"fmt"

	"github.com/fh/opshub/backend/internal/store"
)

// Tickets 提供工单查询（2.5 再加状态操作）。
type Tickets struct {
	st *store.Store
}

func NewTickets(st *store.Store) *Tickets {
	return &Tickets{st: st}
}

// TicketDetail 是工单详情：工单 + 关联系统/厂商名称 + 全部流水。
type TicketDetail struct {
	store.GetTicketViewRow
	Logs []store.TicketLog `json:"logs"`
}

func (s *Tickets) List(ctx context.Context) ([]store.ListTicketsRow, error) {
	return s.st.ListTickets(ctx)
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
