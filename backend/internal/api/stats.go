package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/service"
)

type statsHandler struct {
	svc *service.Stats
}

func (h statsHandler) register(r *gin.RouterGroup) {
	r.GET("/stats/sla", h.sla)
}

// sla 支持 ?group_by=system|vendor&from=<RFC3339>&to=<RFC3339>，例如 from=2026-10-30T00:00:00+08:00。
func (h statsHandler) sla(c *gin.Context) {
	from, ok := queryTime(c, "from")
	if !ok {
		return
	}
	to, ok := queryTime(c, "to")
	if !ok {
		return
	}
	v, err := h.svc.SLA(c.Request.Context(), service.StatsQuery{GroupBy: c.Query("group_by"), From: from, To: to})
	respond(c, http.StatusOK, v, err)
}

// queryTime 解析可选的 RFC3339 时间参数；未提供时返回零值。失败时已写好 400 响应。
func queryTime(c *gin.Context, key string) (time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(c, fmt.Errorf("%w: %s=%q 不是 RFC3339 时间，例如 2026-10-30T00:00:00+08:00", apperr.ErrValidation, key, raw))
		return time.Time{}, false
	}
	return t, true
}
