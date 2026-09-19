package hysteria

import "github.com/sagernet/sing-box/option"

func (h *Inbound) AddUsers(users []option.HysteriaUser) error {
	for _, user := range users {
		h.userNameList = append(h.userNameList, user.AuthString)
	}
	indexs := make([]int, len(h.userNameList))
	for i := range h.userNameList {
		indexs[i] = i
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
	}
	remaining := make([]string, 0, len(h.userNameList))
	for _, user := range h.userNameList {
		if _, found := toDelete[user]; !found {
			remaining = append(remaining, user)
		}
	}
	h.userNameList = remaining
	indexs := make([]int, len(h.userNameList))
	for i := range h.userNameList {
		indexs[i] = i
	}
	h.service.UpdateUsers(indexs, h.userNameList)
	return nil
}
