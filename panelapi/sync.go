package panelapi

import (
	"context"
	"log"
	"sync"
	"time"
)

type Syncer struct {
	Panel    Panel
	Snapshot func() map[string]map[string][2]int64
	Commit   func(map[string]map[string][2]int64)
	Users    func([]User) error
	Every    time.Duration

	// flushMu serializes flushes: the periodic loop and the shutdown-time
	// FinalFlush share it so at most one push per delta is in flight.
	flushMu sync.Mutex
}

// Start launches Run in its own goroutine and returns a channel that is
// closed once Run has fully returned. The channel is created in the caller's
// goroutine, so it can be waited on before Run has even started. Shutdown
// code waits on it before closing the data path, so in-flight user
// application never races the box shutdown.
func (s *Syncer) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	return done
}

func (s *Syncer) Run(ctx context.Context) {
	if s.Every <= 0 {
		s.Every = time.Minute
	}
	s.syncOnce(ctx)
	ticker := time.NewTicker(s.Every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// No final flush here: the caller drives it with FinalFlush
			// after the data path is closed, so the last bytes are counted.
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

// FinalFlush pushes the remaining uncommitted traffic. Call it after the data
// path is closed (box shutdown) and with a bounded context; it is serialized
// with the periodic loop, so it is safe to call while Run is still returning.
func (s *Syncer) FinalFlush(ctx context.Context) {
	s.flush(ctx)
}

func (s *Syncer) syncOnce(ctx context.Context) {
	if s.Panel == nil {
		return
	}
	users, err := s.Panel.FetchUsers(ctx)
	if err != nil {
		log.Printf("panel api fetch users: %v", err)
	} else if s.Users != nil {
		if err := s.Users(users); err != nil {
			log.Printf("apply users: %v", err)
		}
	}
	s.flush(ctx)
}

func (s *Syncer) flush(ctx context.Context) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	if s.Panel == nil || s.Snapshot == nil || s.Commit == nil {
		return
	}
	delta := s.Snapshot()
	if len(delta) == 0 {
		return
	}
	hasTraffic := false
	for _, users := range delta {
		for user := range users {
			if isNumericUser(user) {
				hasTraffic = true
				break
			}
		}
		if hasTraffic {
			break
		}
	}
	if !hasTraffic {
		return
	}
	acked, err := s.Panel.PushTraffic(ctx, delta)
	if err != nil {
		log.Printf("panel api push traffic: %v", err)
		if len(acked) == 0 {
			return
		}
		// Partial success: commit only the acknowledged inbounds so the
		// retry covers the failed nodes without re-billing the confirmed
		// ones.
		s.Commit(commitDelta(delta, acked))
		return
	}
	s.Commit(commitDelta(delta, nil))
}

func flatten(in map[string]map[string][2]int64) map[string][2]int64 {
	out := make(map[string][2]int64)
	for _, users := range in {
		for user, v := range users {
			if !isNumericUser(user) {
				continue
			}
			old := out[user]
			old[0] += v[0]
			old[1] += v[1]
			out[user] = old
		}
	}
	return out
}

// commitDelta filters delta to numeric users, optionally restricted to the
// given inbound tags (nil = all inbounds).
func commitDelta(in map[string]map[string][2]int64, only map[string]bool) map[string]map[string][2]int64 {
	out := make(map[string]map[string][2]int64)
	for inbound, users := range in {
		if only != nil && !only[inbound] {
			continue
		}
		for user, v := range users {
			if !isNumericUser(user) {
				continue
			}
			if out[inbound] == nil {
				out[inbound] = make(map[string][2]int64)
			}
			out[inbound][user] = v
		}
	}
	return out
}

func isNumericUser(user string) bool {
	if user == "" {
		return false
	}
	for _, r := range user {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
