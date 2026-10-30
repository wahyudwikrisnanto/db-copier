//go:build integration

package copier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"db-copier/internal/store"
)

// Run with: DB_COPIER_IT=1 go test -tags integration ./internal/copier -run Integration -v
// Expects the docker setup described in README (source :5433 behind ssh :2222, target :5434).
func TestIntegrationCopyViaSSH(t *testing.T) {
	if os.Getenv("DB_COPIER_IT") == "" {
		t.Skip("set DB_COPIER_IT=1")
	}
	src := store.Connection{
		ID: "src", Name: "source", Host: "dbcopy-src", Port: 5432, Database: "app",
		User: "postgres", Password: "srcpw", SSLMode: "disable",
		SSH: store.SSHConfig{Enabled: true, Host: "127.0.0.1", Port: 2222, User: "tunnel", AuthMethod: "password", Password: "sshpw"},
	}
	dst := store.Connection{
		ID: "dst", Name: "target", Host: "127.0.0.1", Port: 5434, Database: "app_local",
		User: "postgres", Password: "dstpw", SSLMode: "disable",
	}
	pgDump, _ := exec.LookPath("pg_dump")
	pgRestore, _ := exec.LookPath("pg_restore")
	c := &Copier{
		Bins:           Binaries{PgDump: pgDump, PgRestore: pgRestore},
		KnownHostsPath: filepath.Join(t.TempDir(), "known_hosts"),
		Log:            func(s string) { t.Log(s) },
	}
	opts := Options{NoOwner: true, NoACL: true, ExitOnError: true}
	// Run twice: the second run proves the erase step makes copies repeatable.
	for i := 0; i < 2; i++ {
		if err := c.Run(context.Background(), src, dst, opts); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	srcDirect := src
	srcDirect.SSH.Enabled = false
	srcDirect.Host, srcDirect.Port = "127.0.0.1", 5433
	queries := map[string]string{
		"schemas":    `SELECT string_agg(nspname, ',' ORDER BY nspname) FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema'`,
		"extensions": `SELECT string_agg(extname, ',' ORDER BY extname) FROM pg_extension`,
		"indexes":    `SELECT string_agg(schemaname||'.'||indexname||':'||indexdef, E'\n' ORDER BY schemaname, indexname) FROM pg_indexes WHERE schemaname NOT IN ('pg_catalog','information_schema')`,
		"seq_users":  `SELECT last_value::text FROM users_id_seq`,
		"seq_inv":    `SELECT last_value::text FROM billing.invoice_no`,
		"seq_ident":  `SELECT last_value::text FROM billing.invoices_id_seq`,
		"users":      `SELECT count(*)::text FROM users`,
		"invoices":   `SELECT count(*)::text||':'||sum(amount)::text FROM billing.invoices`,
		"matview":    `SELECT count(*)::text FROM billing.totals_mv`,
		"triggers":   `SELECT string_agg(tgname, ',' ORDER BY tgname) FROM pg_trigger WHERE NOT tgisinternal`,
		"comment":    `SELECT obj_description('users'::regclass)`,
		"next_user":  `SELECT pg_get_serial_sequence('users','id')`,
	}
	for name, q := range queries {
		want := psql(t, srcDirect, q)
		got := psql(t, dst, q)
		if want != got {
			t.Errorf("%s mismatch:\nsource: %s\ntarget: %s", name, want, got)
		}
	}
	if got := psql(t, dst, `SELECT count(*) FROM pg_namespace WHERE nspname = 'junk'`); got != "0" {
		t.Errorf("junk schema survived erase")
	}
}

func psql(t *testing.T, c store.Connection, q string) string {
	t.Helper()
	cmd := exec.Command("psql", "-XAt", "-h", c.Host, "-p", strconv.Itoa(c.Port), "-U", c.User, "-d", c.Database, "-c", q)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("psql %q: %v\n%s", q, err, out)
	}
	return strings.TrimSpace(string(out))
}
