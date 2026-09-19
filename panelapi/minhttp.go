package panelapi

// Minimal HTTP/1.1 client for panel API calls. It exists so the agent binary
// does not need net/http (and the HTTP/2 machinery it drags in); panel
// endpoints are plain GET/POST exchanges with small JSON bodies. Responses are
// read fully into memory, which is fine for these APIs (< a few MB).
// Redirects are NOT followed (unlike net/http): callers treat 3xx as errors,
// so the panel URL must be configured as the final destination.

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	minHTTPTimeout  = 15 * time.Second
	minHTTPMaxBody  = 16 << 20 // 16MB hard cap for a broken panel response
	minHTTPReadBuf = 16 << 10
)

type minHTTPClient struct {
	Timeout time.Duration
}

type minHTTPResponse struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (m minHTTPClient) do(ctx context.Context, method, rawURL string, headers map[string]string, body []byte) (*minHTTPResponse, error) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = minHTTPTimeout
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported panel url scheme %q", u.Scheme)
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: u.Hostname(),
			NextProtos: []string{"http/1.1"},
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}

	hostHeader := u.Host
	if u.Port() != "" {
		hostHeader = net.JoinHostPort(u.Hostname(), u.Port())
	}
	var req strings.Builder
	req.WriteString(method)
	req.WriteByte(' ')
	req.WriteString(u.RequestURI())
	req.WriteString(" HTTP/1.1\r\nHost: ")
	req.WriteString(hostHeader)
	req.WriteString("\r\n")
	for key, value := range headers {
		req.WriteString(key)
		req.WriteString(": ")
		req.WriteString(value)
		req.WriteString("\r\n")
	}
	if len(body) > 0 {
		req.WriteString("Content-Length: ")
		req.WriteString(strconv.Itoa(len(body)))
		req.WriteString("\r\n")
	}
	req.WriteString("Connection: close\r\n\r\n")
	if _, err := conn.Write([]byte(req.String())); err != nil {
		return nil, err
	}
	if len(body) > 0 {
		if _, err := conn.Write(body); err != nil {
			return nil, err
		}
	}

	reader := bufio.NewReaderSize(conn, minHTTPReadBuf)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(strings.TrimSpace(statusLine))
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return nil, fmt.Errorf("malformed panel response status line %q", strings.TrimSpace(statusLine))
	}
	statusCode, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("malformed panel response status code in %q", strings.TrimSpace(statusLine))
	}
	respHeaders := make(map[string]string, 8)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			respHeaders[strings.ToLower(strings.TrimSpace(line[:idx]))] = strings.TrimSpace(line[idx+1:])
		}
	}
	var respBody []byte
	if strings.Contains(respHeaders["transfer-encoding"], "chunked") {
		respBody, err = readChunkedBody(reader)
	} else if cl := respHeaders["content-length"]; cl != "" {
		var n int64
		n, err = strconv.ParseInt(cl, 10, 64)
		if err == nil && n < 0 {
			err = fmt.Errorf("negative content-length: %d", n)
		}
		if err == nil && n > minHTTPMaxBody {
			err = fmt.Errorf("panel response too large: %d bytes", n)
		}
		if err == nil {
			respBody = make([]byte, n)
			_, err = io.ReadFull(reader, respBody)
		}
	} else {
		respBody, err = io.ReadAll(io.LimitReader(reader, minHTTPMaxBody))
	}
	if err != nil {
		return nil, err
	}
	return &minHTTPResponse{
		StatusCode: statusCode,
		Status:     strings.Join(fields[1:], " "),
		Body:       respBody,
	}, nil
}

func readChunkedBody(reader *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		sizeLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeText := strings.TrimSpace(sizeLine)
		if idx := strings.IndexByte(sizeText, ';'); idx >= 0 {
			sizeText = sizeText[:idx]
		}
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed chunk size %q", sizeText)
		}
		if size < 0 {
			return nil, fmt.Errorf("negative chunk size %q", sizeText)
		}
		if size == 0 {
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return nil, err
				}
				if strings.TrimRight(line, "\r\n") == "" {
					return out, nil
				}
			}
		}
		if size > minHTTPMaxBody || len(out)+int(size) > minHTTPMaxBody {
			return nil, fmt.Errorf("panel chunked response too large")
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		var crlf [2]byte
		if _, err := io.ReadFull(reader, crlf[:]); err != nil {
			return nil, err
		}
	}
}
