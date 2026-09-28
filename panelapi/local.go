package panelapi

import (
	"context"
	"encoding/json"
	"os"
)

type LocalUsers struct {
	Path string
}

type localUserFile struct {
	Users    []User            `json:"users"`
	Data     []User            `json:"data,omitempty"`
	Inbounds map[string][]User `json:"inbounds,omitempty"`
}

func ParseUsers(data []byte) ([]User, error) {
	var list localUserFile
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	if len(list.Users) > 0 {
		return list.Users, nil
	}
	if len(list.Data) > 0 {
		return list.Data, nil
	}
	if len(list.Inbounds) == 0 {
		return nil, nil
	}
	var out []User
	for _, users := range list.Inbounds {
		out = append(out, users...)
	}
	return out, nil
}

func (l LocalUsers) FetchUsers(ctx context.Context) ([]User, error) {
	data, err := os.ReadFile(l.Path)
	if err != nil {
		return nil, err
	}
	return ParseUsers(data)
}

func (l LocalUsers) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (map[string]bool, error) {
	// Local mode has no panel to account traffic to; success keeps the
	// counters committed so unreported deltas do not accumulate.
	return nil, nil
}

type Panel interface {
	FetchUsers(ctx context.Context) ([]User, error)
	// PushTraffic returns the inbound tags whose delta the panel accepted.
	// On error the caller must commit only the acknowledged tags, so failed
	// nodes are retried without re-billing the already confirmed ones.
	PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (acked map[string]bool, err error)
}
