package panelapi

import (
	"context"
	"errors"
	"testing"
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

func (p *syncTestPanel) PushTraffic(ctx context.Context, delta map[string][2]int64) error {
	p.pushes++
	return p.pushErr
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
