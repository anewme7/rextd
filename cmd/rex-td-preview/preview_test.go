package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func TestSidebarLabelsOnlyDemoMode(t *testing.T) {
	for _, l := range []language{chinese, english} {
		m := newModel(false)
		m.language = l
		m.Update(tea.WindowSizeMsg{Width: 72, Height: 28})
		view := ansi.Strip(m.View())
		header, _, _ := strings.Cut(view, "\n")
		if strings.Contains(view, "预览") || strings.Contains(header, " Live ") ||
			strings.Contains(header, "演示") || strings.Contains(header, "Demo") {
			t.Errorf("%s: ordinary sidebar has a mode label in %q", l, header)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		if view := ansi.Strip(m.View()); !strings.Contains(view, l.text("rextd · 帮助")) {
			t.Errorf("%s: expected help heading", l)
		}
		m = newModel(true)
		m.language = l
		m.Update(tea.WindowSizeMsg{Width: 72, Height: 28})
		header, _, _ = strings.Cut(ansi.Strip(m.View()), "\n")
		if !strings.Contains(header, l.text("演示")) {
			t.Errorf("%s: demo sidebar is missing the demo label in %q", l, header)
		}
	}
}

func TestHostShortcutFiltersAttentionWithoutInterruptingTaskEdits(t *testing.T) {
	m := newModel(true)
	press := func(key rune) {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{key}})
	}
	press('2')
	if !m.attention || m.hostFilter != "host:build" || len(m.livePanes) != 2 {
		t.Fatalf("expected Build alerts, got host %q and %d panes", m.hostFilter, len(m.livePanes))
	}
	m.editing = true
	press('0')
	if !m.editing || m.hostFilter != "host:build" {
		t.Fatal("host shortcut interrupted a task edit")
	}
	m.editing = false
	press('9')
	if m.notice == "" || m.hostFilter != "host:build" {
		t.Fatal("missing host changed the filter without a notice")
	}
	press('0')
	if m.notice != "" || m.hostFilter != "" || len(m.livePanes) != 6 {
		t.Fatalf("expected all alerts, got host %q and %d panes", m.hostFilter, len(m.livePanes))
	}
}

func TestLanguageShortcutKeepsTaskSelectionAndUserText(t *testing.T) {
	m := newModel(true)
	m.Update(tea.WindowSizeMsg{Width: 78, Height: 32})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	shiftL := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}}
	m.Update(shiftL)
	view := ansi.Strip(m.View())
	for _, want := range []string{"Tasks 11", "Attention 6", "Add CSV export to reports", "Create a sample CSV", "Linked panes", "Hosts", "Usage"} {
		if !strings.Contains(view, want) {
			t.Fatalf("English expanded task view missing %q:\n%s", want, view)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if view = ansi.Strip(m.View()); !strings.Contains(view, "Release preflight") || !strings.Contains(view, "Approval") {
		t.Fatalf("English attention should display sample content and status:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if view = ansi.Strip(m.View()); !strings.Contains(view, "Key events") {
		t.Fatalf("English pane details should show the milestone heading:\n%s", view)
	}
	m.Update(shiftL)
	if view = ansi.Strip(m.View()); !strings.Contains(view, "关键节点") || !strings.Contains(view, "发布前检查") {
		t.Fatalf("switching back should keep the currently open pane:\n%s", view)
	}
	m = newModel(true)
	m.focusPreview, m.grouping, m.attention = true, byAgent, true
	m.Update(tea.WindowSizeMsg{Width: 78, Height: 32})
	m.Update(shiftL)
	if view = ansi.Strip(m.View()); !strings.Contains(view, "Status") || !strings.Contains(view, "Session") || !strings.Contains(view, "Agent") || !strings.Contains(view, "Host") {
		t.Fatalf("English task board should translate the grouping control:\n%s", view)
	}

	m = newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 78, Height: 24})
	m.tasks = []*task{{title: "整理发布清单"}}
	m.Update(shiftL)
	if view = ansi.Strip(m.View()); !strings.Contains(view, "整理发布清单") || strings.Contains(view, "Prepare the release checklist") {
		t.Fatalf("user task names must stay as written:\n%s", view)
	}
}

func TestPaneDetailsShowsReportedTimeAndNextAction(t *testing.T) {
	m := newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 54, Height: 30})
	host := rex.Host{ID: "local", Label: "本机"}
	pane := rex.Pane{
		Host: host, SessionID: "session:one", Session: "开发", ID: "block:one",
		Window: "检查安装", App: "codex", Status: "done", Since: time.Date(2026, 11, 18, 19, 23, 0, 0, time.Local),
	}
	m.Update(rex.Fleet{Hosts: []rex.HostStatus{{Host: host, State: rex.Live}}, Panes: []rex.Pane{pane}})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	view := ansi.Strip(m.View())
	for _, expected := range []string{"检查安装", "关键节点", "11-18 19:23", "本轮结束", "核对结果", "↵ 跳转"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("pane details missing %q:\n%s", expected, view)
		}
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("Enter should use the selected Rex pane even when its detail is open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "▾ 开发") {
		t.Fatalf("Esc should return to the attention list:\n%s", view)
	}
}

func TestDemoKeysAndNarrowView(t *testing.T) {
	m := newModel(true)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	if !strings.Contains(ansi.Strip(m.View()), "2/3") {
		t.Fatal("demo should show subtask progress")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.peek {
		t.Fatal("space should show the selected task's state")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !m.expanded[m.selected()] || m.page != listPage {
		t.Fatal("first right arrow should expand in place")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.page != detailPage || !strings.Contains(ansi.Strip(m.View()), "子任务") {
		t.Fatal("second right arrow should open details")
	}

	for _, width := range []int{25, 40, 64} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		rows := strings.Split(m.View(), "\n")
		if len(rows) != m.height {
			t.Fatalf("want %d rows, got %d", m.height, len(rows))
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > width {
				t.Fatalf("row exceeds %d columns: %q", width, ansi.Strip(row))
			}
		}
	}
}

func TestLivePreviewStartsEmptyAndAcceptsChineseTitle(t *testing.T) {
	m := newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	if !strings.Contains(ansi.Strip(m.View()), "暂无任务") {
		t.Fatal("ordinary preview should start empty")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("中文测试"), Paste: true})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	got := m.selected()
	if got == nil || got.title != "中文测试" || !got.pinned || got.color != 1 {
		t.Fatalf("unexpected new task: %+v", got)
	}
	for i := 0; i < 3; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	}
	if m.density != 1 {
		t.Fatalf("wanted standard view after one cycle, got %s", densityLabels[m.density])
	}
}

func TestFooterShowsMachineAndQuotaValuesInAlignedColumns(t *testing.T) {
	m := newModel(true)
	for _, width := range []int{22, 27, 45, 72} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		view := ansi.Strip(m.View())
		_, footer, ok := strings.Cut(view, "\n 机器 · CPU")
		if !ok {
			t.Fatalf("%d columns: machine heading missing:\n%s", width, view)
		}
		cpuColumn := -1
		for _, fields := range [][3]string{{"1 本机", "23%", "2"}, {"2 Build", "87%", "2"}, {"3 QA", "3%", "1"}, {"4 Archive", "9%", "1"}} {
			found := false
			for _, line := range strings.Split(footer, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), fields[0]) {
					continue
				}
				found = true
				column := strings.Index(line, fields[1]+" · "+fields[2])
				if column < 0 {
					t.Fatalf("%d columns: missing CPU and reminder count in %q", width, line)
				}
				end := ansi.StringWidth(line[:column]) + len(fields[1])
				if cpuColumn != -1 && end != cpuColumn {
					t.Errorf("%d columns: CPU readings are not aligned: %q", width, line)
				}
				cpuColumn = end
				break
			}
			if !found {
				t.Fatalf("%d columns: machine %q missing:\n%s", width, fields[0], footer)
			}
		}
		if width < 45 {
			m.handle("f")
			m.handle("f")
			footer = ansi.Strip(m.View())
		}
		for _, value := range []string{"额度 5h/wk", "Claude Max", "Codex Pro", "1h20", "4h10", "61% /  34%", "12% / 100%"} {
			if !strings.Contains(footer, value) {
				t.Errorf("%d columns: quota rows are missing %q:\n%s", width, value, footer)
			}
		}
		m.handle("esc")
	}
}

func TestFooterShowsReportedQuotaAndCountsRemindersByHost(t *testing.T) {
	m := newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 47, Height: 30})
	home := rex.Host{ID: "local", Label: "本机"}
	archive := rex.Host{ID: "archive", Label: "Archive"}
	m.Update(rex.Fleet{
		Hosts: []rex.HostStatus{{Host: home, State: rex.Live, SessionCount: 2, WindowCount: 5}, {Host: archive, State: rex.Live}},
		Panes: []rex.Pane{
			{Host: home, Status: "done"},
			{Host: home, Status: "question"},
			{Host: home, Status: "idle"},
			{Host: archive, Status: "working"},
		},
	})
	zero := 0
	m.Update(cpuUpdate{host: home.ID, value: &zero})
	m.Update(codexUpdate{reading: quotaReading{Windows: [2]quotaWindow{
		{Present: true, Minutes: 10080, Used: 99},
	}}})
	view := ansi.Strip(m.View())
	for _, want := range []string{"0% · 2", "额度 5h/wk", "— /  99%"} {
		if !strings.Contains(view, want) {
			t.Fatalf("overview missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "2会话") {
		t.Fatalf("overview should leave session and pane counts in details:\n%s", view)
	}
	for y, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "2 Archive") && !strings.Contains(line, "—") {
			t.Fatalf("a missing CPU sample should be a dash: %q", line)
		}
		if strings.Contains(line, "机器 · CPU") {
			m.Update(tea.MouseMsg{X: 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		}
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "0% 2会话 5窗") {
		t.Fatalf("clicking Machines should reveal the exact reading and counts:\n%s", view)
	}
	m.handle("esc")

	m.Update(codexUpdate{reading: quotaReading{Windows: [2]quotaWindow{
		{Present: true, Minutes: 60, Used: 25},
	}}})
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "额度 · 已用") || !strings.Contains(view, "1h 25%") || strings.Contains(view, "5h/wk") {
		t.Fatalf("nonstandard quota windows should display their actual period:\n%s", view)
	}
}

func TestFooterShowsClaudeWeeklyLimitFromLocalRex(t *testing.T) {
	m := newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	now := time.Now().UTC().Truncate(time.Minute)
	limit := rex.Pane{
		Host: rex.Host{ID: "local"}, App: "claude-code", Status: "error",
		ReportedAt: now.Add(-time.Hour),
		Message:    "You've hit your weekly limit · resets " + now.Add(time.Hour).Format("Jan 2 at 3:04pm") + " (UTC)",
	}
	fleet := rex.Fleet{Panes: []rex.Pane{limit}}
	m.Update(fleet)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "— / 100%") {
		t.Fatalf("a current local Claude rejection should show 100%% weekly use:\n%s", view)
	}
	m.Update(claudeUpdate{reading: quotaReading{
		Received: now,
		Windows:  [2]quotaWindow{{}, {Minutes: quotaWeekMinutes, Used: 45, Present: true}},
	}})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "— /  45%") {
		t.Fatalf("a newer weekly reading from Claude should take precedence:\n%s", view)
	}
	m.Update(claudeUpdate{})
	for _, ignored := range []rex.Pane{
		{Host: rex.Host{ID: "remote"}, App: limit.App, Status: limit.Status, Message: limit.Message, ReportedAt: limit.ReportedAt},
		{Host: limit.Host, App: "codex", Status: limit.Status, Message: limit.Message, ReportedAt: limit.ReportedAt},
		{Host: limit.Host, App: limit.App, Status: limit.Status, Message: "You've hit your weekly limit · resets " + now.Add(-time.Hour).Format("Jan 2 at 3:04pm") + " (UTC)", ReportedAt: now.Add(-2 * time.Hour)},
	} {
		fleet.Panes = []rex.Pane{ignored}
		m.Update(fleet)
		if view := ansi.Strip(m.View()); strings.Contains(view, "100%") {
			t.Fatalf("an unrelated or expired limit should never appear in Claude's quota:\n%s", view)
		}
	}
}

func TestDemoFooterPlacesSummaryAboveResourcesAndSpacesShortcutsEvenly(t *testing.T) {
	m := newModel(true)
	for _, width := range []int{22, 27, 45} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		view := ansi.Strip(m.View())
		summary := strings.Index(view, "2 个窗格")
		machines := strings.Index(view, "机器")
		if summary < 0 || machines < 0 || summary > machines {
			t.Errorf("%d columns: the task summary should be above the machines", width)
		}
		lines := strings.Split(view, "\n")
		if lines[len(lines)-2] != strings.Repeat("─", width) {
			t.Errorf("%d columns: missing divider above the shortcuts", width)
		}
		footer := lines[len(lines)-1]
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		attentionLines := strings.Split(ansi.Strip(m.View()), "\n")
		if got := attentionLines[len(attentionLines)-1]; got != footer {
			t.Errorf("%d columns: the two tab footers differ: %q and %q", width, footer, got)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		for i, label := range []string{"详情", "跳转", "状态", "置顶"} {
			from := 1 + i*(width-2)/4
			to := 1 + (i+1)*(width-2)/4
			slot := ansi.Cut(footer, from, to)
			if !strings.HasSuffix(strings.TrimSpace(slot), label) {
				t.Errorf("%d columns: shortcut %q is outside its quarter: %q", width, label, slot)
			}
			left := len(slot) - len(strings.TrimLeft(slot, " "))
			right := len(slot) - len(strings.TrimRight(slot, " "))
			if left-right < -1 || left-right > 1 {
				t.Errorf("%d columns: shortcut %q is not centered in %q", width, label, slot)
			}
		}
	}
}

func TestLogRetainsOnlyThreeFiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "preview.log")
	writer := &rotatingLog{path: file, limit: 4}
	for _, record := range []string{"one\n", "two\n", "tri\n", "end\n"} {
		if _, err := writer.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"end\n", "tri\n", "two\n"} {
		path := file + []string{"", ".1", ".2"}[i]
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != want {
			t.Fatalf("%s: got %q, err %v, want %q", path, raw, err, want)
		}
	}
	if _, err := os.Stat(file + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected fourth file: %v", err)
	}
}

func TestLiveLinkKeepsHostAndPaneIdentity(t *testing.T) {
	m := newModel(false)
	m.Update(tea.WindowSizeMsg{Width: 44, Height: 30})
	m.tasks = []*task{{title: "测试任务"}}
	home := rex.Host{ID: "local", Label: "本机"}
	remote := rex.Host{ID: "host:build", Label: "Build", Server: "build"}
	fromHome := rex.Pane{Host: home, SessionID: "session:duplicate", Session: "A home session", Window: "Agent", ID: "block:duplicate"}
	fromRemote := fromHome
	fromRemote.Host, fromRemote.Session = remote, "B remote session"
	m.Update(rex.Fleet{
		Hosts: []rex.HostStatus{{Host: home, State: rex.Live}, {Host: remote, State: rex.Live}},
		Panes: []rex.Pane{fromHome, fromRemote},
	})
	for _, key := range []string{"f", "down", "enter", "a", "l"} {
		m.handle(key)
	}
	if len(m.livePanes) != 1 || len(m.selected().panes) != 1 || m.selected().panes[0].Host.ID != remote.ID {
		t.Fatalf("the link should identify the chosen remote pane: %+v", m.selected().panes)
	}
	m.handle("g")
	m.handle("up")
	m.handle("l")
	if len(m.selected().panes) != 2 {
		t.Fatal("the same block ID on another host must be a distinct link")
	}
}

func TestPaneTitleUsesRealNamesBeforeRexGeneratedLabel(t *testing.T) {
	id := "window:00000000-0000-4000-8000-0000beef1234"
	pane := rex.Pane{WindowID: id, Window: "Window beef1234", Label: "shell",
		App: "codex", TerminalTitle: "Review project notes"}
	if got := paneTitle(pane); got != "Review project notes" {
		t.Errorf("generated label hid the terminal title: %q", got)
	}
	pane.AgentTitle = "Check project notes"
	if got := paneTitle(pane); got != pane.AgentTitle {
		t.Errorf("structured title not used: %q", got)
	}
	pane.Window = "Project review"
	if got := paneTitle(pane); got != pane.Window {
		t.Errorf("custom Rex label did not win: %q", got)
	}
	pane.Window = "Window beef1234"
	pane.AgentTitle, pane.TerminalTitle, pane.App = "", "zsh", "claude-code"
	if got := paneTitle(pane); got != "Claude Code" {
		t.Errorf("missing agent name: %q", got)
	}
}

func TestExpandedTaskLabelsAndSelectsLinkedPanes(t *testing.T) {
	m := newModel(false)
	host := rex.Host{ID: "host:build", Label: "Build", Server: "build"}
	m.tasks = []*task{{title: "Project follow-up", panes: []rex.Pane{
		{ID: "block:a", Host: host, SessionID: "session:one", Session: "Planning", Window: "Review notes"},
		{ID: "block:b", Host: host, SessionID: "session:two", Session: "Execution", Window: "Build checks"},
	}}}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "已关联窗格") || strings.Contains(view, "子任务") {
		t.Fatalf("a task with links must distinguish those links from subtasks:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "› ↗ Build checks") || !strings.Contains(view, "Build · Execution") {
		t.Fatalf("detail navigation must identify the second linked pane and its session:\n%s", view)
	}
}

func TestAttentionHidesIdlePanesAndChangesGroupingOnBothTabs(t *testing.T) {
	m := newModel(false)
	home := rex.Host{ID: "local", Label: "本机"}
	waiting := rex.Pane{Host: home, ID: "block:wait", SessionID: "session:one", Session: "Ship review",
		Window: "Approve packages", App: "claude-code", Status: "permission"}
	idle := rex.Pane{Host: home, ID: "block:idle", SessionID: "session:one", Session: "Ship review",
		Window: "Idle console", App: "codex", Status: "idle"}
	m.tasks = []*task{{title: "Shipping", panes: []rex.Pane{waiting}}}
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	m.Update(rex.Fleet{Hosts: []rex.HostStatus{{Host: home, State: rex.Live}}, Panes: []rex.Pane{waiting, idle}})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Ship review") || !strings.Contains(view, "Approve packages") || strings.Contains(view, "Idle console") {
		t.Fatalf("attention should show only panes that need action, grouped by session:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "★ Approve packages") {
		t.Fatalf("a pinned reminder should appear in the common pinned section:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Claude Code") || !strings.Contains(view, "Idle console") || !strings.Contains(view, "Codex") {
		t.Fatalf("all panes should be available by agent on demand:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Claude Code") || !strings.Contains(view, "Shipping") {
		t.Fatalf("tasks should follow the same grouping:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "按主机") || !strings.Contains(view, "本机") {
		t.Fatalf("the third grouping should use the pane's host:\n%s", view)
	}
}

func TestHostViewKeepsSessionPanesTogether(t *testing.T) {
	m := newModel(false)
	home := rex.Host{ID: "local", Label: "本机"}
	remote := rex.Host{ID: "remote", Label: "Archive"}
	review := rex.Pane{Host: home, SessionID: "session:one", Session: "评审", ID: "block:one", Window: "审查代码", Status: "done"}
	build := rex.Pane{Host: home, SessionID: "session:two", Session: "开发", ID: "block:two", Window: "运行测试", Status: "done"}
	secondReview := review
	secondReview.ID, secondReview.Window = "block:three", "检查发布"
	remoteReview := review
	remoteReview.Host, remoteReview.Window = remote, "远程审查"
	m.tasks = []*task{{title: "发布预检", panes: []rex.Pane{review, build}}}
	m.Update(tea.WindowSizeMsg{Width: 68, Height: 40})
	m.Update(rex.Fleet{
		Hosts:     []rex.HostStatus{{Host: home, State: rex.Live}, {Host: remote, State: rex.Live}},
		Panes:     []rex.Pane{review, build, secondReview, remoteReview},
		LocalName: "workstation",
	})
	for _, key := range []string{"tab", "v", "v"} {
		m.handle(key)
	}
	view := ansi.Strip(m.View())
	remainder := view
	for _, label := range []string{"▾ workstation·本机", "▾ 开发", "运行测试", "▾ 评审", "审查代码", "检查发布", "▾ Archive", "▾ 评审", "远程审查"} {
		_, after, ok := strings.Cut(remainder, label)
		if !ok {
			t.Fatalf("missing %q in host → session → pane order:\n%s", label, view)
		}
		remainder = after
	}
	m.handle("tab")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "▾ workstation·本机") || !strings.Contains(view, "▾ 跨会话") || strings.Count(view, "发布预检") != 1 {
		t.Fatalf("a task across sessions should appear once under its host:\n%s", view)
	}
}

func TestMouseSwitchesTabsAndFoldsHostAndSession(t *testing.T) {
	m := newModel(true)
	m.Update(tea.WindowSizeMsg{Width: 83, Height: 65})
	click := func(label string) {
		t.Helper()
		for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if x := strings.Index(line, label); x >= 0 {
				x = ansi.StringWidth(line[:x])
				m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
				return
			}
		}
		t.Fatalf("cannot find %q to click", label)
	}

	click("需要处理 6")
	click("按会话")
	click("按Agent")
	click("▾ 备份巡检")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "▸ 备份巡检") || strings.Contains(view, "备份目录权限") || !strings.Contains(view, "查日志轮换") {
		t.Fatalf("clicking a session should fold only that session:\n%s", view)
	}
	click("▸ 备份巡检")
	click("▾ Archive")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "▸ Archive") || strings.Contains(view, "备份巡检") {
		t.Fatalf("clicking a host should fold its sessions:\n%s", view)
	}
	m.handle("enter")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "备份目录权限") {
		t.Fatalf("Enter should reopen the host selected by the mouse:\n%s", view)
	}
	click("全部窗格")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "整理发布清单") {
		t.Fatalf("clicking the scope control should show idle panes:\n%s", view)
	}
	click("任务 11")
	click("▾ ★ 全局置顶")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "▸ ★ 全局置顶") || strings.Contains(view, "给报表加 CSV 导出") {
		t.Fatalf("task groups should be clickable as well:\n%s", view)
	}
}
