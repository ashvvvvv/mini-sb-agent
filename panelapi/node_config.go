package panelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type BaseConfig struct {
	PushInterval int `json:"push_interval,omitempty"`
	PullInterval int `json:"pull_interval,omitempty"`
}

type ECHConfig struct {
	Enabled         bool    `json:"enabled,omitempty"`
	Config          any     `json:"config,omitempty"`
	QueryServerName *string `json:"query_server_name,omitempty"`
	Key             string  `json:"key,omitempty"`
	KeyPath         string  `json:"key_path,omitempty"`
	ConfigPath      *string `json:"config_path,omitempty"`
}

type TLSSettings struct {
	ServerName    string    `json:"server_name,omitempty"`
	ServerPort    string    `json:"server_port,omitempty"`
	PublicKey     string    `json:"public_key,omitempty"`
	PrivateKey    string    `json:"private_key,omitempty"`
	ShortID       string    `json:"short_id,omitempty"`
	AllowInsecure bool      `json:"allow_insecure,omitempty"`
	ECH           ECHConfig `json:"ech,omitempty"`
}

type NodeConfig struct {
	Protocol      string      `json:"protocol,omitempty"`
	ListenIP      string      `json:"listen_ip,omitempty"`
	ServerPort    int         `json:"server_port,omitempty"`
	Network       string      `json:"network,omitempty"`
	NetworkConfig any         `json:"networkSettings,omitempty"`
	TLS           int         `json:"tls,omitempty"`
	Flow          string      `json:"flow,omitempty"`
	Decryption    *string     `json:"decryption,omitempty"`
	TLSSettings   TLSSettings `json:"tls_settings,omitempty"`
	Version       int         `json:"version,omitempty"`
	Host          string      `json:"host,omitempty"`
	ServerName    string      `json:"server_name,omitempty"`
	UpMbps        int         `json:"up_mbps,omitempty"`
	DownMbps      int         `json:"down_mbps,omitempty"`
	Obfs          string      `json:"obfs,omitempty"`
	ObfsPassword  string      `json:"obfs-password,omitempty"`
	BaseConfig    BaseConfig  `json:"base_config,omitempty"`
}

func (c *Client) FetchNodeConfig(ctx context.Context) (NodeConfig, error) {
	ep, err := c.endpoint("/api/v1/server/UniProxy/config")
	if err != nil {
		return NodeConfig{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
	if err != nil {
		return NodeConfig{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return NodeConfig{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var msg struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&msg)
		text := strings.TrimSpace(msg.Message)
		if text == "" {
			text = strings.TrimSpace(msg.Error)
		}
		if text != "" {
			return NodeConfig{}, fmt.Errorf("panel api config status %s: %s", resp.Status, text)
		}
		return NodeConfig{}, fmt.Errorf("panel api config status %s", resp.Status)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return NodeConfig{}, err
	}
	cfg, err := decodeNodeConfig(raw)
	if err != nil {
		return NodeConfig{}, err
	}
	return cfg, nil
}

func decodeNodeConfig(raw json.RawMessage) (NodeConfig, error) {
	var cfg NodeConfig
	if err := json.Unmarshal(raw, &cfg); err == nil && (cfg.Protocol != "" || cfg.ServerPort != 0) {
		return cfg, nil
	}
	var wrapped struct {
		Data NodeConfig `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return NodeConfig{}, err
	}
	if wrapped.Data.Protocol == "" && wrapped.Data.ServerPort == 0 {
		return NodeConfig{}, fmt.Errorf("panel api config response missing node config")
	}
	return wrapped.Data, nil
}

func ProbeNodeConfig(ctx context.Context, baseURL, token, nodeID string) (string, NodeConfig, error) {
	var lastErr error
	for _, nodeType := range []string{"vless", "hysteria2", "hysteria"} {
		cfg, err := NewClient(baseURL, token, nodeID, nodeType).FetchNodeConfig(ctx)
		if err == nil {
			return nodeType, cfg, nil
		}
		lastErr = err
	}
	return "", NodeConfig{}, fmt.Errorf("probe node config failed: %w", lastErr)
}
