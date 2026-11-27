package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"rex-td/rex"
)

func TestHomeControlsAndViewportWidths(t *testing.T) {
	for _, lang := range []language{chinese, english} {
		var widths []int
		for w := 28; w <= 55; w++ {
			widths = append(widths, w)
		}
		for _, width := range append(widths, 56, 80, 103, 104, 120) {
			t.Run(fmt.Sprintf("%s-%d", lang, width), func(t *testing.T) {
				m := newModel(false)
				m.focusPreview, m.language, m.width, m.height = true, lang, width, 32
				host := rex.Host{ID: "local"}
				p := rex.Pane{Host: host, ID: "test", SessionID: "session", Session: strings.Repeat("session", 12), Label: "Write", Window: strings.Repeat("window", 12), App: "codex", Status: "question"}
				m.feed.Hosts = []rex.HostStatus{{Host: host, State: rex.Live}}
				m.feed.Panes = []rex.Pane{p}
				m.tasks = []*task{{id: 1, title: "Task", state: working, panes: []rex.Pane{p}}}
				m.switchHomeTab(nowTab)
				screen := func() []string {
					lines := strings.Split(ansi.Strip(m.View()), "\n")
					if len(lines) != m.height {
						t.Fatalf("wrong row count %d", len(lines))
					}
					for _, line := range lines {
						if ansi.StringWidth(line) > width {
							t.Errorf("overwide row %q", line)
						}
						if lang == english && strings.ContainsFunc(line, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
							t.Errorf("untranslated interface %q", line)
						}
					}
					return lines
				}
				header := screen()[0]
				for i := range homeTabNames {
					label := m.tabLabel(i)
					start := strings.Index(header, label)
					if start < 0 {
						t.Errorf("missing tab %q in %q", label, header)
						continue
					}
					startX := ansi.StringWidth(header[:start])
					m.focusMouse(tea.MouseMsg{X: startX, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
					if int(m.homeTab) != i {
						t.Errorf("click on %s selected %d", label, m.homeTab)
					}
				}
				m.switchHomeTab(nowTab)
				boardKeys(m, "v")
				screen()
				boardKeys(m, "f")
				found := false
				for _, row := range screen() {
					if strings.Contains(row, m.text("机器")) && strings.Contains(row, m.text("额度")) && !strings.Contains(row, "f ") {
						found = true
						if strings.Contains(row, "…") {
							t.Errorf("drawer control was clipped: %q", row)
						}
					}
				}
				if !found {
					t.Error("missing drawer controls")
				}
				boardKeys(m, "f")
				screen()
				boardKeys(m, tea.KeyEsc, tea.KeyEnter)
				screen()
				boardKeys(m, tea.KeyEsc, "n")
				for i := 0; i < 4; i++ {
					screen()
					boardKeys(m, "v")
				}
			})
		}
	}
}

func TestResourceDrawerPointerAndHeight(t *testing.T) {
	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 34, 16
	local := rex.Host{ID: "local"}
	remote := rex.Host{ID: "remote", Label: "Remote"}
	m.feed.Hosts = []rex.HostStatus{{Host: local, State: rex.Live}, {Host: remote, State: rex.Live}}
	click := func(label string) {
		t.Helper()
		for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if i := strings.Index(line, label); i >= 0 {
				m.focusMouse(tea.MouseMsg{X: ansi.StringWidth(line[:i]), Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
				return
			}
		}
		t.Fatalf("missing %q in %s", label, m.View())
	}
	click("f 机器")
	click("额度 ·")
	if m.focus != 2 {
		t.Fatal("tapping Usage did not select the usage drawer")
	}
	click("机器 · CPU")
	if m.focus != 1 {
		t.Fatal("tapping Hosts did not select the hosts drawer")
	}
	click("2 Remote")
	click("2 Remote")
	if !m.attention || m.hostFilter != remote.ID {
		t.Fatal("tapping the chosen host did not filter Alerts")
	}
	boardKeys(m, "f")
	m.height = 13
	if !strings.Contains(ansi.Strip(m.View()), "机器 · CPU") {
		t.Fatal("short pane hid the focused drawer")
	}
}

func TestBoardHasClickableAlertsWithoutReminders(t *testing.T) {
	for _, lang := range []language{english, chinese} {
		for _, width := range []int{28, 55, 56, 80, 104} {
			m := newModel(false)
			m.focusPreview, m.language, m.width, m.height = true, lang, width, 24
			label := "n " + m.text("待处理")
			for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
				if i := strings.Index(line, label); i >= 0 {
					m.focusMouse(tea.MouseMsg{X: ansi.StringWidth(line[:i]), Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					break
				}
			}
			if !m.attention {
				t.Errorf("%s %d: quiet Alerts button unavailable", lang, width)
			}
		}
	}
}

func TestNarrowDetailsShowFullTaskTitle(t *testing.T) {
	for _, width := range []int{28, 35, 55} {
		m := newModel(false)
		m.focusPreview, m.language, m.width, m.height = true, chinese, width, 32
		title := strings.Repeat("核对窗格", 9) + "Release"
		m.tasks = []*task{{id: 1, title: title, state: working}}
		m.switchHomeTab(nowTab)
		boardKeys(m, tea.KeyEnter)
		var text string
		for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d: overflowing detail row %q", width, line)
			}
			text += strings.TrimSpace(line)
		}
		if !strings.Contains(text, title) {
			t.Errorf("width %d: missing full title in detail view: %s", width, text)
		}
	}
}

func TestWheelPansBodyAndTapSelectsVisibleRow(t *testing.T) {
	m := newModel(false)
	m.focusPreview, m.language, m.width, m.height = true, chinese, 35, 20
	for i := 1; i <= 35; i++ {
		m.tasks = append(m.tasks, &task{id: int64(i), title: fmt.Sprintf("Task %02d", i), state: working})
	}
	m.switchHomeTab(nowTab)
	view := m.View()
	first := m.selected()
	wheel := func(button tea.MouseButton) {
		m.focusMouse(tea.MouseMsg{X: 5, Y: m.mouseBodyStart + 1, Action: tea.MouseActionPress, Button: button})
		m.View()
	}
	for i := 0; i < 50; i++ {
		wheel(tea.MouseButtonWheelDown)
	}
	if m.selected() != first || m.homeScrollTop != m.homeScrollMax || m.homeScrollTop <= 0 || m.View() == view {
		t.Fatal("swiping up should pan to later rows without changing the selected task")
	}
	wheel(tea.MouseButtonWheelUp)
	if m.homeScrollTop != m.homeScrollMax-1 {
		t.Fatal("swiping down should reveal earlier rows immediately")
	}
	for i, hit := range m.mouseRows {
		if hit.task != nil && hit.task != first {
			m.focusMouse(tea.MouseMsg{X: 8, Y: m.mouseBodyStart + i, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if m.selected() != hit.task {
				t.Fatal("tap after scrolling selected a different task")
			}
			return
		}
	}
	t.Fatal("expected a visible task after scrolling")
}
