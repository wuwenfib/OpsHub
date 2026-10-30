package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/cmdb"
	"github.com/fh/opshub/backend/internal/service"
)

type cmdbHandler struct {
	svc *service.CMDB
}

func (h cmdbHandler) register(r *gin.RouterGroup) {
	r.GET("/vendors", h.listVendors)
	r.POST("/vendors", h.createVendor)
	r.GET("/vendors/:id", h.getVendor)
	r.PUT("/vendors/:id", h.updateVendor)
	r.DELETE("/vendors/:id", h.deleteVendor)

	r.GET("/systems", h.listSystems)
	r.POST("/systems", h.createSystem)
	r.GET("/systems/:code", h.getSystem)
	r.PUT("/systems/:code", h.updateSystem)
	r.DELETE("/systems/:code", h.deleteSystem)
	r.PUT("/systems/:code/hosts", h.setSystemHosts)
	r.PUT("/systems/:code/dependencies", h.setSystemDependencies)

	r.GET("/hosts", h.listHosts)
	r.POST("/hosts", h.createHost)
	r.PUT("/hosts/:id", h.updateHost)
	r.DELETE("/hosts/:id", h.deleteHost)
}

// respond 统一处理"调用 service → 出错走 writeError，否则返回 JSON"。
func respond[T any](c *gin.Context, status int, v T, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(status, v)
}

func respondNoContent(c *gin.Context, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------- 厂商 ----------

func (h cmdbHandler) listVendors(c *gin.Context) {
	v, err := h.svc.ListVendors(c.Request.Context())
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) getVendor(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	v, err := h.svc.GetVendor(c.Request.Context(), id)
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) createVendor(c *gin.Context) {
	var in cmdb.VendorInput
	if !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.CreateVendor(c.Request.Context(), in)
	respond(c, http.StatusCreated, v, err)
}

func (h cmdbHandler) updateVendor(c *gin.Context) {
	id, ok := pathID(c)
	var in cmdb.VendorInput
	if !ok || !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.UpdateVendor(c.Request.Context(), id, in)
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) deleteVendor(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	respondNoContent(c, h.svc.DeleteVendor(c.Request.Context(), id))
}

// ---------- 业务系统 ----------

func (h cmdbHandler) listSystems(c *gin.Context) {
	v, err := h.svc.ListSystems(c.Request.Context())
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) getSystem(c *gin.Context) {
	v, err := h.svc.GetSystem(c.Request.Context(), c.Param("code"))
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) createSystem(c *gin.Context) {
	var in cmdb.SystemInput
	if !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.CreateSystem(c.Request.Context(), in)
	respond(c, http.StatusCreated, v, err)
}

func (h cmdbHandler) updateSystem(c *gin.Context) {
	var in cmdb.SystemInput
	if !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.UpdateSystem(c.Request.Context(), c.Param("code"), in)
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) deleteSystem(c *gin.Context) {
	respondNoContent(c, h.svc.DeleteSystem(c.Request.Context(), c.Param("code")))
}

func (h cmdbHandler) setSystemHosts(c *gin.Context) {
	var body struct {
		HostIDs []int64 `json:"host_ids"`
	}
	if !bindJSON(c, &body) {
		return
	}
	v, err := h.svc.SetSystemHosts(c.Request.Context(), c.Param("code"), body.HostIDs)
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) setSystemDependencies(c *gin.Context) {
	var body struct {
		DependsOn []string `json:"depends_on"`
	}
	if !bindJSON(c, &body) {
		return
	}
	v, err := h.svc.SetSystemDependencies(c.Request.Context(), c.Param("code"), body.DependsOn)
	respond(c, http.StatusOK, v, err)
}

// ---------- 主机 ----------

func (h cmdbHandler) listHosts(c *gin.Context) {
	v, err := h.svc.ListHosts(c.Request.Context())
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) createHost(c *gin.Context) {
	var in cmdb.HostInput
	if !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.CreateHost(c.Request.Context(), in)
	respond(c, http.StatusCreated, v, err)
}

func (h cmdbHandler) updateHost(c *gin.Context) {
	id, ok := pathID(c)
	var in cmdb.HostInput
	if !ok || !bindJSON(c, &in) {
		return
	}
	v, err := h.svc.UpdateHost(c.Request.Context(), id, in)
	respond(c, http.StatusOK, v, err)
}

func (h cmdbHandler) deleteHost(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	respondNoContent(c, h.svc.DeleteHost(c.Request.Context(), id))
}
