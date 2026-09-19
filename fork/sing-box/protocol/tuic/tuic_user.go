package tuic

import (
	"net"

	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

func (h *Inbound) AddUsers(users []option.TUICUser, ids []int) error {
	for i, user := range users {
		h.userNameList = append(h.userNameList, user.Name)
		h.uuidToUid[user.UUID] = ids[i]
		h.uidToUuid[ids[i]] = user.UUID
	}
	var userUUIDList [][16]byte
	indexs := make([]int, len(h.userNameList))
	for i, UUID := range h.userNameList {
		indexs[i] = h.uuidToUid[UUID]
		userUUID, err := uuid.FromString(UUID)
		if err != nil {
			return E.Cause(err, "invalid uuid for user ", i)
		}
		userUUIDList = append(userUUIDList, userUUID)
	}
	h.server.UpdateUsers(indexs, userUUIDList, h.userNameList)
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
	var userUUIDList [][16]byte
	indexs := make([]int, len(h.userNameList))
	for i, UUID := range h.userNameList {
		indexs[i] = h.uuidToUid[UUID]
		userUUID, err := uuid.FromString(UUID)
		if err != nil {
			return E.Cause(err, "invalid uuid for user ", i)
		}
		userUUIDList = append(userUUIDList, userUUID)
	}
	h.server.UpdateUsers(indexs, userUUIDList, h.userNameList)
	return nil
}
