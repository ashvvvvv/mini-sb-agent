package panelapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControlPlaneFetchesTopologyWithETag(t *testing.T) {
	var gotAuth string
	var gotAgent string
	var gotMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/agent/topology" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotAgent = r.Header.Get("X-Agent-ID")
		gotMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"revision-7"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"revision":"7","topology":{"nodes":[{"node_id":"42","node_type":"vless","outbound":{"type":"direct"}}]}}`))
	}))
	defer server.Close()

	client := NewControlPlaneClient(server.URL, "agent-secret", "edge-a")
	response, changed, err := client.FetchTopology(t.Context(), `"revision-6"`)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || response.Revision != "7" || response.Topology.Nodes[0].NodeID != "42" {
		t.Fatalf("unexpected response: changed=%v response=%+v", changed, response)
	}
	if gotAuth != "Bearer agent-secret" || gotAgent != "edge-a" || gotMatch != `"revision-6"` {
		t.Fatalf("headers auth=%q agent=%q match=%q", gotAuth, gotAgent, gotMatch)
	}
}

func TestControlPlaneTopologyNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	_, changed, err := NewControlPlaneClient(server.URL, "secret", "edge-a").FetchTopology(t.Context(), `"revision-7"`)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestControlPlanePushTrafficSendsIdempotencyKey(t *testing.T) {
	var gotKey string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewControlPlaneClient(server.URL, "secret", "edge-a")
	err := client.PushTraffic(t.Context(), TrafficPushRequest{
		BatchID: "batch-1",
		Items:   []TrafficItem{{NodeID: "42", UserID: 9, Upload: 100, Download: 200}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "batch-1" || !strings.Contains(body, `"node_id":"42"`) || !strings.Contains(body, `"download":200`) {
		t.Fatalf("key=%q body=%s", gotKey, body)
	}
}
