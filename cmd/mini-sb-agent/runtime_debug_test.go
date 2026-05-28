package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRuntimeDebugLoggerWritesCSV(t *testing.T) {
	path := t.TempDir() + "/runtime.csv"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := startRuntimeDebugLogger(ctx, path, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if strings.Count(string(data), "\n") >= 2 {
			text := string(data)
			if !strings.HasPrefix(text, "ts,pid,utime_ticks") {
				t.Fatalf("missing csv header: %q", text)
			}
			if !strings.Contains(text, ",goroutines,") {
				t.Fatalf("missing runtime fields: %q", text)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("runtime debug log was not written")
}

func TestCollectRuntimeDebugSample(t *testing.T) {
	s := collectRuntimeDebugSample()
	if s.pid <= 0 {
		t.Fatalf("pid not populated: %+v", s)
	}
	if s.goroutines <= 0 {
		t.Fatalf("goroutines not populated: %+v", s)
	}
	if s.threads <= 0 {
		t.Fatalf("threads not populated: %+v", s)
	}
}
