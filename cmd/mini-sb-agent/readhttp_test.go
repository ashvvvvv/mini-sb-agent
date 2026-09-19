package main

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestReadHTTPRequestParsesTarget(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = server.Write([]byte("GET /stats?delta=1 HTTP/1.1\r\nHost: x\r\nUser-Agent: t\r\n\r\n"))
	}()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	req, err := readHTTPRequest(client)
	if err != nil {
		t.Fatal(err)
	}
	if req.target != "/stats?delta=1" {
		t.Fatalf("target = %q", req.target)
	}
}

func TestReadHTTPRequestRejectsEndlessLine(t *testing.T) {
	// A hostile client must be rejected at the 4KB line buffer, not buffer
	// unboundedly until the connection deadline.
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		chunk := strings.Repeat("A", 64*1024)
		for {
			if _, err := server.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}()
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	_, err := readHTTPRequest(client)
	if err == nil {
		t.Fatal("expected error for endless header line")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("rejection took %s; the line buffer is not bounded", time.Since(start))
	}
}

func TestReadHTTPRequestRejectsMalformed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = server.Write([]byte("\r\n"))
	}()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := readHTTPRequest(client); err == nil {
		t.Fatal("expected error for malformed request")
	}
}
