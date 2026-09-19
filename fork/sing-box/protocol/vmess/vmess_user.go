package vmess

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) AddUsers(users []option.VMessUser) error {
	h.users = append(h.users, users...)
	err := h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VMessUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VMessUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VMessUser) int {
			return it.AlterId
		}))
	if err != nil {
		return err
	}
	return nil
}
func (h *Inbound) DelUsers(uuids []string) error {
	toDelete := make(map[string]struct{})
	for _, uuid := range uuids {
		toDelete[uuid] = struct{}{}
	}
	remaining := make([]option.VMessUser, 0)
	for _, user := range h.users {
		if _, found := toDelete[user.Name]; !found {
			remaining = append(remaining, user)
		}
	}
	h.users = remaining
	err := h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VMessUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VMessUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VMessUser) int {
			return it.AlterId
		}))
	if err != nil {
		return err
	}
	return nil
}
