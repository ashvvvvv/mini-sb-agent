package counter

import (
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type recordingPacketConn struct {
	written int
	readBuf *buf.Buffer
}

func (c *recordingPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if c.readBuf != nil {
		_, _ = buffer.Write(c.readBuf.Bytes())
	}
	return M.Socksaddr{}, nil
}

func (c *recordingPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	c.written++
	return nil
}

func (c *recordingPacketConn) Close() error                     { return nil }
func (c *recordingPacketConn) LocalAddr() net.Addr              { return nil }
func (c *recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestRateLimitedPacketConnWriteDropsWithoutSleeping(t *testing.T) {
	limiter := NewRateLimiter(1) // 1 byte/sec; 64 KiB packet would sleep for hours if Wait were used.
	conn := &recordingPacketConn{}
	wrapped := NewRateLimitedPacketConn(conn, nil, limiter)
	packet := buf.As(make([]byte, maxWriteChunk))
	start := time.Now()

	if err := wrapped.WritePacket(packet, M.Socksaddr{}); err != nil {
		t.Fatal(err)
	}

	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("packet limiter slept instead of making a fast drop/pass decision: %s", elapsed)
	}
	if conn.written != 0 {
		t.Fatalf("expected over-limit packet to be dropped, wrote %d packets", conn.written)
	}
}

func TestRateLimitedPacketConnReadDropsWithoutSleeping(t *testing.T) {
	limiter := NewRateLimiter(1)
	conn := &recordingPacketConn{readBuf: buf.As(make([]byte, maxReadChunk))}
	wrapped := NewRateLimitedPacketConn(conn, limiter, nil)
	packet := buf.NewPacket()
	start := time.Now()

	_, err := wrapped.ReadPacket(packet)
	if err != nil {
		t.Fatal(err)
	}

	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("packet read limiter slept instead of making a fast drop/pass decision: %s", elapsed)
	}
	if packet.Len() != 0 {
		t.Fatalf("expected over-limit packet to be dropped/reset, got %d bytes", packet.Len())
	}
}

func TestRateLimiterWaitStillSleepsForTCPPath(t *testing.T) {
	limiter := NewRateLimiter(64 * 1024) // maxWriteChunk should take about one second.
	start := time.Now()
	if err := limiter.Wait(maxWriteChunk); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
		t.Fatalf("Wait returned too quickly; TCP sleep semantics were changed: %s", elapsed)
	}
}
