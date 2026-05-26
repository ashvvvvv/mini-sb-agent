package counter

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
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
	l.burst = bytesPerSecond
	if bytesPerSecond <= 0 {
		l.disabled = true
		l.tokens = 0
		return
	}
	l.disabled = false
	if l.tokens <= 0 || l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}
	l.last = time.Now()
}

func (l *RateLimiter) Rate() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rate
}

func (l *RateLimiter) Wait(n int) {
	if l == nil || n <= 0 {
		return
	}
	remaining := n
	for remaining > 0 {
		wait := l.reserve(remaining)
		if wait <= 0 {
			return
		}
		time.Sleep(wait)
		remaining = 0
	}
}

func (l *RateLimiter) reserve(n int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.disabled || l.rate <= 0 || n <= 0 {
		return 0
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
		return 0
	}
	missing := need - l.tokens
	l.tokens = 0
	return time.Duration(missing / float64(l.rate) * float64(time.Second))
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
	n, err := c.ExtendedConn.Read(b)
	if n > 0 && c.readLimiter != nil {
		c.readLimiter.Wait(n)
	}
	return n, err
}

func (c *RateLimitedConn) Write(b []byte) (int, error) {
	n, err := c.ExtendedConn.Write(b)
	if n > 0 && c.writeLimiter != nil {
		c.writeLimiter.Wait(n)
	}
	return n, err
}

func (c *RateLimitedConn) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 && c.readLimiter != nil {
		c.readLimiter.Wait(buffer.Len())
	}
	return err
}

func (c *RateLimitedConn) WriteBuffer(buffer *buf.Buffer) error {
	n := buffer.Len()
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err == nil && n > 0 && c.writeLimiter != nil {
		c.writeLimiter.Wait(n)
	}
	return err
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
		p.readLimiter.Wait(buff.Len())
	}
	return dest, err
}

func (p *RateLimitedPacketConn) WritePacket(buff *buf.Buffer, dest M.Socksaddr) error {
	n := buff.Len()
	err := p.PacketConn.WritePacket(buff, dest)
	if err == nil && n > 0 && p.writeLimiter != nil {
		p.writeLimiter.Wait(n)
	}
	return err
}

func (p *RateLimitedPacketConn) Upstream() any { return p.PacketConn }

var _ io.Reader = (*RateLimitedConn)(nil)
