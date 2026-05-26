package xboard

import (
	"context"
	"log"
	"time"
)

type Syncer struct {
	Panel    Panel
	Snapshot func() map[string]map[string][2]int64
	Commit   func(map[string]map[string][2]int64)
	Users    func([]User) error
	Every    time.Duration
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
			s.flush(ctx)
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context) {
	if s.Panel == nil {
		return
	}
	users, err := s.Panel.FetchUsers(ctx)
	if err != nil {
		log.Printf("xboard fetch users: %v", err)
	} else if s.Users != nil {
		if err := s.Users(users); err != nil {
			log.Printf("apply users: %v", err)
		}
	}
	s.flush(ctx)
}

func (s *Syncer) flush(ctx context.Context) {
	if s.Panel == nil || s.Snapshot == nil || s.Commit == nil {
		return
	}
	delta := s.Snapshot()
	if len(delta) == 0 {
		return
	}
	flat := flatten(delta)
	if len(flat) == 0 {
		return
	}
	if err := s.Panel.PushTraffic(ctx, flat); err != nil {
		log.Printf("xboard push traffic: %v", err)
		return
	}
	s.Commit(delta)
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
