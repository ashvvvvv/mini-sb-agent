//go:build tun

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOptionsAcceptsTunInboundWhenTunTagEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [{"type": "tun", "tag": "tun-in"}],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOptions(path); err != nil {
		t.Fatalf("tun inbound should load when built with -tags tun: %v", err)
	}
}
