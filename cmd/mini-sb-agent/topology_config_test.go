//go:build with_vless || !minimal

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mini-sb-agent/panelapi"
)

func TestGenerateTopologyConfigFailsClosedFinalRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"127.0.0.1","server_port":10021,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv1","short_id":"aa"}}`))
	}))
	defer srv.Close()

	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}}
	out := filepath.Join(t.TempDir(), "config.json")
	if err := generateTopologyConfig(context.Background(), srv.URL, "tok", topology, out, "", ""); err != nil {
		t.Fatalf("generateTopologyConfig: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"final": "block"`) {
		t.Fatalf("topology route must fail closed to block, got:\n%s", data)
	}
}

func TestGenerateTopologyConfigRejectsProtocolMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Panel says hysteria, topology declares vless.
		_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"127.0.0.1","server_port":10021,"version":2}`))
	}))
	defer srv.Close()

	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}}
	out := filepath.Join(t.TempDir(), "config.json")
	err := generateTopologyConfig(context.Background(), srv.URL, "tok", topology, out, "", "")
	if err == nil || !strings.Contains(err.Error(), "protocol mismatch") {
		t.Fatalf("expected protocol mismatch error, got %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("config file must not be written when validation fails")
	}
}
