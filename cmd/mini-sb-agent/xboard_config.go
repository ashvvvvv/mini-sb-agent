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
	fs.StringVar(&opts.PanelNodeID, "panel-node-id", "", "Panel API node id")
	fs.StringVar(&opts.PanelNodeType, "panel-node-type", "auto", "Panel API node type: auto, vless, hysteria2, hysteria")
	fs.StringVar(&opts.Out, "out", "config.generated.json", "output sing-box config path")
	fs.StringVar(&opts.NodeTypeOutPath, "node-type-out", "", "optional file to write detected node type")
	fs.StringVar(&opts.CertPath, "cert", "", "HY2 certificate path; default is cert.pem next to --out")
	fs.StringVar(&opts.KeyPath, "key", "", "HY2 private key path; default is key.pem next to --out")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opts.PanelURL == "" || opts.PanelToken == "" || opts.PanelNodeID == "" || opts.Out == "" {
		fmt.Fprintln(os.Stderr, "missing required --panel-url/--panel-token/--panel-node-id/--out")
		return 2
	}
	nodeType, err := generateXboardConfig(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("generated %s for node_type=%s\n", opts.Out, nodeType)
	return 0
}

func generateXboardConfig(ctx context.Context, opts xboardGenerateOptions) (string, error) {
	nodeType := opts.PanelNodeType
	var cfg panelapi.NodeConfig
	var err error
	if nodeType == "" || nodeType == "auto" {
		nodeType, cfg, err = panelapi.ProbeNodeConfig(ctx, opts.PanelURL, opts.PanelToken, opts.PanelNodeID)
	} else {
		cfg, err = panelapi.NewClient(opts.PanelURL, opts.PanelToken, opts.PanelNodeID, nodeType).FetchNodeConfig(ctx)
	}
	if err != nil {
		return "", err
	}
	data, err := buildSingBoxConfigFromNode(cfg, opts)
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
		if err := os.WriteFile(opts.NodeTypeOutPath, []byte(nodeType+"\n"), 0600); err != nil {
			return "", err
		}
	}
	return nodeType, nil
}

func buildSingBoxConfigFromNode(cfg panelapi.NodeConfig, opts xboardGenerateOptions) ([]byte, error) {
	listen := cfg.ListenIP
	if listen == "" {
		listen = "::"
	}
	inbound, err := inboundFromNodeConfig(cfg, opts, listen)
	if err != nil {
		return nil, err
	}
	root := map[string]any{
		"log":      map[string]any{"level": "warn", "timestamp": true},
		"inbounds": []any{inbound},
		"outbounds": []any{
			map[string]any{"type": "direct", "tag": "direct"},
			map[string]any{"type": "block", "tag": "block"},
			map[string]any{"type": "dns", "tag": "dns-out"},
		},
		"route": map[string]any{"final": "direct"},
	}
	return json.MarshalIndent(root, "", "  ")
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
	if cfg.TLSSettings.ServerPort != "" {
		if p, err := strconv.Atoi(cfg.TLSSettings.ServerPort); err == nil && p > 0 {
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
