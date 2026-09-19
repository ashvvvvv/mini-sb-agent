package panelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	BaseURL    string
	Token      string
	NodeID     string
	NodeType   string
	InboundTag string
	HTTP       minHTTPClient
}

func NewClient(baseURL, token, nodeID, nodeType string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Token:    token,
		NodeID:   nodeID,
		NodeType: nodeType,
		HTTP:     minHTTPClient{Timeout: minHTTPTimeout},
	}
}

func NewNodeClient(baseURL, token string, node NodeSpec) *Client {
	client := NewClient(baseURL, token, node.NodeID, node.PanelNodeType())
	client.InboundTag = node.InboundTag()
	return client
}

func (c *Client) endpoint(path string) (string, error) {
	if c.BaseURL == "" {
		return "", fmt.Errorf("panel api base url is empty")
	}
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if c.Token != "" {
		q.Set("token", c.Token)
	}
	if c.NodeID != "" {
		q.Set("node_id", c.NodeID)
	}
	if c.NodeType != "" {
		q.Set("node_type", c.NodeType)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) FetchUsers(ctx context.Context) ([]User, error) {
	ep, err := c.endpoint("/api/v1/server/UniProxy/user")
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.do(ctx, "GET", ep, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("panel api user status %s", resp.Status)
	}
	var list UserList
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return nil, err
	}
	users := list.Users
	if len(users) == 0 {
		users = list.Data
	}
	for i := range users {
		// Xboard's UniProxy user endpoint returns id/uuid/speed_limit only for
		// vless-like nodes. For HY2 in this lightweight agent, use the same UUID
		// as the Hysteria2 password so one panel user can authenticate on both
		// VLESS Reality and HY2 and still bill to the numeric user id.
		if users[i].Password == "" {
			users[i].Password = users[i].UUID
		}
		if users[i].Name == "" {
			users[i].Name = strconv.Itoa(users[i].ID)
		}
	}
	return users, nil
}

func (c *Client) matchesInbound(tag string) bool {
	if c.InboundTag != "" {
		return tag == c.InboundTag
	}
	switch strings.ToLower(c.NodeType) {
	case "vless", "vless-reality", "reality":
		return tag == "vless-in"
	case "hy2", "hysteria", "hysteria2":
		return tag == "hy2-in"
	default:
		return strings.Contains(strings.ToLower(tag), strings.ToLower(c.NodeType))
	}
}

func (c *Client) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	flat := make(map[string][2]int64)
	for tag, users := range delta {
		if !c.matchesInbound(tag) {
			continue
		}
		for user, d := range users {
			if !isNumericUser(user) {
				continue
			}
			old := flat[user]
			old[0] += d[0]
			old[1] += d[1]
			flat[user] = old
		}
	}
	if len(flat) == 0 {
		return nil
	}
	ep, err := c.endpoint("/api/v1/server/UniProxy/push")
	if err != nil {
		return err
	}
	payload := make(PushRequest, len(flat))
	for user, d := range flat {
		uid, err := strconv.Atoi(user)
		if err != nil {
			continue
		}
		payload[uid] = []int64{d[0], d[1]}
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.do(ctx, "POST", ep, map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("panel api push status %s", resp.Status)
	}
	return nil
}
