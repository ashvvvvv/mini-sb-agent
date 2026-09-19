//go:build !minimal || (with_vless && with_hysteria2 && with_shadowsocks_outbound)

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateConfigFromXboardVLESS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/config" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("node_type") != "vless" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":10001,"network":"tcp","flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"},"base_config":{"pull_interval":60,"push_interval":60}}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "21", PanelNodeType: "vless", NodeMode: "vless", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "vless" {
		t.Fatalf("node type = %q, want vless", gotType)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("generated config is not json: %v\n%s", err, data)
	}
	opts, err := loadOptions(out)
	if err != nil {
		t.Fatalf("generated config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Type != "vless" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigFromXboardHY2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("node_type") != "hysteria" {
			t.Fatalf("node_type = %s", r.URL.Query().Get("node_type"))
		}
		_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":10001,"version":2,"server_name":"www.apple.com","tls_settings":{"server_name":"www.apple.com","allow_insecure":true},"up_mbps":0,"down_mbps":0,"obfs":"salamander","obfs-password":"obfspass","base_config":{"pull_interval":60,"push_interval":60}}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "22", PanelNodeType: "hysteria", NodeMode: "hy2", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "hy2" {
		t.Fatalf("node type = %q, want hy2", gotType)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated HY2 config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 1 || opts.Inbounds[0].Type != "hysteria2" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}

func TestGenerateConfigFromXboardBothVLESSAndHY2(t *testing.T) {
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node_id")
		nodeType := r.URL.Query().Get("node_type")
		seen[nodeID] = nodeType
		switch nodeID + ":" + nodeType {
		case "21:vless":
			_, _ = w.Write([]byte(`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":10001,"network":"tcp","flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid"}}`))
		case "22:hysteria":
			_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":10002,"version":2,"server_name":"www.apple.com","tls_settings":{"server_name":"www.apple.com"},"obfs":"salamander","obfs-password":"obfspass"}`))
		default:
			http.Error(w, `{"message":"unexpected node"}`, http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	mode, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", NodeMode: "both", VLESSNodeID: "21", HY2NodeID: "22", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if mode != "both" {
		t.Fatalf("mode = %q, want both", mode)
	}
	if seen["21"] != "vless" || seen["22"] != "hysteria" {
		t.Fatalf("seen queries = %+v", seen)
	}
	opts, err := loadOptions(out)
	if err != nil {
		data, _ := os.ReadFile(out)
		t.Fatalf("generated dual config not loadable by sing-box: %v\n%s", err, data)
	}
	if len(opts.Inbounds) != 2 || opts.Inbounds[0].Type != "vless" || opts.Inbounds[1].Type != "hysteria2" {
		t.Fatalf("inbounds = %+v", opts.Inbounds)
	}
}
