package counter

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const (
	defaultBurstWindow = 200 * time.Millisecond
	maxWriteChunk      = 64 * 1024
	maxReadChunk       = 64 * 1024
)

// RateLimiter is a small token-bucket limiter shared by all connections for a
// node or a user. It is intentionally byte-oriented and dependency-free so the
// no-user-limit build can compile it out with build tags.
type RateLimiter struct {
	mu       sync.Mutex
	rate     int64
	burst    int64
	tokens   float64
	last     time.Time
	disabled bool
	closed   bool
}

func NewRateLimiter(bytesPerSecond int64) *RateLimiter {
	l := &RateLimiter{last: time.Now()}
	l.SetRate(bytesPerSecond)
	return l
}

func (l *RateLimiter) SetRate(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = bytesPerSecond
	l.burst = bytesPerSecond * int64(defaultBurstWindow) / int64(time.Second)
	if l.burst < maxWriteChunk {
		l.burst = maxWriteChunk
	}
	if bytesPerSecond <= 0 {
		l.disabled = true
		l.tokens = 0
		return
	}
	l.closed = false
	l.disabled = false
	if l.tokens < 0 || l.tokens > float64(l.burst) {
		l.tokens = 0
	}
	l.last = time.Now()
}

func (l *RateLimiter) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	l.disabled = true
	l.rate = 0
	l.burst = 0
	l.tokens = 0
}

func (l *RateLimiter) Closed() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *RateLimiter) Rate() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rate
}

func (l *RateLimiter) Wait(n int) error {
	if l == nil || n <= 0 {
		return nil
	}
	remaining := n
	for remaining > 0 {
		chunk := remaining
		l.mu.Lock()
		burst := l.burst
		disabled := l.disabled || l.rate <= 0 || l.closed
		l.mu.Unlock()
		if disabled {
			_, err := l.reserve(0)
			return err
		}
		if burst > 0 && int64(chunk) > burst {
			chunk = int(burst)
		}
		wait, err := l.reserve(chunk)
		if err != nil {
			return err
		}
		if wait > 0 {
			time.Sleep(wait)
		}
		remaining -= chunk
	}
	return nil
}

var ErrLimiterClosed = errors.New("rate limiter closed")

// Allow reports whether n bytes can pass immediately. Unlike Wait, it never
// sleeps. Packet-based protocols such as QUIC/Hysteria2 run their own pacing,
// ACK, and congestion-control loops above the UDP socket; blocking the packet
// read/write path with time.Sleep can stall those loops and cause severe
// throughput oscillation. Allow is therefore used only by PacketConn wrappers
// to make a fast pass/drop decision while still charging accepted bytes to the
// shared limiter bucket.
func (l *RateLimiter) Allow(n int) (bool, error) {
	if l == nil || n <= 0 {
		return true, nil
	}
	return l.allow(n)
}

func (l *RateLimiter) allow(n int) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false, ErrLimiterClosed
	}
	if l.disabled || l.rate <= 0 || n <= 0 {
		return true, nil
	}
	now := time.Now()
	if l.last.IsZero() {
		l.last = now
	}
	elapsed := now.Sub(l.last).Seconds()
	if elapsed > 0 {
		l.tokens += elapsed * float64(l.rate)
		if l.tokens > float64(l.burst) {
			l.tokens = float64(l.burst)
		}
		l.last = now
	}
	need := float64(n)
	if need > float64(l.burst) {
		need = float64(l.burst)
	}
	if l.tokens < need {
		return false, nil
	}
	l.tokens -= need
	return true, nil
}

func (l *RateLimiter) reserve(n int) (time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, ErrLimiterClosed
	}
	if l.disabled || l.rate <= 0 || n <= 0 {
		return 0, nil
	}
	now := time.Now()
	if l.last.IsZero() {
		l.last = now
	}
	elapsed := now.Sub(l.last).Seconds()
	if elapsed > 0 {
		l.tokens += elapsed * float64(l.rate)
		if l.tokens > float64(l.burst) {
			l.tokens = float64(l.burst)
		}
		l.last = now
	}
	need := float64(n)
	if need > float64(l.burst) {
		need = float64(l.burst)
	}
	if l.tokens >= need {
		l.tokens -= need
		return 0, nil
	}
	missing := need - l.tokens
	l.tokens -= need
	return time.Duration(missing / float64(l.rate) * float64(time.Second)), nil
}

type RateLimitedConn struct {
	N.ExtendedConn
	readLimiter  *RateLimiter
	writeLimiter *RateLimiter
}

func NewRateLimitedConn(conn net.Conn, readLimiter, writeLimiter *RateLimiter) net.Conn {
	if readLimiter == nil && writeLimiter == nil {
		return conn
	}
	return &RateLimitedConn{ExtendedConn: bufio.NewExtendedConn(conn), readLimiter: readLimiter, writeLimiter: writeLimiter}
}

func (c *RateLimitedConn) Read(b []byte) (int, error) {
	readBuf := b
	if c.readLimiter != nil && len(readBuf) > maxReadChunk {
		readBuf = readBuf[:maxReadChunk]
	}
	n, err := c.ExtendedConn.Read(readBuf)
	if n > 0 && c.readLimiter != nil {
		if waitErr := c.readLimiter.Wait(n); waitErr != nil && err == nil {
			err = waitErr
		}
	}
	return n, err
}

func (c *RateLimitedConn) Write(b []byte) (int, error) {
	if len(b) == 0 || c.writeLimiter == nil {
		return c.ExtendedConn.Write(b)
	}
	total := 0
	for total < len(b) {
		end := total + maxWriteChunk
		if end > len(b) {
			end = len(b)
		}
		chunk := b[total:end]
		if err := c.writeLimiter.Wait(len(chunk)); err != nil {
			return total, err
		}
		n, err := c.ExtendedConn.Write(chunk)
		total += n
		if err != nil {
			return total, err
		}
		if n != len(chunk) {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func (c *RateLimitedConn) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 && c.readLimiter != nil {
		err = c.readLimiter.Wait(buffer.Len())
	}
	return err
}

func (c *RateLimitedConn) WriteBuffer(buffer *buf.Buffer) error {
	if buffer.Len() == 0 || c.writeLimiter == nil {
		return c.ExtendedConn.WriteBuffer(buffer)
	}
	for buffer.Len() > maxWriteChunk {
		if err := c.writeLimiter.Wait(maxWriteChunk); err != nil {
			return err
		}
		if _, err := c.ExtendedConn.Write(buffer.To(maxWriteChunk)); err != nil {
			return err
		}
		buffer.Advance(maxWriteChunk)
	}
	if buffer.Len() > 0 {
		if err := c.writeLimiter.Wait(buffer.Len()); err != nil {
			return err
		}
		return c.ExtendedConn.WriteBuffer(buffer)
	}
	return nil
}

func (c *RateLimitedConn) Upstream() any { return c.ExtendedConn }

type RateLimitedPacketConn struct {
	N.PacketConn
	readLimiter  *RateLimiter
	writeLimiter *RateLimiter
}

func NewRateLimitedPacketConn(conn N.PacketConn, readLimiter, writeLimiter *RateLimiter) N.PacketConn {
	if readLimiter == nil && writeLimiter == nil {
		return conn
	}
	return &RateLimitedPacketConn{PacketConn: conn, readLimiter: readLimiter, writeLimiter: writeLimiter}
}

func (p *RateLimitedPacketConn) ReadPacket(buff *buf.Buffer) (M.Socksaddr, error) {
	dest, err := p.PacketConn.ReadPacket(buff)
	if err == nil && buff.Len() > 0 && p.readLimiter != nil {
		allowed, waitErr := p.readLimiter.Allow(buff.Len())
		if waitErr != nil {
			err = waitErr
		} else if !allowed {
			buff.Reset()
		}
	}
	return dest, err
}

func (p *RateLimitedPacketConn) WritePacket(buff *buf.Buffer, dest M.Socksaddr) error {
	if buff.Len() > 0 && p.writeLimiter != nil {
		allowed, err := p.writeLimiter.Allow(buff.Len())
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
	}
	return p.PacketConn.WritePacket(buff, dest)
}

func (p *RateLimitedPacketConn) Upstream() any { return p.PacketConn }

var _ io.Reader = (*RateLimitedConn)(nil)
