package panelserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardIsServed(t *testing.T) {
	h := New("admin-secret", NewStore(""))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "mini-sb 控制台") || !strings.Contains(body, "/api/v1/admin/agents") {
		t.Fatalf("dashboard missing expected controls: %s", body)
	}
}
