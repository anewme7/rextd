package main

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"rex-td/rex"
)

type taskCandidate struct {
	pane                 rex.Pane
	title, topic, source string
}

func (m *model) identifyCandidate(p rex.Pane) (taskCandidate, bool) {
	if p.ID == "" || p.SessionID == "" || p.Host.ID == "" ||
		(p.App != "codex" && p.App != "claude-code") ||
		p.Status == "" || p.Status == "idle" || p.Status == "auth" ||
		m.feed.HostState(p.Host.ID) == rex.Offline {
		return taskCandidate{}, false
	}
	matches := func(title string) bool {
		return usefulTitle(title, p) && !strings.EqualFold(title, appName(p.App)) &&
			!strings.EqualFold(title, "rextd")
	}
	title, source := p.AgentTitle, "Agent 上报"
	if !matches(title) {
		title, source = p.Label, "Rex 窗格名"
	}
	if !matches(title) {
		if !matches(p.Window) || p.WindowID == "" {
			return taskCandidate{}, false
		}
		for _, other := range m.feed.Panes {
			if other.App != "" && other.Host.ID == p.Host.ID && other.SessionID == p.SessionID &&
				other.WindowID == p.WindowID && other.ID != p.ID {
				return taskCandidate{}, false
			}
		}
		title, source = p.Window, "Rex 窗口名"
	}
	title = strings.TrimSpace(storedDisplay(title))
	topic := strings.ToLower(strings.Join(strings.Fields(title), " "))
	return taskCandidate{p, title, topic, source}, topic != ""
}

func (m *model) linkedPaneID(id paneIdentity) bool {
	for _, task := range m.tasks {
		if slices.ContainsFunc(task.panes, func(p rex.Pane) bool { return identityOf(p) == id }) {
			return true
		}
	}
	return false
}

func (m *model) refreshCandidates() {
	m.candidates = m.candidates[:0]
	if len(m.candidateRequests) == 0 && len(m.candidateWatches) == 0 {
		m.candidatePane = paneIdentity{}
		return
	}
	for _, p := range m.feed.Panes {
		id := identityOf(p)
		topic := m.candidateRequests[id]
		watched := m.candidateWatches[id]
		if !watched && topic == "" {
			continue
		}
		if c, ok := m.identifyCandidate(p); ok && (watched || c.topic == topic) &&
			c.topic != m.candidateIgnored[id] && !m.linkedPaneID(id) {
			m.candidates = append(m.candidates, c)
		}
	}
	if m.candidateIndex() < 0 {
		m.candidatePane = paneIdentity{}
	}
}

func (m *model) toggleCandidateWatch() {
	p := m.scene.pane
	id := identityOf(p)
	value := !m.candidateWatches[id]
	if value && (id.host == "" || id.session == "" || id.block == "" ||
		(p.App != "codex" && p.App != "claude-code")) {
		m.notice = m.text("此窗格暂无可观察的 Agent")
		return
	}
	if m.store != nil && !m.saved(m.store.setCandidateWatch(id, value)) {
		return
	}
	if value {
		m.candidateWatches[id] = true
		m.notice = m.text("正在观察此窗格")
	} else {
		delete(m.candidateWatches, id)
		m.notice = m.text("已停止观察此窗格")
	}
	m.refreshCandidates()
}

func (m *model) discoverCandidate() {
	id := identityOf(m.scene.pane)
	index := slices.IndexFunc(m.feed.Panes, func(p rex.Pane) bool { return identityOf(p) == id })
	if index < 0 {
		m.notice = m.text("候选来源已离线或关闭")
		return
	}
	c, ok := m.identifyCandidate(m.feed.Panes[index])
	switch {
	case m.linkedPaneID(id):
		m.notice = m.text("这个窗格已关联任务")
	case !ok:
		m.notice = m.text("当前窗格没有可发现的主题")
	case c.topic == m.candidateIgnored[id]:
		m.notice = m.text("这个主题已忽略")
	default:
		m.candidateRequests[id] = c.topic
		m.refreshCandidates()
		m.cancelPaneTurn()
		m.page, m.attention, m.focus, m.peek = listPage, false, 0, false
		m.candidatesOpen, m.candidatesFocused, m.candidatePane = true, true, id
		m.notice = ""
	}
}

func (m *model) candidateIndex() int {
	if m.candidatePane == (paneIdentity{}) {
		return -1
	}
	return slices.IndexFunc(m.candidates, func(c taskCandidate) bool {
		return identityOf(c.pane) == m.candidatePane
	})
}

func (m *model) onCandidates() bool {
	return m.page == listPage && !m.attention && m.focus == 0 && m.candidatesFocused &&
		(!m.onHomeBoard() || len(m.candidates) > 0)
}

func (m *model) moveCandidates(delta int) {
	i := m.candidateIndex()
	if m.onHomeBoard() && ((i < 0 && delta < 0) ||
		(delta > 0 && len(m.newAgents) > 0 && m.unassignedUrgency() == 0 &&
			(!m.candidatesOpen || i == len(m.candidates)-1))) {
		m.moveHome(delta)
		return
	}
	if i < 0 && delta < 0 {
		m.candidatesFocused = false
		return
	}
	if !m.candidatesOpen || len(m.candidates) == 0 {
		return
	}
	i = clamp(i+delta, -1, len(m.candidates)-1)
	m.candidatePane = paneIdentity{}
	if i >= 0 {
		m.candidatePane = identityOf(m.candidates[i].pane)
	}
}

func (m *model) candidateKey(key string) bool {
	i := m.candidateIndex()
	switch key {
	case "enter", "right", "o":
		if i < 0 {
			m.candidatesOpen = !m.candidatesOpen || key == "right"
			if key == "right" && len(m.candidates) > 0 {
				m.candidatePane = identityOf(m.candidates[0].pane)
			}
		} else {
			c := m.candidates[i]
			m.candidateDraft = &c
			m.editing = true
			m.input.set(m.sampleText(c.title))
		}
	case "i":
		if i >= 0 {
			c := m.candidates[i]
			id := identityOf(c.pane)
			if m.store != nil && !m.saved(m.store.ignoreCandidate(id, c.topic)) {
				return true
			}
			m.candidateIgnored[id] = c.topic
			delete(m.candidateRequests, id)
			m.refreshCandidates()
			m.notice = m.text("已忽略候选")
		}
	case "left", "esc":
		m.candidatesOpen, m.candidatePane = false, paneIdentity{}
	case "up", "k":
		m.moveCandidates(-1)
	case "down", "j":
		m.moveCandidates(1)
	default:
		return false
	}
	return true
}

func (m *model) acceptCandidate(title string) {
	draft := *m.candidateDraft
	id := identityOf(draft.pane)
	i := slices.IndexFunc(m.candidates, func(c taskCandidate) bool {
		return identityOf(c.pane) == id && c.topic == draft.topic
	})
	if i < 0 {
		m.editing, m.input, m.candidateDraft = false, lineInput{}, nil
		m.notice = m.text("候选已变化，请重新选择")
		return
	}
	c := m.candidates[i]
	status, _ := m.paneMilestone(c.pane.Status)
	t := &task{id: m.nextTaskID(), title: title, panes: []rex.Pane{c.pane},
		note: fmt.Sprintf("%s · %s · %s", m.text("候选来源"), m.text(c.source), status)}
	if m.store != nil {
		rowID, err := m.store.addCandidateTask(t, c.pane)
		if errors.Is(err, sql.ErrNoRows) {
			m.editing, m.input, m.candidateDraft = false, lineInput{}, nil
			m.notice = m.text("这个窗格已关联任务")
			delete(m.candidateRequests, id)
			m.candidateIgnored[id] = c.topic
			m.refreshCandidates()
			return
		}
		if !m.saved(err) {
			return
		}
		t.id = rowID
	}
	m.tasks = append(m.tasks, t)
	m.selectTask(t)
	delete(m.candidateRequests, id)
	m.editing, m.input, m.candidateDraft = false, lineInput{}, nil
	m.candidatesFocused, m.candidatePane = false, paneIdentity{}
	m.refreshCandidates()
	m.notice = m.text("已收录并关联窗格")
}

func (m *model) appendCandidateRows(screen *listView, width int) {
	screen.append("", listHit{})
	active := m.onCandidates()
	style := groupHeadingStyle(groupKey{})
	marker := "▸ "
	if m.candidatesOpen {
		marker = "▾ "
	}
	if active && m.candidateIndex() < 0 {
		screen.active = len(screen.lines)
		style = accent
	}
	screen.append(pair(style.Render("  "+marker+m.text("候选任务")), dim.Render(fmt.Sprint(len(m.candidates))), width), listHit{candidateHeader: true})
	if !m.candidatesOpen {
		return
	}
	if len(m.candidates) == 0 {
		screen.append(dim.Render(m.text("暂无候选任务")), listHit{})
	}
	for _, c := range m.candidates {
		id := identityOf(c.pane)
		hit := listHit{candidate: id}
		prefix, rowStyle := "  ", ink
		if active && id == m.candidatePane {
			screen.active = len(screen.lines)
			prefix, rowStyle = "› ", selected
		}
		screen.append(rowStyle.Render(pair(prefix+m.sampleText(c.title), m.text(statusText(c.pane.Status)), width)), hit)
		screen.append(dim.Render(clip("    "+m.paneLocation(c.pane)+" · "+m.text(c.source), width)), hit)
	}
}
