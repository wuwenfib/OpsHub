package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/service"
	"github.com/fh/opshub/backend/internal/ticket"
)

type ticketHandler struct {
	svc *service.Tickets
}

func (h ticketHandler) register(r *gin.RouterGroup) {
	r.GET("/tickets", h.list)
	r.POST("/tickets", h.create)
	r.GET("/tickets/:id", h.get)
	for _, act := range ticket.AllActions {
		r.POST("/tickets/:id/"+string(act), h.do(act))
	}
}

// list 支持 ?status=new&system=GOV-APPLY&overdue=true 筛选。
func (h ticketHandler) list(c *gin.Context) {
	v, err := h.svc.List(c.Request.Context(), service.ListFilter{
		Status:  c.Query("status"),
		System:  c.Query("system"),
		Overdue: c.Query("overdue") == "true",
	})
	respond(c, http.StatusOK, v, err)
}

func (h ticketHandler) get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	v, err := h.svc.Get(c.Request.Context(), id)
	respond(c, http.StatusOK, v, err)
}

func (h ticketHandler) create(c *gin.Context) {
	var in service.ManualInput
	if !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.CreateManual(c.Request.Context(), in)
	respond(c, http.StatusCreated, v, err)
}

// do 返回执行某个状态操作的 handler；成功时返回更新后的工单详情。
func (h ticketHandler) do(act ticket.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := pathID(c)
		var in service.ActionInput
		if !ok || !bindJSON(c, &in) {
			return
		}
		v, err := h.svc.Do(c.Request.Context(), id, act, in)
		respond(c, http.StatusOK, v, err)
	}
}
