package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/xuri/excelize/v2"
)

func TestParseRelayUpstreamLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want RelayUpstream
	}{
		{name: "colon format", line: "proxy.example:1080:user:pass", want: RelayUpstream{Server: "proxy.example", Port: 1080, Username: "user", Password: "pass"}},
		{name: "ipv6 colon format", line: "[2001:db8::10]:1080:user:pass", want: RelayUpstream{Server: "2001:db8::10", Port: 1080, Username: "user", Password: "pass"}},
		{name: "socks url", line: "socks5://user:p%40ss@[2001:db8::10]:1080", want: RelayUpstream{Server: "2001:db8::10", Port: 1080, Username: "user", Password: "p@ss"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRelayUpstreamLine(test.line)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %#v, want %#v", got, test.want)
			}
		})
	}
	item := model.RelayItem{Username: "relay-user", Password: "relay-password"}
	_, mixed, err := relayProtocolConfig(RelayCreateRequest{Protocol: "mixed"}, &item)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mixed", "socks", "http"} {
		if mixed[key] == nil {
			t.Fatalf("mixed client config does not contain %q", key)
		}
	}
}

func TestRandomRelayIPv6KeepsPrefix(t *testing.T) {
	base := mustParseRelayTestAddr("2001:db8:1234:5678::1")
	for i := 0; i < 32; i++ {
		candidate, err := randomRelayIPv6(base, 64)
		if err != nil {
			t.Fatal(err)
		}
		if !mustParseRelayTestPrefix("2001:db8:1234:5678::/64").Contains(candidate) {
			t.Fatalf("candidate %s escaped prefix", candidate)
		}
	}
}

func TestNormalizeRelayPublicHost(t *testing.T) {
	tests := []struct {
		value string
		want  string
		valid bool
	}{
		{value: "example.com", want: "example.com", valid: true},
		{value: "[2001:db8::10]", want: "2001:db8::10", valid: true},
		{value: "[2001:db8::10]:443", valid: false},
		{value: "example.com:443", valid: false},
		{value: "bad host", valid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := normalizeRelayPublicHost(test.value)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("got %q, %v; want %q", got, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %q to be rejected", test.value)
			}
		})
	}
}

func TestRelaySOCKSURIFormatsIPv6(t *testing.T) {
	got := relaySOCKSURI("2001:db8::10", 1080, "user", "p@ss")
	want := "socks5://user:p%40ss@[2001:db8::10]:1080"
	if got != want {
		t.Fatalf("URI = %q, want %q", got, want)
	}
}

func TestRelaySOCKSExportUsesBrowserFormat(t *testing.T) {
	tests := []struct {
		name string
		mode string
		host string
		ipv6 string
		port int
		user string
		pass string
		want string
	}{
		{name: "ipv4", mode: relayModeUpstream, host: "88.214.24.57", port: 1020, user: "proxy_xbhwi8qipf", pass: "ohNuE5VXWeta6jb@xn", want: "88.214.24.57:1020:proxy_xbhwi8qipf:ohNuE5VXWeta6jb@xn"},
		{name: "ipv6", mode: relayModeIPv6, host: "88.214.24.57", ipv6: "2001:db8::10", port: 1021, user: "user", pass: "pass", want: "2001:db8::10:1021:user:pass"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := relaySOCKSExport(test.mode, test.host, test.ipv6, test.port, test.user, test.pass)
			if got != test.want {
				t.Fatalf("export = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRelayBitBrowserProxyInfo(t *testing.T) {
	tests := []struct {
		name string
		pool model.RelayPool
		item model.RelayItem
		want string
	}{
		{
			name: "ipv4",
			pool: model.RelayPool{Mode: relayModeUpstream, Protocol: "socks", ListenHost: "88.214.24.57"},
			item: model.RelayItem{ListenPort: 1020, Username: "proxy-user", Password: "proxy-pass"},
			want: "88.214.24.57:1020:proxy-user:proxy-pass",
		},
		{
			name: "ipv6",
			pool: model.RelayPool{Mode: relayModeIPv6, Protocol: "socks", ListenHost: "88.214.24.57"},
			item: model.RelayItem{ListenPort: 30005, Username: "proxy-user", Password: "proxy-pass", IPv6: "2a05:f480:3400:282d:a75c:71ba:1e76:7c1b"},
			want: "ipv6:2a05:f480:3400:282d:a75c:71ba:1e76:7c1b:30005:proxy-user:proxy-pass",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := relayBitBrowserProxyInfo(test.pool, test.item)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("proxy info = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildRelayBitBrowserWorkbook(t *testing.T) {
	pool := model.RelayPool{
		Name:       "IPv6 pool",
		Mode:       relayModeIPv6,
		Protocol:   "socks",
		ListenHost: "88.214.24.57",
		Items: mustJSON([]model.RelayItem{
			{ListenPort: 30005, Username: "user-1", Password: "pass-1", IPv6: "2001:db8::10"},
			{ListenPort: 30006, Username: "user-2", Password: "pass-2", IPv6: "2001:db8::11"},
		}),
	}
	data, err := buildRelayBitBrowserWorkbook(pool)
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	const sheet = "批量导入窗口"
	checks := map[string]string{
		"A1": "窗口名称",
		"E1": "代理类型",
		"F1": "代理信息",
		"A4": "IPv6 pool-001",
		"E4": "socks5",
		"F4": "ipv6:2001:db8::10:30005:user-1:pass-1",
		"F5": "ipv6:2001:db8::11:30006:user-2:pass-2",
	}
	for cell, want := range checks {
		got, err := workbook.GetCellValue(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", cell, got, want)
		}
	}
}

func TestRelayProtocolConfigCreatesIndependentCredentials(t *testing.T) {
	tests := []struct {
		protocol string
		wantKey  string
	}{
		{protocol: "socks", wantKey: "socks"},
		{protocol: "http", wantKey: "http"},
		{protocol: "vless", wantKey: "vless"},
		{protocol: "vmess", wantKey: "vmess"},
		{protocol: "trojan", wantKey: "trojan"},
		{protocol: "hysteria2", wantKey: "hysteria2"},
		{protocol: "tuic", wantKey: "tuic"},
		{protocol: "naive", wantKey: "naive"},
		{protocol: "anytls", wantKey: "anytls"},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			item := model.RelayItem{Username: "relay-user", Password: "relay-password"}
			options, client, err := relayProtocolConfig(RelayCreateRequest{Protocol: test.protocol, Transport: "http"}, &item)
			if err != nil {
				t.Fatal(err)
			}
			if client[test.wantKey] == nil {
				t.Fatalf("client config does not contain %q: %#v", test.wantKey, client)
			}
			if (test.protocol == "vless" || test.protocol == "vmess" || test.protocol == "tuic") && item.UUID == "" {
				t.Fatal("UUID was not generated")
			}
			if (test.protocol == "vless" || test.protocol == "vmess" || test.protocol == "trojan") && options["transport"] == nil {
				t.Fatal("transport was not generated")
			}
		})
	}
}

func TestRelayShadowsocksUses256BitKeys(t *testing.T) {
	item := model.RelayItem{Username: "relay-user", Password: "unused"}
	options, client, err := relayProtocolConfig(RelayCreateRequest{Protocol: "shadowsocks", ShadowsocksMethod: "2022-blake3-aes-256-gcm"}, &item)
	if err != nil {
		t.Fatal(err)
	}
	if options["password"] == "" || item.InboundPassword == "" {
		t.Fatal("missing Shadowsocks inbound password")
	}
	user, ok := client["shadowsocks"].(map[string]interface{})
	if !ok || user["password"] == "" {
		t.Fatalf("missing Shadowsocks user password: %#v", client)
	}
	for name, value := range map[string]string{"inbound": item.InboundPassword, "user": item.Password} {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("%s key is not a 256-bit standard base64 PSK: len=%d err=%v", name, len(decoded), err)
		}
	}
}

func TestRelayClientLinkFormatsIPv6(t *testing.T) {
	req := RelayCreateRequest{Mode: relayModeIPv6, Protocol: "socks"}
	inbound := model.Inbound{
		Type: "socks", Tag: "relay-test", CoreType: model.CoreTypeSingBox,
		Options: mustJSON(map[string]interface{}{"listen": "2001:db8::10", "listen_port": 1080}),
		OutJson: mustJSON(map[string]interface{}{}),
	}
	client := map[string]interface{}{"socks": map[string]interface{}{"username": "user", "password": "pass"}}
	got := relayClientLink(req, inbound, client, "example.com")
	if !strings.Contains(got, "@[2001:db8::10]:1080") {
		t.Fatalf("unexpected IPv6 link %q", got)
	}
}

func TestApplyRelayAutoAddIPv6Preset(t *testing.T) {
	req := RelayCreateRequest{
		Source:             relaySourceAutoAddIPv6,
		Mode:               relayModeUpstream,
		Protocol:           "vmess",
		CoreType:           model.CoreTypeXray,
		TlsID:              9,
		Transport:          "ws",
		AddSystemAddresses: false,
	}
	if err := applyRelaySourcePreset(&req); err != nil {
		t.Fatal(err)
	}
	if req.Mode != relayModeIPv6 || req.Protocol != "socks" || req.CoreType != relayCoreSingBox || !req.AddSystemAddresses || req.TlsID != 0 || req.Transport != "" {
		t.Fatalf("auto-add-ipv6 preset was not enforced: %#v", req)
	}
	if err := applyRelaySourcePreset(&RelayCreateRequest{Source: "unknown/source"}); err == nil {
		t.Fatal("unknown relay source was accepted")
	}
}

func TestNormalizeRelayDomainStrategy(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		value string
		want  string
		valid bool
	}{
		{name: "IPv6 default", mode: relayModeIPv6, want: relayDomainStrategyIPv6Only, valid: true},
		{name: "IPv6 only", mode: relayModeIPv6, value: relayDomainStrategyIPv6Only, want: relayDomainStrategyIPv6Only, valid: true},
		{name: "dual stack", mode: relayModeIPv6, value: relayDomainStrategyPreferIPv6, want: relayDomainStrategyPreferIPv6, valid: true},
		{name: "upstream ignores strategy", mode: relayModeUpstream, value: relayDomainStrategyIPv6Only, valid: true},
		{name: "invalid", mode: relayModeIPv6, value: "prefer_ipv4"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeRelayDomainStrategy(test.mode, test.value)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("strategy = %q, err=%v; want %q", got, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected strategy %q to be rejected", test.value)
			}
		})
	}
}

func TestRelayDirectOutboundOptionsForceIPv6(t *testing.T) {
	item := model.RelayItem{IPv6: "2001:db8::10"}
	options := relayDirectOutboundOptions(RelayCreateRequest{Mode: relayModeIPv6, DomainStrategy: relayDomainStrategyIPv6Only}, item)
	if options["inet6_bind_address"] != item.IPv6 || options["domain_strategy"] != relayDomainStrategyIPv6Only {
		t.Fatalf("unexpected IPv6 direct options: %#v", options)
	}
}

func TestRepairRelayIPv6OutboundStrategies(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("SUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "relay-strategy.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	outbound := model.Outbound{Type: "direct", Tag: "relay-out-old", Options: mustJSON(map[string]interface{}{"inet6_bind_address": "2001:db8::10"})}
	if err := db.Create(&outbound).Error; err != nil {
		t.Fatal(err)
	}
	items := []model.RelayItem{{IPv6: "2001:db8::10", InboundTag: "relay-in-old", OutboundTag: outbound.Tag}}
	pool := model.RelayPool{Name: "old-ipv6-pool", Mode: relayModeIPv6, Items: mustJSON(items)}
	if err := db.Create(&pool).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&ConfigService{}).repairRelayIPv6OutboundStrategies(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&pool, pool.Id).Error; err != nil {
		t.Fatal(err)
	}
	if pool.DomainStrategy != relayDomainStrategyIPv6Only {
		t.Fatalf("pool strategy = %q", pool.DomainStrategy)
	}
	if err := db.First(&outbound, outbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var options map[string]interface{}
	if err := json.Unmarshal(outbound.Options, &options); err != nil {
		t.Fatal(err)
	}
	if options["domain_strategy"] != relayDomainStrategyIPv6Only || options["inet6_bind_address"] != "2001:db8::10" {
		t.Fatalf("repaired options = %#v", options)
	}
	if err := (&ConfigService{}).repairRelayIPv6OutboundStrategies(); err != nil {
		t.Fatal(err)
	}
	var setting model.Setting
	if err := db.Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		t.Fatal(err)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 4 {
		t.Fatalf("rule count after idempotent repair = %d, want 4", len(rules))
	}
	firstRule := rules[0].(map[string]interface{})
	if firstRule["action"] != "reject" || relayRuleIPVersion(firstRule) != 4 {
		t.Fatalf("first repaired rule = %#v, want IPv4 reject", firstRule)
	}
}

func TestBuildRelayCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		root       bool
		ipCommand  bool
		wantReady  bool
		wantReason string
	}{
		{name: "ready linux", goos: "linux", root: true, ipCommand: true, wantReady: true},
		{name: "macOS", goos: "darwin", root: true, ipCommand: true, wantReason: "unsupported_os"},
		{name: "unprivileged linux", goos: "linux", ipCommand: true, wantReason: "root_required"},
		{name: "missing iproute2", goos: "linux", root: true, wantReason: "iproute2_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildRelayCapabilities(test.goos, test.root, test.ipCommand)
			if got.OS != test.goos || got.CanAddSystemIPv6 != test.wantReady || got.UnavailableReason != test.wantReason {
				t.Fatalf("capabilities = %#v, want ready=%v reason=%q", got, test.wantReady, test.wantReason)
			}
		})
	}
}

func TestRelayIPv6AddressState(t *testing.T) {
	const address = "2001:db8:1234::10"
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "ready", output: "2: eth0    inet6 2001:db8:1234::10/64 scope global valid_lft forever preferred_lft forever", want: relayAddressReady},
		{name: "tentative", output: "2: eth0    inet6 2001:db8:1234::10/64 scope global tentative valid_lft forever preferred_lft forever", want: relayAddressTentative},
		{name: "dad failed", output: "2: eth0    inet6 2001:db8:1234::10/64 scope global tentative dadfailed valid_lft forever preferred_lft forever", want: relayAddressDADFailed},
		{name: "different address", output: "2: eth0    inet6 2001:db8:1234::11/64 scope global", want: relayAddressMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := relayIPv6AddressState(test.output, address); got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUpdateRelayRouteRulesAddsAndRemovesOnlyRelayRules(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("SUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "relay-rules.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	item := model.RelayItem{InboundTag: "relay-in", OutboundTag: "relay-out"}
	if err := updateRelayRouteRules(db, []model.RelayItem{item}, true, false); err != nil {
		t.Fatal(err)
	}
	var setting model.Setting
	if err := db.Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		t.Fatal(err)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 4 {
		t.Fatalf("rule count = %d, want 4", len(rules))
	}
	firstRule := rules[0].(map[string]interface{})
	if firstRule["action"] != "reject" || relayRuleIPVersion(firstRule) != 4 {
		t.Fatalf("first rule = %#v, want IPv4 reject", firstRule)
	}
	if err := updateRelayRouteRules(db, []model.RelayItem{item}, true, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		t.Fatal(err)
	}
	rules = config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 4 {
		t.Fatalf("rule count after idempotent update = %d, want 4", len(rules))
	}
	if err := updateRelayRouteRules(db, []model.RelayItem{item}, false, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		t.Fatal(err)
	}
	rules = config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 3 {
		t.Fatalf("dual-stack rule count = %d, want 3", len(rules))
	}
	if err := updateRelayRouteRules(db, []model.RelayItem{item}, true, false); err != nil {
		t.Fatal(err)
	}
	if err := updateRelayRouteRules(db, []model.RelayItem{item}, false, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		t.Fatal(err)
	}
	rules = config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("rule count after removal = %d, want 2", len(rules))
	}
}

func mustParseRelayTestAddr(value string) (addr netip.Addr) {
	addr, _ = netip.ParseAddr(value)
	return addr
}

func mustParseRelayTestPrefix(value string) (prefix netip.Prefix) {
	prefix, _ = netip.ParsePrefix(value)
	return prefix
}
