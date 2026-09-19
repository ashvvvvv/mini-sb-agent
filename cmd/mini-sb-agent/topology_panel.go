package main

import (
	"context"
	"fmt"

	"mini-sb-agent/panelapi"
)

type topologyPanel struct {
	clients []*panelapi.Client
	nodes   []panelapi.NodeSpec
	apply   func(map[string][]panelapi.User) error
}

func newTopologyPanel(baseURL, token string, nodes []panelapi.NodeSpec, apply func(map[string][]panelapi.User) error) *topologyPanel {
	clients := make([]*panelapi.Client, 0, len(nodes))
	for _, node := range nodes {
		clients = append(clients, panelapi.NewNodeClient(baseURL, token, node))
	}
	return &topologyPanel{clients: clients, nodes: nodes, apply: apply}
}

func (p *topologyPanel) FetchUsers(ctx context.Context) ([]panelapi.User, error) {
	byNode := make(map[string][]panelapi.User, len(p.clients))
	for i, client := range p.clients {
		users, err := client.FetchUsers(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch users for %s: %w", p.nodes[i].Key(), err)
		}
		byNode[p.nodes[i].Key()] = users
	}
	// Validate the merge BEFORE applying anything: conflicting user
	// definitions across nodes must not be partially applied.
	merged, err := mergeUsersByNode(byNode)
	if err != nil {
		return nil, err
	}
	if p.apply != nil {
		if err := p.apply(byNode); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

func (p *topologyPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	for i, client := range p.clients {
		if err := client.PushTraffic(ctx, delta); err != nil {
			return fmt.Errorf("push traffic for %s: %w", p.nodes[i].Key(), err)
		}
	}
	return nil
}

func mergeUsersByNode(usersByNode map[string][]panelapi.User) ([]panelapi.User, error) {
	merged := make(map[int]panelapi.User)
	for nodeKey, users := range usersByNode {
		for _, u := range users {
			existing, exists := merged[u.ID]
			if exists {
				if existing.UUID != u.UUID || existing.Password != u.Password || existing.SpeedLimit != u.SpeedLimit {
					return nil, fmt.Errorf("conflicting user definition for ID %d across nodes (%s)", u.ID, nodeKey)
				}
			} else {
				merged[u.ID] = u
			}
		}
	}
	out := make([]panelapi.User, 0, len(merged))
	for _, u := range merged {
		out = append(out, u)
	}
	return out, nil
}
