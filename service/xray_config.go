//go:build !openwrt_lite

package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Hhz0823/1s-ui/config"
	"github.com/Hhz0823/1s-ui/core"
	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/Hhz0823/1s-ui/util/common"

	"gorm.io/gorm"
)

type XrayConfig struct {
	Log       map[string]interface{}   `json:"log,omitempty"`
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing   map[string]interface{}   `json:"routing,omitempty"`
}

func (s *ConfigService) HasXrayInbounds() (bool, error) {
	return s.hasXrayInbounds(database.GetDB())
}

func (s *ConfigService) hasXrayInbounds(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Model(model.Inbound{}).
		Where("core_type = ?", model.CoreTypeXray).
		Count(&count).Error
	return count > 0, err
}

func (s *ConfigService) GetXrayConfig() (*[]byte, error) {
	return s.getXrayConfig(database.GetDB())
}

func (s *ConfigService) getXrayConfig(db *gorm.DB) (*[]byte, error) {
	inbounds, err := s.InboundService.GetAllXrayConfig(db)
	if err != nil {
		return nil, err
	}

	xrayConfig := XrayConfig{
		Log: map[string]interface{}{
			"loglevel": "warning",
		},
		Inbounds: inbounds,
		Outbounds: []map[string]interface{}{
			{
				"protocol": "freedom",
				"tag":      "direct",
			},
			{
				"protocol": "blackhole",
				"tag":      "block",
			},
		},
		Routing: map[string]interface{}{
			"rules": []interface{}{},
		},
	}

	rawConfig, err := json.MarshalIndent(xrayConfig, "", "  ")
	if err != nil {
		return nil, err
	}
	return &rawConfig, nil
}

func (s *ConfigService) validateXrayConfig(db *gorm.DB) error {
	hasXray, err := s.hasXrayInbounds(db)
	if err != nil || !hasXray {
		return err
	}
	rawConfig, err := s.getXrayConfig(db)
	if err != nil {
		return err
	}
	if xrayPtr == nil {
		xrayPtr = core.NewXrayRuntime()
	}
	return xrayPtr.Validate(*rawConfig)
}

func (s *InboundService) GetAllXrayConfig(db *gorm.DB) ([]map[string]interface{}, error) {
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Preload("Tls").
		Where("core_type = ?", model.CoreTypeXray).
		Find(&inbounds).Error
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(inbounds))
	for _, inbound := range inbounds {
		switch inbound.Type {
		case "vless":
			config, err := s.buildXrayVlessInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "vmess":
			config, err := s.buildXrayVMessInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "trojan":
			config, err := s.buildXrayTrojanInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "shadowsocks":
			config, err := s.buildXrayShadowsocksInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "socks":
			config, err := s.buildXraySocksInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "http":
			config, err := s.buildXrayHTTPInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "mixed":
			config, err := s.buildXrayMixedInbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "hysteria2":
			config, err := s.buildXrayHysteria2Inbound(db, inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		case "dokodemo-door":
			config, err := s.buildXrayDokodemoInbound(inbound)
			if err != nil {
				return nil, err
			}
			result = append(result, config)
		default:
			return nil, common.NewErrorf("xray inbound type <%s> is not supported yet", inbound.Type)
		}
	}
	return result, nil
}

func (s *InboundService) buildXrayVlessInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, err
	}

	listen, _ := (*full)["listen"].(string)
	if listen == "" {
		listen = "0.0.0.0"
	}
	port := toInt((*full)["listen_port"])
	if port == 0 {
		return nil, common.NewErrorf("xray inbound <%s> missing listen_port", inbound.Tag)
	}

	transport, _ := (*full)["transport"].(map[string]interface{})
	network := "xhttp"
	if tp, ok := transport["type"].(string); ok && tp != "" {
		network = tp
	}

	clients, err := s.fetchXrayVlessClients(db, inbound.Id, network)
	if err != nil {
		return nil, err
	}

	streamSettings, err := buildXrayStreamSettings(inbound, transport, network)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "vless",
		"settings": map[string]interface{}{
			"clients":    clients,
			"decryption": "none",
		},
		"streamSettings": streamSettings,
	}, nil
}

func (s *InboundService) buildXrayVMessInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	_, listen, port, transport, network, err := xrayInboundBasics(inbound, "ws")
	if err != nil {
		return nil, err
	}

	clients, err := s.fetchXrayVMessClients(db, inbound.Id)
	if err != nil {
		return nil, err
	}

	streamSettings, err := buildXrayStreamSettings(inbound, transport, network)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "vmess",
		"settings": map[string]interface{}{
			"clients": clients,
		},
		"streamSettings": streamSettings,
	}, nil
}

func (s *InboundService) buildXrayTrojanInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	_, listen, port, transport, network, err := xrayInboundBasics(inbound, "ws")
	if err != nil {
		return nil, err
	}

	clients, err := s.fetchXrayPasswordClients(db, inbound.Id, "trojan")
	if err != nil {
		return nil, err
	}

	streamSettings, err := buildXrayStreamSettings(inbound, transport, network)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "trojan",
		"settings": map[string]interface{}{
			"clients": clients,
		},
		"streamSettings": streamSettings,
	}, nil
}

func (s *InboundService) buildXrayShadowsocksInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, err
	}

	listen, port, err := xrayListenAndPort(full, inbound.Tag)
	if err != nil {
		return nil, err
	}

	method := stringValue((*full)["method"], "2022-blake3-aes-256-gcm")
	password := stringValue((*full)["password"], "")
	network := stringValue((*full)["network"], "tcp,udp")
	if network == "" {
		network = "tcp,udp"
	}

	clients, err := s.fetchXrayShadowsocksClients(db, inbound.Id, method)
	if err != nil {
		return nil, err
	}
	if password == "" && len(clients) > 0 {
		password, _ = clients[0]["password"].(string)
	}
	if password == "" {
		return nil, common.NewErrorf("xray shadowsocks inbound <%s> missing password", inbound.Tag)
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "shadowsocks",
		"settings": map[string]interface{}{
			"method":   method,
			"password": password,
			"network":  network,
			"clients":  clients,
		},
	}, nil
}

func (s *InboundService) buildXraySocksInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, err
	}
	listen, port, err := xrayListenAndPort(full, inbound.Tag)
	if err != nil {
		return nil, err
	}

	accounts, err := s.fetchXrayAccountClients(db, inbound.Id, "socks")
	if err != nil {
		return nil, err
	}
	settings := map[string]interface{}{
		"udp": true,
	}
	if len(accounts) > 0 {
		settings["auth"] = "password"
		settings["accounts"] = accounts
	} else {
		settings["auth"] = "noauth"
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "socks",
		"settings": settings,
	}, nil
}

func (s *InboundService) buildXrayHTTPInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, err
	}
	listen, port, err := xrayListenAndPort(full, inbound.Tag)
	if err != nil {
		return nil, err
	}

	accounts, err := s.fetchXrayAccountClients(db, inbound.Id, "http")
	if err != nil {
		return nil, err
	}
	settings := map[string]interface{}{}
	if len(accounts) > 0 {
		settings["accounts"] = accounts
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "http",
		"settings": settings,
	}, nil
}

func (s *InboundService) buildXrayMixedInbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	config, err := s.buildXraySocksInbound(db, inbound)
	if err != nil {
		return nil, err
	}
	config["protocol"] = "mixed"
	return config, nil
}

func (s *InboundService) buildXrayHysteria2Inbound(db *gorm.DB, inbound *model.Inbound) (map[string]interface{}, error) {
	full, listen, port, transport, _, err := xrayInboundBasics(inbound, "hysteria")
	if err != nil {
		return nil, err
	}
	if inbound.Tls == nil {
		return nil, common.NewErrorf("xray hysteria2 inbound <%s> requires TLS", inbound.Tag)
	}

	clients, err := s.fetchXrayHysteria2Clients(db, inbound.Id)
	if err != nil {
		return nil, err
	}
	streamSettings, err := buildXrayStreamSettings(inbound, transport, "hysteria")
	if err != nil {
		return nil, err
	}
	if idleTimeout := toInt((*full)["udp_idle_timeout"]); idleTimeout > 0 {
		streamSettings["hysteriaSettings"].(map[string]interface{})["udpIdleTimeout"] = idleTimeout
	}
	if masquerade, ok := xrayHysteriaMasquerade((*full)["masquerade"]); ok {
		streamSettings["hysteriaSettings"].(map[string]interface{})["masquerade"] = masquerade
	}

	return map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "hysteria",
		"settings": map[string]interface{}{
			"version": 2,
			"clients": clients,
		},
		"streamSettings": streamSettings,
	}, nil
}

func (s *InboundService) buildXrayDokodemoInbound(inbound *model.Inbound) (map[string]interface{}, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, err
	}
	listen, port, err := xrayListenAndPort(full, inbound.Tag)
	if err != nil {
		return nil, err
	}

	settings := map[string]interface{}{
		"network":        stringValue((*full)["network"], "tcp,udp"),
		"followRedirect": boolValue((*full)["follow_redirect"]),
	}
	if address, _ := (*full)["address"].(string); address != "" {
		settings["address"] = address
	}
	if targetPort := toInt((*full)["port"]); targetPort > 0 {
		settings["port"] = targetPort
	}

	config := map[string]interface{}{
		"tag":      inbound.Tag,
		"listen":   listen,
		"port":     port,
		"protocol": "dokodemo-door",
		"settings": settings,
	}
	if sniffing, ok := (*full)["sniffing"].(map[string]interface{}); ok && len(sniffing) > 0 {
		config["sniffing"] = sniffing
	}
	return config, nil
}

func (s *InboundService) fetchXrayVlessClients(db *gorm.DB, inboundId uint, network string) ([]map[string]interface{}, error) {
	var users []struct {
		Name   string
		Config string
	}
	err := db.Raw(`SELECT name, json_extract(config, "$.vless") AS config
		FROM clients
		WHERE enable = true
			AND json_extract(config, "$.vless") IS NOT NULL
			AND ? IN (SELECT json_each.value FROM json_each(clients.inbounds))`, inboundId).Scan(&users).Error
	if err != nil {
		return nil, err
	}

	clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		uuid, _ := cfg["uuid"].(string)
		if uuid == "" {
			continue
		}
		client := map[string]interface{}{
			"id":    uuid,
			"email": user.Name,
		}
		if network == "tcp" || network == "raw" {
			if flow, _ := cfg["flow"].(string); flow != "" {
				client["flow"] = flow
			}
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func (s *InboundService) fetchXrayVMessClients(db *gorm.DB, inboundId uint) ([]map[string]interface{}, error) {
	users, err := fetchXrayClientConfigs(db, inboundId, "vmess")
	if err != nil {
		return nil, err
	}

	clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		uuid, _ := cfg["uuid"].(string)
		if uuid == "" {
			continue
		}
		client := map[string]interface{}{
			"id":    uuid,
			"email": user.Name,
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func (s *InboundService) fetchXrayPasswordClients(db *gorm.DB, inboundId uint, protocol string) ([]map[string]interface{}, error) {
	users, err := fetchXrayClientConfigs(db, inboundId, protocol)
	if err != nil {
		return nil, err
	}

	clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		password, _ := cfg["password"].(string)
		if password == "" {
			continue
		}
		clients = append(clients, map[string]interface{}{
			"password": password,
			"email":    user.Name,
		})
	}
	return clients, nil
}

func (s *InboundService) fetchXrayShadowsocksClients(db *gorm.DB, inboundId uint, method string) ([]map[string]interface{}, error) {
	protocol := "shadowsocks"
	if method == "2022-blake3-aes-128-gcm" {
		protocol = "shadowsocks16"
	}
	users, err := fetchXrayClientConfigs(db, inboundId, protocol)
	if err != nil {
		return nil, err
	}

	clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		password, _ := cfg["password"].(string)
		if password == "" {
			continue
		}
		client := map[string]interface{}{
			"password": password,
			"email":    user.Name,
		}
		if !strings.HasPrefix(method, "2022") {
			client["method"] = method
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func (s *InboundService) fetchXrayAccountClients(db *gorm.DB, inboundId uint, protocol string) ([]map[string]interface{}, error) {
	users, err := fetchXrayClientConfigs(db, inboundId, protocol)
	if err != nil {
		return nil, err
	}

	accounts := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		username, _ := cfg["username"].(string)
		password, _ := cfg["password"].(string)
		if username == "" || password == "" {
			continue
		}
		accounts = append(accounts, map[string]interface{}{
			"user": username,
			"pass": password,
		})
	}
	return accounts, nil
}

func (s *InboundService) fetchXrayHysteria2Clients(db *gorm.DB, inboundId uint) ([]map[string]interface{}, error) {
	users, err := fetchXrayClientConfigs(db, inboundId, "hysteria2")
	if err != nil {
		return nil, err
	}
	clients := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(user.Config), &cfg); err != nil {
			return nil, err
		}
		password, _ := cfg["password"].(string)
		if password == "" {
			continue
		}
		clients = append(clients, map[string]interface{}{"auth": password, "email": user.Name})
	}
	return clients, nil
}

type xrayClientConfigRow struct {
	Name   string
	Config string
}

func fetchXrayClientConfigs(db *gorm.DB, inboundId uint, protocol string) ([]xrayClientConfigRow, error) {
	var users []xrayClientConfigRow
	err := db.Raw(fmt.Sprintf(`SELECT name, json_extract(config, "$.%s") AS config
		FROM clients
		WHERE enable = true
			AND json_extract(config, "$.%s") IS NOT NULL
			AND ? IN (SELECT json_each.value FROM json_each(clients.inbounds))`, protocol, protocol), inboundId).Scan(&users).Error
	return users, err
}

func buildXrayStreamSettings(inbound *model.Inbound, transport map[string]interface{}, network string) (map[string]interface{}, error) {
	stream := map[string]interface{}{
		"network": network,
	}

	switch network {
	case "xhttp":
		stream["xhttpSettings"] = map[string]interface{}{
			"path": stringValue(transport["path"], "/xhttp"),
			"host": stringValue(transport["host"], ""),
			"mode": stringValue(transport["mode"], "auto"),
		}
	case "ws":
		stream["wsSettings"] = map[string]interface{}{
			"path":                stringValue(transport["path"], "/"),
			"host":                stringValue(transport["host"], ""),
			"heartbeatPeriod":     toInt(transport["heartbeat_period"]),
			"acceptProxyProtocol": boolValue(transport["accept_proxy_protocol"]),
		}
	case "grpc":
		stream["grpcSettings"] = map[string]interface{}{
			"serviceName":          stringValue(transport["service_name"], ""),
			"authority":            stringValue(transport["authority"], ""),
			"multiMode":            boolValue(transport["multi_mode"]),
			"idle_timeout":         toInt(transport["idle_timeout"]),
			"health_check_timeout": toInt(transport["health_check_timeout"]),
		}
	case "httpupgrade":
		stream["httpupgradeSettings"] = map[string]interface{}{
			"path":                stringValue(transport["path"], "/"),
			"host":                stringValue(transport["host"], ""),
			"acceptProxyProtocol": boolValue(transport["accept_proxy_protocol"]),
		}
	case "kcp", "mkcp":
		stream["network"] = "kcp"
		stream["kcpSettings"] = map[string]interface{}{
			"mtu":              intValue(transport["mtu"], 1350),
			"tti":              intValue(transport["tti"], 50),
			"uplinkCapacity":   intValue(transport["uplink_capacity"], 5),
			"downlinkCapacity": intValue(transport["downlink_capacity"], 20),
			"cwndMultiplier":   intValue(transport["cwnd_multiplier"], 2),
			"maxSendingWindow": intValue(transport["max_sending_window"], 2048),
		}
	case "hysteria":
		stream["hysteriaSettings"] = map[string]interface{}{
			"version":        2,
			"udpIdleTimeout": intValue(transport["udp_idle_timeout"], 60),
		}
	case "tcp", "raw":
		stream["network"] = "raw"
		stream["rawSettings"] = map[string]interface{}{
			"acceptProxyProtocol": boolValue(transport["accept_proxy_protocol"]),
		}
	default:
		return nil, common.NewErrorf("xray transport <%s> is not supported yet", network)
	}

	if inbound.Tls != nil && len(inbound.Tls.Server) > 2 {
		if xrayRealityEnabled(inbound.Tls) && network != "tcp" && network != "raw" && network != "xhttp" && network != "grpc" {
			return nil, common.NewErrorf("Xray Reality only supports RAW, XHTTP and gRPC, got <%s>", network)
		}
		if err := addXraySecurity(stream, inbound.Tls); err != nil {
			return nil, err
		}
	}

	return stream, nil
}

func xrayInboundBasics(inbound *model.Inbound, defaultNetwork string) (*map[string]interface{}, string, int, map[string]interface{}, string, error) {
	full, err := inbound.MarshalFull()
	if err != nil {
		return nil, "", 0, nil, "", err
	}
	listen, port, err := xrayListenAndPort(full, inbound.Tag)
	if err != nil {
		return nil, "", 0, nil, "", err
	}

	transport, _ := (*full)["transport"].(map[string]interface{})
	network := defaultNetwork
	if tp, ok := transport["type"].(string); ok && tp != "" {
		network = tp
	}
	return full, listen, port, transport, network, nil
}

func xrayListenAndPort(full *map[string]interface{}, tag string) (string, int, error) {
	listen, _ := (*full)["listen"].(string)
	if listen == "" {
		listen = "0.0.0.0"
	}
	port := toInt((*full)["listen_port"])
	if port == 0 {
		return "", 0, common.NewErrorf("xray inbound <%s> missing listen_port", tag)
	}
	return listen, port, nil
}

func addXraySecurity(stream map[string]interface{}, tlsConfig *model.Tls) error {
	var server map[string]interface{}
	if err := json.Unmarshal(tlsConfig.Server, &server); err != nil {
		return err
	}
	if enabled, ok := server["enabled"].(bool); ok && !enabled {
		return nil
	}

	if reality, ok := server["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			stream["security"] = "reality"
			stream["realitySettings"] = buildXrayRealitySettings(server, reality)
			return nil
		}
	}

	stream["security"] = "tls"
	tlsSettings, err := buildXrayTLSSettings(tlsConfig, server)
	if err != nil {
		return err
	}
	stream["tlsSettings"] = tlsSettings
	return nil
}

func buildXrayRealitySettings(server map[string]interface{}, reality map[string]interface{}) map[string]interface{} {
	settings := map[string]interface{}{
		"show": false,
	}
	if privateKey, _ := reality["private_key"].(string); privateKey != "" {
		settings["privateKey"] = privateKey
	}
	if shortIds := toStringSlice(reality["short_id"]); len(shortIds) > 0 {
		settings["shortIds"] = shortIds
	}
	if serverName, _ := server["server_name"].(string); serverName != "" {
		settings["serverNames"] = []string{serverName}
	}
	if handshake, ok := reality["handshake"].(map[string]interface{}); ok {
		host, _ := handshake["server"].(string)
		port := toInt(handshake["server_port"])
		if host != "" && port > 0 {
			settings["dest"] = fmt.Sprintf("%s:%d", host, port)
		}
	}
	return settings
}

func buildXrayTLSSettings(tlsConfig *model.Tls, server map[string]interface{}) (map[string]interface{}, error) {
	settings := map[string]interface{}{}
	if serverName, _ := server["server_name"].(string); serverName != "" {
		settings["serverName"] = serverName
	}
	if alpn := toStringSlice(server["alpn"]); len(alpn) > 0 {
		settings["alpn"] = alpn
	}
	if minVersion, _ := server["min_version"].(string); minVersion != "" {
		settings["minVersion"] = minVersion
	}
	if maxVersion, _ := server["max_version"].(string); maxVersion != "" {
		settings["maxVersion"] = maxVersion
	}

	cert := map[string]interface{}{}
	certFile, _ := server["certificate_path"].(string)
	keyFile, _ := server["key_path"].(string)
	if certFile == "" || keyFile == "" {
		var err error
		certFile, keyFile, err = writeXrayCertificateFiles(tlsConfig, server)
		if err != nil {
			return nil, err
		}
	}
	if certFile != "" {
		cert["certificateFile"] = certFile
	}
	if keyFile != "" {
		cert["keyFile"] = keyFile
	}
	if len(cert) > 0 {
		settings["certificates"] = []map[string]interface{}{cert}
	}
	return settings, nil
}

func writeXrayCertificateFiles(tlsConfig *model.Tls, server map[string]interface{}) (string, string, error) {
	certificate := strings.Join(toStringSlice(server["certificate"]), "\n")
	key := strings.Join(toStringSlice(server["key"]), "\n")
	if certificate == "" || key == "" {
		return "", "", nil
	}

	dir := filepath.Join(config.GetBinFolderPath(), "xray-certs")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", "", err
	}
	baseName := sanitizeFileName(fmt.Sprintf("%d-%s", tlsConfig.Id, tlsConfig.Name))
	certFile := filepath.Join(dir, baseName+".crt")
	keyFile := filepath.Join(dir, baseName+".key")
	if err := os.WriteFile(certFile, []byte(certificate+"\n"), 0600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, []byte(key+"\n"), 0600); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

func toInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case uint:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

func stringValue(value interface{}, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}

func intValue(value interface{}, fallback int) int {
	if result := toInt(value); result > 0 {
		return result
	}
	return fallback
}

func boolValue(value interface{}) bool {
	result, _ := value.(bool)
	return result
}

func xrayRealityEnabled(tlsConfig *model.Tls) bool {
	if tlsConfig == nil {
		return false
	}
	var server map[string]interface{}
	if json.Unmarshal(tlsConfig.Server, &server) != nil {
		return false
	}
	reality, _ := server["reality"].(map[string]interface{})
	enabled, _ := reality["enabled"].(bool)
	return enabled
}

func xrayHysteriaMasquerade(value interface{}) (map[string]interface{}, bool) {
	result := map[string]interface{}{}
	switch masquerade := value.(type) {
	case string:
		masquerade = strings.TrimSpace(masquerade)
		if masquerade == "" {
			return nil, false
		}
		switch {
		case strings.HasPrefix(masquerade, "http://"), strings.HasPrefix(masquerade, "https://"):
			result["type"], result["url"] = "proxy", masquerade
		case strings.HasPrefix(masquerade, "file://"):
			result["type"], result["dir"] = "file", strings.TrimPrefix(masquerade, "file://")
		default:
			result["type"], result["content"] = "string", masquerade
		}
	case map[string]interface{}:
		if kind, _ := masquerade["type"].(string); kind != "" {
			result["type"] = kind
		}
		if directory, _ := masquerade["directory"].(string); directory != "" {
			result["dir"] = directory
		}
		if target, _ := masquerade["url"].(string); target != "" {
			result["url"] = target
		}
		if rewrite, ok := masquerade["rewrite_host"].(bool); ok {
			result["rewriteHost"] = rewrite
		}
		if insecure, ok := masquerade["insecure"].(bool); ok {
			result["insecure"] = insecure
		}
		if content, _ := masquerade["content"].(string); content != "" {
			result["content"] = content
		}
		if headers, ok := masquerade["headers"].(map[string]interface{}); ok {
			result["headers"] = headers
		}
		if status := toInt(masquerade["status_code"]); status > 0 {
			result["statusCode"] = status
		}
	default:
		return nil, false
	}
	return result, len(result) > 0
}

func sanitizeFileName(value string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	value = re.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
