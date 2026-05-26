package panelapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalUsersFetchesNeutralUsersList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte(`{"users":[{"id":7,"uuid":"uuid-7","password":"pw-7","name":"name-7","speed_limit":3}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	users, err := (LocalUsers{Path: path}).FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != 7 || users[0].UUID != "uuid-7" || users[0].Password != "pw-7" || users[0].Name != "name-7" || users[0].SpeedLimit != 3 {
		t.Fatalf("unexpected users: %#v", users)
	}
}

func TestLocalUsersFetchesLegacyInboundMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte(`{"inbounds":{"hy2":[{"id":8,"password":"pw-8","name":"name-8","speed_limit":4}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	users, err := (LocalUsers{Path: path}).FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != 8 || users[0].Password != "pw-8" || users[0].Name != "name-8" || users[0].SpeedLimit != 4 {
		t.Fatalf("unexpected users: %#v", users)
	}
}
