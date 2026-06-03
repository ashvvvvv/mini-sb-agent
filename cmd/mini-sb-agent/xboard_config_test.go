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
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "21", PanelNodeType: "auto", Out: out})
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
		if r.URL.Query().Get("node_type") == "vless" {
			http.Error(w, `{"message":"Server does not exist"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":10001,"version":2,"server_name":"www.apple.com","tls_settings":{"server_name":"www.apple.com","allow_insecure":true},"up_mbps":0,"down_mbps":0,"obfs":"salamander","obfs-password":"obfspass","base_config":{"pull_interval":60,"push_interval":60}}`))
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "config.json")
	gotType, err := generateXboardConfig(context.Background(), xboardGenerateOptions{PanelURL: srv.URL, PanelToken: "tok", PanelNodeID: "22", PanelNodeType: "auto", Out: out})
	if err != nil {
		t.Fatalf("generateXboardConfig: %v", err)
	}
	if gotType != "hysteria2" {
		t.Fatalf("node type = %q, want hysteria2", gotType)
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
