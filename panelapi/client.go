package panelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	Token    string
	NodeID   string
	NodeType string
	HTTP     *http.Client
}

func NewClient(baseURL, token, nodeID, nodeType string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Token:    token,
		NodeID:   nodeID,
		NodeType: nodeType,
		HTTP: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("panel api user status %s", resp.Status)
	}
	var list UserList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
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

func (c *Client) PushTraffic(ctx context.Context, delta map[string][2]int64) error {
	if len(delta) == 0 {
		return nil
	}
	ep, err := c.endpoint("/api/v1/server/UniProxy/push")
	if err != nil {
		return err
	}
	payload := make(PushRequest, len(delta))
	for user, d := range delta {
		uid, err := strconv.Atoi(user)
		if err != nil {
			continue
		}
		payload[uid] = []int64{d[0], d[1]}
	}
	if len(payload) == 0 {
		return nil
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("panel api push status %s", resp.Status)
	}
	return nil
}
