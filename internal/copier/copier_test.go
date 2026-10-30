package copier

import (
	"testing"

	"db-copier/internal/store"
)

func TestParseToolMajor(t *testing.T) {
	cases := map[string]int{
		"pg_dump (PostgreSQL) 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1)\n": 18,
		"pg_restore (PostgreSQL) 9.6.24":                             9,
		"pg_dump (PostgreSQL) 17beta1":                               17,
	}
	for in, want := range cases {
		got, err := ParseToolMajor(in)
		if err != nil || got != want {
			t.Errorf("ParseToolMajor(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseToolMajor("garbage"); err == nil {
		t.Error("expected error for garbage input")
	}
}

func TestSameTarget(t *testing.T) {
	base := store.Connection{ID: "a", Host: "localhost", Port: 5432, Database: "app"}
	cases := []struct {
		name string
		b    store.Connection
		want bool
	}{
		{"same id", store.Connection{ID: "a", Host: "other"}, true},
		{"loopback alias + default port", store.Connection{ID: "b", Host: "127.0.0.1", Database: "app"}, true},
		{"different db", store.Connection{ID: "b", Host: "localhost", Port: 5432, Database: "app2"}, false},
		{"different port", store.Connection{ID: "b", Host: "localhost", Port: 5433, Database: "app"}, false},
		{"via ssh", store.Connection{ID: "b", Host: "localhost", Port: 5432, Database: "app", SSH: store.SSHConfig{Enabled: true, Host: "prod"}}, false},
	}
	for _, c := range cases {
		if got := SameTarget(base, c.b); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
