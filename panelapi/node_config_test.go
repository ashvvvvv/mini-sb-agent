package panelapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchNodeConfigReadsVLESSRealityFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/config" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "tok" || r.URL.Query().Get("node_id") != "21" || r.URL.Query().Get("node_type") != "vless" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"protocol":"vless",
			"listen_ip":"0.0.0.0",
			"server_port":10001,
			"network":"tcp",
			"flow":"xtls-rprx-vision",
			"tls":2,
			"tls_settings":{"server_name":"www.apple.com","server_port":"443","private_key":"priv","short_id":"sid","allow_insecure":true},
			"base_config":{"push_interval":60,"pull_interval":60}
		}`))
	}))
	defer srv.Close()

	cfg, err := NewClient(srv.URL, "tok", "21", "vless").FetchNodeConfig(context.Background())
	if err != nil {
		t.Fatalf("FetchNodeConfig: %v", err)
	}
	if cfg.Protocol != "vless" || cfg.ListenIP != "0.0.0.0" || cfg.ServerPort != 10001 || cfg.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.TLSSettings.ServerName != "www.apple.com" || cfg.TLSSettings.ServerPort != "443" || cfg.TLSSettings.PrivateKey != "priv" || cfg.TLSSettings.ShortID != "sid" {
		t.Fatalf("unexpected tls settings: %+v", cfg.TLSSettings)
	}
	if cfg.BaseConfig.PullInterval != 60 || cfg.BaseConfig.PushInterval != 60 {
		t.Fatalf("unexpected base config: %+v", cfg.BaseConfig)
	}
}

func TestProbeNodeTypeSkipsMissingAndAcceptsHysteria2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("node_type") {
		case "vless":
			http.Error(w, `{"message":"Server does not exist"}`, http.StatusNotFound)
		case "hysteria2":
			_, _ = w.Write([]byte(`{"protocol":"hysteria","version":2,"server_port":10001,"tls_settings":{"server_name":"www.apple.com"}}`))
		default:
			http.Error(w, `{"message":"unexpected"}`, http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	nodeType, cfg, err := ProbeNodeConfig(context.Background(), srv.URL, "tok", "22")
	if err != nil {
		t.Fatalf("ProbeNodeConfig: %v", err)
	}
	if nodeType != "hysteria2" || cfg.Protocol != "hysteria" || cfg.Version != 2 {
		t.Fatalf("got nodeType=%q cfg=%+v", nodeType, cfg)
	}
}
