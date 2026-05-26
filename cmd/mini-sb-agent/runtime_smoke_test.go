package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRuntimeSmokeIdleMemory(t *testing.T) {
	if os.Getenv("MINI_SB_AGENT_RUNTIME_SMOKE") != "1" {
		t.Skip("set MINI_SB_AGENT_RUNTIME_SMOKE=1 to run runtime smoke memory test")
	}
	bin := os.Getenv("MINI_SB_AGENT_BIN")
	if bin == "" {
		t.Fatal("MINI_SB_AGENT_BIN is required")
	}
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")
	socketPath := filepath.Join(tmp, "mini-sb-agent.sock")
	config := `{
  "log": {"disabled": true},
  "inbounds": [],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-config", configPath, "-api", "unix:"+socketPath, "-panel-every", "10s")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stats socket not ready at %s", socketPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)
	rssKB, err := readRSSKB(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mini-sb-agent idle RSS: %.2f MiB (%d KB)", float64(rssKB)/1024, rssKB)
	fmt.Printf("MEM_RSS_KB=%d\n", rssKB)
}

func readRSSKB(pid int) (int64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("bad VmRSS line: %q", line)
			}
			return strconv.ParseInt(fields[1], 10, 64)
		}
	}
	return 0, fmt.Errorf("VmRSS not found")
}
