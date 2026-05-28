package panelapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUsersFallbackAndFillMissingPasswordAndName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":101,"uuid":"uuid-101","speed_limit":7}]}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", "1", "vless")
	users, err := c.FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("users len=%d, want 1", len(users))
	}
	if users[0].Password != "uuid-101" {
		t.Fatalf("password=%q, want uuid fallback", users[0].Password)
	}
	if users[0].Name != "101" {
		t.Fatalf("name=%q, want numeric id fallback", users[0].Name)
	}
}
