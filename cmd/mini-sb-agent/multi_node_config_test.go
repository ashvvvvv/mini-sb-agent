//go:build !minimal || (with_vless && with_hysteria2 && with_shadowsocks_outbound)

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mini-sb-agent/panelapi"
)

func TestGenerateTopologyConfigCreatesSameProtocolInboundsAndPerNodeSSRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("node_id") {
		case "21":
			_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"127.0.0.1","server_port":10021,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv1","short_id":"aa"}}`))
		case "22":
			_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"127.0.0.1","server_port":10022,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.microsoft.com","server_port":"443","private_key":"priv2","short_id":"bb"}}`))
		default:
			http.Error(w, "unexpected node", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "shadowsocks", Server: "198.51.100.10", ServerPort: 8388, Method: "aes-128-gcm", Password: "pw1"}},
		{NodeID: "22", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "shadowsocks", Server: "198.51.100.20", ServerPort: 8389, Method: "aes-256-gcm", Password: "pw2"}},
	}}
	out := filepath.Join(t.TempDir(), "config.json")
	if err := generateTopologyConfig(context.Background(), srv.URL, "tok", topology, out, "", ""); err != nil {
		t.Fatalf("generateTopologyConfig: %v", err)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated config not loadable: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 2 || opts.Inbounds[0].Tag != "vless-21-in" || opts.Inbounds[1].Tag != "vless-22-in" {
		t.Fatalf("inbounds=%+v", opts.Inbounds)
	}
	if len(opts.Outbounds) != 5 { // direct, block, dns, and two SS outbounds
		t.Fatalf("outbounds=%d, want 5", len(opts.Outbounds))
	}
	if len(opts.Route.Rules) != 2 {
		t.Fatalf("route rules=%d, want 2", len(opts.Route.Rules))
	}
}

func TestLoadOptionsAcceptsShadowsocksOutbound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "inbounds": [],
  "outbounds": [
    {"type":"shadowsocks","tag":"ss-out","server":"127.0.0.1","server_port":8388,"method":"aes-128-gcm","password":"pw"}
  ],
  "route": {"final":"ss-out"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOptions(path); err != nil {
		t.Fatalf("shadowsocks outbound should load: %v", err)
	}
}
