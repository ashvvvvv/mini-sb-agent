package panelapi

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

// rawServer serves one canned raw response per connection; it exists because
// net/http test servers cannot emit malformed headers like a hostile panel.
func rawServer(t *testing.T, response string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 1024)
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = conn.Read(buf)
				_, _ = fmt.Fprint(conn, response)
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return "http://" + ln.Addr().String()
}

func TestMinHTTPRejectsNegativeContentLength(t *testing.T) {
	// Found by external review: ParseInt("-1") succeeds and make([]byte, -1)
	// panics the whole agent. The guard must turn this into an error.
	url := rawServer(t, "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\n")
	client := minHTTPClient{Timeout: 5 * time.Second}
	_, err := client.do(context.Background(), "GET", url, nil, nil)
	if err == nil {
		t.Fatal("expected error for negative content-length")
	}
}

func TestMinHTTPRejectsNegativeChunkSize(t *testing.T) {
	// Found by fix-verification review: the chunked path had the same panic
	// (ParseInt accepts "-1" in hex too); a hostile panel sending a negative
	// chunk size must get an error, not a process panic.
	url := rawServer(t, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n-1\r\nhello\r\n")
	client := minHTTPClient{Timeout: 5 * time.Second}
	_, err := client.do(context.Background(), "GET", url, nil, nil)
	if err == nil {
		t.Fatal("expected error for negative chunk size")
	}
}

func TestMinHTTPRejectsMalformedStatusLine(t *testing.T) {
	url := rawServer(t, "garbage\r\n\r\n")
	client := minHTTPClient{Timeout: 5 * time.Second}
	_, err := client.do(context.Background(), "GET", url, nil, nil)
	if err == nil {
		t.Fatal("expected error for malformed status line")
	}
}

func TestMinHTTPChunkedBody(t *testing.T) {
	url := rawServer(t, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n3;x=1\r\n wo\r\n0\r\n\r\n")
	client := minHTTPClient{Timeout: 5 * time.Second}
	resp, err := client.do(context.Background(), "GET", url, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "hello wo" {
		t.Fatalf("chunked body = %q", resp.Body)
	}
}
