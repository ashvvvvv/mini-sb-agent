package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"mini-sb-agent/panelapi"
)

type BuildCapabilities struct {
	BuildID             string `json:"build_id"`
	VLESSInbound        bool   `json:"vless_inbound"`
	Hysteria2Inbound    bool   `json:"hysteria2_inbound"`
	ShadowsocksOutbound bool   `json:"shadowsocks_outbound"`
	DirectOutbound      bool   `json:"direct_outbound"`
	UserRateLimit       bool   `json:"user_rate_limit"`
	TunInbound          bool   `json:"tun_inbound"`
}

func currentBuildCapabilities() BuildCapabilities {
	caps := BuildCapabilities{
		VLESSInbound:        vlessBuildEnabled,
		Hysteria2Inbound:    hysteria2BuildEnabled,
		ShadowsocksOutbound: shadowsocksOutboundBuildEnabled,
		DirectOutbound:      true,
		UserRateLimit:       userRateLimitBuildEnabled,
		TunInbound:          tunBuildEnabled,
	}
	parts := make([]string, 0, 4)
	if caps.VLESSInbound {
		parts = append(parts, "vless")
	}
	if caps.Hysteria2Inbound {
		parts = append(parts, "hy2")
	}
	parts = append(parts, "direct")
	if caps.ShadowsocksOutbound {
		parts = append(parts, "ss")
	}
	caps.BuildID = strings.Join(parts, "-")
	return caps
}

func validateTopologyCapabilities(topology panelapi.Topology, caps BuildCapabilities) error {
	for _, node := range topology.Nodes {
		switch node.PanelNodeType() {
		case "vless":
			if !caps.VLESSInbound {
				return fmt.Errorf("build %q does not include vless inbound", caps.BuildID)
			}
		case "hysteria":
			if !caps.Hysteria2Inbound {
				return fmt.Errorf("build %q does not include hysteria2 inbound", caps.BuildID)
			}
		}
		if strings.EqualFold(node.Outbound.Type, "shadowsocks") || strings.EqualFold(node.Outbound.Type, "ss") {
			if !caps.ShadowsocksOutbound {
				return fmt.Errorf("build %q does not include shadowsocks outbound", caps.BuildID)
			}
		}
	}
	return nil
}

// printCapabilities reports the compiled-in protocol capabilities of this
// binary so installers can pick the right build for a topology.
func printCapabilities() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(currentBuildCapabilities()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
