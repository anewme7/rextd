package main

import (
	"cmp"
	"errors"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

type boardEntry struct {
	task            *task
	pane            paneIdentity
	suggest         bool
	more            bool
	offer           bool
	candidate       bool
	secondary       bool
	overviewSection bool
	sectionTab      taskTab
}

func (m *model) onHomeBoard() bool {
	return m.focusPreview && m.page == listPage && !m.attention && !m.editing
}

func (m *model) onHomeAttention() bool {
	return m.focusPreview && m.page == listPage && m.attention && !m.editing
}

func (m *model) taskByID(id int64) *task {
	for _, t := range m.tasks {
		if t.id == id {
			return t
		}
	}
	return nil
}

func (m *model) boardSessions(t *task) []rex.Pane {
	var panes []rex.Pane
	for _, saved := range t.panes {
		p := m.currentPane(saved)
		if p.App == "" {
			continue
		}
		if m.feed.HostState(p.Host.ID) == rex.Offline {
			p.Status, p.Message = "offline", ""
		}
		panes = append(panes, p)
	}
	return panes
}

type taskBoardStatus struct {
	total, waiting, idle int
	running              bool
}

func (m *model) boardStatus(t *task) taskBoardStatus {
	panes := m.boardSessions(t)
	status := taskBoardStatus{total: len(panes)}
	for _, p := range panes {
		switch p.Status {
		case "working":
			status.running = true
		case "idle":
			status.idle++
		case "permission", "question", "auth", "waiting", "error":
			status.waiting++
		}
	}
	return status
}

func (m *model) homeSelection() boardEntry {
	if m.homeOverview && m.overviewSectionSelected {
		return boardEntry{overviewSection: true, sectionTab: m.overviewSectionTab}
	}
	if m.secondarySelected {
		return boardEntry{secondary: true}
	}
	if len(m.newAgents) > 0 && m.boardOfferSelected {
		return boardEntry{offer: true}
	}
	if m.boardSuggest {
		if p, available := m.suggestionPane(); available {
			return boardEntry{pane: identityOf(p), suggest: true}
		}
	}
	t := m.selected()
	if t != nil && (m.boardMore || m.boardPane != (paneIdentity{})) {
		panes, extra := m.homeSessions(t)
		if m.boardMore && extra > 0 {
			return boardEntry{task: t, more: true}
		}
		for _, p := range panes {
			if identityOf(p) == m.boardPane {
				return boardEntry{task: t, pane: m.boardPane}
			}
		}
	}
	return boardEntry{task: t}
}

func (m *model) selectHome(entry boardEntry) {
	if entry.task != nil {
		// Do not expand a parent simply by walking the board.
		m.taskSelection = slices.Index(m.tasks, entry.task)
	}
	m.boardSuggest, m.boardMore, m.boardPane = entry.suggest, entry.more, entry.pane
	m.boardOfferSelected = entry.offer
	m.secondarySelected = entry.secondary
	m.overviewSectionSelected = entry.overviewSection
	m.overviewSectionTab = entry.sectionTab
	m.boardSuggestionPane = paneIdentity{}
	if entry.suggest {
		m.boardPane = paneIdentity{}
		m.boardSuggestionPane = entry.pane
	}
	m.candidatesFocused, m.peek, m.focus = entry.candidate, false, 0
	if entry.candidate {
		m.candidatePane = paneIdentity{}
	}
}

func (m *model) homeEntries() []boardEntry {
	var rows []boardEntry
	if m.homeOverview {
		hasOffers := len(m.newAgents) > 0
		urgent := m.unassignedUrgency()
		for _, tab := range [...]taskTab{nowTab, todoTab, doneTab, archiveTab} {
			rows = append(rows, boardEntry{overviewSection: true, sectionTab: tab})
			if m.overviewFolded[tab] {
				continue
			}
			if tab == nowTab && hasOffers && urgent > 0 {
				rows = append(rows, boardEntry{offer: true})
			}
			for _, idx := range m.overviewTasksFor(tab) {
				rows = append(rows, boardEntry{task: m.tasks[idx]})
			}
			if tab == nowTab {
				if len(m.candidates) > 0 {
					rows = append(rows, boardEntry{candidate: true})
				}
				if hasOffers && urgent == 0 {
					rows = append(rows, boardEntry{offer: true})
				}
			}
		}
		current := m.homeSelection()
		if hasOffers && (current.offer || current.suggest) {
			for _, p := range m.homeUnassignedPanes() {
				rows = append(rows, boardEntry{pane: identityOf(p), suggest: true})
			}
			return rows
		}
		if t := m.sessionPanelTask(); t != nil {
			panes, extra := m.homeSessions(t)
			for _, p := range panes {
				rows = append(rows, boardEntry{task: t, pane: identityOf(p)})
			}
			if extra > 0 {
				rows = append(rows, boardEntry{task: t, more: true})
			}
		}
		return rows
	}
	hasOffers := m.homeTab == nowTab && len(m.newAgents) > 0
	urgent := m.unassignedUrgency()
	if hasOffers && urgent > 0 {
		rows = append(rows, boardEntry{offer: true})
	}
	for _, group := range m.taskGroups() {
		if m.isSecondary(group) {
			rows = append(rows, boardEntry{secondary: true})
			if !m.secondaryOpen[m.homeTab] {
				continue
			}
		}
		for _, i := range group.rows {
			rows = append(rows, boardEntry{task: m.tasks[i]})
		}
	}
	if m.homeTab == nowTab && len(m.candidates) > 0 {
		rows = append(rows, boardEntry{candidate: true})
	}
	if hasOffers && urgent == 0 {
		rows = append(rows, boardEntry{offer: true})
	}
	current := m.homeSelection()
	if hasOffers && (current.offer || current.suggest) {
		for _, p := range m.homeUnassignedPanes() {
			rows = append(rows, boardEntry{pane: identityOf(p), suggest: true})
		}
		return rows
	}
	if t := m.sessionPanelTask(); t != nil {
		panes, extra := m.homeSessions(t)
		for _, p := range panes {
			rows = append(rows, boardEntry{task: t, pane: identityOf(p)})
		}
		if extra > 0 {
			rows = append(rows, boardEntry{task: t, more: true})
		}
	}
	return rows
}

func (m *model) moveHome(delta int) {
	rows := m.homeEntries()
	if len(rows) > 0 {
		current := m.homeSelection()
		if m.onCandidates() {
			current = boardEntry{candidate: true}
		}
		if delta < 0 && current.suggest {
			panes := m.homeUnassignedPanes()
			if len(panes) > 0 && current.pane == identityOf(panes[0]) {
				m.selectHome(boardEntry{offer: true})
				return
			}
		}
		if delta < 0 && current.task != nil && (current.pane != (paneIdentity{}) || current.more) {
			panes, _ := m.homeSessions(current.task)
			if (len(panes) > 0 && current.pane == identityOf(panes[0])) || (len(panes) == 0 && current.more) {
				m.selectHome(boardEntry{task: current.task})
				return
			}
		}
		next := rows[clamp(slices.Index(rows, current)+delta, 0, len(rows)-1)]
		m.selectHome(next)
		if next.candidate && delta < 0 && m.candidatesOpen {
			m.candidatePane = identityOf(m.candidates[len(m.candidates)-1].pane)
		}
	}
}

func (m *model) homePaneCommand(key string) (tea.Cmd, bool) {
	entry := m.homeSelection()
	if entry.overviewSection {
		switch key {
		case "enter", " ":
			m.overviewFolded[entry.sectionTab] = !m.overviewFolded[entry.sectionTab]
			return nil, true
		case "right":
			m.overviewFolded[entry.sectionTab] = false
			if rows := m.overviewTasksFor(entry.sectionTab); len(rows) > 0 {
				m.selectHome(boardEntry{task: m.tasks[rows[0]]})
			} else if entry.sectionTab == nowTab && len(m.newAgents) > 0 {
				m.selectHome(boardEntry{offer: true})
			}
			return nil, true
		case "left":
			m.overviewFolded[entry.sectionTab] = true
			return nil, true
		case "up", "k", "down", "j", "n", "?", "/", "+", "A", "f", "v", "esc":
			return nil, false
		default:
			return nil, true
		}
	}
	if entry.secondary {
		switch key {
		case "enter", "right", "left":
			open := key == "right" || (key == "enter" && !m.secondaryOpen[m.homeTab])
			m.secondaryOpen[m.homeTab] = open
			if !open {
				m.selectVisibleTask()
			}
			return nil, true
		case "up", "k", "down", "j", "n", "?", "/", "+", "A", "f", "v", "esc":
			return nil, false
		default:
			return nil, true
		}
	}
	if key == "right" && entry.task != nil && entry.pane == (paneIdentity{}) && !entry.more {
		panes, _ := m.homeSessions(entry.task)
		if len(panes) > 0 {
			m.selectHome(boardEntry{task: entry.task, pane: identityOf(panes[0])})
			return nil, true
		}
	}
	if entry.offer {
		switch key {
		case "enter", "right":
			panes := m.homeUnassignedPanes()
			if len(panes) > 0 {
				m.selectHome(boardEntry{pane: identityOf(panes[0]), suggest: true})
			}
			return nil, true
		case "left", "esc":
			return nil, true
		case "N", "s", "e", "m", "x", "p", "t", "b", "o":
			return nil, true
		}
	}
	if entry.more {
		switch key {
		case "enter":
			if m.boardOpenIdle[entry.task] {
				delete(m.boardOpenIdle, entry.task)
			} else {
				if m.boardOpenIdle == nil {
					m.boardOpenIdle = make(map[*task]bool)
				}
				m.boardOpenIdle[entry.task] = true
				// Start on the first newly visible pane, keeping the rest below it.
				panes, extra := m.homeSessions(entry.task)
				m.selectHome(boardEntry{task: entry.task, pane: identityOf(panes[len(panes)-extra])})
			}
			return nil, true
		case "left", "esc":
			m.selectHome(boardEntry{task: entry.task})
			return nil, true
		case "N", "s", "e", "m", "x", "p", "t", "b", "right", "o":
			return nil, true
		}
	}
	if entry.suggest {
		p, _ := m.suggestionPane()
		move := paneMove{pane: p}
		switch key {
		case "enter", "m":
			m.openPaneMove(move)
			return nil, true
		case " ":
			return m.openTerminalPreview(p), true
		case "right", "o":
			m.showPane(p)
			return m.readPaneAgent("", false), true
		case "left", "esc":
			m.selectHome(boardEntry{offer: true})
			return nil, true
		case "N", "s", "e", "x", "p", "t", "b":
			return nil, true
		}
	}
	if entry.task == nil || entry.pane == (paneIdentity{}) {
		return nil, false
	}
	panes, _ := m.homeSessions(entry.task)
	for _, p := range panes {
		if identityOf(p) != entry.pane {
			continue
		}
		switch key {
		case "enter":
			if !m.demo {
				return m.jump(p), true
			}
			m.showPane(p)
			return nil, true
		case "m":
			m.openPaneMove(paneMove{pane: p, from: entry.task.id})
			return nil, true
		case " ":
			return m.openTerminalPreview(p), true
		case "right", "o":
			m.showPane(p)
			return m.readPaneAgent("", false), true
		case "left", "esc":
			m.selectHome(boardEntry{task: entry.task})
			return nil, true
		}
	}
	return nil, false
}

func (m *model) unassignedUrgency() int {
	count := 0
	for _, p := range m.newAgents {
		if homeStatus(p.Status) <= homeFailed {
			count++
		}
	}
	return count
}

func (m *model) homeUnassignedPanes() []rex.Pane {
	panes := slices.Clone(m.newAgents)
	slices.SortStableFunc(panes, func(a, b rex.Pane) int {
		return cmp.Compare(homeStatus(a.Status), homeStatus(b.Status))
	})
	return panes
}

func (m *model) homePanes(t *task) []rex.Pane {
	panes := make([]rex.Pane, 0, len(t.panes))
	for _, saved := range t.panes {
		p := m.currentPane(saved)
		if m.feed.HostState(p.Host.ID) == rex.Offline {
			p.Status = "offline"
		}
		panes = append(panes, p)
	}
	return panes
}

func (m *model) homeSessions(t *task) ([]rex.Pane, int) {
	panes := m.homePanes(t)
	slices.SortStableFunc(panes, func(a, b rex.Pane) int {
		return cmp.Compare(homeStatus(a.Status), homeStatus(b.Status))
	})
	idle := 0
	for _, p := range panes {
		if homeStatus(p.Status) == homeQuiet {
			idle++
		}
	}
	extra := max(0, idle-3)
	if !m.boardOpenIdle[t] {
		panes = panes[:len(panes)-extra]
	}
	return panes, extra
}

type paneMove struct {
	pane rex.Pane
	from int64
}

func (m *model) touchTask(t *task) {
	m.taskTouched[t.id] = time.Now().UnixMilli()
	m.autolinkDirty = true
}

func (m *model) suggestionPane() (rex.Pane, bool) {
	for _, p := range m.newAgents {
		if m.boardSuggestionPane == (paneIdentity{}) || identityOf(p) == m.boardSuggestionPane {
			return p, true
		}
	}
	return rex.Pane{}, false
}

func (m *model) openPaneMove(move paneMove) {
	m.startTaskPick(nil)
	m.taskPick.paneMove = &move
	if move.from == 0 {
		if preferred := m.taskByID(m.suggestedTask); preferred != nil && preferred.state == working {
			if i := slices.Index(m.pickTasks(), preferred); i >= 0 {
				m.taskPick.selection = i
			}
		}
	}
}

func (m *model) finishPaneMove(move paneMove, destination *task) {
	from := m.taskByID(move.from)
	id := identityOf(move.pane)
	if (move.from != 0 && from == nil) || (destination == nil && from == nil) {
		m.notice = m.text(errPaneLinkChanged.Error())
		return
	}
	var target int64
	if destination != nil {
		target = destination.id
	}
	binding := paneBinding{source: linkManual, linkedAt: time.Now().UnixMilli()}
	if m.store != nil {
		var err error
		if from != nil {
			err = m.store.movePaneBinding(from.id, target, move.pane, binding)
		} else {
			var added bool
			added, err = m.store.bindNewPane(target, move.pane, binding, false)
			if err == nil && !added {
				err = errPaneLinkChanged
			}
		}
		if err == errPaneLinkChanged {
			m.notice = m.text(err.Error())
			return
		}
		if !m.saved(err) {
			return
		}
	}
	if from != nil {
		from.panes = slices.DeleteFunc(from.panes, func(p rex.Pane) bool { return identityOf(p) == id })
		delete(from.bindings, id)
	}
	m.page, m.taskPick, m.boardPane, m.boardSuggestionPane, m.boardSuggest = listPage, nil, paneIdentity{}, paneIdentity{}, false
	if destination == nil {
		m.removedAgents[id] = true
		m.notice = m.text("已移出任务")
		m.touchTask(from)
	} else {
		if !slices.ContainsFunc(destination.panes, func(p rex.Pane) bool { return identityOf(p) == id }) {
			destination.addBinding(move.pane, binding)
		}
		delete(m.removedAgents, id)
		m.selectTask(destination)
		m.boardPane = id
		m.notice = m.text("已关联 · ") + m.taskText(destination, destination.title)
		m.touchTask(destination)
	}
	m.refreshCandidates()
}

type taskTabResult struct {
	task int64
	pane rex.Pane
	err  error
}

func (m *model) openTaskTab(t *task) tea.Cmd {
	switch {
	case t == nil || m.openingTaskTab != 0:
		return nil
	case m.archivedTask(t):
		m.notice = m.text("请先还原任务")
		return nil
	case !m.rexContext.Inside():
		m.notice = m.text("需在 Rex 中启动")
		return nil
	}
	label := []rune(m.taskText(t, t.title))
	if len(label) > 128 {
		label = label[:128]
	}
	title, ctx, session, taskID := string(label), m.ctx, m.rexContext.SessionID, t.id
	m.openingTaskTab = taskID
	m.notice = m.text("正在开 Rex 标签…")
	return func() tea.Msg {
		host := rex.Host{ID: "local", Label: "本机"}
		input := map[string]any{
			"session_id": session, "window_label": title,
			"layout": map[string]any{"block": map[string]string{
				"flavor": "com.superlogical.terminal.shell", "label": title,
			}},
		}
		var window struct {
			ID     string   `json:"window_id"`
			Blocks []string `json:"block_ids"`
		}
		err := callRexAPI(ctx, host, session, "session.new_window", input, &window)
		if err == nil && (window.ID == "" || len(window.Blocks) != 1 || window.Blocks[0] == "") {
			err = errors.New("Rex 未返回新标签和窗格")
		}
		if err != nil {
			return taskTabResult{task: taskID, err: err}
		}
		return taskTabResult{task: taskID, pane: rex.Pane{
			Host: host, SessionID: session, WindowID: window.ID, ID: window.Blocks[0], Label: title, Window: title,
		}}
	}
}

func (m *model) finishTaskTab(result taskTabResult) {
	if result.err != nil {
		m.notice = m.text("开标签失败 · ") + m.text(result.err.Error())
		return
	}
	t := m.taskByID(result.task)
	if t == nil || m.archivedTask(t) {
		m.notice = m.text(errPaneLinkChanged.Error())
		return
	}
	for _, p := range m.feed.Panes {
		if samePane(p, result.pane) {
			result.pane = p
			break
		}
	}
	binding := paneBinding{source: linkRexTD, linkedAt: time.Now().UnixMilli()}
	added, err := m.store.bindNewPane(t.id, result.pane, binding, false)
	if !m.saved(err) {
		return
	}
	if !added {
		m.notice = m.text(errPaneLinkChanged.Error())
		return
	}
	t.addBinding(result.pane, binding)
	m.touchTask(t)
	m.refreshCandidates()
	m.notice = m.text("已开标签 · ") + m.taskText(t, t.title)
}
