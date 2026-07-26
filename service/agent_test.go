package service

import (
	"path/filepath"
	"testing"

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
	report := agent.Report{Hostname: "vps-1", OS: "linux", Arch: "amd64", AgentVersion: "test", CPUPercent: 12.5}
	if err := service.Heartbeat(enrollment.Token, "203.0.113.10", report); err != nil {
		t.Fatal(err)
	}
	nodes, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || !nodes[0].Online || nodes[0].Report.Hostname != "vps-1" {
		t.Fatalf("unexpected node status: %#v", nodes)
	}

	rotated, err := service.Rotate(enrollment.Node.Id)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == enrollment.Token {
		t.Fatal("rotated token did not change")
	}
	if err := service.Heartbeat(enrollment.Token, "203.0.113.10", report); err == nil {
		t.Fatal("old agent token remained valid after rotation")
	}
	if err := service.Heartbeat(rotated.Token, "203.0.113.10", report); err != nil {
		t.Fatalf("rotated token failed: %v", err)
	}
}

func TestAgentNameValidation(t *testing.T) {
	for _, name := range []string{"", "\n", string(make([]byte, 81))} {
		if _, err := normalizeAgentName(name); err == nil {
			t.Fatalf("accepted invalid agent name %q", name)
		}
	}
}
