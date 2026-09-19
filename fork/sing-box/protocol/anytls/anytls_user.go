package anytls

import (
	"net"

	anytls "github.com/anytls/sing-anytls"
	"github.com/sagernet/sing-box/option"
)

func (h *Inbound) AddUsers(users []option.AnyTLSUser) error {
	for _, user := range users {
		h.uuidlist = append(h.uuidlist, user.Name)
	}
	userList := make([]anytls.User, len(h.uuidlist))
	for i, uuid := range h.uuidlist {
		userList[i] = anytls.User{Name: uuid, Password: uuid}
	}
	h.service.UpdateUsers(userList)
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	if len(names) == 0 {
		return nil
	}

	toDelete := make(map[string]struct{})
	for _, name := range names {
		toDelete[name] = struct{}{}
		h.userconns.Range(func(key, value interface{}) bool {
			if value.(string) == name {
				key.(net.Conn).Close()
				h.userconns.Delete(key)
			}
			return true
		})
	}

	remaining := make([]string, 0, len(h.uuidlist))
	for _, uuid := range h.uuidlist {
		if _, found := toDelete[uuid]; !found {
			remaining = append(remaining, uuid)
		}
	}

	h.uuidlist = remaining
	userList := make([]anytls.User, len(h.uuidlist))
	for i, uuid := range h.uuidlist {
		userList[i] = anytls.User{Name: uuid, Password: uuid}
	}
	h.service.UpdateUsers(userList)
	return nil
}
