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

func (l LocalUsers) PushTraffic(ctx context.Context, delta map[string][2]int64) error {
	return nil
}

type Panel interface {
	FetchUsers(ctx context.Context) ([]User, error)
	PushTraffic(ctx context.Context, delta map[string][2]int64) error
}
