package service

import (
	"context"
	"fmt"
	"time"

	"github.com/fh/opshub/backend/internal/sla"
	"github.com/fh/opshub/backend/internal/store"
)

// policyFor 查询"系统等级 × 严重级别"对应的 SLA 策略。
// 接收 *store.Queries，这样建单时可以和插入工单放在同一个事务里。
func policyFor(ctx context.Context, q *store.Queries, level int, sev sla.Severity) (sla.Policy, error) {
	p, err := q.GetSLAPolicy(ctx, store.GetSLAPolicyParams{Level: int16(level), Severity: string(sev)})
	if err != nil {
		return sla.Policy{}, store.MapError(err, fmt.Sprintf("sla policy level=%d severity=%s", level, sev))
	}
	return sla.Policy{
		Response: time.Duration(p.ResponseMinutes) * time.Minute,
		Resolve:  time.Duration(p.ResolveMinutes) * time.Minute,
	}, nil
}
