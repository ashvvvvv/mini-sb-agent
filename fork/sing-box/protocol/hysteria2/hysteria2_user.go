package hysteria2

import (
	"net"

	"github.com/sagernet/sing-box/option"
)

func (h *Inbound) AddUsers(users []option.Hysteria2User, ids []int) error {
	for i, user := range users {
		h.userNameList = append(h.userNameList, user.Password)
		h.uuidToUid[user.Password] = ids[i]
		h.uidToUuid[ids[i]] = user.Password
	}

	indexs := make([]int, len(h.userNameList))
	for i, uuid := range h.userNameList {
		indexs[i] = h.uuidToUid[uuid]
	}

	h.service.UpdateUsers(indexs, h.userNameList)
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	if len(names) == 0 {
		return nil
	}

	toDelete := make(map[string]struct{})
	for _, name := range names {
		toDelete[name] = struct{}{}
		delete(h.uidToUuid, h.uuidToUid[name])
		delete(h.uuidToUid, name)
		h.userconns.Range(func(key, value interface{}) bool {
			if value.(string) == name {
				key.(net.Conn).Close()
				h.userconns.Delete(key)
			}
			return true
		})
	}

	remaining := make([]string, 0, len(h.userNameList))
	for _, user := range h.userNameList {
		if _, found := toDelete[user]; !found {
			remaining = append(remaining, user)
		}
	}

	h.userNameList = remaining
	indexs := make([]int, len(h.userNameList))
	for i, uuid := range h.userNameList {
		indexs[i] = h.uuidToUid[uuid]
	}
	h.service.UpdateUsers(indexs, h.userNameList)
	return nil
}
