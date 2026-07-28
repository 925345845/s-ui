package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Hhz0823/1s-ui/agent"
	"github.com/Hhz0823/1s-ui/database"
)

func TestAgentEnrollmentHeartbeatAndRotation(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "agents.db")); err != nil {
		t.Fatal(err)
	}
	service := AgentService{}
	enrollment, err := service.Create("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Token == "" || enrollment.Node.Name != "edge-1" {
		t.Fatalf("unexpected enrollment: %#v", enrollment)
	}
	report := agent.Report{Hostname: "vps-1", OS: "linux", Arch: "amd64", AgentVersion: "test", CPUPercent: 12.5, ConnMode: "http"}
	resp, err := service.Heartbeat(enrollment.Token, "203.0.113.10", report)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.ServerTime == 0 {
		t.Fatalf("expected heartbeat response, got %#v", resp)
	}
	nodes, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || !nodes[0].Online || nodes[0].Report.Hostname != "vps-1" {
		t.Fatalf("unexpected node status: %#v", nodes)
	}
	if nodes[0].ConnMode != "http" {
		t.Fatalf("expected conn_mode http, got %#v", nodes[0])
	}

	detail, err := service.Get(nodes[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.History) == 0 {
		t.Fatal("expected metric history after heartbeat")
	}

	rotated, err := service.Rotate(enrollment.Node.Id)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == enrollment.Token {
		t.Fatal("rotated token did not change")
	}
	if _, err := service.Heartbeat(enrollment.Token, "203.0.113.10", report); err == nil {
		t.Fatal("old agent token remained valid after rotation")
	}
	if _, err := service.Heartbeat(rotated.Token, "203.0.113.10", report); err != nil {
		t.Fatalf("rotated token failed: %v", err)
	}

	if time.Since(time.Unix(detail.LastSeen, 0)) > agentOnlineWindow {
		t.Fatal("last seen unexpectedly old")
	}
}

func TestAgentNameValidation(t *testing.T) {
	for _, name := range []string{"", "\n", string(make([]byte, 81))} {
		if _, err := normalizeAgentName(name); err == nil {
			t.Fatalf("accepted invalid agent name %q", name)
		}
	}
}
