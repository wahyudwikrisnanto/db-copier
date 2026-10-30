package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"db-copier/internal/copier"
	"db-copier/internal/pg"
	"db-copier/internal/store"
)

func TestRecordCopyJob(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := store.Connection{ID: "a", Name: "prod", Database: "app", Password: "secret", SSH: store.SSHConfig{Enabled: true, Host: "bastion"}}
	rec, err := s.Start(Job{Kind: KindCopy, Source: RefOf(src), Copy: &CopyInfo{Options: copier.Options{NoOwner: true}}})
	if err != nil {
		t.Fatal(err)
	}
	rec.Plan(pg.Plan{Tables: []pg.Table{{Name: "public.a", Bytes: 10}, {Name: "public.b", Bytes: 5}}})
	rec.TableDone("public.a")
	rec.Progress(copier.Progress{Phase: copier.PhaseData, Errors: 1})
	rec.Log("line one")
	rec.Log("line two")

	if err := s.Delete(rec.ID()); err == nil {
		t.Fatal("deleting a running job should fail")
	}
	if _, err := rec.Finish(StatusFailed, "boom"); err != nil {
		t.Fatal(err)
	}

	d, err := s.Get(rec.ID())
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusFailed || d.Error != "boom" || d.FinishedAt == nil {
		t.Fatalf("bad job state: %+v", d.Job)
	}
	if d.Copy.TablesTotal != 2 || d.Copy.TablesDone != 1 || d.Copy.BytesTotal != 15 || d.Copy.Errors != 1 || !d.Copy.Options.NoOwner {
		t.Fatalf("bad copy summary: %+v", d.Copy)
	}
	if d.Source.SSHHost != "bastion" {
		t.Fatalf("bad source ref: %+v", d.Source)
	}
	if d.CopyDetail == nil || len(d.CopyDetail.DoneTables) != 1 || d.CopyDetail.Plan == nil {
		t.Fatalf("bad copy detail: %+v", d.CopyDetail)
	}
	if strings.Join(d.Log, "|") != "line one|line two" || d.LogTruncated {
		t.Fatalf("bad log: %q truncated=%v", d.Log, d.LogTruncated)
	}
	b, _ := os.ReadFile(filepath.Join(s.jobDir(rec.ID()), "job.json"))
	if strings.Contains(string(b), "secret") {
		t.Fatal("password leaked into history")
	}

	if err := s.Delete(rec.ID()); err != nil {
		t.Fatal(err)
	}
	if jobs, _ := s.List(); len(jobs) != 0 {
		t.Fatalf("expected no jobs, got %d", len(jobs))
	}
}

func TestOpenMarksInterruptedJobsFailed(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	rec, err := s.Start(Job{Kind: KindCopy})
	if err != nil {
		t.Fatal(err)
	}
	rec.Flush() // simulate the process dying without Finish

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s2.Get(rec.ID())
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusFailed || d.Error != interruptedError || d.FinishedAt == nil {
		t.Fatalf("interrupted job not marked failed: %+v", d.Job)
	}
}

func TestGetRejectsPathEscape(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, id := range []string{"", "..", "../x", "a/b"} {
		if _, err := s.Get(id); err == nil {
			t.Errorf("Get(%q) should fail", id)
		}
	}
}

func TestTailLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.txt")
	var b strings.Builder
	for i := 1; i <= 7; i++ {
		fmt.Fprintf(&b, "%d\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0o600)

	got, truncated, err := tailLines(path, 3)
	if err != nil || !truncated || strings.Join(got, ",") != "5,6,7" {
		t.Fatalf("tail 3: %v %v %v", got, truncated, err)
	}
	got, truncated, _ = tailLines(path, 10)
	if truncated || len(got) != 7 {
		t.Fatalf("tail 10: %v %v", got, truncated)
	}
}
