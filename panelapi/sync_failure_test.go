package panelapi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type syncTestPanel struct {
	users   []User
	fetches int
	pushes  int
	pushErr error
}

func (p *syncTestPanel) FetchUsers(ctx context.Context) ([]User, error) {
	p.fetches++
	return p.users, nil
}

func (p *syncTestPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (map[string]bool, error) {
	p.pushes++
	if p.pushErr != nil {
		return nil, p.pushErr
	}
	acked := make(map[string]bool, len(delta))
	for tag := range delta {
		acked[tag] = true
	}
	return acked, nil
}

func TestSyncerDoesNotCommitOnPushFailure(t *testing.T) {
	panel := &syncTestPanel{pushErr: errors.New("temporary panel outage")}
	delta := map[string]map[string][2]int64{
		"vless-tcp": {"1": {123, 456}},
	}
	commits := 0
	s := &Syncer{
		Panel:    panel,
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit: func(got map[string]map[string][2]int64) {
			commits++
		},
	}

	s.flush(context.Background())

	if panel.pushes != 1 {
		t.Fatalf("PushTraffic calls = %d, want 1", panel.pushes)
	}
	if commits != 0 {
		t.Fatalf("Commit called %d times on failed push, want 0", commits)
	}
}

func TestSyncerCommitsOnlyPushedNumericDelta(t *testing.T) {
	panel := &syncTestPanel{}
	delta := map[string]map[string][2]int64{
		"vless-tcp": {"1": {100, 200}},
		"hy2-udp":   {"uuid-user": {300, 400}},
	}
	commits := 0
	var committed map[string]map[string][2]int64
	s := &Syncer{
		Panel:    panel,
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit: func(got map[string]map[string][2]int64) {
			commits++
			committed = got
		},
	}

	s.flush(context.Background())

	if panel.pushes != 1 {
		t.Fatalf("PushTraffic calls = %d, want 1", panel.pushes)
	}
	if commits != 1 {
		t.Fatalf("Commit calls = %d, want 1", commits)
	}
	if _, ok := committed["hy2-udp"]["uuid-user"]; ok {
		t.Fatalf("non-numeric user was committed even though it was not pushed: %#v", committed)
	}
	if committed["vless-tcp"]["1"] != [2]int64{100, 200} {
		t.Fatalf("numeric pushed delta missing from commit: %#v", committed)
	}
}

// partialPanel simulates a multi-node panel where some nodes accept the push
// and others fail: it reports the acknowledged tags together with the error.
type partialPanel struct {
	acked  map[string]bool
	err    error
	pushes int
}

func (p *partialPanel) FetchUsers(ctx context.Context) ([]User, error) { return nil, nil }

func (p *partialPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (map[string]bool, error) {
	p.pushes++
	return p.acked, p.err
}

func TestSyncerCommitsOnlyAckedInboundsOnPartialPushFailure(t *testing.T) {
	panel := &partialPanel{
		acked: map[string]bool{"vless-21-in": true},
		err:   errors.New("push traffic for vless:22: node down"),
	}
	total := map[string]map[string][2]int64{
		"vless-21-in": {"1": {100, 0}},
		"vless-22-in": {"2": {100, 0}},
	}
	committed := map[string]map[string][2]int64{}
	s := &Syncer{
		Panel: panel,
		// SnapshotDelta semantics: only uncommitted traffic is returned.
		Snapshot: func() map[string]map[string][2]int64 {
			out := map[string]map[string][2]int64{}
			for tag, users := range total {
				for user, v := range users {
					c := committed[tag][user]
					up, down := v[0]-c[0], v[1]-c[1]
					if up > 0 || down > 0 {
						if out[tag] == nil {
							out[tag] = map[string][2]int64{}
						}
						out[tag][user] = [2]int64{up, down}
					}
				}
			}
			return out
		},
		Commit: func(got map[string]map[string][2]int64) {
			for tag, users := range got {
				if committed[tag] == nil {
					committed[tag] = map[string][2]int64{}
				}
				for user, v := range users {
					c := committed[tag][user]
					committed[tag][user] = [2]int64{c[0] + v[0], c[1] + v[1]}
				}
			}
		},
	}

	// Round 1: node 21 is acknowledged, node 22 fails. Only node 21 commits.
	s.flush(context.Background())
	if got := committed["vless-21-in"]["1"]; got != [2]int64{100, 0} {
		t.Fatalf("acknowledged node not committed: %#v", committed)
	}
	if _, ok := committed["vless-22-in"]["2"]; ok {
		t.Fatalf("failed node committed on partial failure: %#v", committed)
	}

	// Round 2: node 22 recovers. The snapshot now contains only node 22's
	// unconfirmed traffic, so node 21 is not billed twice.
	panel.acked = map[string]bool{"vless-21-in": true, "vless-22-in": true}
	panel.err = nil
	s.flush(context.Background())
	if got := committed["vless-21-in"]["1"]; got != [2]int64{100, 0} {
		t.Fatalf("node 21 double-committed: %#v", committed)
	}
	if got := committed["vless-22-in"]["2"]; got != [2]int64{100, 0} {
		t.Fatalf("failed node not committed after retry: %#v", committed)
	}
}

// shutdownPanel fails pushes on a canceled context, like a real HTTP client.
type shutdownPanel struct {
	pushes int
}

func (p *shutdownPanel) FetchUsers(ctx context.Context) ([]User, error) {
	return nil, ctx.Err()
}

func (p *shutdownPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (map[string]bool, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p.pushes++
	acked := make(map[string]bool, len(delta))
	for tag := range delta {
		acked[tag] = true
	}
	return acked, nil
}

func TestSyncerRunStopsWithoutFlushAndFinalFlushPushes(t *testing.T) {
	panel := &shutdownPanel{}
	delta := map[string]map[string][2]int64{"vless-in": {"1": {10, 20}}}
	commits := 0
	s := &Syncer{
		Panel:    panel,
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit:   func(map[string]map[string][2]int64) { commits++ },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Run must return on cancellation without a final flush: the caller
	// closes the data path first and then drives the last push itself.
	s.Run(ctx)
	if panel.pushes != 0 || commits != 0 {
		t.Fatalf("canceled Run pushed=%d committed=%d, want 0/0", panel.pushes, commits)
	}

	s.FinalFlush(context.Background())
	if panel.pushes != 1 || commits != 1 {
		t.Fatalf("FinalFlush pushed=%d committed=%d, want 1/1", panel.pushes, commits)
	}
}

func TestSyncerStartDoneClosesOnlyAfterRunFinishes(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var callbackReturned atomic.Bool
	s := &Syncer{
		Panel: &syncTestPanel{},
		Users: func([]User) error {
			entered <- struct{}{}
			<-release
			callbackReturned.Store(true)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := s.Start(ctx)
	<-entered // the periodic sync is inside the Users callback
	cancel()  // shutdown signal arrives mid-sync

	// Run must not report done while the sync goroutine is still applying
	// users: the caller relies on done to close the data path safely.
	select {
	case <-done:
		t.Fatal("done closed while the periodic sync was still running")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("done did not close after Run returned")
	}
	if !callbackReturned.Load() {
		t.Fatal("done closed before the Users callback returned")
	}
}
