package shadowsocks

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

func (h *MultiInbound) AddUsers(users []option.ShadowsocksUser) error {
	h.users = append(h.users, users...)
	err := h.service.UpdateUsersWithPasswords(common.MapIndexed(h.users, func(index int, user option.ShadowsocksUser) int {
		return index
	}), common.Map(h.users, func(user option.ShadowsocksUser) string {
		return user.Password
	}))
	return err
}
func (h *MultiInbound) DelUsers(names []string) error {
	toDelete := make(map[string]struct{})
	for _, name := range names {
		toDelete[name] = struct{}{}
	}
	remaining := make([]option.ShadowsocksUser, 0, len(h.users))
	for _, user := range h.users {
		if _, found := toDelete[user.Name]; !found {
			remaining = append(remaining, user)
		}
	}
	h.users = remaining
	err := h.service.UpdateUsersWithPasswords(common.MapIndexed(h.users, func(index int, user option.ShadowsocksUser) int {
		return index
	}), common.Map(h.users, func(user option.ShadowsocksUser) string {
		return user.Password
	}))
	return err
}
