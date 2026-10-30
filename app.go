package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"db-copier/internal/copier"
	"db-copier/internal/pg"
	"db-copier/internal/store"
)

// App is bound to the frontend; its exported methods become JS functions.
type App struct {
	ctx   context.Context
	store *store.Store

	mu         sync.Mutex
	cancelCopy context.CancelFunc
}

func NewApp(s *store.Store) *App {
	return &App{store: s}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) knownHostsPath() string {
	return filepath.Join(a.store.Dir(), "known_hosts")
}

func (a *App) ListConnections() ([]store.Connection, error) {
	return a.store.List()
}

func (a *App) SaveConnection(c store.Connection) (store.Connection, error) {
	return a.store.Save(c)
}

func (a *App) DeleteConnection(id string) error {
	return a.store.Delete(id)
}

// TestConnection connects (through SSH if configured) and returns the server version.
func (a *App) TestConnection(c store.Connection) (string, error) {
	sess, err := copier.OpenSession(c, a.knownHostsPath())
	if err != nil {
		return "", err
	}
	defer sess.Close()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	version, _, err := pg.ServerInfo(ctx, sess.Endpoint)
	return version, err
}

// PickKeyFile opens a native file dialog for choosing an SSH private key.
func (a *App) PickKeyFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select SSH private key"})
}

type ToolInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

type SettingsInfo struct {
	Settings  store.Settings `json:"settings"`
	PgDump    ToolInfo       `json:"pgDump"`
	PgRestore ToolInfo       `json:"pgRestore"`
	ConfigDir string         `json:"configDir"`
}

func (a *App) GetSettings() SettingsInfo {
	st := a.store.Settings()
	bins := a.binaries(st)
	return SettingsInfo{
		Settings:  st,
		PgDump:    toolInfo(bins.PgDump, "pg_dump"),
		PgRestore: toolInfo(bins.PgRestore, "pg_restore"),
		ConfigDir: a.store.Dir(),
	}
}

func (a *App) SaveSettings(st store.Settings) (SettingsInfo, error) {
	if err := a.store.SaveSettings(st); err != nil {
		return SettingsInfo{}, err
	}
	return a.GetSettings(), nil
}

func (a *App) binaries(st store.Settings) copier.Binaries {
	return copier.Binaries{
		PgDump:    resolveBin(st.PgDumpPath, "pg_dump"),
		PgRestore: resolveBin(st.PgRestorePath, "pg_restore"),
	}
}

func resolveBin(configured, name string) string {
	if configured != "" {
		return configured
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func toolInfo(path, name string) ToolInfo {
	info := ToolInfo{Path: path}
	v, _, err := copier.ToolVersion(path)
	if err != nil {
		info.Error = name + " not found or not runnable: " + err.Error()
	}
	info.Version = v
	return info
}

type CopyResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error"`
	DurationMs int64  `json:"durationMs"`
}

// StartCopy erases the target and copies the source into it in the background.
// Progress arrives as "copy:log" events (batches of lines); completion as "copy:done".
func (a *App) StartCopy(sourceID, targetID string, opts copier.Options) error {
	a.mu.Lock()
	if a.cancelCopy != nil {
		a.mu.Unlock()
		return errors.New("a copy is already running")
	}
	src, err := a.store.Get(sourceID)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	dst, err := a.store.Get(targetID)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	if copier.SameTarget(src, dst) {
		a.mu.Unlock()
		return errors.New("source and target are the same database")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelCopy = cancel
	a.mu.Unlock()

	go func() {
		start := time.Now()
		logs := newLogBatcher(func(lines []string) { runtime.EventsEmit(a.ctx, "copy:log", lines) })
		c := &copier.Copier{
			Bins:           a.binaries(a.store.Settings()),
			KnownHostsPath: a.knownHostsPath(),
			Log:            logs.Add,
		}
		err := c.Run(ctx, src, dst, opts)
		res := CopyResult{OK: err == nil, DurationMs: time.Since(start).Milliseconds()}
		if err != nil {
			res.Error = err.Error()
			logs.Add("ERROR: " + err.Error())
		} else {
			logs.Add("done in " + time.Since(start).Round(time.Millisecond).String())
		}
		logs.Close()

		a.mu.Lock()
		a.cancelCopy = nil
		a.mu.Unlock()
		cancel()
		runtime.EventsEmit(a.ctx, "copy:done", res)
	}()
	return nil
}

func (a *App) CancelCopy() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelCopy != nil {
		a.cancelCopy()
	}
}

func (a *App) IsCopyRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cancelCopy != nil
}

// logBatcher coalesces log lines so verbose pg_dump/pg_restore output does not
// flood the frontend with one event per line.
type logBatcher struct {
	mu   sync.Mutex
	buf  []string
	emit func([]string)
	stop chan struct{}
	done chan struct{}
}

func newLogBatcher(emit func([]string)) *logBatcher {
	b := &logBatcher{emit: emit, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(b.done)
		t := time.NewTicker(150 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				b.flush()
			case <-b.stop:
				b.flush()
				return
			}
		}
	}()
	return b
}

func (b *logBatcher) Add(line string) {
	b.mu.Lock()
	b.buf = append(b.buf, time.Now().Format("15:04:05")+"  "+line)
	b.mu.Unlock()
}

func (b *logBatcher) flush() {
	b.mu.Lock()
	lines := b.buf
	b.buf = nil
	b.mu.Unlock()
	if len(lines) > 0 {
		b.emit(lines)
	}
}

func (b *logBatcher) Close() {
	close(b.stop)
	<-b.done
}
