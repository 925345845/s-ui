package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
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
	"sync"
	"time"

	"github.com/Hhz0823/1s-ui/config"
	"github.com/coder/websocket"
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
	// PreferWS enables WebSocket long-connection mode (falls back to HTTP).
	PreferWS bool
}

var (
	lastNetMu   sync.Mutex
	lastNetSent uint64
	lastNetRecv uint64
	lastNetAt   time.Time
)

func Run(ctx context.Context, cfg ClientConfig) error {
	if cfg.PreferWS {
		if err := runWebSocket(ctx, cfg); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		} else {
			fmt.Fprintf(os.Stderr, "agent websocket failed, falling back to HTTP: %v\n", err)
		}
	}
	return runHTTP(ctx, cfg)
}

func SendOnce(ctx context.Context, cfg ClientConfig) error {
	client, endpoint, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}
	_, err = sendHeartbeat(ctx, client, endpoint, cfg.Token)
	return err
}

func runHTTP(ctx context.Context, cfg ClientConfig) error {
	client, endpoint, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}
	if _, err := sendHeartbeat(ctx, client, endpoint, cfg.Token); err != nil {
		return err
	}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := sendHeartbeat(ctx, client, endpoint, cfg.Token); err != nil {
				fmt.Fprintf(os.Stderr, "agent heartbeat failed: %v\n", err)
			}
		}
	}
}

func runWebSocket(ctx context.Context, cfg ClientConfig) error {
	if cfg.Interval < 5*time.Second || cfg.Interval > 5*time.Minute {
		return fmt.Errorf("agent interval must be between 5s and 5m")
	}
	panel, err := url.Parse(strings.TrimSpace(cfg.PanelURL))
	if err != nil || panel.Host == "" || (panel.Scheme != "http" && panel.Scheme != "https") {
		return fmt.Errorf("invalid panel URL")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("agent token is required")
	}
	wsURL := *panel
	wsURL.RawQuery = ""
	wsURL.Fragment = ""
	if panel.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = strings.TrimRight(panel.Path, "/") + "/agent/v1/ws"

	dialOpts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + cfg.Token},
			"User-Agent":    []string{"1s-ui-agent/" + config.GetVersion()},
		},
	}
	if cfg.Insecure {
		dialOpts.HTTPClient = &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec
			},
		}
	}

	conn, _, err := websocket.Dial(ctx, wsURL.String(), dialOpts)
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")
	conn.SetReadLimit(512 << 10)

	var writeMu sync.Mutex
	write := func(v interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeJSON(ctx, conn, v)
	}

	// Initial report
	if err := wsSendReport(ctx, write); err != nil {
		return err
	}

	interval := cfg.Interval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	reportNow := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	restartAgent := make(chan struct{}, 1)

	var termMu sync.Mutex
	terminals := map[string]*localTerminal{}
	closeAllTerminals := func() {
		termMu.Lock()
		defer termMu.Unlock()
		for id, t := range terminals {
			t.Close()
			delete(terminals, id)
		}
	}
	defer closeAllTerminals()

	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			var msg struct {
				Type            string                 `json:"type"`
				ID              string                 `json:"id"`
				Command         string                 `json:"command"`
				Args            map[string]interface{} `json:"args"`
				IntervalSeconds int                    `json:"interval_seconds"`
				ServerTime      int64                  `json:"server_time"`
				Data            string                 `json:"data"`
				Cols            int                    `json:"cols"`
				Rows            int                    `json:"rows"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case MsgTypePong, MsgTypeAck, MsgTypeConfig:
				if msg.IntervalSeconds >= 5 && msg.IntervalSeconds <= 300 {
					newInterval := time.Duration(msg.IntervalSeconds) * time.Second
					if newInterval != interval {
						interval = newInterval
						ticker.Reset(interval)
					}
				}
			case MsgTypePing:
				_ = write(map[string]interface{}{"type": MsgTypePong, "time": time.Now().Unix()})
			case MsgTypeCommand:
				cmd := Command{ID: msg.ID, Type: msg.Command, Args: msg.Args}
				// Apply set_interval immediately on the loop side.
				if cmd.Type == CmdSetInterval && cmd.Args != nil {
					if sec, ok := numberArg(cmd.Args["seconds"]); ok && sec >= 5 && sec <= 300 {
						newInterval := time.Duration(sec) * time.Second
						interval = newInterval
						ticker.Reset(interval)
					}
				}
				if cmd.Type == CmdReportNow {
					select {
					case reportNow <- struct{}{}:
					default:
					}
				}
				result := HandleCommand(ctx, cmd)
				_ = write(map[string]interface{}{
					"type":       MsgTypeCommandResult,
					"id":         result.ID,
					"command":    result.Type,
					"ok":         result.OK,
					"output":     result.Output,
					"error":      result.Error,
					"code":       result.Code,
					"elapsed_ms": result.Elapsed,
					"time":       time.Now().Unix(),
				})
				if cmd.Type == CmdRestartAgent || result.Code == 77 {
					select {
					case restartAgent <- struct{}{}:
					default:
					}
				}
			case MsgTypeTerminalOpen:
				cols := uint16(msg.Cols)
				rows := uint16(msg.Rows)
				if cols == 0 {
					cols = 80
				}
				if rows == 0 {
					rows = 24
				}
				term, err := startLocalTerminal(msg.ID, cols, rows)
				if err != nil {
					_ = write(map[string]interface{}{
						"type":  MsgTypeTerminalClosed,
						"id":    msg.ID,
						"error": err.Error(),
					})
					continue
				}
				termMu.Lock()
				if old, ok := terminals[msg.ID]; ok {
					old.Close()
				}
				terminals[msg.ID] = term
				termMu.Unlock()
				_ = write(map[string]interface{}{"type": MsgTypeTerminalOpened, "id": msg.ID})
				go pumpTerminalOutput(ctx, term, write)
			case MsgTypeTerminalInput:
				raw, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					raw = []byte(msg.Data)
				}
				termMu.Lock()
				term := terminals[msg.ID]
				termMu.Unlock()
				if term != nil {
					_, _ = term.Write(raw)
				}
			case MsgTypeTerminalResize:
				termMu.Lock()
				term := terminals[msg.ID]
				termMu.Unlock()
				if term != nil {
					_ = term.Resize(uint16(msg.Cols), uint16(msg.Rows))
				}
			case MsgTypeTerminalClose:
				termMu.Lock()
				term := terminals[msg.ID]
				delete(terminals, msg.ID)
				termMu.Unlock()
				if term != nil {
					term.Close()
				}
				_ = write(map[string]interface{}{"type": MsgTypeTerminalClosed, "id": msg.ID})
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-restartAgent:
			// Allow the command_result frame to flush, then exit for systemd restart.
			time.Sleep(300 * time.Millisecond)
			os.Exit(0)
			return nil
		case <-reportNow:
			if err := wsSendReport(ctx, write); err != nil {
				return err
			}
		case <-ticker.C:
			if err := wsSendReport(ctx, write); err != nil {
				return err
			}
		}
	}
}

func numberArg(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func wsSendReport(ctx context.Context, write func(interface{}) error) error {
	report := CollectReport()
	report.ConnMode = "ws"
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return write(map[string]interface{}{
		"type":    MsgTypeReport,
		"payload": json.RawMessage(payload),
		"time":    time.Now().Unix(),
	})
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data)
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

func sendHeartbeat(ctx context.Context, client *http.Client, endpoint, token string) (*HeartbeatResponse, error) {
	report := CollectReport()
	report.ConnMode = "http"
	payload, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "1s-ui-agent/"+config.GetVersion())
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("panel returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Success bool               `json:"success"`
		Msg     string             `json:"msg"`
		Obj     *HeartbeatResponse `json:"obj"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &result)
		if !result.Success && result.Msg != "" {
			return nil, fmt.Errorf("panel rejected heartbeat: %s", result.Msg)
		}
		return result.Obj, nil
	}
	return nil, nil
}

func CollectReport() Report {
	report := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, AgentVersion: config.GetVersion()}
	report.Hostname, _ = os.Hostname()
	report.Uptime, _ = host.Uptime()
	if count, err := cpu.Counts(true); err == nil {
		report.CPUCores = count
	}
	if values, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(values) > 0 {
		report.CPUPercent = values[0]
	}
	if value, err := mem.VirtualMemory(); err == nil {
		report.Memory = ResourceUsage{Used: value.Used, Total: value.Total}
	}
	if value, err := mem.SwapMemory(); err == nil {
		report.Swap = ResourceUsage{Used: value.Used, Total: value.Total}
	}
	if value, err := disk.Usage("/"); err == nil {
		report.Disk = ResourceUsage{Used: value.Used, Total: value.Total}
	}
	if values, err := psnet.IOCounters(false); err == nil && len(values) > 0 {
		sent, recv := values[0].BytesSent, values[0].BytesRecv
		report.Network = NetworkUsage{Sent: sent, Recv: recv}
		report.NetRate = estimateNetRate(sent, recv)
	}
	if value, err := load.Avg(); err == nil {
		report.Load = LoadAverage{Load1: value.Load1, Load5: value.Load5, Load15: value.Load15}
	}
	if procs, err := process.Processes(); err == nil {
		report.ProcessCount = len(procs)
	}
	report.IPv4, report.IPv6 = localAddresses()
	report.Cores = detectCoreStatus()
	return report
}

func estimateNetRate(sent, recv uint64) NetworkUsage {
	lastNetMu.Lock()
	defer lastNetMu.Unlock()
	now := time.Now()
	var rate NetworkUsage
	if !lastNetAt.IsZero() && sent >= lastNetSent && recv >= lastNetRecv {
		elapsed := now.Sub(lastNetAt).Seconds()
		if elapsed > 0.2 {
			rate.Sent = uint64(float64(sent-lastNetSent) / elapsed)
			rate.Recv = uint64(float64(recv-lastNetRecv) / elapsed)
		}
	}
	lastNetSent, lastNetRecv, lastNetAt = sent, recv, now
	return rate
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
