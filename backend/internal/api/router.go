// Package api 是 HTTP 层：解析请求、调用 service、把结果和错误翻译成 HTTP 响应。
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/service"
)

// Pinger 是健康检查唯一需要的能力。*pgxpool.Pool 天然满足它，测试里可以换成假的。
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps 汇总路由需要的全部依赖，由 main 组装后传入。
type Deps struct {
	DB      Pinger
	CMDB    *service.CMDB
	Alerts  *service.Alerts
	Tickets *service.Tickets
}

func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", healthz(d.DB))

	apiGroup := r.Group("/api")
	cmdbHandler{svc: d.CMDB}.register(apiGroup)
	alertHandler{svc: d.Alerts}.register(apiGroup)
	ticketHandler{svc: d.Tickets}.register(apiGroup)
	return r
}

// healthz 在数据库可达时返回 200，否则 503。
func healthz(db Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
