package copier

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"db-copier/internal/pg"
)

type Phase string

const (
	PhaseConnect  Phase = "connect"
	PhaseDump     Phase = "dump" // parallel mode only: dumping to disk
	PhaseErase    Phase = "erase"
	PhaseSchema   Phase = "schema"
	PhaseData     Phase = "data"
	PhaseFinalize Phase = "finalize"
	PhaseDone     Phase = "done"
)

// ActiveTable is a table whose data is being dumped or restored right now.
// Done is only estimated in streaming mode; parallel mode reports 0.
type ActiveTable struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Done  int64  `json:"done"`
}

// Progress is a snapshot of a running copy. Tables and bytes count the current
// stage (the dump in PhaseDump, the restore afterwards). Byte counts are
// estimates based on table sizes from the source catalog.
type Progress struct {
	Phase         Phase         `json:"phase"`
	Parallel      bool          `json:"parallel"`
	TablesDone    int           `json:"tablesDone"`
	TablesTotal   int           `json:"tablesTotal"`
	BytesDone     int64         `json:"bytesDone"`
	BytesTotal    int64         `json:"bytesTotal"`
	Streamed      int64         `json:"streamed"`
	Active        []ActiveTable `json:"active"`
	SchemaObjects int           `json:"schemaObjects"`
	ObjectsDone   int           `json:"objectsDone"`
	ObjectsTotal  int           `json:"objectsTotal"`
	Errors        int           `json:"errors"`
}

// Reporter receives progress from a copy. All methods may be called from
// several goroutines.
type Reporter interface {
	Plan(pg.Plan)
	Progress(Progress)
	TableDone(name string)
}

var (
	creatingRe    = regexp.MustCompile(`^pg_restore: creating (.+?) "(.*)"`)
	restoreDataRe = regexp.MustCompile(`^pg_restore: processing data for table "(.*)"`)
	dumpDataRe    = regexp.MustCompile(`^pg_dump: dumping contents of table "(.*)"`)
	// Parallel workers report completion by tag, which is the bare table name.
	finishedDataRe = regexp.MustCompile(`^pg_(?:dump|restore): finished item \d+ TABLE DATA (.+)$`)
)

// Kinds pg_dump places in the post-data section.
var postDataKinds = map[string]bool{
	"INDEX": true, "INDEX ATTACH": true, "CONSTRAINT": true, "FK CONSTRAINT": true,
	"CHECK CONSTRAINT": true, "TRIGGER": true, "RULE": true, "POLICY": true,
	"EVENT TRIGGER": true, "MATERIALIZED VIEW DATA": true, "PUBLICATION TABLE": true,
	"STATISTICS": true,
}

type activeTable struct {
	name  string
	bytes int64
	start int64 // streamed count when the table started
}

// tracker turns pg_dump / pg_restore verbose output into Progress snapshots.
type tracker struct {
	rep      Reporter
	parallel bool
	streamed atomic.Int64

	mu     sync.Mutex
	p      Progress
	sizes  map[string]int64
	active []activeTable
}

func newTracker(rep Reporter, plan pg.Plan, parallel bool) *tracker {
	t := &tracker{rep: rep, parallel: parallel, sizes: map[string]int64{}}
	for _, tb := range plan.Tables {
		t.sizes[tb.Name] = tb.Bytes
		t.p.BytesTotal += tb.Bytes
	}
	t.p.TablesTotal = len(plan.Tables)
	t.p.ObjectsTotal = plan.PostData
	t.p.Parallel = parallel
	t.p.Phase = PhaseConnect
	return t
}

// stage starts a new phase, resetting the per-stage table counters.
func (t *tracker) stage(ph Phase) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.p.Phase = ph
	t.p.TablesDone = 0
	t.p.BytesDone = 0
	t.active = nil
}

// Write counts bytes flowing from pg_dump into pg_restore (streaming mode).
func (t *tracker) Write(b []byte) (int, error) {
	t.streamed.Add(int64(len(b)))
	return len(b), nil
}

func (t *tracker) dumpLine(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if m := dumpDataRe.FindStringSubmatch(line); m != nil {
		t.start(m[1])
	} else if m := finishedDataRe.FindStringSubmatch(line); m != nil {
		t.finish(m[1], false)
	} else if strings.Contains(line, "error:") {
		t.p.Errors++
	}
}

func (t *tracker) restoreLine(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if m := restoreDataRe.FindStringSubmatch(line); m != nil {
		if !t.parallel {
			// Streaming restores one table at a time: the next one starting
			// means the previous one is done.
			t.finishAll()
		}
		t.p.Phase = PhaseData
		t.start(m[1])
	} else if m := finishedDataRe.FindStringSubmatch(line); m != nil {
		t.finish(m[1], true)
		t.maybeFinalize()
	} else if m := creatingRe.FindStringSubmatch(line); m != nil {
		if postDataKinds[m[1]] || t.p.Phase == PhaseData || t.p.Phase == PhaseFinalize {
			t.p.ObjectsDone++
			if !t.parallel {
				t.finishAll()
			}
			t.maybeFinalize()
		} else {
			t.p.SchemaObjects++
		}
	} else if strings.Contains(line, "error:") && !isTransactionTimeoutError(line) {
		t.p.Errors++
	}
}

// maybeFinalize enters the post-data phase once no table data is pending.
// Parallel restores interleave index builds with data, so wait for all tables.
// Caller holds mu.
func (t *tracker) maybeFinalize() {
	if t.p.ObjectsDone == 0 {
		return
	}
	if !t.parallel || (len(t.active) == 0 && t.p.TablesDone >= t.p.TablesTotal) {
		t.p.Phase = PhaseFinalize
	}
}

// Caller holds mu.
func (t *tracker) start(name string) {
	t.active = append(t.active, activeTable{name: name, bytes: t.sizes[name], start: t.streamed.Load()})
}

// finish marks the active table matching name (qualified, or a bare tag) done.
// Caller holds mu.
func (t *tracker) finish(name string, restored bool) {
	for i, a := range t.active {
		if a.name == name || strings.HasSuffix(a.name, "."+name) {
			t.active = append(t.active[:i], t.active[i+1:]...)
			t.complete(a, restored)
			return
		}
	}
}

// Caller holds mu.
func (t *tracker) finishAll() {
	for _, a := range t.active {
		t.complete(a, true)
	}
	t.active = nil
}

// Caller holds mu.
func (t *tracker) complete(a activeTable, restored bool) {
	t.p.TablesDone++
	t.p.BytesDone += a.bytes
	if restored {
		t.rep.TableDone(a.name)
	}
}

func (t *tracker) snapshot() Progress {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.p
	p.Streamed = t.streamed.Load()
	p.Active = make([]ActiveTable, len(t.active))
	for i, a := range t.active {
		p.Active[i] = ActiveTable{Name: a.name, Bytes: a.bytes}
		if !t.parallel {
			// Count the table's streamed share, but never claim it is done.
			p.Active[i].Done = min(p.Streamed-a.start, a.bytes*99/100)
			p.BytesDone += p.Active[i].Done
		}
	}
	return p
}

func (t *tracker) report() { t.rep.Progress(t.snapshot()) }

// tick reports progress periodically until the returned stop is called.
func (t *tracker) tick(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		tk := time.NewTicker(250 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				t.report()
			case <-ctx.Done():
				return
			}
		}
	}()
	return cancel
}

func (t *tracker) done() {
	t.mu.Lock()
	t.finishAll()
	t.p.Phase = PhaseDone
	t.mu.Unlock()
	t.report()
}
