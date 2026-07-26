package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	stdnet "net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Hhz0823/1s-ui/config"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type ClientConfig struct {
	PanelURL string
	Token    string
	Interval time.Duration
	Insecure bool
}

func Run(ctx context.Context, cfg ClientConfig) error {
	client, endpoint, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}
	if err := sendHeartbeat(ctx, client, endpoint, cfg.Token); err != nil {
		return err
	}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := sendHeartbeat(ctx, client, endpoint, cfg.Token); err != nil {
				fmt.Fprintf(os.Stderr, "agent heartbeat failed: %v\n", err)
			}
		}
	}
}

func SendOnce(ctx context.Context, cfg ClientConfig) error {
	client, endpoint, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}
	return sendHeartbeat(ctx, client, endpoint, cfg.Token)
}

func newHTTPClient(cfg ClientConfig) (*http.Client, string, error) {
	if cfg.Interval < 5*time.Second || cfg.Interval > 5*time.Minute {
		return nil, "", fmt.Errorf("agent interval must be between 5s and 5m")
	}
	panel, err := url.Parse(strings.TrimSpace(cfg.PanelURL))
	if err != nil || panel.Host == "" || (panel.Scheme != "http" && panel.Scheme != "https") {
		return nil, "", fmt.Errorf("invalid panel URL")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, "", fmt.Errorf("agent token is required")
	}
	panel.RawQuery = ""
	panel.Fragment = ""
	panel.Path = strings.TrimRight(panel.Path, "/") + "/agent/v1/heartbeat"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.Insecure} //nolint:gosec
	return &http.Client{Timeout: 15 * time.Second, Transport: transport}, panel.String(), nil
}

func sendHeartbeat(ctx context.Context, client *http.Client, endpoint, token string) error {
	report := CollectReport()
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "1s-ui-agent/"+config.GetVersion())
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("panel returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if len(body) > 0 && json.Unmarshal(body, &result) == nil && !result.Success {
		return fmt.Errorf("panel rejected heartbeat: %s", result.Msg)
	}
	return nil
}

func CollectReport() Report {
	report := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, AgentVersion: config.GetVersion()}
	report.Hostname, _ = os.Hostname()
	report.Uptime, _ = host.Uptime()
	if values, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(values) > 0 {
		report.CPUPercent = values[0]
	}
	if value, err := mem.VirtualMemory(); err == nil {
		report.Memory = ResourceUsage{Used: value.Used, Total: value.Total}
	}
	if value, err := disk.Usage("/"); err == nil {
		report.Disk = ResourceUsage{Used: value.Used, Total: value.Total}
	}
	if values, err := psnet.IOCounters(false); err == nil && len(values) > 0 {
		report.Network = NetworkUsage{Sent: values[0].BytesSent, Recv: values[0].BytesRecv}
	}
	if value, err := load.Avg(); err == nil {
		report.Load = LoadAverage{Load1: value.Load1, Load5: value.Load5, Load15: value.Load15}
	}
	report.IPv4, report.IPv6 = localAddresses()
	report.Cores = detectCoreStatus()
	return report
}

func localAddresses() ([]string, []string) {
	var ipv4, ipv6 []string
	interfaces, _ := stdnet.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&stdnet.FlagUp == 0 || iface.Flags&stdnet.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, err := stdnet.ParseCIDR(address.String())
			if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip.To4() != nil {
				ipv4 = append(ipv4, ip.String())
			} else {
				ipv6 = append(ipv6, ip.String())
			}
		}
	}
	return ipv4, ipv6
}

func detectCoreStatus() CoreStatus {
	status := CoreStatus{}
	processes, _ := process.Processes()
	for _, item := range processes {
		name, err := item.Name()
		if err != nil {
			continue
		}
		switch strings.ToLower(name) {
		case "sing-box", "sing-box.exe":
			status.SingBoxRunning = true
		case "xray", "xray.exe":
			status.XrayRunning = true
		}
	}
	if version, err := xrayVersion(); err == nil {
		status.XrayVersion = version
	}
	return status
}

func xrayVersion() (string, error) {
	path := config.GetXrayPath()
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", fmt.Errorf("xray binary unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return line, nil
}
