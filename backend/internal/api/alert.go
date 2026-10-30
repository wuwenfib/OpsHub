package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/alert"
	"github.com/fh/opshub/backend/internal/service"
)

type alertHandler struct {
	svc *service.Alerts
}

func (h alertHandler) register(r *gin.RouterGroup) {
	r.POST("/alerts/webhook", h.webhook)
}

// webhook 接收 Alertmanager 推送。只有请求体无法解析时返回 400；
// 单条告警处理失败不影响响应码（记在结果的 failed 里），避免 Alertmanager 整批重试。
func (h alertHandler) webhook(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		writeError(c, err)
		return
	}
	p, err := alert.Parse(body)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.svc.Handle(c.Request.Context(), p))
}
