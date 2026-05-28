package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOptionsAcceptsCoreNodeProtocols(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless-in",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "users": []
    },
    {
      "type": "hysteria2",
      "tag": "hy2-in",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "users": [],
      "tls": {"enabled": true, "server_name": "example.com", "certificate_path": "/tmp/missing.crt", "key_path": "/tmp/missing.key"}
    }
  ],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOptions(path); err != nil {
		t.Fatalf("core VLESS/Hysteria2/direct options should load: %v", err)
	}
}

func TestLoadOptionsAcceptsCoreOutboundProtocols(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [],
  "outbounds": [
    {"type": "direct", "tag": "direct"},
    {"type": "block", "tag": "block"},
    {"type": "dns", "tag": "dns-out"}
  ],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOptions(path); err != nil {
		t.Fatalf("core direct/block/dns outbound options should load: %v", err)
	}
}

func TestLoadOptionsRejectsUnsupportedTunInbound(t *testing.T) {
	if tunBuildEnabled {
		t.Skip("tun inbound is intentionally available when built with -tags tun")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [{"type": "tun", "tag": "tun-in"}],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOptions(path); err == nil {
		t.Fatal("tun inbound unexpectedly loaded; this build should only support VLESS Reality/Vision and Hysteria2")
	}
}

func TestLoadOptionsRejectsUnsupportedProxyProtocols(t *testing.T) {
	unsupported := []string{"vmess", "trojan", "shadowsocks", "socks", "http", "mixed"}
	for _, protocol := range unsupported {
		t.Run(protocol, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			config := `{
  "log": {"disabled": true},
  "inbounds": [{"type": "` + protocol + `", "tag": "unsupported-in"}],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
			if err := os.WriteFile(path, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadOptions(path); err == nil {
				t.Fatalf("%s inbound unexpectedly loaded in node-only build", protocol)
			}
		})
	}
}
