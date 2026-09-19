package main

import "mini-sb-agent/panelapi"

// userChanges returns authentication changes only. Speed-limit updates do not
// require removing users from live inbounds and therefore must not disconnect
// active proxy sessions.
func userChanges(old map[int]panelapi.User, next []panelapi.User) (removed, added []panelapi.User) {
	nextByID := make(map[int]panelapi.User, len(next))
	for _, user := range next {
		if user.ID > 0 {
			nextByID[user.ID] = user
		}
	}
	for id, oldUser := range old {
		newUser, exists := nextByID[id]
		if !exists || oldUser.UUID != newUser.UUID || oldUser.Password != newUser.Password || oldUser.Name != newUser.Name {
			removed = append(removed, oldUser)
		}
	}
	for _, newUser := range nextByID {
		oldUser, exists := old[newUser.ID]
		if !exists || oldUser.UUID != newUser.UUID || oldUser.Password != newUser.Password || oldUser.Name != newUser.Name {
			added = append(added, newUser)
		}
	}
	return
}
