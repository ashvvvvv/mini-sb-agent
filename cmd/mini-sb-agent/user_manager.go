package main

import (
	"fmt"
	"sync"

	"mini-sb-agent/counter"
	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
)

// limiterPair holds separate read (rx) and write (tx) rate limiters so that
// upstream and downstream traffic do not compete for the same token bucket.
// This prevents ACK / control-packet starvation: a saturated download no
// longer blocks the upload direction (and vice versa).
type limiterPair struct {
	rx *counter.RateLimiter // applied on the Read / ReadPacket path
	tx *counter.RateLimiter // applied on the Write / WritePacket path
}

func newLimiterPair(bytesPerSecond int64) limiterPair {
	return limiterPair{
		rx: counter.NewRateLimiter(bytesPerSecond),
		tx: counter.NewRateLimiter(bytesPerSecond),
	}
}

func (p limiterPair) SetRate(bytesPerSecond int64) {
	p.rx.SetRate(bytesPerSecond)
	p.tx.SetRate(bytesPerSecond)
}

func (p limiterPair) Close() {
	p.rx.Close()
	p.tx.Close()
}

// UserManager keeps per-inbound user snapshots so that multiple nodes of the
// same protocol never share credentials, and applies authentication deltas
// only (speed-limit changes must not disconnect live sessions).
//
// Locking: applyMu serializes apply operations; m.mu guards the state maps and
// is NEVER held while inbound methods (AddUsers/DelUsers) are called, because
// those may in turn invoke authentication callbacks that resolve users.
type UserManager struct {
	applyMu   sync.Mutex
	mu        sync.Mutex
	users     map[int]panelapi.User
	bySecret  map[string]int
	byInbound map[string]map[int]panelapi.User
	nodeLimit *limiterPair
	limiters  map[int]*limiterPair
}

func NewUserManager(nodeMbps int) *UserManager {
	var nodeLim *limiterPair
	if nodeMbps > 0 {
		p := newLimiterPair(mbpsToBytes(nodeMbps))
		nodeLim = &p
	}
	return &UserManager{
		users:     make(map[int]panelapi.User),
		bySecret:  make(map[string]int),
		byInbound: make(map[string]map[int]panelapi.User),
		nodeLimit: nodeLim,
		limiters:  make(map[int]*limiterPair),
	}
}

func mbpsToBytes(mbps int) int64 {
	if mbps <= 0 {
		return 0
	}
	return int64(mbps) * 1000 * 1000 / 8
}

func usersByID(users []panelapi.User) map[int]panelapi.User {
	m := make(map[int]panelapi.User, len(users))
	for _, u := range users {
		if u.ID > 0 {
			m[u.ID] = u
		}
	}
	return m
}

func secretsFrom(users map[int]panelapi.User) map[string]int {
	out := make(map[string]int, len(users)*3)
	for id, u := range users {
		for _, secret := range []string{u.UUID, u.Password, u.Name} {
			if secret != "" {
				out[secret] = id
			}
		}
	}
	return out
}

// ApplyBox applies one shared user list to every inbound (legacy single- or
// dual-node mode). With no inbounds it only refreshes global state.
func (m *UserManager) ApplyBox(inbounds map[string]adapter.Inbound, users []panelapi.User) error {
	if len(inbounds) == 0 {
		return m.applyGlobal(users)
	}
	byInbound := make(map[string][]panelapi.User, len(inbounds))
	for tag := range inbounds {
		byInbound[tag] = users
	}
	return m.ApplyBoxByInbound(inbounds, byInbound)
}

// ApplyBoxByInbound applies an independent user list per inbound tag. Tags not
// present in usersByInbound end up with no users. This is the multi-node
// topology path: users from node A must never authenticate on node B even when
// both nodes use the same protocol.
func (m *UserManager) ApplyBoxByInbound(inbounds map[string]adapter.Inbound, usersByInbound map[string][]panelapi.User) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()

	oldByInbound := make(map[string]map[int]panelapi.User, len(inbounds))
	m.mu.Lock()
	for tag := range inbounds {
		if old, ok := m.byInbound[tag]; ok {
			oldByInbound[tag] = old
		}
	}
	m.mu.Unlock()

	// Apply authentication deltas outside m.mu: inbound methods may trigger
	// callbacks that call back into Resolve.
	for tag, raw := range inbounds {
		next := usersByInbound[tag]
		old := oldByInbound[tag]
		if old == nil {
			old = map[int]panelapi.User{}
		}
		handled, err := applyVLESSUsers(raw, tag, next, old)
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		if _, err := applyHysteria2Users(raw, tag, next, old); err != nil {
			return err
		}
	}

	// Commit snapshots. Conflicting credentials for one ID across nodes are
	// rejected upstream by mergeUsersByNode; the union below is a last-writer
	// defense only.
	nextByInbound := make(map[string]map[int]panelapi.User, len(usersByInbound))
	union := make(map[int]panelapi.User)
	for tag, users := range usersByInbound {
		snapshot := usersByID(users)
		nextByInbound[tag] = snapshot
		for id, u := range snapshot {
			union[id] = u
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.byInbound = nextByInbound
	m.users = union
	m.bySecret = secretsFrom(union)
	for id := range m.limiters {
		if _, ok := union[id]; !ok {
			m.closeLimiterLocked(id)
		}
	}
	for _, u := range union {
		m.updateLimiterLocked(u)
	}
	return nil
}

// applyGlobal refreshes global user state without touching any inbound. Used
// by ApplyBox when no live inbounds exist yet (startup, tests).
func (m *UserManager) applyGlobal(users []panelapi.User) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()
	next := usersByID(users)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.limiters {
		if _, ok := next[id]; !ok {
			m.closeLimiterLocked(id)
		}
	}
	for _, u := range next {
		m.updateLimiterLocked(u)
	}
	m.users = next
	m.bySecret = secretsFrom(next)
	return nil
}

func (m *UserManager) updateLimiterLocked(u panelapi.User) {
	if !userRateLimitBuildEnabled || u.SpeedLimit <= 0 {
		m.closeLimiterLocked(u.ID)
		return
	}
	bytesPerSecond := mbpsToBytes(u.SpeedLimit)
	if l, ok := m.limiters[u.ID]; ok {
		l.SetRate(bytesPerSecond)
		return
	}
	p := newLimiterPair(bytesPerSecond)
	m.limiters[u.ID] = &p
}

func (m *UserManager) closeLimiterLocked(id int) {
	if l, ok := m.limiters[id]; ok {
		l.Close()
		delete(m.limiters, id)
	}
}

func (m *UserManager) Resolve(user string) string {
	if user == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.bySecret[user]; ok {
		return fmt.Sprint(id)
	}
	return user
}

func (m *UserManager) ActiveIDs() map[string]struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]struct{}, len(m.users))
	for id := range m.users {
		out[fmt.Sprint(id)] = struct{}{}
	}
	return out
}

// InboundSnapshot returns the committed user snapshot for one inbound tag.
func (m *UserManager) InboundSnapshot(tag string) map[int]panelapi.User {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int]panelapi.User, len(m.byInbound[tag]))
	for id, u := range m.byInbound[tag] {
		out[id] = u
	}
	return out
}

// DirectionalLimiters returns separate read and write limiters for both the
// node level and the per-user level. Each direction gets its own token bucket
// so that saturated download traffic cannot starve upload ACKs (or vice versa).
func (m *UserManager) DirectionalLimiters(user string) (nodeRead, nodeWrite, userRead, userWrite *counter.RateLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nodeLimit != nil {
		nodeRead = m.nodeLimit.rx
		nodeWrite = m.nodeLimit.tx
	}
	if userRateLimitBuildEnabled {
		if id, ok := m.bySecret[user]; ok {
			if p := m.limiters[id]; p != nil {
				userRead = p.rx
				userWrite = p.tx
			}
		}
	}
	return
}

// Limiters returns a representative limiter for inspection / testing.
// Deprecated: prefer DirectionalLimiters for connection wiring.
func (m *UserManager) Limiters(user string) (*counter.RateLimiter, *counter.RateLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var nodeLim, userLim *counter.RateLimiter
	if m.nodeLimit != nil {
		nodeLim = m.nodeLimit.rx
	}
	if userRateLimitBuildEnabled {
		if id, ok := m.bySecret[user]; ok {
			if p := m.limiters[id]; p != nil {
				userLim = p.rx
			}
		}
	}
	return nodeLim, userLim
}
