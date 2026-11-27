package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func TestNewTaskCanEditInMiddle(t *testing.T) {
	m := openTaskBoard(t, filepath.Join(t.TempDir(), "tasks.sqlite3"))
	boardKeys(m, "+", "删除工作任务列", tea.KeyLeft, tea.KeyLeft, tea.KeyLeft)
	boardShows(t, m, "删除工作▏任务列")
	boardKeys(m, "的", tea.KeyEnd, "表", tea.KeyHome, tea.KeyDelete, "新", tea.KeyEnter)
	tasks, err := m.store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].title != "新除工作的任务列表" {
		t.Fatalf("unexpected saved task: %v", tasks)
	}
}

func TestTaskSearchCaretKeepsTheSelectedResult(t *testing.T) {
	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 72, 25
	m.tasks = []*task{{id: 1, title: "工作一"}, {id: 2, title: "工作二"}}
	boardKeys(m, "/", "工作", tea.KeyDown, tea.KeyLeft)
	boardShows(t, m, "工▏作")
	boardKeys(m, tea.KeyEnter)
	if m.selected().id != 2 || m.page != detailPage {
		t.Fatal("moving the search caret should keep the selected result")
	}
}

func TestOtherTextEditorsMoveTheirCaret(t *testing.T) {
	t.Run("candidate title", func(t *testing.T) {
		m := newModel(false)
		m.focusPreview, m.width, m.height = true, 72, 25
		pane := rex.Pane{ID: "block:candidate"}
		m.candidates = []taskCandidate{{pane: pane, title: "候选标题"}}
		m.candidatesOpen, m.candidatesFocused, m.candidatePane = true, true, identityOf(pane)
		boardKeys(m, tea.KeyEnter, tea.KeyLeft, tea.KeyLeft, "新")
		boardShows(t, m, "候选新▏标题")
		boardKeys(m, tea.KeyEsc)
		if m.editing || m.candidates[0].title != "候选标题" {
			t.Fatal("cancelling should preserve the original candidate")
		}
	})
	t.Run("Codex session ID", func(t *testing.T) {
		m := newModel(false)
		m.focusPreview, m.width, m.height = true, 72, 25
		m.showPane(rex.Pane{App: "codex"})
		m.scene.binding.id = "12ab"
		boardKeys(m, "i", tea.KeyHome, tea.KeyRight, tea.KeyDelete, "3")
		boardShows(t, m, "13▏ab")
		boardKeys(m, tea.KeyEsc)
		if m.scene.editing || m.scene.binding.id != "12ab" {
			t.Fatal("cancelling should preserve the existing session ID")
		}
	})
}

func TestLineInputKeepsGraphemesWholeAndCaretVisible(t *testing.T) {
	input := lineInput{}
	input.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("前e\u0301后"), Paste: true})
	input.update(tea.KeyMsg{Type: tea.KeyLeft})
	input.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if want := "前▏后"; input.view(12) != want {
		t.Fatalf("want %q, got %q", want, input.view(12))
	}
	input.set(strings.Repeat("中", 25))
	input.update(tea.KeyMsg{Type: tea.KeyLeft})
	input.update(tea.KeyMsg{Type: tea.KeyLeft})
	if view := input.view(11); ansi.StringWidth(view) > 11 || !strings.Contains(view, "中▏中") {
		t.Fatalf("narrow view should retain the caret and surrounding text: %q", view)
	}
	input.update(tea.KeyMsg{Type: tea.KeyEnd})
	if view := input.view(10); ansi.StringWidth(view) > 10 || !strings.HasSuffix(view, "▏") {
		t.Fatalf("caret should fit after the last wide character: %q", view)
	}
}
