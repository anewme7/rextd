package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

const linkPage page = helpPage + 1

var focusHighlight = ink.
	Foreground(lipgloss.AdaptiveColor{Light: "#153529", Dark: "#EBF4EC"}).
	Background(lipgloss.AdaptiveColor{Light: "#DDEBE3", Dark: "#2B4540"})

func (m *model) focusView() string {
	if m.terminalPreview != nil {
		return m.terminalPreviewView()
	}
	if m.onHomeBoard() || m.onHomeAttention() {
		return m.homeView()
	}
	if m.page == detailPage && !m.editing {
		return m.homeDetailView()
	}
	w, h := m.width, m.height
	m.mouseRows, m.mouseResourceTitle = nil, -1
	if w == 0 || h == 0 {
		return ""
	}
	if w < 22 || h < 8 {
		return clip(m.text("请扩大 Rex 侧栏  ·  q 退出"), w)
	}
	divider := rules.Render(strings.Repeat("─", w))
	inner := w - 2
	header := accent.Render(m.text("‹ 返回"))
	action := ""
	switch {
	case m.page == detailPage:
		header = accent.Render(m.text("‹ 任务"))
		action = dim.Render(m.text("m 更多"))
	case m.page == scenePage && !m.scene.editing:
		action = dim.Render(m.text("m 更多"))
		if m.scene.more {
			action = dim.Render(m.text("m 简要"))
		}
	case m.page == linkPage:
		header = accent.Render(m.text("‹ 任务"))
	}
	top := []string{inset(pair(header, action, inner)), divider, ""}
	if m.page == listPage && !m.editing {
		top = m.focusListTop()
	}
	hints := m.hints(inner)
	if m.page == detailPage {
		shortcuts := [][2]string{{"Esc", m.text("返回")}}
		picker := "关联"
		if m.detailIdlePane != (paneIdentity{}) {
			shortcuts = append(shortcuts, [2]string{"↵", m.text("关联")})
			picker = "更多窗格"
		} else if t := m.selected(); m.selectedPaneUnavailable(t) {
			shortcuts = append(shortcuts, [2]string{"r", m.text("重新关联")}, [2]string{"u", m.text("解除关联")})
		} else if t != nil && len(t.panes) != 0 {
			shortcuts = append(shortcuts, [2]string{"↵", m.text("跳转")}, [2]string{m.text("空格"), m.text("预览")})
		}
		hints = renderShortcuts(inner, append(shortcuts, [2]string{"l", m.text(picker)}))
	}
	if m.page == taskMenuPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("选择")}})
	}
	if m.page == taskDeletePage {
		hints = renderShortcuts(inner, [][2]string{{"y", m.text("确认删除")}, {"Esc", m.text("取消")}})
	}
	if m.page == taskPickPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("选择")}})
	}
	if m.page == linkPage {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("返回")}, {"↵", m.text("关联")}})
	}
	if m.editing && m.candidateDraft == nil {
		hints = renderShortcuts(inner, [][2]string{{"Esc", m.text("取消")}, {"↵", m.text("保存")}})
	}
	onList := m.page == listPage && !m.editing && h >= 10
	bottom := m.focusBottom(hints, onList && m.focus == 0)
	if onList && m.focus != 0 {
		res := m.resources(inner, h)
		if len(top)+len(res)+len(bottom)+1 < h {
			bottom = append(append([]string{divider}, res...), bottom...)
			m.mouseResourceTitle = h - len(bottom) + 1
		}
	}
	m.mouseBodyStart = len(top)
	body := m.body(inner, max(1, h-len(top)-len(bottom)))
	return strings.Join(append(append(top, body...), bottom...), "\n")
}

func (m *model) focusBottom(hints string, showStatus bool) []string {
	inner := max(0, m.width-2)
	var bottom []string
	if m.notice != "" && m.page != scenePage {
		bottom = append(bottom, inset(clip(accent.Render(m.notice), inner)))
	}
	if showStatus {
		status := m.text("正在连接 Rex…")
		switch {
		case m.demo:
			status = m.text("演示")
		case m.feed.HostState("local") == rex.Live:
			status = m.text("Rex 已连接")
		case m.rexContext.SidebarID == "":
			status = m.text("需在 Rex 中启动")
		case !m.rexContext.Inside() || m.feed.HostState("local") == rex.Offline:
			status = m.text("Rex 未连接")
		}
		bottom = append(bottom, inset(pair(dim.Render(status), dim.Render(m.text("f 机器与额度")), inner)))
		m.mouseResourceTitle = m.height - 3
	}
	divider := rules.Render(strings.Repeat("─", m.width))
	return append(bottom, divider, inset(clip(hints, inner)))
}

func (m *model) focusListTop() []string {
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	inner := max(0, m.width-margin*2)
	m.homeHeaderHits = nil
	m.homeTopHits = nil

	tier := homeWidth(m.width)
	var leftHeader string
	if m.homeOverview && !m.attention {
		leftHeader = homeSelectedTitle.Render(m.text("任务总览"))
	} else {
		leftHeader = m.homeTabs()
	}
	leftHeaderW := ansi.StringWidth(leftHeader)
	add := m.text("+ 新建")
	if leftHeaderW+2+ansi.StringWidth(add) > inner {
		add = "+"
	}
	addW := ansi.StringWidth(add)
	addStart := max(margin, m.width-margin-addW)

	// Place compact alert/session pulse quietly in the top right next to "+ 新建".
	summaryCompact := m.homeAlertSummary(true)
	headerPulse := summaryCompact
	if headerPulse == "" || ansi.StringWidth(headerPulse) > inner {
		headerPulse = "n " + m.text("待处理")
	}
	alertInHeader := false
	rightHeader := homeAdd.Render(add)
	m.homeHeaderHits = append(m.homeHeaderHits, homeClickSpan{start: addStart, end: m.width - margin, cmd: "+"})
	if headerPulse != "" && tier >= homeMedium {
		alertW := ansi.StringWidth(headerPulse)
		gapBetween := 4
		if leftHeaderW+2+alertW+gapBetween+addW <= inner {
			alertInHeader = true
			alertStart := addStart - gapBetween - alertW
			m.homeHeaderHits = append([]homeClickSpan{{start: alertStart, end: alertStart + alertW, cmd: "n"}}, m.homeHeaderHits...)
			rightHeader = renderHomeAlert(headerPulse, m.attention, summaryCompact != "") + strings.Repeat(" ", gapBetween) + homeAdd.Render(add)
		}
	}
	header := pad + pair(leftHeader, rightHeader, inner)

	// No horizontal rule under tabs — a quiet blank line separates navigation from utilities.
	spacer := ""
	if m.attention {
		return []string{header, spacer, m.homeAttentionSubBar(margin, inner, tier)}
	}

	pulse := ""
	if !alertInHeader {
		pulse = renderHomeAlert(headerPulse, false, summaryCompact != "")
	}
	return append([]string{header, spacer}, m.homeUtilitySubBar(margin, inner, tier, pulse)...)
}

func (m *model) homeUtilitySubBar(margin, inner int, tier homeTier, pulse string) []string {
	pad := strings.Repeat(" ", margin)
	sep := "  ·  "
	var line strings.Builder
	x := margin
	control := func(label, cmd string, selected bool) {
		width := ansi.StringWidth(label)
		m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + width, cmd: cmd})
		style := homeMuted
		if selected {
			style = homeBlue.Bold(true)
		}
		line.WriteString(style.Render(label))
		x += width
	}
	gap := func(s string) {
		line.WriteString(homeFaint.Render(s))
		x += ansi.StringWidth(s)
	}
	if tier == homeNarrow {
		label := m.text("总览")
		if m.homeOverview {
			label = m.text("分页")
		}
		control("v "+label, "v", false)
	} else {
		control(m.text("分页"), "view:tabs", !m.homeOverview)
		if tier == homeFull {
			gap("   ")
		} else {
			gap("  ")
		}
		control(m.text("总览"), "view:overview", m.homeOverview)
	}
	gap(sep)
	control(m.text("/ 搜索"), "/", false)
	if pulse == "" {
		return []string{pad + line.String()}
	}
	width := ansi.StringWidth(pulse)
	if x-margin+ansi.StringWidth(sep)+width <= inner {
		gap(sep)
		m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + width, cmd: "n"})
		line.WriteString(pulse)
		return []string{pad + line.String()}
	}
	m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: margin, end: margin + width, rowOffset: 1, cmd: "n"})
	return []string{pad + line.String(), pad + pulse}
}

func renderHomeAlert(summary string, active, hasAlerts bool) string {
	body, hasKey := strings.CutSuffix(summary, " (n)")
	var rendered string
	if active {
		if rest, ok := strings.CutPrefix(body, "● "); ok {
			dotStyle := homeBlue
			if hasAlerts {
				dotStyle = homeWait
			}
			rendered = dotStyle.Render("●") + " " + homeBlue.Bold(true).Render(rest)
		} else {
			rendered = homeBlue.Bold(true).Render(body)
		}
	} else if hasAlerts {
		rendered = homeWait.Render(body)
	} else {
		rendered = homeMuted.Render(body)
	}
	if hasKey {
		return rendered + homeFaint.Render(" (n)")
	}
	return rendered
}

func (m *model) focusTabClick(x int) tea.Cmd {
	if !m.homeOverview || m.attention {
		if tab, ok := m.clickedTab(x); ok {
			m.switchHomeTab(tab)
			return nil
		}
	}
	for _, span := range m.homeHeaderHits {
		if x >= span.start && x < span.end {
			return m.listCommand(span.cmd)
		}
	}
	return nil
}

func (m *model) focusTasks(w int) listView {
	var view listView
	if m.taskCount() == 0 {
		if m.archivedView {
			view.append(dim.Render(m.text("暂无归档任务")), listHit{})
			return view
		}
		view.append("", listHit{})
		view.append(ink.Render(m.text("还没有任务")), listHit{})
		view.append("", listHit{})
		view.append(accent.Render(m.text("+ 新建任务")), listHit{action: "+"})
		view.append(dim.Render(m.text("a 查看 Rex 窗格")), listHit{action: "browse"})
	}
	if !m.archivedView && len(m.newAgents) != 0 {
		view.append(m.suggestionRow(w), listHit{boardSuggest: true})
		if m.boardSuggest && m.focus == 0 {
			view.active = len(view.lines) - 1
			if p, available := m.suggestionPane(); available {
				view.append("    "+dim.Render(strings.Join(nonEmpty(m.paneDisplayTitle(p), appName(p.App)), "  ")), listHit{boardSuggest: true})
			}
		}
	}
	children := m.childRows()
	for _, group := range m.taskGroups() {
		if group.key.title != "" {
			view.append(dim.Render(m.text(group.key.title)), listHit{})
		}
		for _, index := range group.rows {
			t := m.tasks[index]
			current := index == m.taskSelection && !m.onCandidates() && !m.boardSuggest && m.boardPane == (paneIdentity{})
			indent := ""
			if parent := m.parentOf(t); parent != nil && (!m.archivedView || parent.archived) {
				indent = "  "
			}
			width := w - len(indent)
			if current && m.focus == 0 {
				view.active = len(view.lines)
				width -= 2
			}
			for row, line := range m.taskRowWithProgress(index, width, m.progressLabel(t, children[t.id])) {
				hit := listHit{}
				if row == 0 {
					hit = listHit{task: t, taskArrowEnd: len(indent) + 5, taskCheckX: len(indent) + 5}
					if current && m.focus == 0 {
						line += " " + dim.Render("…")
						hit.taskMore = true
					}
				}
				view.append(indent+line, hit)
			}
			if !m.archivedView && t.state == working {
				m.appendBoardSessions(&view, t, indent, w)
			}
		}
	}
	if !m.archivedView {
		m.appendCandidateRows(&view, w)
	}
	return view
}

func (m *model) suggestionRow(w int) string {
	label := fmt.Sprintf(m.text("新会话 %d 个没归属"), len(m.newAgents))
	if t := m.taskByID(m.suggestedTask); t != nil {
		label += " · " + fmt.Sprintf(m.text("归入「%s」？"), m.taskText(t, t.title))
	}
	line := pair(label, dim.Render(m.text("m 选任务")), w)
	if m.boardSuggest && m.focus == 0 {
		line = focusHighlight.Render(line)
	}
	return line
}

func (m *model) appendBoardSessions(view *listView, t *task, indent string, w int) {
	indent += "    "
	width := w - len(indent)
	if next := m.taskText(t, t.note); next != "" {
		view.append(indent+dim.Render(clip(m.text("下一步")+" · "+next, width)), listHit{})
	}
	for _, p := range m.boardSessions(t) {
		id := identityOf(p)
		status := m.status(p.Status)
		if p.Status == "working" {
			status = accent.Render(m.text("在跑"))
		}
		left := []string{status, m.paneDisplayTitle(p), appName(p.App)}
		if m.density > 0 {
			left = append(left, p.Message)
		}
		right := m.linkLabel(t.bindings[id].source)
		if w >= 64 && m.density > 0 && !p.Since.IsZero() {
			minutes := int(time.Since(p.Since) / time.Minute)
			if minutes > 0 {
				right = strings.Join(nonEmpty(fmt.Sprintf(m.text("%d 分钟"), minutes), right), "  ")
			}
		}
		line := pair(strings.Join(nonEmpty(left...), "  "), dim.Render(right), width)
		if m.focus == 0 && m.selected() == t && m.boardPane == id && !m.boardSuggest && !m.onCandidates() {
			view.active = len(view.lines)
			line = focusHighlight.Render(line)
		}
		view.append(indent+line, listHit{task: t, pane: id})
	}
}

func (m *model) linkLabel(source linkSource) string {
	switch source {
	case linkRexTD:
		return m.text("从 rextd 开")
	case linkTab:
		return m.text("同标签")
	case linkDir:
		return m.text("同目录")
	case linkOnly:
		return m.text("唯一进行中")
	default:
		return ""
	}
}

func (m *model) focusSummary(t *task, w int) []string {
	var lines []string
	next := m.taskText(t, t.note)
	if next == "" && t.completed < len(t.steps) {
		next = m.taskText(t, t.steps[t.completed])
	}
	if next != "" {
		lines = append(lines, "    "+dim.Render(clip(m.text("下一步")+" · "+next, w-4)))
	}
	if len(t.panes) > 0 {
		lines = append(lines, "    "+dim.Render(clip(fmt.Sprintf(m.text("%d 个会话 · %d 个窗格"), len(m.associatedSessions(t)), len(t.panes)), w-4)))
	} else {
		lines = append(lines, "    "+dim.Render(m.text("还没有关联窗格")))
	}
	return lines
}

func (m *model) associatedSessions(t *task) []listGroup {
	var groups []listGroup
	for i, p := range t.panes {
		addGroup(&groups, m.sessionKey(m.currentPane(p), true), i)
	}
	return groups
}

func (m *model) orderedAssociations(t *task) []int {
	var order []int
	for _, g := range m.associatedSessions(t) {
		order = append(order, g.rows...)
	}
	return order
}

func (m *model) focusDetails(w int) listView {
	var v listView
	t := m.selected()
	if t == nil {
		return v
	}
	add := func(s string) { v.append(s, listHit{}) }
	children := m.childRows()[t.id]
	state := m.text(stateLabels[t.state])
	if m.archivedTask(t) {
		state = m.text("归档") + " · " + state
	}
	if progress := m.progressLabel(t, children); progress != "" {
		state += " · " + progress
	}
	v.append(pair(ink.Render(taskCheckbox(t)+" "+m.taskText(t, t.title)), statusColor(stateLabels[t.state]).Render(state), w),
		listHit{task: t, taskCheckX: 1})
	if parent := m.parentOf(t); parent != nil {
		v.append(dim.Render(m.text("主任务")+" · "+m.taskText(parent, parent.title)), listHit{task: parent})
	}
	next := m.taskText(t, t.note)
	if next == "" && t.completed < len(t.steps) {
		next = m.taskText(t, t.steps[t.completed])
	}
	if next != "" {
		add("")
		add(dim.Render(m.text("下一步")))
		for _, line := range wrap(next, w) {
			add(line)
		}
	}
	if pending, ok := m.urgentTaskPane(t); ok {
		add("")
		v.append(pair(accent.Render(m.text("w 看待处理")), m.status(pending.Status), w), listHit{action: "w"})
		v.append(clip("  "+m.paneDisplayTitle(pending)+" · "+m.hostLabel(pending.Host), w), listHit{action: "w"})
	}
	if len(children) != 0 {
		add("")
		add(dim.Render(m.text("子任务")))
		for _, index := range children {
			child := m.tasks[index]
			label := m.text(stateLabels[child.state])
			if child.archived {
				label = m.text("归档")
			}
			if len(child.panes) == 1 {
				label += " · " + m.hostLabel(child.panes[0].Host)
			}
			v.append(pair("  "+taskCheckbox(child)+" "+m.taskText(child, child.title), dim.Render(label), w),
				listHit{task: child, taskCheckX: 3})
		}
	}
	add("")
	groups := m.associatedSessions(t)
	idle := m.idleUnassignedPanes(t)
	if len(groups) == 0 {
		add(dim.Render(m.text("还没有关联窗格")))
		if len(idle) == 0 && !m.archivedTask(t) {
			v.append(accent.Render(m.text("l 关联 Rex 窗格")), listHit{action: "l"})
		}
	} else {
		add(dim.Render(m.text("关联会话")) + fmt.Sprintf("  %d", len(groups)))
		for _, g := range groups {
			first := m.currentPane(t.panes[g.rows[0]])
			expanded := slices.Contains(g.rows, m.detailSelection)
			marker := "▸ "
			if expanded {
				marker = "▾ "
			}
			v.append(pair(ink.Render(marker+g.key.title), dim.Render(m.hostLabel(first.Host)), w),
				listHit{groupID: g.key.id})
			if !expanded {
				continue
			}
			for _, i := range g.rows {
				p := m.currentPane(t.panes[i])
				status := m.status(p.Status)
				meta := nonEmpty(appName(p.App), m.linkLabel(t.bindings[identityOf(p)].source))
				reason := m.bindingReason(p)
				if reason != "" {
					status = warn.Render(reason)
				}
				row := pair("  "+m.paneDisplayTitle(p), status, w)
				selected := i == m.detailSelection && m.detailIdlePane == (paneIdentity{})
				if selected {
					v.active = len(v.lines)
					row = focusHighlight.Render(pair("› "+m.paneDisplayTitle(p), status, w))
				}
				v.append(row, listHit{pane: identityOf(p)})
				if p.Window != "" && p.Window != m.paneDisplayTitle(p) && !generatedWindowLabel(p.Window, p.WindowID) {
					meta = append(meta, m.sampleText(p.Window))
				}
				if len(meta) != 0 {
					add("    " + dim.Render(strings.Join(meta, " · ")))
				}
				if selected && reason != "" {
					v.append("    "+accent.Render(m.text("r 重新关联")), listHit{action: "r"})
					v.append("    "+accent.Render("u "+m.text("解除关联")), listHit{action: "u"})
				}
			}
			add("")
		}
	}
	m.appendDetailUnassigned(&v, t, idle, w)
	if len(t.steps) > 0 {
		add(dim.Render(fmt.Sprintf(m.text("子任务 %d/%d"), t.completed, len(t.steps))))
		for i, step := range t.steps {
			check := "○ "
			if i < t.completed {
				check = "✓ "
			}
			add("  " + check + m.taskText(t, step))
		}
	}
	if t.result != "" {
		add("")
		add(dim.Render(m.text("结果")))
		for _, line := range wrap(m.taskText(t, t.result), w) {
			add(line)
		}
	}
	if t.kind != uncategorized {
		add(dim.Render(m.text("t 类型  ") + m.text(kindLabels[t.kind])))
	}
	return v
}

func (m *model) selectedPaneUnavailable(t *task) bool {
	return m.page == detailPage && m.detailIdlePane == (paneIdentity{}) && t != nil &&
		m.detailSelection >= 0 && m.detailSelection < len(t.panes) &&
		m.bindingReason(t.panes[m.detailSelection]) != ""
}

func (m *model) bindingReason(p rex.Pane) string {
	if m.demo {
		return ""
	}
	switch m.feed.HostState(p.Host.ID) {
	case "":
		if p.Host.ID != "local" && m.feed.HostState("local") == rex.Live {
			return m.text("无法连接目标主机")
		}
	case rex.Offline:
		return m.text("主机失联")
	case rex.Live:
		if !slices.ContainsFunc(m.feed.Panes, func(live rex.Pane) bool { return samePane(live, p) }) {
			return m.text("窗格当前不可用")
		}
	}
	return ""
}

func (m *model) pickable() []rex.Pane {
	var out []rex.Pane
	for _, p := range m.feed.Panes {
		if p.ID == m.rexContext.SidebarID || m.feed.HostState(p.Host.ID) == rex.Offline || samePane(p, m.replacePane) {
			continue
		}
		if p.App != "" || m.pickerAll {
			out = append(out, p)
		}
	}
	return out
}

func (m *model) focusPicker(w int) listView {
	var v listView
	v.append(pair(ink.Render(m.text("关联 Rex 窗格")), dim.Render(m.text("a 切换范围")), w), listHit{action: "a"})
	v.append("", listHit{})
	panes := m.pickable()
	if len(panes) == 0 {
		v.append(dim.Render(m.text("没有可关联的窗格")), listHit{})
	}
	for i, pane := range panes {
		row := pair("  "+m.paneDisplayTitle(pane), m.status(pane.Status), w)
		if i == m.pickerSelection {
			v.active = len(v.lines) + 1
			row = focusHighlight.Render(pair("› "+m.paneDisplayTitle(pane), m.text(statusText(pane.Status)), w))
		}
		v.append(row, listHit{pane: identityOf(pane)})
		v.append("    "+dim.Render(m.paneLocation(pane)), listHit{pane: identityOf(pane)})
	}
	return v
}

func (m *model) openFocusPicker(replace bool) {
	if t := m.selected(); t == nil || m.archivedTask(t) {
		m.notice = m.text("请先还原任务")
		return
	}
	m.replacePane = rex.Pane{}
	if replace {
		if t := m.selected(); t != nil && len(t.panes) > 0 {
			m.replacePane = t.panes[clamp(m.detailSelection, 0, len(t.panes)-1)]
		}
	}
	m.notice, m.pickerSelection, m.pickerAll = "", 0, false
	m.page = linkPage
}

func (m *model) focusLinkKey(key string) {
	panes := m.pickable()
	switch key {
	case "esc", "left":
		m.page, m.notice = detailPage, ""
	case "a":
		m.pickerAll, m.pickerSelection = !m.pickerAll, 0
	case "up", "k":
		m.pickerSelection = clamp(m.pickerSelection-1, 0, len(panes)-1)
	case "down", "j":
		m.pickerSelection = clamp(m.pickerSelection+1, 0, len(panes)-1)
	case "enter":
		t := m.selected()
		if t == nil || m.pickerSelection < 0 || m.pickerSelection >= len(panes) {
			return
		}
		pane := panes[m.pickerSelection]
		index := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, m.replacePane) })
		existing := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, pane) })
		binding := paneBinding{source: linkManual, linkedAt: time.Now().UnixMilli()}
		if index >= 0 {
			if m.store != nil && !m.saved(m.store.replace(t.id, m.replacePane, pane)) {
				return
			}
			previous := identityOf(m.replacePane)
			delete(t.bindings, previous)
			m.removedAgents[previous] = true
			if existing >= 0 {
				t.panes = slices.Delete(t.panes, index, index+1)
			} else {
				t.panes[index] = pane
				t.setBinding(pane, binding)
			}
		} else if existing < 0 {
			if m.store != nil && !m.saved(m.store.bind(t.id, pane, false)) {
				return
			}
			t.addBinding(pane, binding)
		}
		delete(m.removedAgents, identityOf(pane))
		m.touchTask(t)
		m.detailSelection = slices.IndexFunc(t.panes, func(p rex.Pane) bool { return samePane(p, pane) })
		m.detailIdlePane = paneIdentity{}
		m.page, m.notice = detailPage, m.text("已关联 · ")+m.paneDisplayTitle(pane)
		m.refreshCandidates()
	}
}

func (s *taskStore) replace(task int64, previous, next rex.Pane) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, insertPaneBinding, paneBindingValues(task, next)...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rex_pane_binding WHERE task=? AND host_id=? AND session_id=? AND block_id=?",
		task, previous.Host.ID, previous.SessionID, previous.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *model) focusBrowse() {
	m.page, m.attention, m.allPanes, m.focus = listPage, true, true, 0
	m.refreshPanes()
}

func (m *model) focusMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress || msg.X < 1 || msg.X >= m.width-1 || msg.Y < 0 || msg.Y >= m.height {
		return nil
	}
	if m.page == scenePage && m.scene.editing {
		if msg.Button == tea.MouseButtonLeft && (msg.Y == 0 || msg.Y == m.height-1) {
			key := tea.KeyEsc
			if msg.Y == m.height-1 && m.shortcutAt(msg.X, 2) == 1 {
				key = tea.KeyEnter
			}
			return m.editPaneAgent(tea.KeyMsg{Type: key})
		}
		return nil
	}
	if m.editing {
		if msg.Button == tea.MouseButtonLeft && msg.Y == m.height-1 {
			key := tea.KeyEsc
			confirm := m.shortcutAt(msg.X, 2) == 1
			if m.candidateDraft != nil {
				confirm = !confirm
			}
			if confirm {
				key = tea.KeyEnter
			}
			m.edit(tea.KeyMsg{Type: key})
		}
		return nil
	}
	at := msg.Y - m.mouseBodyStart
	inBody := at >= 0 && at < len(m.mouseRows)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if m.onHomeBoard() || m.onHomeAttention() || m.page == detailPage {
			if msg.Y >= m.mouseBodyStart && msg.Y < m.homeBodyEnd {
				step := 1
				if msg.Button == tea.MouseButtonWheelUp {
					step = -1
				}
				m.homeScrollManual = true
				m.homeScrollTop = clamp(m.homeScrollTop+step, 0, m.homeScrollMax)
				return nil
			}
			if m.focus == 0 || msg.Y < m.homeBodyEnd || (m.mouseResourceTitle >= 0 && msg.Y >= m.mouseResourceTitle) {
				return nil
			}
		}
		if msg.Y >= m.mouseBodyStart && msg.Y < m.height-1 {
			key := "up"
			if msg.Button == tea.MouseButtonWheelDown {
				key = "down"
			}
			return m.listCommand(key)
		}
		return nil
	}
	if !inBody || m.mouseRows[at] != (listHit{}) {
		m.homeScrollManual = false
	}
	if m.page == listPage && msg.Button == tea.MouseButtonLeft {
		if msg.Y == m.mouseResourceTitle {
			end := m.width - m.homeMargin()
			if msg.X >= end-ansi.StringWidth(m.resourceShortcutLabel()) && msg.X < end {
				return m.listCommand("f")
			}
		}
		for _, hit := range m.homeDrawerHits {
			if msg.Y != hit.row || msg.X < hit.start || msg.X >= hit.end {
				continue
			}
			m.focus = hit.focus
			if hit.hostSlot != 0 {
				if m.hostSelection+1 == hit.hostSlot {
					m.showHost(hit.hostSlot)
				} else {
					m.hostSelection = hit.hostSlot - 1
				}
			}
			return nil
		}
	}
	if m.onHomeBoard() {
		return m.homeMouse(msg, at)
	}
	if m.onHomeAttention() {
		return m.homeAttentionMouse(msg, at)
	}
	if m.page == detailPage && !m.editing {
		return m.homeDetailMouse(msg, at)
	}
	if msg.Button == tea.MouseButtonRight && inBody && m.page == listPage {
		hit := m.mouseRows[at]
		if m.attention && hit.pane != (paneIdentity{}) {
			m.clickListRow(hit, msg.X)
			return m.listCommand("o")
		}
		if !m.attention && (hit.task != nil || hit.boardSuggest) {
			m.selectHome(boardEntry{task: hit.task, pane: hit.pane, suggest: hit.boardSuggest})
			return m.listCommand("m")
		}
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if msg.Y == 0 {
		if m.page != listPage {
			if (m.page == detailPage || m.page == scenePage) && msg.X >= m.width-9 {
				return m.listCommand("m")
			}
			return m.listCommand("esc")
		}
		return m.focusTabClick(msg.X)
	}
	if msg.Y == m.height-1 {
		switch m.page {
		case detailPage:
			if m.detailIdlePane != (paneIdentity{}) {
				return m.listCommand([...]string{"esc", "enter", "l"}[m.shortcutAt(msg.X, 3)])
			}
			if t := m.selected(); m.selectedPaneUnavailable(t) {
				return m.listCommand([...]string{"esc", "r", "u", "l"}[m.shortcutAt(msg.X, 4)])
			} else if t != nil && len(t.panes) != 0 {
				return m.listCommand([...]string{"esc", "enter", " ", "l"}[m.shortcutAt(msg.X, 4)])
			}
			return m.listCommand([...]string{"esc", "l"}[m.shortcutAt(msg.X, 2)])
		case taskDeletePage:
			return m.listCommand([...]string{"y", "esc"}[m.shortcutAt(msg.X, 2)])
		case taskMenuPage:
			if m.shortcutAt(msg.X, 2) == 1 {
				return m.listCommand("enter")
			}
			return m.listCommand("esc")
		case taskPickPage:
			if m.shortcutAt(msg.X, 2) == 1 {
				m.confirmTaskPick()
			} else {
				m.page, m.taskPick = m.taskPick.back, nil
			}
			return nil
		case linkPage:
			return m.listCommand([...]string{"esc", "enter"}[m.shortcutAt(msg.X, 2)])
		case scenePage:
			return m.listCommand([...]string{"enter", "d", "w", "esc"}[m.shortcutAt(msg.X, 4)])
		case helpPage:
			return m.listCommand([...]string{"down", "esc"}[m.shortcutAt(msg.X, 2)])
		case listPage:
			if m.focus != 0 {
				return m.listCommand("esc")
			}
			if m.onCandidates() {
				if m.candidateIndex() >= 0 {
					return m.listCommand([...]string{"enter", "i", "left"}[m.shortcutAt(msg.X, 3)])
				}
				return m.listCommand([...]string{"enter", "up"}[m.shortcutAt(msg.X, 2)])
			}
			return m.listCommand([...]string{"right", "enter", " ", "p"}[m.shortcutAt(msg.X, 4)])
		default:
			return m.listCommand("esc")
		}
	}
	if msg.Y == m.mouseResourceTitle {
		return m.listCommand("f")
	}
	if msg.Y == 2 && m.page == listPage {
		for _, span := range m.homeTopHits {
			if msg.X >= span.start && msg.X < span.end {
				return m.listCommand(span.cmd)
			}
		}
		return nil
	}
	if !inBody {
		return nil
	}
	hit := m.mouseRows[at]
	if hit.action == "browse" {
		m.focusBrowse()
		return nil
	}
	if hit.action != "" {
		return m.listCommand(hit.action)
	}
	switch m.page {
	case listPage:
		if hit.boardSuggest || (!m.attention && hit.pane != (paneIdentity{})) {
			entry := boardEntry{task: hit.task, pane: hit.pane, suggest: hit.boardSuggest}
			current := m.homeSelection()
			m.selectHome(entry)
			if entry == current {
				return m.listCommand("enter")
			}
			return nil
		}
		if hit.task != nil {
			wasTask := m.boardPane == (paneIdentity{}) && !m.boardSuggest
			m.boardPane, m.boardSuggest = paneIdentity{}, false
			i := slices.Index(m.tasks, hit.task)
			if hit.taskMore && msg.X == m.width-2 {
				m.selectTask(hit.task)
				return m.listCommand("m")
			}
			if i >= 0 && msg.X == hit.taskCheckX {
				m.taskSelection, m.candidatesFocused = i, false
				return m.listCommand("x")
			}
			if i >= 0 && wasTask && i == m.taskSelection && !m.onCandidates() {
				return m.listCommand("right")
			}
		}
		m.clickListRow(hit, msg.X)
	case detailPage:
		if hit.task != nil {
			if hit.taskCheckX != 0 && msg.X == hit.taskCheckX {
				m.toggleTaskCheck(hit.task)
			} else if hit.task != m.selected() {
				m.selectTask(hit.task)
				m.showDetails()
			}
			return nil
		}
		if t := m.selected(); t != nil {
			if hit.detailUnassigned {
				if m.detailIdlePane == hit.pane {
					return m.listCommand("enter")
				}
				m.detailIdlePane = hit.pane
			} else if hit.groupID != "" {
				for _, group := range m.associatedSessions(t) {
					if group.key.id == hit.groupID {
						m.detailSelection = group.rows[0]
						m.detailIdlePane = paneIdentity{}
						break
					}
				}
			} else if i := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return identityOf(p) == hit.pane }); i >= 0 {
				if m.detailSelection == i && m.detailIdlePane == (paneIdentity{}) {
					return m.listCommand("enter")
				}
				m.detailSelection = i
				m.detailIdlePane = paneIdentity{}
			}
		}
	case taskPickPage:
		if hit.task != nil {
			m.chooseTask(hit.task)
		} else if hit.pane != (paneIdentity{}) && m.taskPick.paneMove != nil {
			m.chooseTask(nil)
		}
	case linkPage:
		if i := slices.IndexFunc(m.pickable(), func(p rex.Pane) bool { return identityOf(p) == hit.pane }); i >= 0 {
			if i == m.pickerSelection {
				m.focusLinkKey("enter")
			} else {
				m.pickerSelection = i
			}
		}
	}
	return nil
}
