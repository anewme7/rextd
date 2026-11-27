package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func openTaskBoard(t *testing.T, path string) *model {
	t.Helper()
	m := newModel(false)
	m.focusPreview = true
	store, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.db.Close() })
	m.store = store
	m.tasks, err = store.load()
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 72, Height: 31})
	return m
}

func boardKeys(m *model, keys ...any) {
	for _, key := range keys {
		switch k := key.(type) {
		case string:
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		case tea.KeyType:
			m.Update(tea.KeyMsg{Type: k})
		}
	}
}

func boardShows(t *testing.T, m *model, text string) {
	t.Helper()
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, text) {
		t.Fatalf("expected %q in:\n%s", text, screen)
	}
}

func TestTaskBoardChecksReopensAndRestoresArchivedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "Release", tea.KeyEnter, "s", tea.KeyEsc)
	boardShows(t, m, "进行中")

	boardKeys(m, "m", "s")
	boardShows(t, m, "暂缓")
	boardKeys(m, "m", "s")
	boardShows(t, m, "进行中")

	// Completion still uses x after the home checklist glyph was removed.
	boardKeys(m, "x")
	if m.tasks[0].state != done {
		t.Fatal("x should complete the selected task")
	}
	boardShows(t, m, "完成")
	boardKeys(m, "x")
	boardShows(t, m, "进行中")

	boardKeys(m, "m", "c")
	boardShows(t, m, "已取消")
	boardKeys(m, "m", "a")
	if screen := ansi.Strip(m.View()); strings.Contains(screen, "Release") {
		t.Fatalf("archived task remained active:\n%s", screen)
	}
	if err := m.store.db.Close(); err != nil {
		t.Fatal(err)
	}

	m = openTaskBoard(t, path)
	boardKeys(m, "A")
	boardShows(t, m, "Release")
	boardShows(t, m, "已取消")
	boardKeys(m, "m", "a")
	boardShows(t, m, "Release")
	boardKeys(m, "m", "c")
	boardShows(t, m, "进行中")
}

func boardOrder(t *testing.T, m *model, titles ...string) {
	t.Helper()
	screen := ansi.Strip(m.View())
	last := -1
	for _, title := range titles {
		index := strings.Index(screen, title)
		if index <= last {
			t.Fatalf("expected order %v, missing or misplaced %q:\n%s", titles, title, screen)
		}
		last = index
	}
}

func TestTaskBoardPersistsOrderWithinPinnedAndOrdinaryTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	for _, title := range []string{"Alpha", "Bravo", "Charlie", "Delta"} {
		boardKeys(m, "+", title, tea.KeyEnter, tea.KeyEsc)
	}
	boardKeys(m, "m", "u")
	boardOrder(t, m, "Alpha", "Bravo", "Delta", "Charlie")
	boardKeys(m, "p", tea.KeyDown, "p")
	boardOrder(t, m, "Alpha", "Delta", "Bravo", "Charlie")
	boardKeys(m, "m", "d", "m", "d")
	boardOrder(t, m, "Delta", "Alpha", "Bravo", "Charlie")
	boardKeys(m, tea.KeyDown, "m", "d")
	boardOrder(t, m, "Delta", "Alpha", "Charlie", "Bravo")
	if err := m.store.db.Close(); err != nil {
		t.Fatal(err)
	}
	m = openTaskBoard(t, path)
	boardKeys(m, "2")
	boardOrder(t, m, "Delta", "Alpha", "Charlie", "Bravo")
}

func TestTaskBoardKeepsChildrenIndependentAndSearchable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "Run verification", tea.KeyEnter, tea.KeyEsc)
	boardKeys(m, "+", "Ship release", tea.KeyEnter, tea.KeyEsc)
	boardKeys(m, "m", "n", "Validate installation on the second host", tea.KeyEnter, tea.KeyEsc)
	boardKeys(m, "m", "+", "Build package", tea.KeyEnter)
	boardShows(t, m, "Build package")
	boardKeys(m, "x", tea.KeyEsc, "2", "/", "Ship release", tea.KeyEnter, "m", "l")
	boardShows(t, m, "Run verification")
	boardKeys(m, tea.KeyEnter)
	boardOrder(t, m, "Ship release", "下一步", "Run verification", "Build package")
	boardShows(t, m, "1/2")
	boardShows(t, m, "○ Ship release")
	boardShows(t, m, "✓ Build package")
	if err := m.store.db.Close(); err != nil {
		t.Fatal(err)
	}
	m = openTaskBoard(t, path)
	if got := m.taskCount(); got != 1 {
		t.Fatalf("root task count = %d, want 1", got)
	}
	boardKeys(m, "2")
	boardShows(t, m, "Ship release")
	boardKeys(m, "/", "Run verification", tea.KeyEnter)
	boardShows(t, m, "Run verification")
	boardShows(t, m, "Ship release")
	boardKeys(m, "x", tea.KeyEsc)
	if child := m.selected(); child.state != done || m.parentOf(child).state != todo {
		t.Fatal("completing a child should leave its parent to do")
	}
	boardShows(t, m, "Run verification")
	boardKeys(m, "2")
	boardShows(t, m, "2/2")
	boardShows(t, m, "Ship release")
	boardKeys(m, "/", "Run verification", tea.KeyEnter)
	boardKeys(m, "m", "l")
	if got := m.taskCount(); got != 2 {
		t.Fatalf("root task count = %d after detaching a child, want 2", got)
	}
}

func TestTaskBoardMovesWithinTheVisibleState(t *testing.T) {
	m := openTaskBoard(t, filepath.Join(t.TempDir(), "tasks.sqlite3"))
	for _, title := range []string{"Alpha", "Bravo", "Charlie"} {
		boardKeys(m, "+", title, tea.KeyEnter, tea.KeyEsc)
	}
	boardKeys(m, tea.KeyUp, "s")
	boardShows(t, m, "Bravo")
	boardKeys(m, "2")
	boardOrder(t, m, "Alpha", "Charlie")
	boardKeys(m, tea.KeyDown, "m", "u")
	boardOrder(t, m, "Charlie", "Alpha")
	boardKeys(m, "1")
	boardShows(t, m, "Bravo")
}
