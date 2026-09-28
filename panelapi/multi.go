package panelapi

import (
	"context"
	"errors"
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

func (m MultiPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) (map[string]bool, error) {
	// Push every panel even when an earlier one fails: each panel accounts
	// only its own inbounds, and the acknowledged tags let the Syncer commit
	// the confirmed portion instead of re-billing it on the next round.
	acked := map[string]bool{}
	var errs []error
	for _, panel := range m.Panels {
		if panel == nil {
			continue
		}
		panelAcked, err := panel.PushTraffic(ctx, delta)
		for tag := range panelAcked {
			acked[tag] = true
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return acked, errors.Join(errs...)
	}
	return acked, nil
}
