package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"rex-td/rex"
)

func TestHomeResizeKeepsStatusAndSelectionOnVisibleRows(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	panes := []rex.Pane{
		{ID: "block:idle1", Label: "idle one", App: "codex", Status: "idle"},
		{ID: "block:idle2", Label: "idle two", App: "codex"},
		{ID: "block:idle3", Label: "idle three", App: "codex"},
		{ID: "block:idle4", Label: "idle four", App: "codex"},
		{ID: "block:done", Label: "turn done", App: "codex", Status: "done"},
		{ID: "block:run", Label: "running", App: "codex", Status: "working"},
		{ID: "block:wait", Label: "approve", App: "claude-code", Status: "permission"},
		{ID: "block:error", Label: "failed", App: "codex", Status: "error"},
	}
	task := &task{id: 1, title: "跨主机查看任务进度和归属", state: working, panes: panes}
	m.tasks = append(m.tasks, task)
	for _, example := range []struct {
		width int
		wait  string
	}{{120, "● 等你批准"}, {60, "● 等你批准"}, {44, "● 等你"}, {32, "●"}, {27, "●"}} {
		t.Run(fmt.Sprint(example.width), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: example.width, Height: 24})
			screen := ansi.Strip(m.View())
			for _, line := range strings.Split(screen, "\n") {
				if ansi.StringWidth(line) > example.width {
					t.Fatalf("row wraps at %d cells: %q", example.width, line)
				}
				if strings.Contains(line, "approve") && !strings.HasSuffix(strings.TrimSpace(line), example.wait) {
					t.Errorf("waiting row does not end in %q: %q", example.wait, line)
				}
			}
			boardOrder(t, m, "approve", "failed", "running", "turn done", "idle one", "idle two", "idle three")
			if strings.Contains(screen, "idle four") {
				t.Fatal("fourth idle row should be collapsed")
			}
		})
	}
	m.selectHome(boardEntry{task: task, more: true})
	m.listCommand("enter")
	if m.homeSelection().pane != identityOf(panes[3]) {
		t.Fatal("expanded idle rows should select the first newly visible pane")
	}
	boardShows(t, m, "idle four")
}

func TestHomePutsUrgentUnassignedPanesBeforeTasks(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	m.width, m.height = 50, 24
	active := &task{id: 1, title: "Current task", state: working}
	idle := rex.Pane{ID: "block:idle", Label: "Quiet session", App: "codex"}
	waiting := rex.Pane{ID: "block:wait", Label: "Question session", App: "claude-code", Status: "question"}
	failed := rex.Pane{ID: "block:error", Label: "Error session", App: "codex", Status: "error"}
	m.tasks, m.newAgents, m.suggestedTask = []*task{active}, []rex.Pane{idle, failed, waiting}, active.id
	boardOrder(t, m, "未归类 · 3", "● 等你", "进行中")
	m.moveHome(-1)
	boardOrder(t, m, "未归类 · 3", "进行中", "Question session", "Error session", "Quiet session")
	m.listCommand("enter")
	m.moveHome(1)
	m.listCommand("enter")
	if m.page != taskPickPage || len(active.panes) != 0 {
		t.Fatal("the first Enter should open the task picker without linking")
	}
	boardKeys(m, tea.KeyEnter)
	if len(active.panes) != 1 || identityOf(active.panes[0]) != identityOf(failed) {
		t.Fatal("confirming the task should link the selected unassigned pane after reordering")
	}
}

func TestHomeAndAttentionShareTheirHeader(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	m.width, m.height = 80, 24
	m.tasks = []*task{{id: 1, title: "Current task", state: working}}
	top := func() []string { return strings.Split(ansi.Strip(m.View()), "\n")[:3] }
	home := top()
	m.listCommand("n")
	attention := top()
	if !strings.Contains(home[0], "1 现在") || !strings.Contains(home[0], "2 待办") ||
		!strings.Contains(home[0], "3 已完成") || !strings.Contains(home[0], "4 归档") ||
		!strings.Contains(home[0], "+ 新建") ||
		home[1] != "" || attention[1] != "" ||
		!strings.Contains(home[2], "/ 搜索") ||
		!strings.Contains(attention[2], "状态") || !strings.Contains(attention[2], "会话") ||
		!strings.Contains(attention[2], "工具") || !strings.Contains(attention[2], "主机") {
		t.Fatalf("unexpected headers: home %q, attention %q", home, attention)
	}
}

func TestHomeFooterFollowsSelectionAndWidth(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	m.width, m.height = 80, 25
	host := rex.Host{ID: "local"}
	m.feed.Hosts = []rex.HostStatus{{Host: host, State: rex.Live}}
	pane := rex.Pane{ID: "block:linked", App: "codex", Status: "working", Message: "Reading files"}
	active := &task{id: 1, title: "Current task", state: working, panes: []rex.Pane{pane}}
	m.tasks = []*task{active}
	footer := func() []string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return rows[len(rows)-2:]
	}
	taskFooter := footer()
	if !strings.HasPrefix(taskFooter[0], "─") || !strings.Contains(taskFooter[1], "Rex 已连接") ||
		!strings.Contains(taskFooter[1], "f 机器与额度") || !strings.Contains(taskFooter[1], "v 视图  ↵ 详情") ||
		!strings.Contains(taskFooter[1], "→ 会话") ||
		!strings.Contains(taskFooter[1], "s 切换状态") || !strings.Contains(taskFooter[1], "n 待处理") {
		t.Fatalf("unexpected task footer: %q", taskFooter)
	}
	for _, tc := range []struct {
		width    int
		tier     homeTier
		expected int
	}{
		{120, homeFull, 5},
		{80, homeMedium, 5},
		{60, homeMedium, 3},
		{44, homeNarrow, 2},
		{28, homeNarrow, 1},
	} {
		m.width = tc.width
		if got := homeWidth(tc.width); got != tc.tier {
			t.Errorf("width %d resolved to tier %d, want %d", tc.width, got, tc.tier)
		}
		if got := len(m.homeShortcuts(tc.tier)); got != tc.expected {
			t.Errorf("width %d (tier %d) has %d shortcuts, want %d", tc.width, tc.tier, got, tc.expected)
		}
	}
	m.width = 80
	m.selectHome(boardEntry{task: active, pane: identityOf(pane)})
	if row := footer()[1]; !strings.Contains(row, "↵ 跳转") || !strings.Contains(row, "m 移动") || !strings.Contains(row, "空格 预览") {
		t.Fatalf("unexpected pane shortcuts: %q", row)
	}
	origin := ansi.Strip(m.View())
	clickX := m.homeFooterHits[0].start + 1
	m.Update(tea.MouseMsg{X: clickX, Y: 24, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	for _, text := range []string{"窗格预览", "窗格暂不可用", "Esc 返回"} {
		boardShows(t, m, text)
	}
	boardKeys(m, "q", "v", tea.KeySpace)
	if got := ansi.Strip(m.View()); got != origin {
		t.Fatalf("Space should restore the original page and selection: %q", got)
	}
	m.newAgents = []rex.Pane{{ID: "block:new", App: "codex"}}
	m.selectHome(boardEntry{offer: true})
	boardShows(t, m, "→ 进入会话")
	boardKeys(m, tea.KeyEnter)
	if row := footer()[1]; !strings.Contains(row, "↵ 归入任务") {
		t.Fatalf("unexpected unassigned shortcuts: %q", row)
	}
	m.Update(tea.MouseMsg{X: 74, Y: 24, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.focus != 1 {
		t.Fatal("the status line should open Hosts")
	}
}

func TestHomeHelpCanScrollToAllShortcutGroups(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	m.width, m.height = 32, 11
	boardKeys(m, "?", "L")
	boardShows(t, m, "rextd · Help")
	boardShows(t, m, "Tasks")
	sawGlobal := false
	for range 80 {
		boardKeys(m, tea.KeyDown)
		sawGlobal = strings.Contains(ansi.Strip(m.View()), "Global") || sawGlobal
	}
	if !sawGlobal {
		t.Fatal("scrolling should reach the Global heading")
	}
	boardShows(t, m, "q Quit")
	boardKeys(m, tea.KeyEsc, "?")
	boardShows(t, m, "rextd · Help")
}

func TestHomeOfferExpandsBeforeAcceptingLink(t *testing.T) {
	m := newModel(false)
	m.focusPreview = true
	m.width, m.height = 70, 24
	active := &task{id: 1, title: "Current task", state: working}
	other := &task{id: 2, title: "Other task", state: todo}
	pane := rex.Pane{ID: "block:unassigned", Label: "New agent", App: "codex"}
	m.tasks, m.newAgents, m.suggestedTask = []*task{active, other}, []rex.Pane{pane}, active.id
	if strings.Contains(m.View(), "建议任务") {
		t.Fatal("unassigned sessions should stay hidden while a task is selected")
	}
	m.moveHome(2)
	if !m.homeSelection().offer {
		t.Fatal("expected the unassigned row to be selected at the end of the upper list")
	}
	boardShows(t, m, "建议任务 · Current task")
	m.listCommand("enter")
	if !m.homeSelection().suggest || len(active.panes) != 0 {
		t.Fatal("entering the unassigned panel should select its first pane without linking")
	}
	boardKeys(m, tea.KeyEnter, tea.KeyEsc)
	if !m.homeSelection().suggest || len(active.panes)+len(other.panes) != 0 {
		t.Fatal("cancelling should return to the same unassigned row without linking")
	}
	boardKeys(m, tea.KeyEnter, "Other", tea.KeyEnter)
	if len(active.panes) != 0 || len(other.panes) != 1 || identityOf(other.panes[0]) != identityOf(pane) {
		t.Fatal("confirming the picker should link to the chosen task instead of the suggestion")
	}
}

func TestWorkTabsCoverTasksAndKeepSessionFocus(t *testing.T) {
	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 80, 17
	one := rex.Pane{ID: "block:one", Label: "Ask about invoice", Status: "question", App: "codex"}
	two := rex.Pane{ID: "block:two", Label: "Quiet terminal", Status: "idle"}
	m.tasks = []*task{{id: 1, title: "Current work", state: working, panes: []rex.Pane{one}}, {id: 2, title: "Other work", state: working, panes: []rex.Pane{two}}, {id: 3, title: "Queued task", state: todo}, {id: 4, title: "Paused task", state: paused}, {id: 5, title: "Old result", state: done}, {id: 6, title: "New result", state: done, completionOrder: 1}, {id: 7, title: "Cancelled task", state: cancelled}, {id: 8, title: "Archived task", state: done, archived: true}}
	m.switchHomeTab(nowTab)
	visible := func(want, absent string) {
		t.Helper()
		screen := ansi.Strip(m.View())
		if !strings.Contains(screen, want) || strings.Contains(screen, absent) {
			t.Fatalf("wanted %q without %q in %s", want, absent, screen)
		}
	}
	visible(one.Label, two.Label)
	boardKeys(m, tea.KeyDown)
	visible(two.Label, one.Label)
	boardKeys(m, tea.KeyRight, tea.KeySpace)
	if m.terminalPreview == nil {
		t.Fatal("session shortcut did not open preview")
	}
	boardKeys(m, tea.KeyEsc)
	if m.homeSelection().pane != identityOf(two) {
		t.Fatal("lost original session")
	}
	boardKeys(m, tea.KeyTab)
	visible("Queued task", "Paused task")
	boardKeys(m, tea.KeyDown, tea.KeyEnter, tea.KeyDown)
	if m.selected() != m.tasks[3] {
		t.Fatal("paused task not reachable")
	}
	m.Update(tea.MouseMsg{X: 23, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	boardOrder(t, m, "New result", "Old result")
	visible("已取消", "Archived task")
	boardKeys(m, "1")
	visible("Current work", "Queued task")
}

func TestCompletionOrderSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	for _, title := range []string{"First", "Second"} {
		boardKeys(m, "+", title, tea.KeyEnter, "x", tea.KeyEsc)
	}
	m = openTaskBoard(t, path)
	boardKeys(m, "3")
	boardOrder(t, m, "Second", "First")
	boardKeys(m, tea.KeyDown, "x", "x")
	boardOrder(t, m, "First", "Second")
}

func TestHomeSelectionAndPlainRowsInBothThemes(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile); lipgloss.SetHasDarkBackground(dark) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	for _, tc := range []struct {
		dark   bool
		bg     string
		waitFg string
	}{
		{false, "48;2;233;237;245", "38;2;179;83;9"},
		{true, "48;2;36;39;48", "38;2;232;185;89"},
	} {
		lipgloss.SetHasDarkBackground(tc.dark)
		m := newModel(false)
		m.focusPreview, m.width, m.height = true, 140, 23
		donePane := rex.Pane{ID: "linked", Label: "Plain session", App: "codex", Status: "done"}
		m.feed.Panes = []rex.Pane{donePane}
		m.tasks = []*task{{title: "Selected task", state: working, panes: []rex.Pane{donePane}}}
		screen := m.View()
		rows := strings.Split(screen, "\n")
		if strings.Contains(ansi.Strip(rows[1]), "─") {
			t.Fatalf("dark=%v top header should not render a horizontal divider under tabs: %q", tc.dark, rows[1])
		}
		if strings.Contains(rows[0], ";4m") || strings.Contains(rows[0], "[4m") || strings.Contains(rows[0], "[1;4;") {
			t.Fatalf("dark=%v active tab should stay quiet without underline: %q", tc.dark, rows[0])
		}
		foundSelected := false
		foundSession := false
		foundAlert := false
		for _, line := range rows {
			plain := ansi.Strip(line)
			if strings.Contains(plain, "Selected task") && strings.Contains(plain, "▸") {
				foundSelected = true
				if !strings.Contains(line, tc.bg) {
					t.Fatalf("dark=%v incorrect selection background: %q", tc.dark, line)
				}
				if gotW := ansi.StringWidth(plain); gotW != m.width-1 {
					t.Fatalf("dark=%v selection bar did not span full width: got %d, want %d", tc.dark, gotW, m.width-1)
				}
				if !strings.Contains(line, tc.waitFg) {
					t.Fatalf("dark=%v selected task lost amber status accent: %q", tc.dark, line)
				}
			}
			if strings.Contains(plain, "Plain session") {
				foundSession = true
				if strings.Contains(line, "48;") {
					t.Fatalf("dark=%v background on unselected session: %q", tc.dark, line)
				}
				if strings.Contains(line, ";34m") || strings.Contains(line, "[34m") {
					t.Fatalf("dark=%v unselected session still uses ANSI blue: %q", tc.dark, line)
				}
			}
			if strings.Contains(plain, "1 待看") {
				foundAlert = true
				if !strings.Contains(line, tc.waitFg) {
					t.Fatalf("dark=%v alert summary missing amber accent: %q", tc.dark, line)
				}
			}
		}
		if !foundSelected || !foundSession || !foundAlert {
			t.Fatalf("dark=%v missing expected rows (selected=%v session=%v alert=%v)", tc.dark, foundSelected, foundSession, foundAlert)
		}
	}
}

func TestHomeFullPageAlignsTaskAndSessionStatusesAndSeparatesUnassigned(t *testing.T) {
	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 160, 26
	p1 := rex.Pane{ID: "p1", Label: "负责人-新机安装", App: "codex", Status: "done"}
	p2 := rex.Pane{ID: "p2", Label: "管家-bussiness", App: "codex", Status: "idle"}
	pRun := rex.Pane{ID: "p3", Label: "loop", App: "codex", Status: "working"}
	m.feed.Panes = []rex.Pane{p1, p2, pRun}
	m.tasks = []*task{
		{id: 1, title: "business，多店铺", state: working, panes: []rex.Pane{p1, p2}},
		{id: 2, title: "补全构建步骤以及使用说明", state: working, panes: []rex.Pane{pRun}},
	}
	m.newAgents = []rex.Pane{{ID: "u1", Label: "unassigned", App: "codex"}}
	screen := ansi.Strip(m.View())
	lines := strings.Split(screen, "\n")

	taskDotCol, sessionDotCol := -1, -1
	unassignedRow, dividerRow, sessionHeaderRow := -1, -1, -1
	for i, line := range lines {
		if strings.Contains(line, "business，多店铺") && strings.Contains(line, "▸") {
			if idx := strings.Index(line, "● 本轮完成"); idx >= 0 {
				taskDotCol = ansi.StringWidth(line[:idx])
			}
		}
		if strings.Contains(line, "负责人-新机安装") {
			if idx := strings.Index(line, "● 本轮完成"); idx >= 0 {
				sessionDotCol = ansi.StringWidth(line[:idx])
			}
		}
		if strings.Contains(line, "未归类") && strings.Contains(line, "1 个会话") {
			unassignedRow = i
		}
		if i > 2 && strings.HasPrefix(line, "────") && dividerRow < 0 {
			dividerRow = i
		}
		if strings.Contains(line, "business，多店铺 · 2 个会话") {
			sessionHeaderRow = i
		}
	}
	if taskDotCol < 0 || sessionDotCol < 0 || taskDotCol != sessionDotCol {
		t.Fatalf("task status col (%d) and session status col (%d) should align:\n%s", taskDotCol, sessionDotCol, screen)
	}
	if unassignedRow < 0 || dividerRow < 0 || sessionHeaderRow < 0 || !(unassignedRow < dividerRow && dividerRow < sessionHeaderRow) {
		t.Fatalf("expected 未归类 (%d) above divider (%d) above session panel (%d):\n%s", unassignedRow, dividerRow, sessionHeaderRow, screen)
	}
}

func TestHomeThemeProbeAndLiveNotifications(t *testing.T) {
	dark, ok, rest := extractOSC11Background([]byte("abc\x1b]11;rgb:0d0d/1111/1717\x1b\\xyz"))
	if !ok || !dark || string(rest) != "abcxyz" {
		t.Fatalf("dark OSC 11 failed: dark=%v ok=%v rest=%q", dark, ok, rest)
	}
	dark, ok, rest = extractOSC11Background([]byte("\x1b]11;rgb:ffff/ffff/ffff\x07"))
	if !ok || dark || len(rest) != 0 {
		t.Fatalf("light OSC 11 failed: dark=%v ok=%v rest=%q", dark, ok, rest)
	}
	cleaned, dark, changed := stripThemeNotifications([]byte("j\x1b[?997;1nk"))
	if !changed || !dark || string(cleaned) != "jk" {
		t.Fatalf("dark notification failed: changed=%v dark=%v cleaned=%q", changed, dark, cleaned)
	}
	cleaned, dark, changed = stripThemeNotifications([]byte("\x1b[?997;2n"))
	if !changed || dark || len(cleaned) != 0 {
		t.Fatalf("light notification failed: changed=%v dark=%v cleaned=%q", changed, dark, cleaned)
	}
}

func TestHomeThreeWidthTiersLayout(t *testing.T) {
	local := rex.Host{ID: "local", Label: "本机"}
	pDone := rex.Pane{Host: local, ID: "p1", Label: "负责人-新机安装", App: "codex", Status: "done"}
	pIdle1 := rex.Pane{Host: local, ID: "p2", Label: "管家-bussiness", App: "codex", Status: "idle"}
	pIdle2 := rex.Pane{Host: local, ID: "p3", Label: "0-全局规划", App: "codex", Status: "idle"}
	pIdle3 := rex.Pane{Host: local, ID: "p4", Label: "负责人-备份与回滚", App: "codex", Status: "idle"}
	pIdle4 := rex.Pane{Host: local, ID: "p5", Label: "额外会话-1", App: "codex", Status: "idle"}
	pIdle5 := rex.Pane{Host: local, ID: "p6", Label: "额外会话-2", App: "codex", Status: "idle"}
	pRun := rex.Pane{Host: local, ID: "p7", Label: "agentic-loop", App: "claude-code", Status: "working"}
	pOff := rex.Pane{Host: local, ID: "p8", Label: "offline-pane", App: "codex", Status: "offline"}

	for _, tc := range []struct {
		name  string
		width int
		tier  homeTier
	}{
		{"narrow", 44, homeNarrow},
		{"medium", 80, homeMedium},
		{"full", 140, homeFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := homeWidth(tc.width); got != tc.tier {
				t.Fatalf("homeWidth(%d) = %d, want %d", tc.width, got, tc.tier)
			}
			m := newModel(false)
			m.focusPreview, m.width, m.height = true, tc.width, 24
			m.feed.Hosts = []rex.HostStatus{{Host: rex.Host{ID: "local"}, State: rex.Live}}
			m.feed.Panes = []rex.Pane{pDone, pIdle1, pIdle2, pIdle3, pIdle4, pIdle5, pRun}
			m.tasks = []*task{
				{id: 1, title: "business，多店铺", state: working, panes: []rex.Pane{pDone, pIdle1, pIdle2, pIdle3, pIdle4, pIdle5}},
				{id: 2, title: "business问题区批量操作和uiux优化", state: working, panes: []rex.Pane{pIdle1}},
				{id: 3, title: "业务工作流分组显示，每日定期更新最新数据", state: working},
				{id: 4, title: "补全构建步骤以及使用说明", state: working, panes: []rex.Pane{pRun}},
				{id: 5, title: "dashboard首页重做", state: working, panes: []rex.Pane{pOff}},
			}
			m.newAgents = []rex.Pane{{ID: "u1", Label: "未归类-1", App: "codex"}, {ID: "u2", Label: "未归类-2", App: "codex"}}

			screen := ansi.Strip(m.View())
			lines := strings.Split(screen, "\n")
			if len(lines) != m.height {
				t.Fatalf("line count %d != height %d", len(lines), m.height)
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w > tc.width {
					t.Fatalf("line %d wraps (%d > %d): %q", i, w, tc.width, line)
				}
			}
			if strings.Contains(screen, "窗格当前不可用") {
				t.Fatalf("home view should use compact 不可用 label instead of 窗格当前不可用:\n%s", screen)
			}
			t.Logf("\n=== %s (%d cols) ===\n%s", tc.name, tc.width, screen)
		})
	}
}

func TestHomeArchiveFourthTabAndSessionsGrouping(t *testing.T) {
	local := rex.Host{ID: "local", Label: "本机"}
	remote := rex.Host{ID: "host:m4", Label: "M4"}
	pWait := rex.Pane{Host: local, SessionID: "s1", Session: "business", ID: "b1", Label: "负责人-新机安装", App: "codex", Status: "permission", Message: "允许运行部署脚本？"}
	pDone := rex.Pane{Host: local, SessionID: "s1", Session: "business", ID: "b2", Label: "build-review", App: "claude-code", Status: "done", Message: "首页改造完成"}
	pRun := rex.Pane{Host: remote, SessionID: "s2", Session: "dev", ID: "b3", Label: "worker-m4", App: "kimi-code", Status: "working"}
	pIdle := rex.Pane{Host: remote, SessionID: "s2", Session: "dev", ID: "b4", Label: "shell-m4", App: "codex", Status: "idle"}

	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 120, 26
	m.feed.Hosts = []rex.HostStatus{{Host: local, State: rex.Live}, {Host: remote, State: rex.Live}}
	m.feed.Panes = []rex.Pane{pWait, pDone, pRun, pIdle}
	m.tasks = []*task{
		{id: 1, title: "business，多店铺", state: working, panes: []rex.Pane{pWait}},
		{id: 2, title: "旧版发布", state: done, archived: true},
	}
	m.switchHomeTab(nowTab)

	// Switch to 4 Archive tab via key "4"
	boardKeys(m, "4")
	if m.homeTab != archiveTab || !m.archivedView || !m.onHomeBoard() {
		t.Fatalf("key 4 should open Archive inside homeView: tab=%v archived=%v", m.homeTab, m.archivedView)
	}
	archiveScreen := ansi.Strip(m.View())
	if !strings.Contains(archiveScreen, "已归档 · 1") || !strings.Contains(archiveScreen, "旧版发布") || !strings.Contains(archiveScreen, "已完成") {
		t.Fatalf("unexpected archive tab view:\n%s", archiveScreen)
	}

	// Open Sessions/Attention via "n" (defaults to byStatus + alerts only)
	boardKeys(m, "n")
	if !m.attention || m.grouping != byStatus || m.allPanes {
		t.Fatalf("n should open Attention grouped by status on alerts: attn=%v group=%v all=%v", m.attention, m.grouping, m.allPanes)
	}
	statusScreen := ansi.Strip(m.View())
	for _, want := range []string{"需回应 · 1", "本轮完成 · 1", "负责人-新机安装", "build-review", "允许运行部署脚本？"} {
		if !strings.Contains(statusScreen, want) {
			t.Fatalf("missing %q in status-grouped attention view:\n%s", want, statusScreen)
		}
	}
	if strings.Contains(statusScreen, "worker-m4") {
		t.Fatalf("working pane should stay hidden in alerts-only scope:\n%s", statusScreen)
	}

	// Switch to all panes + cycle grouping through Session, Agent, Host
	boardKeys(m, "a")
	allStatusScreen := ansi.Strip(m.View())
	for _, want := range []string{"工作中 · 1", "空闲与不可用 · 1", "worker-m4", "shell-m4"} {
		if !strings.Contains(allStatusScreen, want) {
			t.Fatalf("missing %q in all-panes status view:\n%s", want, allStatusScreen)
		}
	}
	boardKeys(m, "v") // bySession
	if m.grouping != bySession {
		t.Fatalf("expected bySession, got %v", m.grouping)
	}
	boardKeys(m, "v") // byAgent
	if m.grouping != byAgent || !strings.Contains(ansi.Strip(m.View()), "Claude Code") {
		t.Fatalf("expected byAgent with Claude Code, got group=%v\n%s", m.grouping, ansi.Strip(m.View()))
	}
	boardKeys(m, "v") // byHost
	if m.grouping != byHost || !strings.Contains(ansi.Strip(m.View()), "M4") {
		t.Fatalf("expected byHost with M4, got group=%v\n%s", m.grouping, ansi.Strip(m.View()))
	}
	t.Logf("\n=== Attention (byStatus, all panes, 120 cols) ===\n%s", allStatusScreen)
}

func TestHomeResourcesDrawerKeepsHomePageAndBilingualConsistency(t *testing.T) {
	local := rex.Host{ID: "local", Label: "本机"}
	pDone := rex.Pane{Host: local, SessionID: "s1", Session: "business", ID: "b1", Label: "负责人-新机安装", App: "codex", Status: "done"}
	pOff := rex.Pane{Host: local, SessionID: "s1", Session: "business", ID: "b2", Label: "管家-bussiness", App: "codex", Status: "idle"}

	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 120, 26
	m.feed.Hosts = []rex.HostStatus{{Host: local, State: rex.Live, SessionCount: 3, WindowCount: 8}}
	m.feed.Panes = []rex.Pane{pDone}
	m.tasks = []*task{{id: 1, title: "business，多店铺", state: working, panes: []rex.Pane{pDone, pOff}}}
	m.switchHomeTab(nowTab)

	// Press 'f' to open Hosts drawer: must stay on Home board (no legacy ☐ list!)
	boardKeys(m, "f")
	if m.focus != 1 || !m.onHomeBoard() {
		t.Fatalf("f should focus Hosts drawer inside Home: focus=%d onHome=%v", m.focus, m.onHomeBoard())
	}
	hostsScreen := ansi.Strip(m.View())
	if strings.Contains(hostsScreen, "☐") || !strings.Contains(hostsScreen, "1 现在") || !strings.Contains(hostsScreen, "机器 · CPU") || !strings.Contains(hostsScreen, "5小时/周") {
		t.Fatalf("pressing f should keep Home board with clean bottom Hosts drawer:\n%s", hostsScreen)
	}

	// Press 'f' again for Usage drawer, then 'Esc' to close drawer
	boardKeys(m, "f")
	if m.focus != 2 {
		t.Fatalf("second f should focus Usage drawer, got %d", m.focus)
	}
	boardKeys(m, tea.KeyEsc)
	if m.focus != 0 {
		t.Fatalf("Esc should close bottom drawer, got focus=%d", m.focus)
	}

	// Open redesigned task detail page
	boardKeys(m, tea.KeyEnter)
	detailScreen := ansi.Strip(m.View())
	for _, want := range []string{"‹ 任务", "m 更多", "○ business，多店铺", "关联会话 · 1", "负责人-新机安装", "● 本轮完成"} {
		if !strings.Contains(detailScreen, want) {
			t.Fatalf("missing %q in redesigned detail view:\n%s", want, detailScreen)
		}
	}
	boardKeys(m, tea.KeyEsc)

	// Switch to English via Shift+L and verify pure English labels
	boardKeys(m, "L")
	enHome := ansi.Strip(m.View())
	for _, want := range []string{"1 Now", "2 To Do", "3 Done", "4 Archive", "s State", "n Alerts"} {
		if !strings.Contains(enHome, want) {
			t.Fatalf("missing English label %q:\n%s", want, enHome)
		}
	}
	for _, leaked := range []string{"现在", "待办", "归档", "切换状态", "待处理"} {
		if strings.Contains(enHome, leaked) {
			t.Fatalf("Chinese label %q leaked into English mode:\n%s", leaked, enHome)
		}
	}
	t.Logf("\n=== Home + Hosts Drawer (120 cols) ===\n%s\n=== Detail View (120 cols) ===\n%s", hostsScreen, detailScreen)
}

func TestHomeOverviewVerticalLayoutAndSwitching(t *testing.T) {
	local := rex.Host{ID: "local", Label: "本机"}
	pDone := rex.Pane{Host: local, SessionID: "s1", Session: "business", ID: "b1", Label: "负责人-新机安装", App: "codex", Status: "done"}
	pUnassigned := rex.Pane{Host: local, SessionID: "s2", Session: "misc", ID: "b2", Label: "build-review", App: "claude-code", Status: "done"}

	m := newModel(false)
	m.focusPreview, m.width, m.height = true, 120, 28
	m.feed.Hosts = []rex.HostStatus{{Host: local, State: rex.Live}}
	m.feed.Panes = []rex.Pane{pDone, pUnassigned}
	m.tasks = []*task{
		{id: 1, title: "business，多店铺", state: working, panes: []rex.Pane{pDone}},
		{id: 2, title: "整理发布清单", state: todo},
		{id: 3, title: "历史完成任务", state: done, completionOrder: 1},
		{id: 4, title: "历史归档任务", state: done, archived: true},
	}
	m.newAgents = []rex.Pane{pUnassigned}
	m.switchHomeTab(nowTab)

	// Switch from 分页 -> 总览 via 'v'
	boardKeys(m, "v")
	if !m.homeOverview {
		t.Fatal("v should switch Home into vertical Overview mode")
	}
	overview := ansi.Strip(m.View())
	boardOrder(t, m, "任务总览", "分页   总览", "▾ 现在 · 1", "business，多店铺", "未归类", "▾ 待办 · 1", "整理发布清单", "▸ 已完成 · 1", "▸ 归档 · 1")
	if strings.Contains(overview, "历史完成任务") || strings.Contains(overview, "历史归档任务") {
		t.Fatalf("done and archive sections should start folded in overview:\n%s", overview)
	}

	// Jump to Done section via key '3' — unfolds Done and selects its task
	boardKeys(m, "3")
	if m.overviewFolded[doneTab] || m.selected() == nil || m.selected().title != "历史完成任务" {
		t.Fatalf("key 3 in overview should unfold and select Done task: folded=%v sel=%v", m.overviewFolded[doneTab], m.selected())
	}

	// Switch back to 分页 via 'v' — follows selected task to 已完成 tab
	boardKeys(m, "v")
	if m.homeOverview || m.homeTab != doneTab {
		t.Fatalf("switching back to Tabs should follow selected Done task: overview=%v tab=%v", m.homeOverview, m.homeTab)
	}

	// Switch to English and verify Tabs / All labels
	boardKeys(m, "L", "v")
	enOverview := ansi.Strip(m.View())
	for _, want := range []string{"All Tasks", "Tabs", "All", "▾ Now · 1", "▾ To Do · 1", "▾ Done · 1", "▸ Archive · 1"} {
		if !strings.Contains(enOverview, want) {
			t.Fatalf("missing English overview label %q:\n%s", want, enOverview)
		}
	}
	t.Logf("\n=== Overview (zh, 120 cols) ===\n%s", overview)
}
