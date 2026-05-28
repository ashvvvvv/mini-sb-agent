package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestLoadOptionsWithHY2TuningAppliesOnlyHysteria2Inbound(t *testing.T) {
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
      "up_mbps": 10,
      "down_mbps": 20,
      "ignore_client_bandwidth": false,
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

	opts, err := loadOptionsWithHY2Tuning(path, hy2Tuning{
		Enabled:               true,
		UpMbps:                80,
		DownMbps:              120,
		IgnoreClientBandwidth: true,
	})
	if err != nil {
		t.Fatalf("load options with Hysteria2 tuning: %v", err)
	}

	if got := len(opts.Inbounds); got != 2 {
		t.Fatalf("inbounds = %d, want 2", got)
	}
	if opts.Inbounds[0].Type != "vless" {
		t.Fatalf("first inbound type = %s, want vless", opts.Inbounds[0].Type)
	}
	hy2, ok := opts.Inbounds[1].Options.(*option.Hysteria2InboundOptions)
	if !ok {
		t.Fatalf("second inbound options type = %T, want *option.Hysteria2InboundOptions", opts.Inbounds[1].Options)
	}
	if hy2.UpMbps != 80 || hy2.DownMbps != 120 || !hy2.IgnoreClientBandwidth {
		t.Fatalf("HY2 tuning = up:%d down:%d ignore_client:%v, want up:80 down:120 ignore_client:true", hy2.UpMbps, hy2.DownMbps, hy2.IgnoreClientBandwidth)
	}
}

func TestLoadOptionsWithHY2TuningKeepsConfigWhenDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [
    {
      "type": "hysteria2",
      "tag": "hy2-in",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "up_mbps": 10,
      "down_mbps": 20,
      "ignore_client_bandwidth": false,
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

	opts, err := loadOptionsWithHY2Tuning(path, hy2Tuning{})
	if err != nil {
		t.Fatalf("load options without Hysteria2 tuning: %v", err)
	}
	hy2, ok := opts.Inbounds[0].Options.(*option.Hysteria2InboundOptions)
	if !ok {
		t.Fatalf("inbound options type = %T, want *option.Hysteria2InboundOptions", opts.Inbounds[0].Options)
	}
	if hy2.UpMbps != 10 || hy2.DownMbps != 20 || hy2.IgnoreClientBandwidth {
		t.Fatalf("HY2 tuning changed disabled config: up:%d down:%d ignore_client:%v", hy2.UpMbps, hy2.DownMbps, hy2.IgnoreClientBandwidth)
	}
}
