package panelserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mini-sb-agent/panelapi"
)

func request(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAgentGetsOnlyAssignedTopologyAndETag(t *testing.T) {
	store := NewStore("")
	if err := store.PutAgent(Agent{ID: "edge-a", Secret: "agent-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutTopology("edge-a", panelapi.Topology{Nodes: []panelapi.NodeSpec{{NodeID: "42", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}}}}); err != nil {
		t.Fatal(err)
	}
	h := New("admin-secret", store)

	w := request(t, h, http.MethodGet, "/api/v2/agent/topology", "agent-secret", nil)
	if w.Code != http.StatusOK || w.Header().Get("ETag") == "" {
		t.Fatalf("code=%d etag=%q body=%s", w.Code, w.Header().Get("ETag"), w.Body.String())
	}
	if got := w.Body.String(); !bytes.Contains([]byte(got), []byte(`"node_id":"42"`)) {
		t.Fatalf("unexpected topology body %s", got)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v2/agent/topology", nil)
	r.Header.Set("Authorization", "Bearer agent-secret")
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("conditional request code=%d", rec.Code)
	}
}

func TestTrafficIdempotencyPreventsDoubleCounting(t *testing.T) {
	store := NewStore("")
	if err := store.PutAgent(Agent{ID: "edge-a", Secret: "agent-secret"}); err != nil {
		t.Fatal(err)
	}
	h := New("admin-secret", store)
	payload := panelapi.TrafficPushRequest{BatchID: "batch-1", Items: []panelapi.TrafficItem{{NodeID: "42", UserID: 7, Upload: 10, Download: 20}}}
	for range 2 {
		w := request(t, h, http.MethodPost, "/api/v2/agent/traffic", "agent-secret", payload)
		if w.Code != http.StatusAccepted {
			t.Fatalf("traffic code=%d body=%s", w.Code, w.Body.String())
		}
	}
	usage := store.Usage("42", 7)
	if usage.Upload != 10 || usage.Download != 20 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAdminRequiresAdminToken(t *testing.T) {
	h := New("admin-secret", NewStore(""))
	w := request(t, h, http.MethodGet, "/api/v1/admin/agents", "wrong", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAdminAgentListNeverLeaksSecret(t *testing.T) {
	store := NewStore("")
	if err := store.PutAgent(Agent{ID: "edge-a", Secret: "never-expose"}); err != nil {
		t.Fatal(err)
	}
	w := request(t, New("admin-secret", store), http.MethodGet, "/api/v1/admin/agents", "admin-secret", nil)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "never-expose") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}
