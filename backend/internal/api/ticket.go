package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/service"
)

type ticketHandler struct {
	svc *service.Tickets
}

func (h ticketHandler) register(r *gin.RouterGroup) {
	r.GET("/tickets", h.list)
	r.GET("/tickets/:id", h.get)
}

func (h ticketHandler) list(c *gin.Context) {
	v, err := h.svc.List(c.Request.Context())
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
