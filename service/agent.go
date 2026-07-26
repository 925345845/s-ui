package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Hhz0823/1s-ui/agent"
	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/Hhz0823/1s-ui/util/common"
)

const agentOnlineWindow = 45 * time.Second

type AgentNodeView struct {
	Id        uint         `json:"id"`
	Name      string       `json:"name"`
	CreatedAt int64        `json:"created_at"`
	LastSeen  int64        `json:"last_seen"`
	RemoteIP  string       `json:"remote_ip"`
	Version   string       `json:"version"`
	Online    bool         `json:"online"`
	Report    agent.Report `json:"report"`
}

type AgentEnrollment struct {
	Node  AgentNodeView `json:"node"`
	Token string        `json:"token"`
}

type AgentService struct{}

func (s *AgentService) List() ([]AgentNodeView, error) {
	var nodes []model.AgentNode
	if err := database.GetDB().Order("name ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]AgentNodeView, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, agentNodeView(node, now))
	}
	return result, nil
}

func (s *AgentService) Create(name string) (*AgentEnrollment, error) {
	name, err := normalizeAgentName(name)
	if err != nil {
		return nil, err
	}
	token, hash, err := newAgentToken()
	if err != nil {
		return nil, err
	}
	node := model.AgentNode{Name: name, TokenHash: hash, CreatedAt: time.Now().Unix(), Report: json.RawMessage(`{}`)}
	if err := database.GetDB().Create(&node).Error; err != nil {
		return nil, err
	}
	view := agentNodeView(node, time.Now())
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
	view := agentNodeView(node, time.Now())
	return &AgentEnrollment{Node: view, Token: token}, nil
}

func (s *AgentService) Delete(id uint) error {
	result := database.GetDB().Delete(&model.AgentNode{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("agent node not found")
	}
	return nil
}

func (s *AgentService) Heartbeat(token, remoteIP string, report agent.Report) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return common.NewError("missing agent token")
	}
	if len(report.Hostname) > 255 || len(report.AgentVersion) > 64 || len(report.OS) > 32 || len(report.Arch) > 32 {
		return common.NewError("invalid agent report")
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	hash := hashAgentToken(token)
	result := database.GetDB().Model(&model.AgentNode{}).Where("token_hash = ?", hash).Updates(map[string]interface{}{
		"last_seen": time.Now().Unix(),
		"remote_ip": strings.TrimSpace(remoteIP),
		"version":   report.AgentVersion,
		"report":    payload,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("invalid agent token")
	}
	return nil
}

func agentNodeView(node model.AgentNode, now time.Time) AgentNodeView {
	view := AgentNodeView{
		Id: node.Id, Name: node.Name, CreatedAt: node.CreatedAt, LastSeen: node.LastSeen,
		RemoteIP: node.RemoteIP, Version: node.Version,
	}
	if len(node.Report) > 0 {
		_ = json.Unmarshal(node.Report, &view.Report)
	}
	view.Online = node.LastSeen > 0 && now.Sub(time.Unix(node.LastSeen, 0)) <= agentOnlineWindow
	return view
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
