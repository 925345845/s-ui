package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Hhz0823/1s-ui/agent"
	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/Hhz0823/1s-ui/util/common"
)

const (
	agentOnlineWindow    = 45 * time.Second
	agentHistoryLimit    = 120
	agentDefaultInterval = 15
)

type AgentNodeView struct {
	Id           uint                `json:"id"`
	Name         string              `json:"name"`
	CreatedAt    int64               `json:"created_at"`
	LastSeen     int64               `json:"last_seen"`
	RemoteIP     string              `json:"remote_ip"`
	PublicHost   string              `json:"public_host"`
	Version      string              `json:"version"`
	Online       bool                `json:"online"`
	ConnMode     string              `json:"conn_mode,omitempty"`
	Report       agent.Report        `json:"report"`
	History      []AgentMetricSample `json:"history,omitempty"`
	WSConnected  bool                `json:"ws_connected,omitempty"`
	Controllable bool                `json:"controllable"`
	Commands     []agentCommandLog   `json:"commands,omitempty"`
	Latency      AgentLatencyView    `json:"latency"`
	Managed      bool                `json:"managed"`
}

type AgentEnrollment struct {
	Node  AgentNodeView `json:"node"`
	Token string        `json:"token"`
}

type AgentMetricSample struct {
	Time       int64   `json:"time"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	NetSent    uint64  `json:"net_sent_rate"`
	NetRecv    uint64  `json:"net_recv_rate"`
}

type AgentService struct {
	capacityProvider func() (int, uint64)
}

var (
	agentHistoryMu sync.RWMutex
	agentHistory   = map[uint][]AgentMetricSample{}
	agentWSMu      sync.RWMutex
	agentWSLive    = map[uint]bool{}
)

func (s *AgentService) List() ([]AgentNodeView, error) {
	var nodes []model.AgentNode
	if err := database.GetDB().Order("name ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]AgentNodeView, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, agentNodeView(node, now, false))
	}
	return result, nil
}

func (s *AgentService) Get(id uint) (*AgentNodeView, error) {
	var node model.AgentNode
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, err
	}
	view := agentNodeView(node, time.Now(), true)
	view.Commands = getAgentCommandLogs(id)
	return &view, nil
}

func (s *AgentService) Create(name string) (*AgentEnrollment, error) {
	name, err := normalizeAgentName(name)
	if err != nil {
		return nil, err
	}
	cpuCount, memTotal := currentClusterCapacity()
	if s.capacityProvider != nil {
		cpuCount, memTotal = s.capacityProvider()
	}
	if !meetsClusterRequirements(cpuCount, memTotal) {
		return nil, common.NewErrorf(
			"server monitoring requires at least %d CPU cores and 2 GiB memory; current host: %d CPU cores / %.2f GiB",
			MinClusterCPUCores,
			cpuCount,
			float64(memTotal)/(1024*1024*1024),
		)
	}
	token, hash, err := newAgentToken()
	if err != nil {
		return nil, err
	}
	node := model.AgentNode{Name: name, TokenHash: hash, CreatedAt: time.Now().Unix(), Report: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&node).Error; err != nil {
		return nil, err
	}
	invalidateHostRequirementsCache()
	view := agentNodeView(node, time.Now(), false)
	return &AgentEnrollment{Node: view, Token: token}, nil
}

func (s *AgentService) Rotate(id uint) (*AgentEnrollment, error) {
	var node model.AgentNode
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, err
	}
	token, hash, err := newAgentToken()
	if err != nil {
		return nil, err
	}
	if err := database.GetDB().Model(&node).Updates(map[string]interface{}{"token_hash": hash, "last_seen": 0}).Error; err != nil {
		return nil, err
	}
	node.TokenHash = hash
	node.LastSeen = 0
	view := agentNodeView(node, time.Now(), false)
	return &AgentEnrollment{Node: view, Token: token}, nil
}

func (s *AgentService) Update(id uint, name, publicHost string) (*AgentNodeView, error) {
	name, err := normalizeAgentName(name)
	if err != nil {
		return nil, err
	}
	publicHost, err = normalizeAgentPublicHost(publicHost)
	if err != nil {
		return nil, err
	}
	var node model.AgentNode
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, err
	}
	if err := database.GetDB().Model(&node).Updates(map[string]interface{}{
		"name": name, "public_host": publicHost,
	}).Error; err != nil {
		return nil, err
	}
	node.Name = name
	node.PublicHost = publicHost
	view := agentNodeView(node, time.Now(), true)
	return &view, nil
}

func (s *AgentService) Delete(id uint) error {
	result := database.GetDB().Delete(&model.AgentNode{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("agent node not found")
	}
	invalidateHostRequirementsCache()
	agentHistoryMu.Lock()
	delete(agentHistory, id)
	agentHistoryMu.Unlock()
	agentWSMu.Lock()
	delete(agentWSLive, id)
	agentWSMu.Unlock()
	agentCmdLogMu.Lock()
	delete(agentCmdLogs, id)
	agentCmdLogMu.Unlock()
	agentLatencyMu.Lock()
	delete(agentLatencies, id)
	agentLatencyMu.Unlock()
	return nil
}

func (s *AgentService) Heartbeat(token, remoteIP string, report agent.Report) (*agent.HeartbeatResponse, error) {
	nodeID, err := s.applyReport(token, remoteIP, report)
	if err != nil {
		return nil, err
	}
	_ = nodeID
	return &agent.HeartbeatResponse{
		ServerTime:      time.Now().Unix(),
		IntervalSeconds: agentDefaultInterval,
	}, nil
}

func (s *AgentService) applyReport(token, remoteIP string, report agent.Report) (uint, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, common.NewError("missing agent token")
	}
	if len(report.Hostname) > 255 || len(report.AgentVersion) > 64 || len(report.OS) > 32 || len(report.Arch) > 32 {
		return 0, common.NewError("invalid agent report")
	}
	if report.ConnMode != "" && report.ConnMode != "http" && report.ConnMode != "ws" {
		return 0, common.NewError("invalid agent conn_mode")
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return 0, err
	}
	hash := hashAgentToken(token)
	var node model.AgentNode
	if err := database.GetDB().Where("token_hash = ?", hash).First(&node).Error; err != nil {
		return 0, common.NewError("invalid agent token")
	}
	result := database.GetDB().Model(&node).Updates(map[string]interface{}{
		"last_seen": time.Now().Unix(),
		"remote_ip": strings.TrimSpace(remoteIP),
		"version":   report.AgentVersion,
		"report":    payload,
	})
	if result.Error != nil {
		return 0, result.Error
	}
	appendAgentHistory(node.Id, report)
	return node.Id, nil
}

func (s *AgentService) AuthenticateToken(token string) (uint, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, common.NewError("missing agent token")
	}
	hash := hashAgentToken(token)
	var node model.AgentNode
	if err := database.GetDB().Select("id").Where("token_hash = ?", hash).First(&node).Error; err != nil {
		return 0, common.NewError("invalid agent token")
	}
	return node.Id, nil
}

func (s *AgentService) SetWSConnected(id uint, connected bool) {
	agentWSMu.Lock()
	defer agentWSMu.Unlock()
	if connected {
		agentWSLive[id] = true
	} else {
		delete(agentWSLive, id)
	}
}

func appendAgentHistory(id uint, report agent.Report) {
	sample := AgentMetricSample{
		Time:       time.Now().Unix(),
		CPUPercent: report.CPUPercent,
		NetSent:    report.NetRate.Sent,
		NetRecv:    report.NetRate.Recv,
	}
	if report.Memory.Total > 0 {
		sample.MemPercent = float64(report.Memory.Used) * 100 / float64(report.Memory.Total)
	}
	agentHistoryMu.Lock()
	defer agentHistoryMu.Unlock()
	history := append(agentHistory[id], sample)
	if len(history) > agentHistoryLimit {
		history = history[len(history)-agentHistoryLimit:]
	}
	agentHistory[id] = history
}

func getAgentHistory(id uint) []AgentMetricSample {
	agentHistoryMu.RLock()
	defer agentHistoryMu.RUnlock()
	src := agentHistory[id]
	if len(src) == 0 {
		return nil
	}
	out := make([]AgentMetricSample, len(src))
	copy(out, src)
	return out
}

func agentNodeView(node model.AgentNode, now time.Time, withHistory bool) AgentNodeView {
	view := AgentNodeView{
		Id: node.Id, Name: node.Name, CreatedAt: node.CreatedAt, LastSeen: node.LastSeen,
		RemoteIP: node.RemoteIP, PublicHost: node.PublicHost, Version: node.Version,
	}
	if len(node.Report) > 0 {
		_ = json.Unmarshal(node.Report, &view.Report)
		view.ConnMode = view.Report.ConnMode
	}
	view.Online = node.LastSeen > 0 && now.Sub(time.Unix(node.LastSeen, 0)) <= agentOnlineWindow
	agentHubMu.RLock()
	_, live := agentSessions[node.Id]
	agentHubMu.RUnlock()
	view.WSConnected = live
	view.Controllable = live
	view.Managed = live && view.Report.Panel.ControlAvailable
	view.Latency = getAgentLatency(node.Id)
	if withHistory {
		view.History = getAgentHistory(node.Id)
	}
	return view
}

func normalizeAgentPublicHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 255 || strings.ContainsAny(value, " /?#@\t\r\n") {
		return "", common.NewError("public host must be a hostname or IP address without scheme, path, or port")
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	if strings.Contains(value, ":") && net.ParseIP(value) == nil {
		return "", common.NewError("public host must not include a port")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", common.NewError("public host contains control characters")
		}
	}
	return value, nil
}

func normalizeAgentName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 80 {
		return "", common.NewError("agent name must contain 1-80 characters")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", common.NewError("agent name contains control characters")
		}
	}
	return value, nil
}

func newAgentToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate agent token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashAgentToken(token), nil
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
