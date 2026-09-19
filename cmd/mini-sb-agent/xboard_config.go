package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mini-sb-agent/panelapi"
)

type xboardGenerateOptions struct {
	PanelURL        string
	PanelToken      string
	PanelNodeID     string
	PanelNodeType   string
	NodeMode        string
	VLESSNodeID     string
	HY2NodeID       string
	Out             string
	NodeTypeOutPath string
	CertPath        string
	KeyPath         string
}

func runXboardGenerateConfig(args []string) int {
	fs := flag.NewFlagSet("xboard-generate-config", flag.ContinueOnError)
	var opts xboardGenerateOptions
	fs.StringVar(&opts.PanelURL, "panel-url", "", "Panel API base URL")
	fs.StringVar(&opts.PanelToken, "panel-token", "", "Panel API node token")
	fs.StringVar(&opts.PanelNodeID, "panel-node-id", "", "Panel API node id for single-node mode")
	fs.StringVar(&opts.PanelNodeType, "panel-node-type", "", "deprecated; use --node-mode")
	fs.StringVar(&opts.NodeMode, "node-mode", "vless", "node mode: vless, hy2, both")
	fs.StringVar(&opts.VLESSNodeID, "vless-node-id", "", "VLESS Reality node id; default uses --panel-node-id")
	fs.StringVar(&opts.HY2NodeID, "hy2-node-id", "", "HY2 node id; default uses --panel-node-id")
	fs.StringVar(&opts.Out, "out", "config.generated.json", "output sing-box config path")
	fs.StringVar(&opts.NodeTypeOutPath, "node-type-out", "", "optional file to write selected node mode")
	fs.StringVar(&opts.CertPath, "cert", "", "HY2 certificate path; default is cert.pem next to --out")
	fs.StringVar(&opts.KeyPath, "key", "", "HY2 private key path; default is key.pem next to --out")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opts.PanelURL == "" || opts.PanelToken == "" || opts.Out == "" {
		fmt.Fprintln(os.Stderr, "missing required --panel-url/--panel-token/--out")
		return 2
	}
	if opts.NodeMode == "" && opts.PanelNodeType != "" {
		opts.NodeMode = normalizeNodeMode(opts.PanelNodeType)
	}
	if opts.NodeMode == "" {
		opts.NodeMode = "vless"
	}
	mode, err := generateXboardConfig(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("generated %s for node_mode=%s\n", opts.Out, mode)
	return 0
}

func normalizeNodeMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "vless", "vless-reality", "reality":
		return "vless"
	case "hy2", "hysteria", "hysteria2":
		return "hy2"
	case "both", "dual", "all":
		return "both"
	default:
		return strings.ToLower(strings.TrimSpace(v))
	}
}

func generateXboardConfig(ctx context.Context, opts xboardGenerateOptions) (string, error) {
	mode := normalizeNodeMode(opts.NodeMode)
	if mode == "" || mode == "auto" {
		mode = normalizeNodeMode(opts.PanelNodeType)
	}
	if mode == "" || mode == "auto" {
		mode = "vless"
	}
	var inbounds []any
	switch mode {
	case "vless":
		id := firstNonEmpty(opts.VLESSNodeID, opts.PanelNodeID)
		if id == "" {
			return "", fmt.Errorf("vless mode requires --panel-node-id or --vless-node-id")
		}
		cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, id, "vless").FetchNodeConfig(ctx)
		if err != nil {
			return "", err
		}
		inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, inbound)
	case "hy2":
		id := firstNonEmpty(opts.HY2NodeID, opts.PanelNodeID)
		if id == "" {
			return "", fmt.Errorf("hy2 mode requires --panel-node-id or --hy2-node-id")
		}
		cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, id, "hysteria").FetchNodeConfig(ctx)
		if err != nil {
			return "", err
		}
		inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, inbound)
	case "both":
		vlessID := firstNonEmpty(opts.VLESSNodeID, opts.PanelNodeID)
		hy2ID := opts.HY2NodeID
		if vlessID == "" || hy2ID == "" {
			return "", fmt.Errorf("both mode requires --vless-node-id and --hy2-node-id")
		}
		vlessCfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, vlessID, "vless").FetchNodeConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("fetch vless node config: %w", err)
		}
		hy2Cfg, err := panelapi.NewClient(opts.PanelURL, opts.PanelToken, hy2ID, "hysteria").FetchNodeConfig(ctx)
		if err != nil {
			return "", fmt.Errorf("fetch hy2 node config: %w", err)
		}
		vlessInbound, err := inboundFromNodeConfig(vlessCfg, opts, defaultListen(vlessCfg.ListenIP))
		if err != nil {
			return "", err
		}
		hy2Inbound, err := inboundFromNodeConfig(hy2Cfg, opts, defaultListen(hy2Cfg.ListenIP))
		if err != nil {
			return "", err
		}
		inbounds = append(inbounds, vlessInbound, hy2Inbound)
	default:
		return "", fmt.Errorf("--node-mode must be vless, hy2, or both")
	}
	data, err := buildSingBoxConfigFromInbounds(inbounds)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Out), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(opts.Out, data, 0600); err != nil {
		return "", err
	}
	if opts.NodeTypeOutPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.NodeTypeOutPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(opts.NodeTypeOutPath, []byte(mode+"\n"), 0600); err != nil {
			return "", err
		}
	}
	return mode, nil
}

func defaultListen(listen string) string {
	if listen == "" {
		return "::"
	}
	return listen
}

func buildSingBoxConfigFromInbounds(inbounds []any) ([]byte, error) {
	root := map[string]any{
		"log":      map[string]any{"level": "warn", "timestamp": true},
		"inbounds": inbounds,
		"outbounds": []any{
			map[string]any{"type": "direct", "tag": "direct"},
			map[string]any{"type": "block", "tag": "block"},
			map[string]any{"type": "dns", "tag": "dns-out"},
		},
		"route": map[string]any{"final": "direct"},
	}
	return json.MarshalIndent(root, "", "  ")
}

func buildSingBoxConfigFromNode(cfg panelapi.NodeConfig, opts xboardGenerateOptions) ([]byte, error) {
	inbound, err := inboundFromNodeConfig(cfg, opts, defaultListen(cfg.ListenIP))
	if err != nil {
		return nil, err
	}
	return buildSingBoxConfigFromInbounds([]any{inbound})
}

// generateTopologyConfig builds a single sing-box process containing any number
// of VLESS Reality / Hysteria2 inbounds. Each inbound gets a stable tag and an
// inbound-matching route rule to its own direct or Shadowsocks outbound.
func generateTopologyConfig(ctx context.Context, panelURL, panelToken string, topology panelapi.Topology, out, certPath, keyPath string) error {
	if err := topology.Validate(); err != nil {
		return err
	}
	var inbounds []any
	outbounds := []any{
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	}
	var rules []any
	for _, node := range topology.Nodes {
		cfg, err := panelapi.NewClient(panelURL, panelToken, node.NodeID, node.PanelNodeType()).FetchNodeConfig(ctx)
		if err != nil {
			return fmt.Errorf("fetch node %s: %w", node.Key(), err)
		}
		if err := validateNodeProtocol(node, cfg); err != nil {
			return err
		}
		nodeOpts := xboardGenerateOptions{Out: out, CertPath: certPath, KeyPath: keyPath}
		if len(topology.Nodes) > 1 && normalizeNodeMode(node.NodeType) == "hy2" {
			base := filepath.Dir(out)
			nodeOpts.CertPath = filepath.Join(base, "cert-"+node.NodeID+".pem")
			nodeOpts.KeyPath = filepath.Join(base, "key-"+node.NodeID+".pem")
		}
		inbound, err := inboundFromNodeConfig(cfg, nodeOpts, defaultListen(cfg.ListenIP))
		if err != nil {
			return fmt.Errorf("build node %s inbound: %w", node.Key(), err)
		}
		inbound["tag"] = node.InboundTag()
		inbounds = append(inbounds, inbound)

		outTag := node.OutboundTag()
		switch strings.ToLower(strings.TrimSpace(node.Outbound.Type)) {
		case "direct":
			outTag = "direct"
		case "shadowsocks", "ss":
			outbounds = append(outbounds, outboundFromNodeSpec(node))
		}
		rules = append(rules, map[string]any{"inbound": []string{node.InboundTag()}, "outbound": outTag})
	}
	root := map[string]any{
		"log":       map[string]any{"level": "warn", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     map[string]any{"rules": rules, "final": "block"},
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return os.WriteFile(out, data, 0600)
}

func outboundFromNodeSpec(node panelapi.NodeSpec) map[string]any {
	return map[string]any{
		"type": "shadowsocks", "tag": node.OutboundTag(),
		"server": node.Outbound.Server, "server_port": node.Outbound.ServerPort,
		"method": node.Outbound.Method, "password": node.Outbound.Password,
		"plugin": node.Outbound.Plugin, "plugin_opts": node.Outbound.PluginOpts,
	}
}

func inboundFromNodeConfig(cfg panelapi.NodeConfig, opts xboardGenerateOptions, listen string) (map[string]any, error) {
	switch strings.ToLower(cfg.Protocol) {
	case "vless":
		return vlessInboundFromNodeConfig(cfg, listen)
	case "hysteria", "hysteria2":
		if cfg.Version != 0 && cfg.Version != 2 {
			return nil, fmt.Errorf("unsupported hysteria version %d", cfg.Version)
		}
		return hy2InboundFromNodeConfig(cfg, opts, listen)
	default:
		return nil, fmt.Errorf("unsupported node protocol %q", cfg.Protocol)
	}
}

func vlessInboundFromNodeConfig(cfg panelapi.NodeConfig, listen string) (map[string]any, error) {
	if cfg.ServerPort <= 0 {
		return nil, fmt.Errorf("vless server_port is missing")
	}
	if cfg.TLSSettings.PrivateKey == "" {
		return nil, fmt.Errorf("vless reality private_key is missing")
	}
	serverName := firstNonEmpty(cfg.TLSSettings.ServerName, "www.microsoft.com")
	handshakePort := 443
	if cfg.TLSSettings.ServerPort.String() != "" {
		if p, err := strconv.Atoi(cfg.TLSSettings.ServerPort.String()); err == nil && p > 0 {
			handshakePort = p
		}
	}
	shortID := cfg.TLSSettings.ShortID
	shortIDs := []string{}
	if shortID != "" {
		shortIDs = []string{shortID}
	}
	in := map[string]any{
		"type":        "vless",
		"tag":         "vless-in",
		"listen":      listen,
		"listen_port": cfg.ServerPort,
		"users":       []any{},
		"tls": map[string]any{
			"enabled":     true,
			"server_name": serverName,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": serverName, "server_port": handshakePort},
				"private_key": cfg.TLSSettings.PrivateKey,
				"short_id":    shortIDs,
			},
		},
	}
	return in, nil
}

func hy2InboundFromNodeConfig(cfg panelapi.NodeConfig, opts xboardGenerateOptions, listen string) (map[string]any, error) {
	if cfg.ServerPort <= 0 {
		return nil, fmt.Errorf("hysteria2 server_port is missing")
	}
	outDir := filepath.Dir(opts.Out)
	certPath := opts.CertPath
	if certPath == "" {
		certPath = filepath.Join(outDir, "cert.pem")
	}
	keyPath := opts.KeyPath
	if keyPath == "" {
		keyPath = filepath.Join(outDir, "key.pem")
	}
	serverName := firstNonEmpty(cfg.ServerName, cfg.TLSSettings.ServerName, "bing.com")
	if err := ensureSelfSignedCert(certPath, keyPath, serverName); err != nil {
		return nil, err
	}
	in := map[string]any{
		"type":        "hysteria2",
		"tag":         "hy2-in",
		"listen":      listen,
		"listen_port": cfg.ServerPort,
		"users":       []any{},
		"tls": map[string]any{
			"enabled":          true,
			"server_name":      serverName,
			"certificate_path": certPath,
			"key_path":         keyPath,
		},
	}
	if cfg.UpMbps > 0 {
		in["up_mbps"] = cfg.UpMbps
	}
	if cfg.DownMbps > 0 {
		in["down_mbps"] = cfg.DownMbps
	}
	if cfg.Obfs != "" && cfg.ObfsPassword != "" {
		in["obfs"] = map[string]any{"type": cfg.Obfs, "password": cfg.ObfsPassword}
	}
	return in, nil
}

func ensureSelfSignedCert(certPath, keyPath, serverName string) error {
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("cert/key path is empty")
	}
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	tpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(serverName); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else {
		tpl.DNSNames = []string{serverName}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certFile.Close()
		return err
	}
	if err := certFile.Close(); err != nil {
		return err
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		keyFile.Close()
		return err
	}
	return keyFile.Close()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func validateNodeProtocol(node panelapi.NodeSpec, cfg panelapi.NodeConfig) error {
	norm := func(p string) string {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "vless", "vless-reality", "reality":
			return "vless"
		case "hy2", "hysteria", "hysteria2":
			return "hysteria"
		default:
			return strings.ToLower(strings.TrimSpace(p))
		}
	}
	if norm(node.NodeType) != norm(cfg.Protocol) {
		return fmt.Errorf("node %s protocol mismatch: topology specifies %q but panel returned %q", node.NodeID, node.NodeType, cfg.Protocol)
	}
	return nil
}
