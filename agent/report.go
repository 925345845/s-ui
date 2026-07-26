package agent

type ResourceUsage struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

type NetworkUsage struct {
	Sent uint64 `json:"sent"`
	Recv uint64 `json:"recv"`
}

type LoadAverage struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type CoreStatus struct {
	SingBoxRunning bool   `json:"singbox_running"`
	XrayRunning    bool   `json:"xray_running"`
	XrayVersion    string `json:"xray_version,omitempty"`
}

type Report struct {
	Hostname     string        `json:"hostname"`
	OS           string        `json:"os"`
	Arch         string        `json:"arch"`
	AgentVersion string        `json:"agent_version"`
	Uptime       uint64        `json:"uptime"`
	CPUPercent   float64       `json:"cpu_percent"`
	Memory       ResourceUsage `json:"memory"`
	Disk         ResourceUsage `json:"disk"`
	Network      NetworkUsage  `json:"network"`
	Load         LoadAverage   `json:"load"`
	IPv4         []string      `json:"ipv4,omitempty"`
	IPv6         []string      `json:"ipv6,omitempty"`
	Cores        CoreStatus    `json:"cores"`
}
