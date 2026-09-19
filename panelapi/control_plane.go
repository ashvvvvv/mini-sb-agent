package panelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ControlPlaneClient is the versioned agent-control API. It is separate from
// UniProxy so a native panel can evolve without inheriting Xboard's contract.
type ControlPlaneClient struct {
	BaseURL string
	Token   string
	AgentID string
	HTTP    minHTTPClient
}

type TopologyResponse struct {
	SchemaVersion int      `json:"schema_version"`
	Revision      string   `json:"revision"`
	Topology      Topology `json:"topology"`
}

type TrafficItem struct {
	NodeID   string `json:"node_id"`
	UserID   int    `json:"user_id"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type TrafficPushRequest struct {
	BatchID string        `json:"batch_id"`
	Items   []TrafficItem `json:"items"`
}

func NewControlPlaneClient(baseURL, token, agentID string) *ControlPlaneClient {
	return &ControlPlaneClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		AgentID: agentID,
		HTTP:    minHTTPClient{Timeout: minHTTPTimeout},
	}
}

func (c *ControlPlaneClient) requestHeaders(extra map[string]string) (map[string]string, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("control plane base url is empty")
	}
	headers := make(map[string]string, len(extra)+2)
	for key, value := range extra {
		headers[key] = value
	}
	if c.Token != "" {
		headers["Authorization"] = "Bearer " + c.Token
	}
	if c.AgentID != "" {
		headers["X-Agent-ID"] = c.AgentID
	}
	return headers, nil
}

// FetchTopology returns changed=false for HTTP 304; callers retain their last
// verified topology. A response is rejected if its schema or topology is invalid.
func (c *ControlPlaneClient) FetchTopology(ctx context.Context, etag string) (TopologyResponse, bool, error) {
	headers, err := c.requestHeaders(nil)
	if err != nil {
		return TopologyResponse{}, false, err
	}
	if etag != "" {
		headers["If-None-Match"] = etag
	}
	resp, err := c.HTTP.do(ctx, "GET", c.BaseURL+"/api/v2/agent/topology", headers, nil)
	if err != nil {
		return TopologyResponse{}, false, err
	}
	if resp.StatusCode == 304 {
		return TopologyResponse{}, false, nil
	}
	if resp.StatusCode/100 != 2 {
		return TopologyResponse{}, false, fmt.Errorf("control plane topology status %d", resp.StatusCode)
	}
	var out TopologyResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return TopologyResponse{}, false, err
	}
	if out.SchemaVersion != 1 {
		return TopologyResponse{}, false, fmt.Errorf("unsupported topology schema version %d", out.SchemaVersion)
	}
	if strings.TrimSpace(out.Revision) == "" {
		return TopologyResponse{}, false, fmt.Errorf("control plane topology revision is empty")
	}
	if err := out.Topology.Validate(); err != nil {
		return TopologyResponse{}, false, fmt.Errorf("invalid control plane topology: %w", err)
	}
	return out, true, nil
}

// PushTraffic is safe to retry only when BatchID remains stable. The panel must
// persist Idempotency-Key values before applying counters.
func (c *ControlPlaneClient) PushTraffic(ctx context.Context, payload TrafficPushRequest) error {
	if strings.TrimSpace(payload.BatchID) == "" {
		return fmt.Errorf("traffic batch_id is empty")
	}
	if len(payload.Items) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers, err := c.requestHeaders(map[string]string{
		"Content-Type":    "application/json",
		"Idempotency-Key": payload.BatchID,
	})
	if err != nil {
		return err
	}
	resp, err := c.HTTP.do(ctx, "POST", c.BaseURL+"/api/v2/agent/traffic", headers, body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("control plane traffic status %d", resp.StatusCode)
	}
	return nil
}
