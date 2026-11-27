package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func TestDetailLinksOnlyTheSelectedIdleUnassignedPane(t *testing.T) {
	m := openTaskBoard(t, filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err := m.store.exec("INSERT INTO task(id,title,state) VALUES(1,'Target','To do'),(2,'Other','Blocked')"); err != nil {
		t.Fatal(err)
	}
	host := rex.Host{ID: "local"}
	idle := rex.Pane{ID: "block:idle", Host: host, SessionID: "session:all", Label: "Available idle", Status: "idle"}
	linked := idle
	linked.ID, linked.Label = "block:linked", "Already linked idle"
	busy := idle
	busy.ID, busy.Label, busy.Status = "block:working", "Working pane", "working"
	if err := m.store.bind(2, linked, false); err != nil {
		t.Fatal(err)
	}
	var err error
	m.tasks, err = m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	m.feed = rex.Fleet{Panes: []rex.Pane{idle, linked, busy}, Hosts: []rex.HostStatus{{Host: host, State: rex.Live}}}
	boardKeys(m, "2")
	boardShows(t, m, "暂缓")
	boardKeys(m, tea.KeyEnter)
	boardShows(t, m, "空闲且未关联")
	boardShows(t, m, "Available idle")
	if screen := ansi.Strip(m.View()); strings.Contains(screen, "Already linked idle") || strings.Contains(screen, "Working pane") {
		t.Fatalf("ineligible panes shown in details:\n%s", screen)
	}
	boardKeys(m, tea.KeyDown)
	m.feed.Panes[0].Status = "working"
	boardKeys(m, tea.KeyEnter)
	boardShows(t, m, "窗格已变化")
	if len(m.selected().panes) != 0 {
		t.Fatal("a pane that started working while selected should not be linked")
	}
	m.feed.Panes[0].Status = "idle"
	boardKeys(m, tea.KeyEnter)
	tasks, err := m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks[0].panes) != 1 || !samePane(tasks[0].panes[0], idle) || tasks[0].state != todo ||
		len(tasks[1].panes) != 1 || !samePane(tasks[1].panes[0], linked) || tasks[1].state != paused {
		t.Fatalf("linking the idle pane changed the wrong task or its state: %v", tasks)
	}
}
