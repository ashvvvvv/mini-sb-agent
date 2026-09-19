package vless

import (
	"net"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) AddUsers(users []option.VLESSUser) error {
	h.users = append(h.users, users...)
	h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.Flow
		}),
	)
	return nil
}
func (h *Inbound) DelUsers(names []string) error {
	toDelete := make(map[string]struct{})
	for _, name := range names {
		h.userconns.Range(func(key, value interface{}) bool {
			if value.(string) == name {
				key.(net.Conn).Close()
				h.userconns.Delete(key)
			}
			return true
		})
		toDelete[name] = struct{}{}
	}
	remaining := make([]option.VLESSUser, 0)
	for _, user := range h.users {
		if _, found := toDelete[user.Name]; !found {
			remaining = append(remaining, user)
		}
	}
	h.users = remaining
	h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.Flow
		}),
	)
	return nil
}
