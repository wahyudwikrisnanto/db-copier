// Package history records finished and running jobs (database copies today,
// query runs later) so they can be reviewed afterwards. Each job lives in its
// own directory under <configDir>/history/<id>/:
//
//	job.json   summary shown in the list (Job)
//	copy.json  copy-specific detail: plan, last progress, finished tables
//	log.txt    every log line, appended while the job runs
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"db-copier/internal/copier"
	"db-copier/internal/pg"
	"db-copier/internal/store"
)

type Kind string

const (
	KindCopy Kind = "copy"
	// KindQuery is reserved for the future query executor.
	KindQuery Kind = "query"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// MaxJobs is how many jobs are kept; older ones are pruned when a new job starts.
const MaxJobs = 200

// MaxLogLines caps the log lines returned by Get; the full log stays on disk.
const MaxLogLines = 10000

const interruptedError = "interrupted: the app exited while this job was running"

// ConnRef is a snapshot of a connection at the time the job ran, so history
// still reads correctly after the connection is edited or deleted. It never
// holds secrets.
type ConnRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	SSHHost  string `json:"sshHost,omitempty"`
}

func RefOf(c store.Connection) *ConnRef {
	r := &ConnRef{ID: c.ID, Name: c.Name, Color: c.Color, Host: c.Host, Port: c.Port, Database: c.Database, User: c.User}
	if c.SSH.Enabled {
		r.SSHHost = c.SSH.Host
	}
	return r
}

// CopyInfo is the copy-specific part of a job summary.
type CopyInfo struct {
	Options     copier.Options `json:"options"`
	TablesTotal int            `json:"tablesTotal"`
	TablesDone  int            `json:"tablesDone"`
	BytesTotal  int64          `json:"bytesTotal"`
	Errors      int            `json:"errors"`
}

type Job struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	Status     Status     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	DurationMs int64      `json:"durationMs"`
	Error      string     `json:"error,omitempty"`
	Source     *ConnRef   `json:"source,omitempty"`
	Target     *ConnRef   `json:"target,omitempty"`
	Copy       *CopyInfo  `json:"copy,omitempty"`
}

// CopyDetail is what the copy progress view needs to redraw a finished copy.
type CopyDetail struct {
	Plan       *pg.Plan         `json:"plan,omitempty"`
	Progress   *copier.Progress `json:"progress,omitempty"`
	DoneTables []string         `json:"doneTables"`
}

type Detail struct {
	Job
	CopyDetail   *CopyDetail `json:"copyDetail,omitempty"`
	Log          []string    `json:"log"`
	LogTruncated bool        `json:"logTruncated"`
	LogPath      string      `json:"logPath"`
}

type Store struct {
	mu      sync.Mutex
	dir     string
	running map[string]bool
}

// Open opens the history under dir. Jobs left "running" by a previous process
// are marked failed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, running: map[string]bool{}}
	jobs, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if j.Status != StatusRunning {
			continue
		}
		end := j.StartedAt
		if fi, err := os.Stat(s.logPath(j.ID)); err == nil && fi.ModTime().After(end) {
			end = fi.ModTime()
		}
		j.Status, j.Error = StatusFailed, interruptedError
		j.FinishedAt, j.DurationMs = &end, end.Sub(j.StartedAt).Milliseconds()
		if err := s.writeJSON(j.ID, "job.json", j); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) jobDir(id string) string  { return filepath.Join(s.dir, id) }
func (s *Store) logPath(id string) string { return filepath.Join(s.jobDir(id), "log.txt") }

// List returns all jobs, newest first.
func (s *Store) List() ([]Job, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var j Job
		if err := s.readJSON(e.Name(), "job.json", &j); err != nil {
			continue // half-written or foreign directory
		}
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, k int) bool { return jobs[i].StartedAt.After(jobs[k].StartedAt) })
	return jobs, nil
}

// Get returns a job with its detail and the tail of its log.
func (s *Store) Get(id string) (Detail, error) {
	if !validID(id) {
		return Detail{}, fmt.Errorf("job %q not found", id)
	}
	var d Detail
	if err := s.readJSON(id, "job.json", &d.Job); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Detail{}, fmt.Errorf("job %q not found", id)
		}
		return Detail{}, err
	}
	if d.Kind == KindCopy {
		var cd CopyDetail
		if err := s.readJSON(id, "copy.json", &cd); err == nil {
			d.CopyDetail = &cd
		}
	}
	d.LogPath = s.logPath(id)
	lines, truncated, err := tailLines(d.LogPath, MaxLogLines)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Detail{}, err
	}
	d.Log, d.LogTruncated = lines, truncated
	return d, nil
}

// LogPath returns the path of a job's full log file.
func (s *Store) LogPath(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("job %q not found", id)
	}
	return s.logPath(id), nil
}

func (s *Store) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("job %q not found", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[id] {
		return errors.New("cannot delete a running job")
	}
	return os.RemoveAll(s.jobDir(id))
}

// Clear deletes every job that is not running.
func (s *Store) Clear() error {
	jobs, err := s.List()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		s.mu.Lock()
		running := s.running[j.ID]
		s.mu.Unlock()
		if running {
			continue
		}
		if err := s.Delete(j.ID); err != nil {
			return err
		}
	}
	return nil
}

// Start records a new running job. ID, Status and StartedAt are filled in.
func (s *Store) Start(j Job) (*Recorder, error) {
	j.StartedAt = time.Now()
	j.ID = j.StartedAt.Format("20060102-150405") + "-" + uuid.NewString()[:8]
	j.Status = StatusRunning
	if err := os.MkdirAll(s.jobDir(j.ID), 0o700); err != nil {
		return nil, err
	}
	if err := s.writeJSON(j.ID, "job.json", j); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.logPath(j.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.running[j.ID] = true
	s.mu.Unlock()
	s.prune()
	return &Recorder{s: s, job: j, f: f, w: bufio.NewWriter(f), copy: CopyDetail{DoneTables: []string{}}}, nil
}

// prune removes the oldest finished jobs beyond MaxJobs.
func (s *Store) prune() {
	jobs, err := s.List()
	if err != nil {
		return
	}
	for _, j := range jobs[min(len(jobs), MaxJobs):] {
		_ = s.Delete(j.ID)
	}
}

func (s *Store) readJSON(id, name string, v any) error {
	b, err := os.ReadFile(filepath.Join(s.jobDir(id), name))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Store) writeJSON(id, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.jobDir(id), name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Recorder collects a running job's log and copy progress. All methods are
// safe for concurrent use.
type Recorder struct {
	s *Store

	mu   sync.Mutex
	job  Job
	f    *os.File
	w    *bufio.Writer
	copy CopyDetail
}

func (r *Recorder) ID() string { return r.job.ID }

func (r *Recorder) Log(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.w.WriteString(line)
	_ = r.w.WriteByte('\n')
}

// Flush writes buffered log lines to disk.
func (r *Recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.w.Flush()
}

func (r *Recorder) Plan(p pg.Plan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.copy.Plan = &p
}

func (r *Recorder) Progress(p copier.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.copy.Progress = &p
}

func (r *Recorder) TableDone(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.copy.DoneTables = append(r.copy.DoneTables, name)
}

// Finish closes the log and stores the final job state.
func (r *Recorder) Finish(status Status, errMsg string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		r.s.mu.Lock()
		delete(r.s.running, r.job.ID)
		r.s.mu.Unlock()
	}()

	logErr := errors.Join(r.w.Flush(), r.f.Close())
	end := time.Now()
	r.job.Status, r.job.Error = status, errMsg
	r.job.FinishedAt, r.job.DurationMs = &end, end.Sub(r.job.StartedAt).Milliseconds()
	if r.job.Kind == KindCopy {
		if r.job.Copy == nil {
			r.job.Copy = &CopyInfo{}
		}
		if p := r.copy.Plan; p != nil {
			r.job.Copy.TablesTotal = len(p.Tables)
			for _, t := range p.Tables {
				r.job.Copy.BytesTotal += t.Bytes
			}
		}
		r.job.Copy.TablesDone = len(r.copy.DoneTables)
		if p := r.copy.Progress; p != nil {
			r.job.Copy.Errors = p.Errors
		}
		if err := r.s.writeJSON(r.job.ID, "copy.json", r.copy); err != nil {
			return r.job, err
		}
	}
	if err := r.s.writeJSON(r.job.ID, "job.json", r.job); err != nil {
		return r.job, err
	}
	return r.job, logErr
}

// validID rejects anything that could escape the history directory.
func validID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id
}

// tailLines returns the last n lines of the file at path.
func tailLines(path string, n int) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return []string{}, false, err
	}
	defer f.Close()
	ring := make([]string, 0, n)
	start, total := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		total++
		if len(ring) < n {
			ring = append(ring, sc.Text())
			continue
		}
		ring[start] = sc.Text()
		start = (start + 1) % n
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	out := append(ring[start:len(ring):len(ring)], ring[:start]...)
	return out, total > n, nil
}
