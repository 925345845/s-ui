package api

import (
	"net/http"
	"strings"

	"github.com/Hhz0823/1s-ui/agent"
	"github.com/Hhz0823/1s-ui/service"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	service.AgentService
}

func NewAgentHandler(group *gin.RouterGroup) {
	handler := &AgentHandler{}
	group.POST("/heartbeat", handler.Heartbeat)
}

func (h *AgentHandler) Heartbeat(c *gin.Context) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") {
		c.JSON(http.StatusUnauthorized, Msg{Success: false, Msg: "missing agent authorization"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var report agent.Report
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, Msg{Success: false, Msg: "invalid agent report"})
		return
	}
	if err := h.AgentService.Heartbeat(strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), getRemoteIp(c), report); err != nil {
		c.JSON(http.StatusUnauthorized, Msg{Success: false, Msg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Msg{Success: true})
}
