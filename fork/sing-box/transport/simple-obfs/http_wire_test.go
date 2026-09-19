package obfs

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"
)

type captureConn struct {
	bytes.Buffer
}

func (c *captureConn) Close() error                     { return nil }
func (c *captureConn) LocalAddr() net.Addr              { return nil }
func (c *captureConn) RemoteAddr() net.Addr             { return nil }
func (c *captureConn) SetDeadline(time.Time) error      { return nil }
func (c *captureConn) SetReadDeadline(time.Time) error  { return nil }
func (c *captureConn) SetWriteDeadline(time.Time) error { return nil }
func (c *captureConn) Read(b []byte) (int, error)       { return 0, io.EOF }

// TestHTTPObfsFirstRequestByteIdentical locks the hand-rolled first-packet
// writer to the exact bytes net/http's Request.Write produced before the
// rewrite: header order and the canonical "Sec-Websocket-Key" casing are
// wire-visible and must not drift.
func TestHTTPObfsFirstRequestByteIdentical(t *testing.T) {
	conn := &captureConn{}
	ho := &HTTPObfs{Conn: conn, host: "example.com", port: "8080", firstRequest: true}
	payload := []byte("0123456789ABCDEF")
	if _, err := ho.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := conn.String()

	ua := regexp.MustCompile(`User-Agent: (curl/7\.\d+\.\d+)\r\n`).FindStringSubmatch(got)
	key := regexp.MustCompile(`Sec-Websocket-Key: ([A-Za-z0-9_-]+={0,2})\r\n`).FindStringSubmatch(got)
	if ua == nil || key == nil {
		t.Fatalf("cannot extract random fields from output:\n%q", got)
	}

	req, _ := http.NewRequest("GET", "http://example.com/", bytes.NewReader(payload))
	req.Header.Set("User-Agent", ua[1])
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Host = "example.com:8080"
	req.Header.Set("Sec-WebSocket-Key", key[1])
	req.ContentLength = int64(len(payload))
	var want bytes.Buffer
	if err := req.Write(&want); err != nil {
		t.Fatal(err)
	}

	if got != want.String() {
		t.Fatalf("wire format drifted:\ngot:  %q\nwant: %q", got, want.String())
	}
}
