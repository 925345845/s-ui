package api

import (
	"encoding/json"
	"net/http"

	"github.com/Hhz0823/1s-ui/agent"
	"github.com/Hhz0823/1s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) StartRelayFill(c *gin.Context, actor string) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<10)
	var request service.RelayCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := a.ConfigService.StartRelayFill(request, actor, getHostname(c))
	jsonObj(c, result, err)
}

func (a *ApiService) GetRelayFill(c *gin.Context, actor string) {
	jsonObj(c, service.GetRelayFillStatus(actor, c.Query("request_id")), nil)
}

func (a *ApiService) StopRelayFill(c *gin.Context, actor string) {
	var request struct {
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.StopRelayFill(actor, request.RequestID)
	jsonObj(c, result, err)
}

func (a *ApiService) StartAgentRelayFill(c *gin.Context) {
	id, err := parseAgentNodeID(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<10)
	var request service.RelayCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	node, err := a.AgentService.Get(id)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	actor := GetLoginUser(c)
	payload := service.RemoteRelayCreateRequest{Request: request, Actor: actor, PublicHost: managedNodePublicHost(node)}
	response, err := a.AgentService.DispatchRPC(id, agent.RPCMethodRelayFillStart, payload, actor)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	var result service.RelayFillStatus
	err = json.Unmarshal(response.Payload, &result)
	jsonObj(c, result, err)
}

func (a *ApiService) AgentRelayFillControl(c *gin.Context, stop bool) {
	id, err := parseAgentNodeID(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	request := service.RemoteRelayFillControl{RequestID: c.Query("request_id")}
	method := agent.RPCMethodRelayFillStatus
	if stop {
		if err := c.ShouldBindJSON(&request); err != nil {
			jsonObj(c, nil, err)
			return
		}
		method = agent.RPCMethodRelayFillStop
	}
	request.Actor = GetLoginUser(c)
	response, err := a.AgentService.DispatchRPC(id, method, request, request.Actor)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	var result *service.RelayFillStatus
	err = json.Unmarshal(response.Payload, &result)
	jsonObj(c, result, err)
}
