//go:build !openwrt_lite

package service

import "github.com/Hhz0823/1s-ui/core"

type XrayCapability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Supported   bool   `json:"supported"`
	Shareable   bool   `json:"shareable,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type XraySelfCheck struct {
	Healthy         bool             `json:"healthy"`
	BinaryAvailable bool             `json:"binary_available"`
	Version         string           `json:"version,omitempty"`
	Path            string           `json:"path,omitempty"`
	ConfigValid     bool             `json:"config_valid"`
	HasInbounds     bool             `json:"has_inbounds"`
	Running         bool             `json:"running"`
	Error           string           `json:"error,omitempty"`
	Protocols       []XrayCapability `json:"protocols"`
	Transports      []XrayCapability `json:"transports"`
}

func xrayProtocolCapabilities() []XrayCapability {
	return []XrayCapability{
		{ID: "vless", Name: "VLESS", Supported: true, Shareable: true, Recommended: true},
		{ID: "vmess", Name: "VMess", Supported: true, Shareable: true},
		{ID: "trojan", Name: "Trojan", Supported: true, Shareable: true},
		{ID: "shadowsocks", Name: "Shadowsocks", Supported: true, Shareable: true},
		{ID: "socks", Name: "SOCKS", Supported: true, Shareable: true},
		{ID: "http", Name: "HTTP", Supported: true, Shareable: true},
		{ID: "mixed", Name: "Mixed", Supported: true, Shareable: true},
		{ID: "hysteria2", Name: "Hysteria2", Supported: true, Shareable: true},
		{ID: "dokodemo-door", Name: "Dokodemo-door", Supported: true},
		{ID: "wireguard", Name: "WireGuard inbound", Supported: false, Reason: "requires peer and kernel routing lifecycle management"},
		{ID: "tun", Name: "TUN inbound", Supported: false, Reason: "requires privileged interface and route lifecycle management"},
	}
}

func xrayTransportCapabilities() []XrayCapability {
	return []XrayCapability{
		{ID: "xhttp", Name: "XHTTP", Supported: true, Recommended: true},
		{ID: "raw", Name: "RAW", Supported: true},
		{ID: "kcp", Name: "mKCP", Supported: true},
		{ID: "grpc", Name: "gRPC", Supported: true},
		{ID: "ws", Name: "WebSocket", Supported: true},
		{ID: "httpupgrade", Name: "HTTPUpgrade", Supported: true},
		{ID: "hysteria", Name: "Hysteria2 transport", Supported: true},
		{ID: "http", Name: "HTTP/2", Supported: false, Reason: "removed by Xray-core; use XHTTP stream-one"},
		{ID: "quic", Name: "QUIC", Supported: false, Reason: "removed by Xray-core; use XHTTP stream-one H3"},
	}
}

func (s *ConfigService) CheckXray() XraySelfCheck {
	result := XraySelfCheck{
		Protocols:  xrayProtocolCapabilities(),
		Transports: xrayTransportCapabilities(),
	}
	if xrayPtr == nil {
		xrayPtr = core.NewXrayRuntime()
	}
	status := xrayPtr.Status()
	result.Path, _ = status["path"].(string)
	result.Running, _ = status["running"].(bool)
	result.HasInbounds, _ = s.HasXrayInbounds()

	version, err := xrayPtr.Version()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.BinaryAvailable = true
	result.Version = version

	rawConfig, err := s.GetXrayConfig()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if err = xrayPtr.Validate(*rawConfig); err != nil {
		result.Error = err.Error()
		return result
	}
	result.ConfigValid = true
	result.Healthy = true
	return result
}
