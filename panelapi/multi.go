package panelapi

import (
	"context"
	"fmt"
)

type MultiPanel struct {
	Panels []Panel
}

func (m MultiPanel) FetchUsers(ctx context.Context) ([]User, error) {
	var merged []User
	seen := map[int]bool{}
	for _, panel := range m.Panels {
		if panel == nil {
			continue
		}
		users, err := panel.FetchUsers(ctx)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			if u.ID > 0 {
				if seen[u.ID] {
					continue
				}
				seen[u.ID] = true
			}
			merged = append(merged, u)
		}
	}
	return merged, nil
}

func (m MultiPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	for _, panel := range m.Panels {
		if panel == nil {
			continue
		}
		if err := panel.PushTraffic(ctx, delta); err != nil {
			return fmt.Errorf("push traffic to panel: %w", err)
		}
	}
	return nil
}
