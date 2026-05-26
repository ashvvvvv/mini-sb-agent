package panelapi

type User struct {
	ID         int    `json:"id"`
	UUID       string `json:"uuid,omitempty"`
	Password   string `json:"password,omitempty"`
	Name       string `json:"name,omitempty"`
	SpeedLimit int    `json:"speed_limit,omitempty"`
}

type UserList struct {
	Users []User `json:"users"`
	Data  []User `json:"data,omitempty"`
}

type PushRequest map[int][]int64
