package main

import (
	"fmt"
	"sync"

	"mini-sb-agent/counter"
	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/vless"
)

type UserManager struct {
	mu        sync.Mutex
	users     map[int]panelapi.User
	bySecret  map[string]int
	nodeLimit *counter.RateLimiter
	limiters  map[int]*counter.RateLimiter
}

func NewUserManager(nodeMbps int) *UserManager {
	var nodeLimiter *counter.RateLimiter
	if nodeMbps > 0 {
		nodeLimiter = counter.NewRateLimiter(mbpsToBytes(nodeMbps))
	}
	return &UserManager{
		users:     make(map[int]panelapi.User),
		bySecret:  make(map[string]int),
		nodeLimit: nodeLimiter,
		limiters:  make(map[int]*counter.RateLimiter),
	}
}

func mbpsToBytes(mbps int) int64 {
	if mbps <= 0 {
		return 0
	}
	return int64(mbps) * 1000 * 1000 / 8
}

func sameUser(a, b panelapi.User) bool {
	return a.ID == b.ID && a.UUID == b.UUID && a.Password == b.Password && a.Name == b.Name && a.SpeedLimit == b.SpeedLimit
}

func vlessUserFromPanelUser(u panelapi.User) option.VLESSUser {
	return option.VLESSUser{Name: u.UUID, UUID: u.UUID, Flow: "xtls-rprx-vision"}
}

func (m *UserManager) ApplyBox(inbounds map[string]adapter.Inbound, users []panelapi.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[int]panelapi.User, len(users))
	nextSecrets := make(map[string]int, len(users)*3)
	for _, u := range users {
		if u.ID <= 0 {
			continue
		}
		next[u.ID] = u
		for _, secret := range []string{u.UUID, u.Password, u.Name} {
			if secret != "" {
				nextSecrets[secret] = u.ID
			}
		}
	}

	var addVless []option.VLESSUser
	var delVless []string
	var addHy2 []option.Hysteria2User
	var addHy2IDs []int
	var delHy2 []string

	for id, old := range m.users {
		nu, ok := next[id]
		if !ok {
			if old.UUID != "" {
				delVless = append(delVless, old.UUID)
			}
			if old.Password != "" {
				delHy2 = append(delHy2, old.Password)
			}
			m.closeLimiterLocked(id)
			continue
		}
		if old.UUID != nu.UUID || old.Password != nu.Password || old.Name != nu.Name {
			if old.UUID != "" {
				delVless = append(delVless, old.UUID)
			}
			if old.Password != "" {
				delHy2 = append(delHy2, old.Password)
			}
		}
	}
	for id, nu := range next {
		old, ok := m.users[id]
		if ok && sameUser(old, nu) {
			m.updateLimiterLocked(nu)
			continue
		}
		if nu.UUID != "" {
			addVless = append(addVless, vlessUserFromPanelUser(nu))
		}
		if nu.Password != "" {
			name := nu.Name
			if name == "" {
				name = nu.Password
			}
			addHy2 = append(addHy2, option.Hysteria2User{Name: name, Password: nu.Password})
			addHy2IDs = append(addHy2IDs, nu.ID)
		}
		m.updateLimiterLocked(nu)
	}

	for tag, raw := range inbounds {
		switch in := raw.(type) {
		case *vless.Inbound:
			if len(delVless) > 0 {
				if err := in.DelUsers(delVless); err != nil {
					return fmt.Errorf("delete vless users from %s: %w", tag, err)
				}
			}
			if len(addVless) > 0 {
				if err := in.AddUsers(addVless); err != nil {
					return fmt.Errorf("add vless users to %s: %w", tag, err)
				}
			}
		case *hysteria2.Inbound:
			if len(delHy2) > 0 {
				if err := in.DelUsers(delHy2); err != nil {
					return fmt.Errorf("delete hysteria2 users from %s: %w", tag, err)
				}
			}
			if len(addHy2) > 0 {
				if err := in.AddUsers(addHy2, addHy2IDs); err != nil {
					return fmt.Errorf("add hysteria2 users to %s: %w", tag, err)
				}
			}
		}
	}

	m.users = next
	m.bySecret = nextSecrets
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
	m.limiters[u.ID] = counter.NewRateLimiter(bytesPerSecond)
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

func (m *UserManager) Limiters(user string) (*counter.RateLimiter, *counter.RateLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var userLimiter *counter.RateLimiter
	if userRateLimitBuildEnabled {
		if id, ok := m.bySecret[user]; ok {
			userLimiter = m.limiters[id]
		}
	}
	return m.nodeLimit, userLimiter
}
