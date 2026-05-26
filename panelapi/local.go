package panelapi

import (
	"context"
	"encoding/json"
	"os"
)

type LocalUsers struct {
	Path string
}

func (l LocalUsers) FetchUsers(ctx context.Context) ([]User, error) {
	data, err := os.ReadFile(l.Path)
	if err != nil {
		return nil, err
	}
	var list UserList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	if len(list.Users) > 0 {
		return list.Users, nil
	}
	return list.Data, nil
}

func (l LocalUsers) PushTraffic(ctx context.Context, delta map[string][2]int64) error {
	return nil
}

type Panel interface {
	FetchUsers(ctx context.Context) ([]User, error)
	PushTraffic(ctx context.Context, delta map[string][2]int64) error
}
