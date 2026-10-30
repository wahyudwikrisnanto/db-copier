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
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"db-copier/internal/pg"
	"db-copier/internal/store"
	"db-copier/internal/tunnel"
)

type Options struct {
	NoOwner     bool `json:"noOwner"`
	NoACL       bool `json:"noAcl"`
	ExitOnError bool `json:"exitOnError"`
	// Jobs > 1 dumps to a temporary directory with that many parallel workers,
	// then restores in parallel. 0 or 1 streams pg_dump straight into pg_restore.
	Jobs int `json:"jobs"`
}

type Binaries struct {
	PgDump    string
	PgRestore string
}

type Copier struct {
	Bins           Binaries
	KnownHostsPath string
	Log            func(string)
	Reporter       Reporter // optional
}

type nopReporter struct{}

func (nopReporter) Plan(pg.Plan)      {}
func (nopReporter) Progress(Progress) {}
func (nopReporter) TableDone(string)  {}

func (c *Copier) reporter() Reporter {
	if c.Reporter == nil {
		return nopReporter{}
	}
	return c.Reporter
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
	restoreVer, restoreMajor, err := ToolVersion(c.Bins.PgRestore)
	if err != nil {
		return err
	}
	log("using " + dumpVer + " / " + restoreVer)
	rep := c.reporter()
	parallel := opts.Jobs > 1
	rep.Progress(Progress{Phase: PhaseConnect, Parallel: parallel})

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
	dstVer, dstMajor, err := pg.ServerInfo(ctx, dstSess.Endpoint)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	log("target server: " + dstVer)
	ignoreTimeout := restoreMajor >= 17 && dstMajor < 17
	if ignoreTimeout && opts.ExitOnError {
		// pg_restore 17+ always starts with SET transaction_timeout, which an
		// older server rejects; --exit-on-error would abort on that first line.
		log(fmt.Sprintf("warning: pg_restore %d is newer than target server %d; disabling --exit-on-error and ignoring the transaction_timeout error", restoreMajor, dstMajor))
		opts.ExitOnError = false
	}
	if restoreMajor > dstMajor {
		log(fmt.Sprintf("warning: target server %d is older than pg_restore %d; objects using newer features may fail to restore", dstMajor, restoreMajor))
	}

	plan, err := pg.LoadPlan(ctx, srcSess.Endpoint)
	if err != nil {
		log("warning: could not read table sizes from source, progress will be approximate: " + err.Error())
	}
	rep.Plan(plan)
	log(fmt.Sprintf("source has %d table(s), %s of table data", len(plan.Tables), formatBytes(totalBytes(plan))))

	tr := newTracker(rep, plan, parallel)
	stop := tr.tick(ctx)
	defer stop()
	erase := func() error {
		tr.stage(PhaseErase)
		tr.report()
		log(fmt.Sprintf("erasing target database %q...", dst.Database))
		if err := pg.Erase(ctx, dstSess.Endpoint, func(s string) { log("  " + s) }); err != nil {
			return fmt.Errorf("erase target: %w", err)
		}
		tr.stage(PhaseSchema)
		return nil
	}

	if parallel {
		err = c.runParallel(ctx, srcSess.Endpoint, dstSess.Endpoint, opts, ignoreTimeout, dumpMajor, tr, erase)
	} else if err = erase(); err == nil {
		log("copying...")
		err = c.pipe(ctx, srcSess.Endpoint, dstSess.Endpoint, opts, ignoreTimeout, tr)
	}
	if ctx.Err() != nil {
		err = errors.New("cancelled")
	}
	if err != nil {
		tr.report()
		return err
	}
	tr.done()
	return nil
}

func totalBytes(p pg.Plan) int64 {
	var n int64
	for _, t := range p.Tables {
		n += t.Bytes
	}
	return n
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// isTransactionTimeoutError matches the error an older server raises for the
// SET transaction_timeout that pg_restore 17+ emits at the start of a session.
func isTransactionTimeoutError(line string) bool {
	return strings.Contains(line, `unrecognized configuration parameter "transaction_timeout"`)
}

// restoreErrors collects pg_restore error lines and decides the outcome.
type restoreErrors struct {
	ignoreTimeout bool

	mu     sync.Mutex
	errs   []string
	benign int
}

func (r *restoreErrors) line(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ignoreTimeout && isTransactionTimeoutError(line) {
		r.benign++
	} else if strings.Contains(line, "error:") {
		r.errs = append(r.errs, line)
	}
}

func (r *restoreErrors) result(waitErr error, log func(string)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// pg_restore exits with status 1 when it ignored errors; succeed if every
	// error it printed was the expected transaction_timeout one. (With --jobs
	// its own "errors ignored" count does not match the printed lines.)
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) && exit.ExitCode() == 1 && len(r.errs) == 0 && r.benign > 0 {
		log(fmt.Sprintf("ignored %d transaction_timeout error(s) from older target server", r.benign))
		waitErr = nil
	}
	if waitErr != nil {
		if len(r.errs) > 0 {
			return fmt.Errorf("pg_restore failed: %s", r.errs[len(r.errs)-1])
		}
		return fmt.Errorf("pg_restore failed: %w", waitErr)
	}
	if len(r.errs) > 0 {
		return fmt.Errorf("pg_restore finished with %d error(s); see log", len(r.errs))
	}
	return nil
}

func restoreArgs(dst pg.Endpoint, opts Options) []string {
	args := append(connArgs(dst), "--verbose", "--no-password")
	if opts.NoOwner {
		args = append(args, "--no-owner")
	}
	if opts.NoACL {
		args = append(args, "--no-acl")
	}
	if opts.ExitOnError {
		args = append(args, "--exit-on-error")
	}
	return args
}

func (c *Copier) pipe(parent context.Context, src, dst pg.Endpoint, opts Options, ignoreTimeout bool, tr *tracker) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	// No compression: the archive only crosses a local pipe, and uncompressed
	// bytes make the streamed count comparable to table sizes.
	dumpArgs := append(connArgs(src), "--format=custom", "--compress=0", "--verbose", "--no-password")

	dump := exec.CommandContext(ctx, c.Bins.PgDump, dumpArgs...)
	dump.Env = pgEnv(src)
	restore := exec.CommandContext(ctx, c.Bins.PgRestore, restoreArgs(dst, opts)...)
	restore.Env = pgEnv(dst)

	pr, pw := io.Pipe()
	dump.Stdout = io.MultiWriter(pw, tr)
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
	rerr := &restoreErrors{ignoreTimeout: ignoreTimeout}
	logs.Add(2)
	go func() { defer logs.Done(); c.stream("dump", dumpErr, nil) }()
	go func() {
		defer logs.Done()
		c.stream("restore", restoreErr, func(line string) {
			tr.restoreLine(line)
			rerr.line(line)
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
	if err := rerr.result(restoreWaitErr, c.Log); err != nil {
		return err
	}
	if dumpWaitErr != nil {
		return fmt.Errorf("pg_dump failed: %w", dumpWaitErr)
	}
	return nil
}

// runParallel dumps the source into a temporary directory archive with
// opts.Jobs workers, erases the target only once the dump succeeded, then
// restores with the same number of workers.
func (c *Copier) runParallel(ctx context.Context, src, dst pg.Endpoint, opts Options, ignoreTimeout bool, dumpMajor int, tr *tracker, erase func() error) error {
	dir, err := dumpDir()
	if err != nil {
		return fmt.Errorf("temporary dump directory: %w", err)
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, "archive")
	jobs := strconv.Itoa(opts.Jobs)

	// zstd level 1 costs little CPU and keeps the temporary archive small;
	// pg_dump before 16 only has gzip.
	compress := "1"
	if dumpMajor >= 16 {
		compress = "zstd:1"
	}
	tr.stage(PhaseDump)
	tr.report()
	c.Log(fmt.Sprintf("dumping with %d jobs into %s...", opts.Jobs, dir))
	dumpArgs := append(connArgs(src), "--format=directory", "--jobs="+jobs, "--compress="+compress,
		"--file="+archive, "--verbose", "--no-password")
	var unsupported bool
	err = c.run(ctx, c.Bins.PgDump, dumpArgs, pgEnv(src), "dump", func(line string) {
		tr.dumpLine(line)
		if strings.Contains(line, "not support") && strings.Contains(line, "compress") {
			unsupported = true
		}
	})
	if err != nil && unsupported && compress != "1" && ctx.Err() == nil {
		c.Log("pg_dump was built without zstd; retrying with gzip")
		os.RemoveAll(archive)
		tr.stage(PhaseDump)
		dumpArgs[slices.Index(dumpArgs, "--compress="+compress)] = "--compress=1"
		err = c.run(ctx, c.Bins.PgDump, dumpArgs, pgEnv(src), "dump", tr.dumpLine)
	}
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	if size, err := dirSize(archive); err == nil {
		c.Log("dump finished: " + formatBytes(size) + " on disk")
	}

	if err := erase(); err != nil {
		return err
	}
	c.Log(fmt.Sprintf("restoring with %d jobs...", opts.Jobs))
	rerr := &restoreErrors{ignoreTimeout: ignoreTimeout}
	args := append(restoreArgs(dst, opts), "--jobs="+jobs, archive)
	waitErr := c.run(ctx, c.Bins.PgRestore, args, pgEnv(dst), "restore", func(line string) {
		tr.restoreLine(line)
		rerr.line(line)
	})
	return rerr.result(waitErr, c.Log)
}

// run executes a command, feeding each stderr line to onLine and the log.
func (c *Copier) run(ctx context.Context, bin string, args, env []string, prefix string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.stream(prefix, stderr, onLine)
	return cmd.Wait()
}

// dumpDir creates a fresh directory for a parallel dump under the user cache
// dir (not os.TempDir, which is often RAM-backed), removing leftovers from
// runs that were killed more than a day ago.
func dumpDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	base = filepath.Join(base, "db-copier")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	if old, _ := filepath.Glob(filepath.Join(base, "dump-*")); len(old) > 0 {
		for _, d := range old {
			if fi, err := os.Stat(d); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
				os.RemoveAll(d)
			}
		}
	}
	return os.MkdirTemp(base, "dump-")
}

func dirSize(dir string) (int64, error) {
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n, err
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
