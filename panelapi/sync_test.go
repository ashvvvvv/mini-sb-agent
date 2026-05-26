package panelapi

import "testing"

func TestFlattenKeepsNumericUsersAndDropsUUIDUsers(t *testing.T) {
	in := map[string]map[string][2]int64{
		"vless-tcp": {
			"1": {10, 20},
		},
		"hy2-udp": {
			"2":                                    {30, 40},
			"65ef6ea4-e719-497f-9fd1-e1fab9c0384d": {50, 60},
		},
	}
	got := flatten(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 numeric users, got %d: %#v", len(got), got)
	}
	if got["1"] != [2]int64{10, 20} {
		t.Fatalf("user 1 delta mismatch: %#v", got["1"])
	}
	if got["2"] != [2]int64{30, 40} {
		t.Fatalf("user 2 delta mismatch: %#v", got["2"])
	}
	if _, ok := got["65ef6ea4-e719-497f-9fd1-e1fab9c0384d"]; ok {
		t.Fatalf("uuid-like user must not be pushed to panel api: %#v", got)
	}
}
