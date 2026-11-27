package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

func TestTaskMenuDeletesOnlyConfirmedTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "要删除的主任务", tea.KeyEnter)
	parent := m.selected().id
	boardKeys(m, "m", "+", "要保留的子任务", tea.KeyEnter)
	child := m.selected().id
	boardKeys(m, "m", "h")
	boardKeys(m, "m", "D")
	boardShows(t, m, "永久删除")
	boardKeys(m, tea.KeyEnter, tea.KeyEsc)
	if len(m.tasks) != 2 {
		t.Fatal("Enter or Esc deleted a task")
	}
	boardKeys(m, "D", "y")
	boardShows(t, m, "任务已删除")

	reopened := openTaskBoard(t, path)
	if len(reopened.tasks) != 1 || reopened.tasks[0].id != child || reopened.tasks[0].parentID != 0 {
		t.Fatalf("delete did not preserve the child as a top-level task: %+v", reopened.tasks)
	}
	boardKeys(reopened, "m", "D", "y", "+", "新任务", tea.KeyEnter)
	if task := reopened.selected(); task == nil || task.id <= max(parent, child) {
		t.Fatalf("new task reused a deleted ID: %+v", task)
	}
}

func TestTaskDetailsUnlinksOnlyTheSelectedUnavailablePane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "任务", tea.KeyEnter)
	host := rex.Host{ID: "local"}
	retired := rex.Host{ID: "host:retired", Label: "已移除主机", Server: "Retired"}
	gone := rex.Pane{Host: retired, SessionID: "session:old", ID: "block:gone", Label: "失联窗格"}
	live := rex.Pane{Host: host, SessionID: "session:here", ID: "block:live", Label: "保留窗格"}
	m.link(m.selected(), gone)
	m.link(m.selected(), live)
	m.feed = rex.Fleet{Hosts: []rex.HostStatus{{Host: host, State: rex.Live}}, Panes: []rex.Pane{live}}
	boardKeys(m, tea.KeyEnter)
	boardShows(t, m, "解除关联")
	boardKeys(m, "u")
	reopened := openTaskBoard(t, path)
	if task := reopened.selected(); task == nil || len(task.panes) != 1 || !samePane(task.panes[0], live) {
		t.Fatalf("unlink should keep the task and its other pane: %+v", task)
	}
}

func TestTaskDeleteRollsBackIfTheConfirmedTitleChanged(t *testing.T) {
	m := openTaskBoard(t, filepath.Join(t.TempDir(), "tasks.sqlite3"))
	boardKeys(m, "+", "原任务标题", tea.KeyEnter)
	parent := m.selected().id
	boardKeys(m, "m", "+", "子任务", tea.KeyEnter)
	child := m.selected().id
	for _, err := range []error{
		m.store.pin(parent, true),
		m.store.color(parent, "blue"),
		m.store.setKind(parent, repair),
		m.store.exec("INSERT INTO result(task,body) VALUES (?,?)", parent, "任务结果"),
		m.store.exec("UPDATE task SET title=? WHERE id=?", "新任务标题", parent),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := m.store.deleteTask(taskDeletion{id: parent, title: "原任务标题"}); err == nil {
		t.Fatal("deletion should reject an outdated confirmation")
	}
	tasks, err := m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].title != "新任务标题" || !tasks[0].pinned ||
		tasks[0].color == 0 || tasks[0].kind != repair || tasks[0].result != "任务结果" ||
		tasks[1].id != child || tasks[1].parentID != parent {
		t.Fatalf("stale confirmation partially changed the task: %+v", tasks)
	}
	if err := m.store.deleteTask(taskDeletion{id: parent, title: "新任务标题"}); err != nil {
		t.Fatal(err)
	}
	var resultCount int
	if err := m.store.db.QueryRow("SELECT count(*) FROM result WHERE task=?", parent).Scan(&resultCount); err != nil || resultCount != 0 {
		t.Fatalf("deleted task left %d result records, query error = %v", resultCount, err)
	}
}
