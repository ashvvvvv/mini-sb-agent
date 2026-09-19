package main

import "mini-sb-agent/panelapi"

// groupNodeUsers translates panel node identities into stable inbound tags.
// Keeping the lists separate is essential when multiple nodes use the same
// protocol: users from node A must never authenticate on node B.
func groupNodeUsers(nodes []panelapi.NodeSpec, usersByNode map[string][]panelapi.User) map[string][]panelapi.User {
	out := make(map[string][]panelapi.User, len(nodes))
	for _, node := range nodes {
		out[node.InboundTag()] = usersByNode[node.Key()]
	}
	return out
}
