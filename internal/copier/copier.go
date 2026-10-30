// Package copier runs a full copy: open tunnels, erase the target, then
// stream pg_dump from the source straight into pg_restore on the target.
package copier

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"db-copier/internal/pg"
	"db-copier/internal/store"
	"db-copier/internal/tunnel"
)

type Options struct {
	NoOwner     bool `json:"noOwner"`
	NoACL       bool `json:"noAcl"`
	ExitOnError bool `json:"exitOnError"`
}

type Binaries struct {
	PgDump    string
	PgRestore string
}

type Copier struct {
	Bins           Binaries
	KnownHostsPath string
	Log            func(string)
}

// Session is an open, resolved connection (tunnel already up if needed).
type Session struct {
	Endpoint pg.Endpoint
	tunnel   *tunnel.Tunnel
}

func (s *Session) Close() {
	if s.tunnel != nil {
		s.tunnel.Close()
	}
}

// OpenSession opens the SSH tunnel if the connection needs one and returns
// the endpoint to reach Postgres at.
func OpenSession(c store.Connection, knownHostsPath string) (*Session, error) {
	port := c.Port
	if port == 0 {
		port = 5432
	}
	ep := pg.Endpoint{
		Host: c.Host, Port: port, Database: c.Database,
		User: c.User, Password: c.Password, SSLMode: c.SSLMode,
	}
	if !c.SSH.Enabled {
		return &Session{Endpoint: ep}, nil
	}
	t, err := tunnel.Open(c.SSH, c.Host, port, knownHostsPath)
	if err != nil {
		return nil, err
	}
	ep.Host = "127.0.0.1"
	ep.Port = t.LocalPort()
	return &Session{Endpoint: ep, tunnel: t}, nil
}

// SameTarget reports whether two saved connections point at the same database.
func SameTarget(a, b store.Connection) bool {
	if a.ID != "" && a.ID == b.ID {
		return true
	}
	if a.SSH.Enabled != b.SSH.Enabled {
		return false
	}
	if a.SSH.Enabled && (normHost(a.SSH.Host) != normHost(b.SSH.Host) || orDefault(a.SSH.Port, 22) != orDefault(b.SSH.Port, 22)) {
		return false
	}
	return normHost(a.Host) == normHost(b.Host) &&
		orDefault(a.Port, 5432) == orDefault(b.Port, 5432) &&
		a.Database == b.Database
}

func normHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	switch h {
	case "", "localhost", "127.0.0.1", "::1":
		return "localhost"
	}
	return h
}

func orDefault(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

var versionRe = regexp.MustCompile(`\(PostgreSQL\)\s+(\d+)`)

// ParseToolMajor extracts the major version from `pg_dump --version` output.
func ParseToolMajor(out string) (int, error) {
	m := versionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("cannot parse version from %q", strings.TrimSpace(out))
	}
	return strconv.Atoi(m[1])
}

// ToolVersion runs `<bin> --version` and returns the raw output and major version.
func ToolVersion(bin string) (string, int, error) {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", bin, err)
	}
	major, err := ParseToolMajor(string(out))
	return strings.TrimSpace(string(out)), major, err
}

func (c *Copier) Run(ctx context.Context, src, dst store.Connection, opts Options) error {
	log := c.Log
	if SameTarget(src, dst) {
		return errors.New("source and target are the same database")
	}

	dumpVer, dumpMajor, err := ToolVersion(c.Bins.PgDump)
	if err != nil {
		return err
	}
	restoreVer, _, err := ToolVersion(c.Bins.PgRestore)
	if err != nil {
		return err
	}
	log("using " + dumpVer + " / " + restoreVer)

	log(fmt.Sprintf("connecting to source %q...", src.Name))
	srcSess, err := OpenSession(src, c.KnownHostsPath)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	defer srcSess.Close()

	log(fmt.Sprintf("connecting to target %q...", dst.Name))
	dstSess, err := OpenSession(dst, c.KnownHostsPath)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	defer dstSess.Close()

	srcVer, srcMajor, err := pg.ServerInfo(ctx, srcSess.Endpoint)
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	log("source server: " + srcVer)
	if dumpMajor < srcMajor {
		return fmt.Errorf("pg_dump %d is older than source server %d; install pg_dump %d+ and set its path in Settings", dumpMajor, srcMajor, srcMajor)
	}
	dstVer, _, err := pg.ServerInfo(ctx, dstSess.Endpoint)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	log("target server: " + dstVer)

	log(fmt.Sprintf("erasing target database %q...", dst.Database))
	if err := pg.Erase(ctx, dstSess.Endpoint, func(s string) { log("  " + s) }); err != nil {
		return fmt.Errorf("erase target: %w", err)
	}

	log("copying...")
	return c.pipe(ctx, srcSess.Endpoint, dstSess.Endpoint, opts)
}

func (c *Copier) pipe(parent context.Context, src, dst pg.Endpoint, opts Options) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	dumpArgs := append(connArgs(src), "--format=custom", "--verbose", "--no-password")
	restoreArgs := append(connArgs(dst), "--verbose", "--no-password")
	if opts.NoOwner {
		restoreArgs = append(restoreArgs, "--no-owner")
	}
	if opts.NoACL {
		restoreArgs = append(restoreArgs, "--no-acl")
	}
	if opts.ExitOnError {
		restoreArgs = append(restoreArgs, "--exit-on-error")
	}

	dump := exec.CommandContext(ctx, c.Bins.PgDump, dumpArgs...)
	dump.Env = pgEnv(src)
	restore := exec.CommandContext(ctx, c.Bins.PgRestore, restoreArgs...)
	restore.Env = pgEnv(dst)

	pr, pw := io.Pipe()
	dump.Stdout = pw
	restore.Stdin = pr

	dumpErr, err := dump.StderrPipe()
	if err != nil {
		return err
	}
	restoreErr, err := restore.StderrPipe()
	if err != nil {
		return err
	}

	if err := restore.Start(); err != nil {
		return fmt.Errorf("start pg_restore: %w", err)
	}
	if err := dump.Start(); err != nil {
		cancel()
		restore.Wait()
		return fmt.Errorf("start pg_dump: %w", err)
	}

	var logs sync.WaitGroup
	var restoreErrors []string
	var mu sync.Mutex
	logs.Add(2)
	go func() { defer logs.Done(); c.stream("dump", dumpErr, nil) }()
	go func() {
		defer logs.Done()
		c.stream("restore", restoreErr, func(line string) {
			if strings.Contains(line, "error:") {
				mu.Lock()
				restoreErrors = append(restoreErrors, line)
				mu.Unlock()
			}
		})
	}()

	var dumpWaitErr, restoreWaitErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		dumpWaitErr = dump.Wait()
		pw.CloseWithError(dumpWaitErr)
		if dumpWaitErr != nil {
			cancel()
		}
	}()
	go func() {
		defer wait.Done()
		restoreWaitErr = restore.Wait()
		pr.CloseWithError(io.ErrClosedPipe)
		if restoreWaitErr != nil {
			cancel()
		}
	}()
	wait.Wait()
	logs.Wait()

	if parent.Err() != nil {
		return errors.New("cancelled")
	}
	if restoreWaitErr != nil {
		if len(restoreErrors) > 0 {
			return fmt.Errorf("pg_restore failed: %s", restoreErrors[len(restoreErrors)-1])
		}
		return fmt.Errorf("pg_restore failed: %w", restoreWaitErr)
	}
	if dumpWaitErr != nil {
		return fmt.Errorf("pg_dump failed: %w", dumpWaitErr)
	}
	if len(restoreErrors) > 0 {
		return fmt.Errorf("pg_restore finished with %d error(s); see log", len(restoreErrors))
	}
	return nil
}

func (c *Copier) stream(prefix string, r io.Reader, onLine func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if onLine != nil {
			onLine(line)
		}
		c.Log("[" + prefix + "] " + line)
	}
}

func connArgs(e pg.Endpoint) []string {
	return []string{
		"--host=" + e.Host,
		"--port=" + strconv.Itoa(e.Port),
		"--username=" + e.User,
		"--dbname=" + e.Database,
	}
}

func pgEnv(e pg.Endpoint) []string {
	env := os.Environ()
	env = append(env, "PGPASSWORD="+e.Password, "PGAPPNAME=db-copier", "PGCONNECT_TIMEOUT=15")
	if e.SSLMode != "" {
		env = append(env, "PGSSLMODE="+e.SSLMode)
	}
	return env
}
