package copier

import (
	"slices"
	"testing"

	"db-copier/internal/pg"
)

type recorder struct {
	nopReporter
	done []string
}

func (r *recorder) TableDone(name string) { r.done = append(r.done, name) }

var testPlan = pg.Plan{Tables: []pg.Table{{Name: "public.a", Bytes: 1000}, {Name: "s2.b", Bytes: 3000}}, PostData: 2}

func TestTrackerStreaming(t *testing.T) {
	rec := &recorder{}
	tr := newTracker(rec, testPlan, false)
	tr.stage(PhaseSchema)
	for _, l := range []string{
		`pg_restore: creating TABLE "public.a"`,
		`pg_restore: creating TABLE "s2.b"`,
		`pg_restore: error: could not execute query: ERROR:  unrecognized configuration parameter "transaction_timeout"`,
		`pg_restore: processing data for table "public.a"`,
	} {
		tr.restoreLine(l)
	}
	p := tr.snapshot()
	if p.Phase != PhaseData || p.SchemaObjects != 2 || len(p.Active) != 1 || p.Active[0].Name != "public.a" || p.Errors != 0 {
		t.Fatalf("after first data line: %+v", p)
	}

	tr.Write(make([]byte, 500))
	tr.restoreLine(`pg_restore: processing data for table "s2.b"`)
	tr.Write(make([]byte, 10000))
	p = tr.snapshot()
	if p.TablesDone != 1 || p.Active[0].Done != 2970 || p.BytesDone != 1000+2970 {
		t.Fatalf("mid data: %+v", p)
	}

	tr.restoreLine(`pg_restore: creating CONSTRAINT "public.a a_pkey"`)
	tr.restoreLine(`pg_restore: error: could not execute query: ERROR:  boom`)
	p = tr.snapshot()
	if p.Phase != PhaseFinalize || p.TablesDone != 2 || p.ObjectsDone != 1 || p.Errors != 1 || p.BytesDone != 4000 {
		t.Fatalf("finalize: %+v", p)
	}

	tr.done()
	if p = tr.snapshot(); p.Phase != PhaseDone {
		t.Fatalf("done: %+v", p)
	}
	if !slices.Equal(rec.done, []string{"public.a", "s2.b"}) {
		t.Fatalf("done tables: %v", rec.done)
	}
}

func TestTrackerParallel(t *testing.T) {
	rec := &recorder{}
	tr := newTracker(rec, testPlan, true)

	tr.stage(PhaseDump)
	tr.dumpLine(`pg_dump: dumping contents of table "s2.b"`)
	tr.dumpLine(`pg_dump: dumping contents of table "public.a"`)
	tr.dumpLine(`pg_dump: finished item 3509 TABLE DATA b`)
	p := tr.snapshot()
	if p.Phase != PhaseDump || p.TablesDone != 1 || p.BytesDone != 3000 || len(p.Active) != 1 || p.Active[0].Name != "public.a" {
		t.Fatalf("dump: %+v", p)
	}
	tr.dumpLine(`pg_dump: finished item 3507 TABLE DATA a`)
	if len(rec.done) != 0 {
		t.Fatalf("dump stage must not report restored tables: %v", rec.done)
	}

	tr.stage(PhaseSchema)
	for _, l := range []string{
		`pg_restore: creating TABLE "public.a"`,
		`pg_restore: launching item 3507 TABLE DATA a`,
		`pg_restore: launching item 3509 TABLE DATA b`,
		`pg_restore: processing data for table "public.a"`,
		`pg_restore: processing data for table "s2.b"`,
		`pg_restore: finished item 3507 TABLE DATA a`,
		`pg_restore: creating INDEX "public.a_v_idx"`,
	} {
		tr.restoreLine(l)
	}
	p = tr.snapshot()
	// An index build for a finished table must not end the data phase while b is still loading.
	if p.Phase != PhaseData || p.TablesDone != 1 || p.ObjectsDone != 1 || len(p.Active) != 1 || p.Active[0].Done != 0 {
		t.Fatalf("parallel data: %+v", p)
	}
	tr.restoreLine(`pg_restore: finished item 3509 TABLE DATA b`)
	if p = tr.snapshot(); p.Phase != PhaseFinalize || p.TablesDone != 2 || p.BytesDone != 4000 {
		t.Fatalf("parallel finalize: %+v", p)
	}
	if !slices.Equal(rec.done, []string{"public.a", "s2.b"}) {
		t.Fatalf("done tables: %v", rec.done)
	}
}
