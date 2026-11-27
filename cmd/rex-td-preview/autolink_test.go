package main

import (
	"path/filepath"
	"testing"
	"time"

	"rex-td/rex"
)

func TestAutolinkRules(t *testing.T) {
	pane := func(id, tab, host string) rex.Pane {
		return rex.Pane{ID: id, WindowID: tab, SessionID: "session:work", Host: rex.Host{ID: host}, App: "codex"}
	}
	a := pane("block:a", "window:a", "local")
	b := pane("block:b", "window:b", "local")
	seed := pane("block:seed", "window:seed", "local")
	seed.App = ""
	taskA, taskB := &task{id: 1, state: working}, &task{id: 2, state: working}
	taskA.addBinding(a, paneBinding{source: linkManual})
	taskA.addBinding(seed, paneBinding{source: linkRexTD})
	taskB.addBinding(b, paneBinding{source: linkManual})
	live := map[paneIdentity]rex.Pane{identityOf(a): a, identityOf(b): b, identityOf(seed): seed}
	dirs := map[paneIdentity]string{identityOf(a): "/projects/a", identityOf(b): "/projects/b"}
	touched := map[int64]int64{2: 42}
	both := buildLinkHints([]*task{taskA, taskB}, live, dirs, touched)
	only := buildLinkHints([]*task{taskA}, live, dirs, nil)
	taskB.panes = append(taskB.panes, a)
	dirs[identityOf(b)] = dirs[identityOf(a)]
	ambiguous := buildLinkHints([]*task{taskA, taskB}, live, dirs, touched)

	for _, tt := range []struct {
		name   string
		hints  linkHints
		pane   rex.Pane
		dir    string
		fresh  bool
		task   int64
		source linkSource
	}{
		{"direct link stays as it was opened", both, seed, "", true, 0, ""},
		{"already linked is untouched", both, a, "/projects/b", true, 0, ""},
		{"split inherits from the tab opened with N", both, pane("block:new", seed.WindowID, "local"), "/projects/b", true, 1, linkTab},
		{"tab takes precedence over directory", both, pane("block:new", a.WindowID, "local"), "/projects/b", true, 1, linkTab},
		{"unique directory", both, pane("block:new", "window:new", "local"), "/projects/b", true, 2, linkDir},
		{"same path on another host is distinct", both, pane("block:new", "window:new", "remote"), "/projects/a", true, 2, ""},
		{"only active task", only, pane("block:new", "window:new", "remote"), "", true, 1, linkOnly},
		{"suggest the most recently touched task", both, pane("block:new", "window:new", "local"), "", true, 2, ""},
		{"ambiguous tab and directory require confirmation", ambiguous, pane("block:new", a.WindowID, "local"), "/projects/a", true, 2, ""},
		{"pre-existing Agent only gets a suggestion", only, pane("block:new", a.WindowID, "local"), "/projects/a", false, 1, ""},
		{"plain shell is ignored", both, rex.Pane{ID: "block:shell"}, "", true, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, source := tt.hints.match(tt.pane, tt.dir, tt.fresh)
			if id != tt.task || source != tt.source {
				t.Fatalf("got task %d (%q), want %d (%q)", id, source, tt.task, tt.source)
			}
		})
	}
}

func TestAutolinkDetectsNewPanesAndLateAgentStartup(t *testing.T) {
	created := time.UnixMilli(0x018fed9c1c20)
	pane := rex.Pane{ID: "block:018fed9c-1c20-7000-8000-000000000001"}
	if !newAgentPane(pane, created.Add(-time.Second), false) ||
		newAgentPane(pane, created.Add(time.Second), false) ||
		!newAgentPane(pane, created.Add(time.Second), true) {
		t.Fatal("newness did not account for UUIDv7 and later Agent startup")
	}
	pane.ID = "block:018fed9c-1c20-4000-8000-000000000001"
	if newAgentPane(pane, created.Add(-time.Second), false) || !newAgentPane(pane, created, true) {
		t.Fatal("unsupported IDs should fall back to observing the shell")
	}
}

func TestAutolinkSkipsOnlyHomeRootAndConfiguredDirectories(t *testing.T) {
	const home = "/home/example"
	const workspace = "/home/example/work"
	for _, dir := range []string{"", "~", "/", home, home + "/"} {
		if got := projectDirectory(dir, home, nil); got != "" {
			t.Errorf("%q should not serve as a project directory, got %q", dir, got)
		}
	}
	if got := projectDirectory(workspace, home, nil); got != workspace {
		t.Fatalf("workspace should count by default, got %q", got)
	}
	configured := genericDirectories(workspace + "/:/srv/shared/..//common:relative::~")
	if len(configured) != 2 || !configured[workspace] || !configured["/srv/common"] {
		t.Fatalf("expected two normalized absolute directories, got %v", configured)
	}
	for _, dir := range []string{workspace, "/srv/common/"} {
		if got := projectDirectory(dir, home, configured); got != "" {
			t.Errorf("configured directory %q was retained as %q", dir, got)
		}
	}
	if got := projectDirectory(workspace+"/project/", home, configured); got != workspace+"/project" {
		t.Fatalf("a project under a shared directory should still count, got %q", got)
	}
}

func TestAutolinkBindingPersistsAndNeverOverwrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.sqlite3")
	first, err := openTaskStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.db.Close()
	for _, name := range []string{"A", "B"} {
		id, err := first.addTask(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := first.state(id, working); err != nil {
			t.Fatal(err)
		}
	}
	pane := rex.Pane{ID: "block:new", SessionID: "session:work", Host: rex.Host{ID: "host:remote"}, App: "codex"}
	binding := paneBinding{source: linkDir, linkedAt: 1730000000000}
	if added, err := first.bindNewPane(1, pane, binding, true); err != nil || !added {
		t.Fatalf("initial automatic link = %v, %v", added, err)
	}
	second, err := openTaskStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.db.Close()
	if added, err := second.bindNewPane(2, pane, paneBinding{source: linkOnly}, true); err != nil || added {
		t.Fatalf("already linked pane should be skipped, got %v, %v", added, err)
	}
	tasks, err := second.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks[0].panes) != 1 || tasks[0].bindings[identityOf(pane)] != binding || len(tasks[1].panes) != 0 {
		t.Fatalf("expected the original durable link, got %+v and %+v", tasks[0], tasks[1])
	}
	manual := paneBinding{source: linkManual, linkedAt: binding.linkedAt + 1}
	if err := second.movePaneBinding(1, 99, pane, manual); err != errPaneLinkChanged {
		t.Fatalf("invalid destination should reject the move, got %v", err)
	}
	tasks, err = first.load()
	if err != nil || len(tasks[0].panes) != 1 || tasks[0].bindings[identityOf(pane)] != binding {
		t.Fatalf("the failed move should keep the original binding, got %+v, %v", tasks, err)
	}
	if err := second.movePaneBinding(1, 2, pane, manual); err != nil {
		t.Fatal(err)
	}
	tasks, err = first.load()
	if err != nil || len(tasks[0].panes) != 0 || tasks[1].bindings[identityOf(pane)] != manual {
		t.Fatalf("the explicit move should be visible to another board, got %+v, %v", tasks, err)
	}
}
