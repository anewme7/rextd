package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"rex-td/rex"
)

func TestTaskStoreKeepsLegacyDataAndRemoteBindingAfterReopen(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE task(id INTEGER PRIMARY KEY,title TEXT NOT NULL,state TEXT NOT NULL,
note TEXT NOT NULL DEFAULT '',review INTEGER NOT NULL DEFAULT 0);
CREATE TABLE pane_link(task INTEGER,session_id TEXT,block_id TEXT,label TEXT,PRIMARY KEY(task,session_id,block_id));
INSERT INTO task(id,title,state,note) VALUES (12,'旧任务','In progress','保留备注');
INSERT INTO pane_link VALUES (12,'session:old','block:old','旧窗格');`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.addTask("Rex's 跨机任务")
	if err != nil {
		t.Fatal(err)
	}
	target := rex.Pane{
		Host:      rex.Host{ID: "host:stable", Label: "Build", Server: "build"},
		SessionID: "session:new", Session: "Ship review", WindowID: "window:new", ID: "block:new",
		TerminalTitle: "Pass criteria", App: "codex",
	}
	for _, err := range []error{s.pin(id, true), s.color(id, "green"), s.state(id, working), s.setKind(id, repair), s.bind(id, target, false), s.bind(id, target, false)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	tasks, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("want two tasks, got %d", len(tasks))
	}
	current, old := tasks[0], tasks[1]
	if current.title != "Rex's 跨机任务" || current.state != working || !current.pinned || current.color != 2 || current.kind != repair ||
		len(current.panes) != 1 || !samePane(current.panes[0], target) || current.panes[0].WindowID != target.WindowID ||
		current.panes[0].Session != target.Session || current.panes[0].App != target.App {
		t.Fatalf("reopened task did not keep its fields or unique link: %+v", current)
	}
	if old.note != "保留备注" || old.kind != uncategorized || len(old.panes) != 1 || old.panes[0].Host.ID != "local" || old.panes[0].ID != "block:old" {
		t.Fatalf("legacy link or task was lost: %+v", old)
	}
	if old.bindings[identityOf(old.panes[0])] != (paneBinding{source: linkManual}) {
		t.Fatalf("migrated link should default to manual: %+v", old.bindings)
	}
	if binding := current.bindings[identityOf(target)]; binding.source != linkManual || binding.linkedAt == 0 {
		t.Fatalf("new manual link should have a timestamp: %+v", binding)
	}
	if err := s.bind(id, target, true); err != nil {
		t.Fatal(err)
	}
	tasks, err = s.load()
	if err != nil || len(tasks) != 2 || len(tasks[0].panes) != 0 || len(tasks[1].panes) != 1 {
		t.Fatalf("unlink should remove only the selected binding: %+v, %v", tasks, err)
	}
}

func TestFreshSQLiteReusesPagesAfterRepeatedDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	s, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if _, err := s.addTask("Task"); err != nil {
		t.Fatal(err)
	}
	insert := `WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<200)
INSERT INTO rex_pane_binding(task,host_id,host_label,server,session_id,window_id,block_id,title)
SELECT 1,'local','Local','','session:p','window:p','block:' || i, hex(zeroblob(256)) FROM n;`
	var firstSize int64
	for n := 0; n < 3; n++ {
		if err := s.exec(insert); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			firstSize = info.Size()
		}
		if info.Size() > firstSize {
			t.Fatalf("same 200 bindings grew the file each cycle: %d > %d", info.Size(), firstSize)
		}
		if err := s.exec("DELETE FROM rex_pane_binding; PRAGMA incremental_vacuum(8);"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("unexpected WAL sidecar: %v", err)
	}
}
