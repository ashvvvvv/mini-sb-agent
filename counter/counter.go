package counter

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type TrafficCounter struct {
	Counters sync.Map // map[string]*TrafficStorage, key is stable panel user id/name
}

type TrafficStorage struct {
	UpCounter     atomic.Int64
	DownCounter   atomic.Int64
	CommittedUp   atomic.Int64
	CommittedDown atomic.Int64
}

func NewTrafficCounter() *TrafficCounter { return &TrafficCounter{} }

func (c *TrafficCounter) GetCounter(user string) *TrafficStorage {
	if v, ok := c.Counters.Load(user); ok {
		return v.(*TrafficStorage)
	}
	s := &TrafficStorage{}
	v, _ := c.Counters.LoadOrStore(user, s)
	return v.(*TrafficStorage)
}

func (c *TrafficCounter) Snapshot(reset bool) map[string][2]int64 {
	out := map[string][2]int64{}
	c.Counters.Range(func(k, v any) bool {
		s := v.(*TrafficStorage)
		out[k.(string)] = [2]int64{s.UpCounter.Load(), s.DownCounter.Load()}
		return true
	})
	return out
}

func (c *TrafficCounter) SnapshotDelta() map[string][2]int64 {
	out := map[string][2]int64{}
	c.Counters.Range(func(k, v any) bool {
		s := v.(*TrafficStorage)
		up := s.UpCounter.Load() - s.CommittedUp.Load()
		down := s.DownCounter.Load() - s.CommittedDown.Load()
		if up > 0 || down > 0 {
			out[k.(string)] = [2]int64{up, down}
		}
		return true
	})
	return out
}

// CommitSnapshot must be called serially and exactly once for a successfully pushed snapshot.
// New traffic arriving between SnapshotDelta and CommitSnapshot is intentionally left for the next snapshot.
func (c *TrafficCounter) CommitSnapshot(snapshot map[string][2]int64) {
	for user, d := range snapshot {
		if v, ok := c.Counters.Load(user); ok {
			s := v.(*TrafficStorage)
			s.CommittedUp.Add(d[0])
			s.CommittedDown.Add(d[1])
		}
	}
}

func (c *TrafficCounter) RemoveAbsent(active map[string]struct{}) {
	c.Counters.Range(func(k, v any) bool {
		user := k.(string)
		if _, ok := active[user]; !ok {
			s := v.(*TrafficStorage)
			if s.UpCounter.Load() == s.CommittedUp.Load() && s.DownCounter.Load() == s.CommittedDown.Load() {
				c.Counters.Delete(user)
			}
		}
		return true
	})
}

type ConnCounter struct {
	N.ExtendedConn
	storage *TrafficStorage
}

func NewConnCounter(conn net.Conn, s *TrafficStorage) net.Conn {
	return &ConnCounter{ExtendedConn: bufio.NewExtendedConn(conn), storage: s}
}
func (c *ConnCounter) Read(b []byte) (int, error) {
	n, err := c.ExtendedConn.Read(b)
	if n > 0 {
		c.storage.UpCounter.Add(int64(n))
	}
	return n, err
}
func (c *ConnCounter) Write(b []byte) (int, error) {
	n, err := c.ExtendedConn.Write(b)
	if n > 0 {
		c.storage.DownCounter.Add(int64(n))
	}
	return n, err
}
func (c *ConnCounter) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 {
		c.storage.UpCounter.Add(int64(buffer.Len()))
	}
	return err
}
func (c *ConnCounter) WriteBuffer(buffer *buf.Buffer) error {
	n := buffer.Len()
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err == nil && n > 0 {
		c.storage.DownCounter.Add(int64(n))
	}
	return err
}
func (c *ConnCounter) Upstream() any { return c.ExtendedConn }

type PacketConnCounter struct {
	N.PacketConn
	storage *TrafficStorage
}

func NewPacketConnCounter(conn N.PacketConn, s *TrafficStorage) N.PacketConn {
	return &PacketConnCounter{PacketConn: conn, storage: s}
}
func (p *PacketConnCounter) ReadPacket(buff *buf.Buffer) (M.Socksaddr, error) {
	dest, err := p.PacketConn.ReadPacket(buff)
	if err == nil && buff.Len() > 0 {
		p.storage.UpCounter.Add(int64(buff.Len()))
	}
	return dest, err
}
func (p *PacketConnCounter) WritePacket(buff *buf.Buffer, dest M.Socksaddr) error {
	n := buff.Len()
	err := p.PacketConn.WritePacket(buff, dest)
	if err == nil && n > 0 {
		p.storage.DownCounter.Add(int64(n))
	}
	return err
}
func (p *PacketConnCounter) Upstream() any { return p.PacketConn }
