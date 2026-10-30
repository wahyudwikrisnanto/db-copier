package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"db-copier/internal/copier"
	"db-copier/internal/history"
	"db-copier/internal/pg"
	"db-copier/internal/store"
)

// App is bound to the frontend; its exported methods become JS functions.
type App struct {
	ctx     context.Context
	store   *store.Store
	history *history.Store

	mu         sync.Mutex
	cancelCopy context.CancelFunc
}

func NewApp(s *store.Store, h *history.Store) *App {
	return &App{store: s, history: h}
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
	JobID      string `json:"jobId"`
}

// StartCopy erases the target and copies the source into it in the background.
// Events: "copy:plan" (tables and sizes, once), "copy:log" (batches of lines),
// "copy:progress" (latest snapshot), "copy:tables" (batches of finished table
// names) and finally "copy:done". The run is recorded in the history, which
// emits "history:changed" when it starts and finishes.
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
	rec, err := a.history.Start(history.Job{
		Kind:   history.KindCopy,
		Source: history.RefOf(src),
		Target: history.RefOf(dst),
		Copy:   &history.CopyInfo{Options: opts},
	})
	if err != nil {
		a.mu.Unlock()
		return errors.New("record history: " + err.Error())
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelCopy = cancel
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "history:changed")

	go func() {
		start := time.Now()
		logs := newLogBatcher(a.ctx, rec)
		c := &copier.Copier{
			Bins:           a.binaries(a.store.Settings()),
			KnownHostsPath: a.knownHostsPath(),
			Log:            logs.Add,
			Reporter:       logs,
		}
		err := c.Run(ctx, src, dst, opts)
		res := CopyResult{OK: err == nil, DurationMs: time.Since(start).Milliseconds(), JobID: rec.ID()}
		status := history.StatusCompleted
		if err != nil {
			res.Error = err.Error()
			logs.Add("ERROR: " + err.Error())
			status = history.StatusFailed
			if ctx.Err() != nil {
				status = history.StatusCancelled
			}
		} else {
			logs.Add("done in " + time.Since(start).Round(time.Millisecond).String())
		}
		logs.Close()
		if _, herr := rec.Finish(status, res.Error); herr != nil {
			runtime.LogError(a.ctx, "record history: "+herr.Error())
		}

		a.mu.Lock()
		a.cancelCopy = nil
		a.mu.Unlock()
		cancel()
		runtime.EventsEmit(a.ctx, "copy:done", res)
		runtime.EventsEmit(a.ctx, "history:changed")
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

// ListJobs returns the activity history, newest first.
func (a *App) ListJobs() ([]history.Job, error) {
	return a.history.List()
}

// GetJob returns one job with its detail and the tail of its log.
func (a *App) GetJob(id string) (history.Detail, error) {
	return a.history.Get(id)
}

func (a *App) DeleteJob(id string) error {
	if err := a.history.Delete(id); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "history:changed")
	return nil
}

// ClearHistory deletes every finished job.
func (a *App) ClearHistory() error {
	if err := a.history.Clear(); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "history:changed")
	return nil
}

// ExportJobLog asks for a destination and copies the job's full log there.
// Returns the chosen path, or "" if the dialog was cancelled.
func (a *App) ExportJobLog(id string) (string, error) {
	src, err := a.history.LogPath(id)
	if err != nil {
		return "", err
	}
	dst, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export log",
		DefaultFilename: "db-copier-" + id + ".log",
	})
	if err != nil || dst == "" {
		return "", err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	return dst, out.Close()
}

// logBatcher coalesces log lines, finished tables and progress snapshots so
// verbose pg_dump/pg_restore output does not flood the frontend with one event
// per line. It also implements copier.Reporter and forwards everything to the
// job's history recorder.
type logBatcher struct {
	ctx      context.Context
	rec      *history.Recorder
	mu       sync.Mutex
	buf      []string
	tables   []string
	progress *copier.Progress
	stop     chan struct{}
	done     chan struct{}
}

func newLogBatcher(ctx context.Context, rec *history.Recorder) *logBatcher {
	b := &logBatcher{ctx: ctx, rec: rec, stop: make(chan struct{}), done: make(chan struct{})}
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
	line = time.Now().Format("15:04:05") + "  " + line
	b.rec.Log(line)
	b.mu.Lock()
	b.buf = append(b.buf, line)
	b.mu.Unlock()
}

func (b *logBatcher) Plan(p pg.Plan) {
	if p.Tables == nil {
		p.Tables = []pg.Table{}
	}
	b.rec.Plan(p)
	runtime.EventsEmit(b.ctx, "copy:plan", p)
}

func (b *logBatcher) Progress(p copier.Progress) {
	b.rec.Progress(p)
	b.mu.Lock()
	b.progress = &p
	b.mu.Unlock()
}

func (b *logBatcher) TableDone(name string) {
	b.rec.TableDone(name)
	b.mu.Lock()
	b.tables = append(b.tables, name)
	b.mu.Unlock()
}

func (b *logBatcher) flush() {
	b.mu.Lock()
	lines, tables, progress := b.buf, b.tables, b.progress
	b.buf, b.tables, b.progress = nil, nil, nil
	b.mu.Unlock()
	b.rec.Flush()
	if len(lines) > 0 {
		runtime.EventsEmit(b.ctx, "copy:log", lines)
	}
	if len(tables) > 0 {
		runtime.EventsEmit(b.ctx, "copy:tables", tables)
	}
	if progress != nil {
		runtime.EventsEmit(b.ctx, "copy:progress", *progress)
	}
}

func (b *logBatcher) Close() {
	close(b.stop)
	<-b.done
}
