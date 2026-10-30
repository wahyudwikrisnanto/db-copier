//go:build integration

package copier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"db-copier/internal/store"
)

func TestIntegrationCancel(t *testing.T) {
	if os.Getenv("DB_COPIER_IT") == "" {
		t.Skip("set DB_COPIER_IT=1")
	}
	src := store.Connection{ID: "s", Host: "127.0.0.1", Port: 5433, Database: "app", User: "postgres", Password: "srcpw", SSLMode: "disable"}
	dst := store.Connection{ID: "d", Host: "127.0.0.1", Port: 5434, Database: "app_local", User: "postgres", Password: "dstpw", SSLMode: "disable"}
	pgDump, _ := exec.LookPath("pg_dump")
	pgRestore, _ := exec.LookPath("pg_restore")
	c := &Copier{Bins: Binaries{pgDump, pgRestore}, KnownHostsPath: filepath.Join(t.TempDir(), "kh"), Log: func(string) {}}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(1500*time.Millisecond, cancel)
	start := time.Now()
	err := c.Run(ctx, src, dst, Options{NoOwner: true, NoACL: true, ExitOnError: true})
	if err == nil || err.Error() != "cancelled" {
		t.Fatalf("want cancelled, got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("cancel took too long: %s", d)
	}
}
