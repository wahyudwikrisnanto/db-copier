package store

import (
	"os"
	"strings"
	"testing"
)

func TestSaveKeepsSecretsOutOfJSON(t *testing.T) {
	t.Setenv("DB_COPIER_NO_KEYRING", "1")
	dir := t.TempDir()
	s, err := NewAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Save(Connection{Name: "prod", Host: "db", Password: "topsecret", SSH: SSHConfig{Enabled: true, Password: "sshpw"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.dataPath())
	if strings.Contains(string(b), "topsecret") || strings.Contains(string(b), "sshpw") {
		t.Fatalf("secret leaked into connections.json: %s", b)
	}
	s2, _ := NewAt(dir)
	got, err := s2.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "topsecret" || got.SSH.Password != "sshpw" {
		t.Fatalf("secrets not restored: %+v", got)
	}
	if err := s2.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s2.List(); len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}
