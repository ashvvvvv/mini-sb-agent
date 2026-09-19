package obfs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net"

	B "github.com/sagernet/sing/common/buf"
)

// HTTPObfs is shadowsocks http simple-obfs implementation
type HTTPObfs struct {
	net.Conn
	host          string
	port          string
	buf           []byte
	offset        int
	firstRequest  bool
	firstResponse bool
}

func (ho *HTTPObfs) Read(b []byte) (int, error) {
	if ho.buf != nil {
		n := copy(b, ho.buf[ho.offset:])
		ho.offset += n
		if ho.offset == len(ho.buf) {
			B.Put(ho.buf)
			ho.buf = nil
		}
		return n, nil
	}

	if ho.firstResponse {
		buf := B.Get(B.BufferSize)
		n, err := ho.Conn.Read(buf)
		if err != nil {
			B.Put(buf)
			return 0, err
		}
		idx := bytes.Index(buf[:n], []byte("\r\n\r\n"))
		if idx == -1 {
			B.Put(buf)
			return 0, io.EOF
		}
		ho.firstResponse = false
		length := n - (idx + 4)
		n = copy(b, buf[idx+4:n])
		if length > n {
			ho.buf = buf[:idx+4+length]
			ho.offset = idx + 4 + n
		} else {
			B.Put(buf)
		}
		return n, nil
	}
	return ho.Conn.Read(b)
}

func (ho *HTTPObfs) Write(b []byte) (int, error) {
	if ho.firstRequest {
		randBytes := make([]byte, 16)
		rand.Read(randBytes)
		// Hand-rolled HTTP/1.1 request writer: keeps net/http out of the
		// binary for this optional SS plugin mode.
		host := ho.host
		if ho.port != "80" {
			host = fmt.Sprintf("%s:%s", ho.host, ho.port)
		}
		var req bytes.Buffer
		// Byte-identical to the previous net/http Request.Write output:
		// header order and the canonical "Sec-Websocket-Key" casing (lowercase
		// "socket") must match exactly, verified against the reference encoder.
		req.WriteString("GET / HTTP/1.1\r\n")
		req.WriteString("Host: " + host + "\r\n")
		req.WriteString(fmt.Sprintf("User-Agent: curl/7.%d.%d\r\n", rand.Int()%54, rand.Int()%2))
		req.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(b)))
		req.WriteString("Connection: Upgrade\r\n")
		req.WriteString("Sec-Websocket-Key: " + base64.URLEncoding.EncodeToString(randBytes) + "\r\n")
		req.WriteString("Upgrade: websocket\r\n")
		req.WriteString("\r\n")
		req.Write(b)
		_, err := ho.Conn.Write(req.Bytes())
		ho.firstRequest = false
		return len(b), err
	}

	return ho.Conn.Write(b)
}

func (ho *HTTPObfs) Upstream() any {
	return ho.Conn
}

// NewHTTPObfs return a HTTPObfs
func NewHTTPObfs(conn net.Conn, host string, port string) net.Conn {
	return &HTTPObfs{
		Conn:          conn,
		firstRequest:  true,
		firstResponse: true,
		host:          host,
		port:          port,
	}
}
