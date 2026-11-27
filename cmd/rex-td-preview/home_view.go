package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

var (
	homeSelectionBg = lipgloss.AdaptiveColor{Light: "#E9EDF5", Dark: "#242730"}

	homeTaskText       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2A2E35", Dark: "#CFD2D9"})
	homeSelectedTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#111318", Dark: "#F4F5F7"})
	homeSessionText    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#444952", Dark: "#B2B6BF"})
	homePanelTitle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4B5058", Dark: "#9A9EA8"})
	homeSectionHeading = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6E737D", Dark: "#767982"})
	homeMuted          = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7078", Dark: "#80838C"})
	homeSelectedMuted  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4E535B", Dark: "#9EA2AC"})
	homeFaint          = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#989DA6", Dark: "#5C6069"})
	homeRule           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#E5E7EB", Dark: "#262930"})
	homeKey            = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4E535B", Dark: "#A0A4AE"})

	homeBlue  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#7EA2F8"})
	homeWait  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#E8B959"})
	homeLive  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#63C787"})
	homeError = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#EB7070"})
	homeAdd   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5A5F68", Dark: "#9A9EA8"})
)

type homeTier uint8

const (
	homeNarrow homeTier = iota
	homeMedium
	homeFull
)

func homeWidth(columns int) homeTier {
	switch {
	case columns >= 104:
		return homeFull
	case columns >= 56:
		return homeMedium
	default:
		return homeNarrow
	}
}

func homeGridWidth(w int, tier homeTier) int {
	if tier == homeFull {
		return min(w, 108)
	}
	return w
}

func homePaneIndent(w int, tier homeTier) string {
	switch {
	case w < 30:
		return " "
	case w < 44:
		return "  "
	case tier == homeNarrow:
		return "   "
	default:
		return "    "
	}
}

func homeTaskPrefix(tier homeTier, selected bool) []homeCell {
	if tier == homeNarrow {
		if selected {
			return []homeCell{{text: "▸", style: homeBlue.Bold(true)}, {text: " ", style: homeSelectedTitle}}
		}
		return []homeCell{{text: "  ", style: homeTaskText}}
	}
	if selected {
		return []homeCell{{text: "  ", style: homeSelectedTitle}, {text: "▸", style: homeBlue.Bold(true)}, {text: " ", style: homeSelectedTitle}}
	}
	return []homeCell{{text: "    ", style: homeTaskText}}
}

func homeCursorPrefix(width int, selected bool) []homeCell {
	if width <= 0 {
		return nil
	}
	if !selected {
		return []homeCell{{text: strings.Repeat(" ", width), style: homeSessionText}}
	}
	switch width {
	case 1:
		return []homeCell{{text: "▸", style: homeBlue.Bold(true)}}
	case 2:
		return []homeCell{{text: "▸", style: homeBlue.Bold(true)}, {text: " ", style: homeSelectedTitle}}
	case 3:
		return []homeCell{
			{text: " ", style: homeSelectedTitle},
			{text: "▸", style: homeBlue.Bold(true)},
			{text: " ", style: homeSelectedTitle},
		}
	default:
		return []homeCell{
			{text: "  ", style: homeSelectedTitle},
			{text: "▸", style: homeBlue.Bold(true)},
			{text: strings.Repeat(" ", width-3), style: homeSelectedTitle},
		}
	}
}

type homeState uint8

const (
	homeWaiting homeState = iota
	homeFailed
	homeRunning
	homeFinished
	homeQuiet
)

func homeStatus(status string) homeState {
	switch status {
	case "permission", "question", "auth", "waiting":
		return homeWaiting
	case "error":
		return homeFailed
	case "working":
		return homeRunning
	case "done":
		return homeFinished
	default:
		return homeQuiet
	}
}

type homeCell struct {
	text  string
	style lipgloss.Style
}

func styleHomeCell(c homeCell, selected bool) string {
	if c.text == "" {
		return ""
	}
	st := c.style
	if selected {
		st = st.Background(homeSelectionBg)
	}
	return st.Render(c.text)
}

func padHomeSpace(count int, selected bool) string {
	if count <= 0 {
		return ""
	}
	spaces := strings.Repeat(" ", count)
	if selected {
		return lipgloss.NewStyle().Background(homeSelectionBg).Render(spaces)
	}
	return spaces
}

func renderHomeRow(w, gridW int, selected bool, prefix []homeCell, title string, titleStyle lipgloss.Style, right []homeCell) string {
	if w <= 0 {
		return ""
	}
	gridW = min(w, max(1, gridW))
	prefixW := 0
	for _, c := range prefix {
		prefixW += ansi.StringWidth(c.text)
	}
	rightW := 0
	for _, c := range right {
		rightW += ansi.StringWidth(c.text)
	}
	if rightW > 0 && prefixW+rightW+2 >= gridW {
		// Preserve space for the title on very narrow sidebars.
		if prefixW+rightW+1 > gridW {
			right = nil
			rightW = 0
		}
	}
	gap := 0
	if rightW > 0 {
		gap = 2
		if gridW-1-prefixW-rightW < 6 {
			gap = 1
		}
	}
	titleRoom := max(0, gridW-1-prefixW-rightW-gap)
	title = clip(title, titleRoom)
	titleW := ansi.StringWidth(title)
	midSpaces := max(0, gridW-1-prefixW-titleW-rightW)

	var out strings.Builder
	for _, c := range prefix {
		out.WriteString(styleHomeCell(c, selected))
	}
	if title != "" {
		out.WriteString(styleHomeCell(homeCell{text: title, style: titleStyle}, selected))
	}
	out.WriteString(padHomeSpace(midSpaces, selected))
	for _, c := range right {
		out.WriteString(styleHomeCell(c, selected))
	}
	used := prefixW + titleW + midSpaces + rightW
	if selected {
		out.WriteString(padHomeSpace(max(0, w-used), true))
	} else if used < w {
		out.WriteByte(' ')
	}
	return out.String()
}

func padRightCell(text string, width int, style lipgloss.Style) homeCell {
	text = clip(text, width)
	pad := max(0, width-ansi.StringWidth(text))
	return homeCell{text: text + strings.Repeat(" ", pad), style: style}
}

func padLeftCell(text string, width int, style lipgloss.Style) homeCell {
	text = clip(text, width)
	pad := max(0, width-ansi.StringWidth(text))
	return homeCell{text: strings.Repeat(" ", pad) + text, style: style}
}

func (m *model) homeStatusColWidth(tier homeTier) int {
	switch tier {
	case homeFull:
		if m.language == english {
			return 16
		}
		return 12
	case homeMedium:
		if m.language == english {
			return 16
		}
		return 11
	default:
		return 0
	}
}

func (m *model) homeView() string {
	// Reserve the final column, so full-width bands never wrap in the terminal.
	w, h := m.width-1, m.height
	m.mouseRows, m.mouseResourceTitle, m.homeDrawerHits = nil, -1, nil
	if w < 20 || h < 8 {
		return clip(m.text("请扩大 Rex 侧栏  ·  q 退出"), w)
	}
	tier := homeWidth(m.width)
	lines := m.focusListTop()
	m.mouseBodyStart = len(lines)
	footer := m.homeFooter(w, tier)
	var drawer []string
	if m.focus != 0 {
		drawer = m.homeResourcesDrawer(w, tier, h)
		heading := 1
		if len(lines)+len(drawer)+len(footer) >= h {
			footer = footer[len(footer)-1:]
			m.mouseResourceTitle = -1
		}
		if len(lines)+len(drawer)+len(footer) > h {
			drawer = drawer[1:] // Leave room for the controls on short screens.
			heading = 0
		}
		for i := range m.homeDrawerHits {
			m.homeDrawerHits[i].row += h - len(footer) - len(drawer) + heading
		}
	}
	bottom := append(drawer, footer...)
	bottom = bottom[max(0, len(lines)+len(bottom)-h):]
	height := h - len(lines) - len(bottom)
	var body listView
	if m.attention {
		body = m.homeAttentionRows(w, tier, height)
	} else {
		body = m.homeRows(w, tier, height)
	}
	return m.homeViewport(lines, body, bottom, height, w, tier)
}

func (m *model) homeViewport(lines []string, body listView, bottom []string, height, w int, tier homeTier) string {
	m.homeBodyEnd = m.mouseBodyStart + height
	m.homeScrollMax = max(0, len(body.lines)-height)
	if !m.homeScrollManual {
		m.homeScrollTop = body.active - height + 1
	}
	offset := clamp(m.homeScrollTop, 0, m.homeScrollMax)
	m.homeScrollTop = offset
	end := min(offset+height, len(body.lines))
	for _, line := range body.lines[offset:end] {
		lines = append(lines, clip(line, m.width))
	}
	m.mouseRows = body.hits[offset:end]
	indentW := ansi.StringWidth(homePaneIndent(w, tier))
	for i, hit := range m.mouseRows {
		if hit.homeIcon != noHomeIcon {
			m.homePlacements = append(m.homePlacements, homePlacement{
				row: m.mouseBodyStart + i + 1, column: indentW + 1, icon: hit.homeIcon,
			})
		}
	}
	lines = append(lines, make([]string, height-(end-offset))...)
	return strings.Join(append(lines, bottom...), "\n")
}

func (m *model) homeRows(w int, tier homeTier, height int) listView {
	if m.homeOverview {
		return m.homeOverviewRows(w, tier, height)
	}
	var view listView
	gridW := homeGridWidth(w, tier)
	if height >= 12 {
		view.append("", listHit{})
	}
	current := m.homeSelection()
	if m.onCandidates() {
		current = boardEntry{candidate: true}
	}
	urgent := m.unassignedUrgency()
	if m.homeTab == nowTab && urgent > 0 {
		m.appendHomeUnassigned(&view, tier, w, gridW, urgent, current)
		view.append("", listHit{})
	}
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	groups := m.taskGroups()
	if len(groups) == 0 {
		empty := [...]string{"暂无进行中的任务", "暂无待办任务", "暂无已完成任务", "暂无归档任务"}
		view.append(marginPad+homeMuted.Render(m.text(empty[m.homeTab])), listHit{})
	}
	children := m.childRows()
	for groupIdx, group := range groups {
		if m.isSecondary(group) {
			if groupIdx > 0 {
				view.append("", listHit{})
			}
			arrow := "▸"
			if m.secondaryOpen[m.homeTab] {
				arrow = "▾"
			}
			sel := current.secondary
			heading := fmt.Sprintf("%s %s · %d", arrow, m.text(group.key.title), len(group.rows))
			titleStyle := homeSectionHeading
			if sel {
				titleStyle = homeSelectedTitle
			}
			line := renderHomeRow(w, gridW, sel, homeCursorPrefix(margin, sel), heading, titleStyle, nil)
			view.appendSelectable(line, listHit{action: "toggle-secondary"}, sel)
			if !m.secondaryOpen[m.homeTab] {
				continue
			}
		} else {
			heading := fmt.Sprintf("%s · %d", m.text(group.key.title), len(group.rows))
			view.append(marginPad+homeSectionHeading.Render(heading), listHit{})
		}
		for _, index := range group.rows {
			t := m.tasks[index]
			title := m.taskText(t, t.title)
			if t.parentID != 0 {
				title = "  " + title
			}
			if t.inPinnedSection() {
				title = "★ " + title
			}
			if progress := m.progressLabel(t, children[t.id]); progress != "" && tier >= homeMedium {
				title += "  " + progress
			}
			sel := current == (boardEntry{task: t})
			titleStyle := homeTaskText
			if sel {
				titleStyle = homeSelectedTitle
			}
			tail := m.homeTaskTailCells(t, tier, w, sel)
			line := renderHomeRow(w, gridW, sel, homeTaskPrefix(tier, sel), title, titleStyle, tail)
			view.appendSelectable(line, listHit{task: t}, sel)
		}
	}
	if m.homeTab == nowTab && len(m.candidates) != 0 {
		view.append("", listHit{})
		m.appendCandidateRows(&view, gridW)
	}
	if m.homeTab == nowTab && urgent == 0 && len(m.newAgents) > 0 {
		view.append("", listHit{})
		m.appendHomeUnassigned(&view, tier, w, gridW, urgent, current)
	}
	var panel listView
	anchorRow := -1
	if m.homeTab == nowTab && len(m.newAgents) > 0 && (current.offer || current.suggest) {
		panel.append(m.homeUnassignedPanelHeader(tier, w, gridW, current), listHit{})
		m.appendHomeUnassignedSessions(&panel, tier, w, gridW, current)
		for i, hit := range view.hits {
			if hit.boardOffer {
				anchorRow = i
				break
			}
		}
	} else if t := m.sessionPanelTask(); t != nil && (len(t.panes) > 0 || t.note != "") {
		panel.append(m.homeSessionPanelHeader(t, tier, w, gridW, current), listHit{})
		m.appendHomeSessions(&panel, t, tier, w, gridW, current)
		for i, hit := range view.hits {
			if hit.task == t {
				anchorRow = i
				break
			}
		}
	}
	if len(panel.lines) > 0 {
		m.appendHomeSessionPanel(&view, &panel, anchorRow, height, current)
	}
	return view
}

func (m *model) homeOverviewRows(w int, tier homeTier, height int) listView {
	var view listView
	gridW := homeGridWidth(w, tier)
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	if height >= 12 {
		view.append("", listHit{})
	}
	current := m.homeSelection()
	if m.onCandidates() {
		current = boardEntry{candidate: true}
	}
	children := m.childRows()
	urgent := m.unassignedUrgency()
	hasOffers := len(m.newAgents) > 0

	for secIdx, tab := range [...]taskTab{nowTab, todoTab, doneTab, archiveTab} {
		if secIdx > 0 {
			view.append("", listHit{})
		}
		rows := m.overviewTasksFor(tab)
		marker := "▾"
		if m.overviewFolded[tab] {
			marker = "▸"
		}
		selSec := current.overviewSection && current.sectionTab == tab
		heading := fmt.Sprintf("%s %s · %d", marker, m.text(homeTabNames[tab]), len(rows))
		titleStyle := homeSectionHeading
		if selSec {
			titleStyle = homeSelectedTitle
		}
		line := renderHomeRow(w, gridW, selSec, homeCursorPrefix(margin, selSec), heading, titleStyle, nil)
		view.appendSelectable(line, listHit{overviewSection: true, sectionTab: tab}, selSec)
		if m.overviewFolded[tab] {
			continue
		}
		if tab == nowTab && hasOffers && urgent > 0 {
			m.appendHomeUnassigned(&view, tier, w, gridW, urgent, current)
		}
		if len(rows) == 0 && (tab != nowTab || !hasOffers) {
			empty := [...]string{"暂无进行中的任务", "暂无待办任务", "暂无已完成任务", "暂无归档任务"}
			view.append(marginPad+"  "+homeFaint.Render(m.text(empty[tab])), listHit{})
		}
		for _, index := range rows {
			t := m.tasks[index]
			title := m.taskText(t, t.title)
			if t.parentID != 0 {
				title = "  " + title
			}
			if t.inPinnedSection() {
				title = "★ " + title
			}
			if progress := m.progressLabel(t, children[t.id]); progress != "" && tier >= homeMedium {
				title += "  " + progress
			}
			sel := current == (boardEntry{task: t})
			rowStyle := homeTaskText
			if sel {
				rowStyle = homeSelectedTitle
			}
			tail := m.homeTaskTailCells(t, tier, w, sel)
			taskLine := renderHomeRow(w, gridW, sel, homeTaskPrefix(tier, sel), title, rowStyle, tail)
			view.appendSelectable(taskLine, listHit{task: t}, sel)
		}
		if tab == nowTab {
			if len(m.candidates) != 0 {
				m.appendCandidateRows(&view, gridW)
			}
			if hasOffers && urgent == 0 {
				m.appendHomeUnassigned(&view, tier, w, gridW, urgent, current)
			}
		}
	}

	var panel listView
	anchorRow := -1
	if hasOffers && (current.offer || current.suggest) {
		panel.append(m.homeUnassignedPanelHeader(tier, w, gridW, current), listHit{})
		m.appendHomeUnassignedSessions(&panel, tier, w, gridW, current)
		for i, hit := range view.hits {
			if hit.boardOffer {
				anchorRow = i
				break
			}
		}
	} else if t := m.sessionPanelTask(); t != nil && (len(t.panes) > 0 || t.note != "") {
		panel.append(m.homeSessionPanelHeader(t, tier, w, gridW, current), listHit{})
		m.appendHomeSessions(&panel, t, tier, w, gridW, current)
		for i, hit := range view.hits {
			if hit.task == t {
				anchorRow = i
				break
			}
		}
	}
	if len(panel.lines) > 0 {
		m.appendHomeSessionPanel(&view, &panel, anchorRow, height, current)
	}
	return view
}

func (m *model) appendHomeSessionPanel(view, panel *listView, anchorRow, height int, current boardEntry) {
	sepLines := 1
	if height >= 4 {
		if len(view.lines)+2+len(panel.lines) <= height {
			sepLines = 2
		}
		panelHeight := min(len(panel.lines), max(2, height/2))
		if anchorRow < 0 {
			anchorRow = view.active
		}
		view.cropAround(anchorRow, max(1, height-panelHeight-sepLines))
		panel.cropAround(panel.active, panelHeight)
	}
	if sepLines == 2 {
		view.append("", listHit{})
	}
	view.append(homeRule.Render(strings.Repeat("─", m.width)), listHit{})
	if current.pane != (paneIdentity{}) || current.more || current.suggest {
		view.active = len(view.lines) + panel.active
	}
	view.lines = append(view.lines, panel.lines...)
	view.hits = append(view.hits, panel.hits...)
}

func (m *model) homeSessionPanelHeader(t *task, tier homeTier, w, gridW int, current boardEntry) string {
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	taskName := m.taskText(t, t.title)
	var count string
	if tier == homeNarrow && w < 42 {
		count = fmt.Sprintf("· %d", len(t.panes))
	} else {
		count = fmt.Sprintf("· "+m.text("%d 个会话"), len(t.panes))
	}
	var right []homeCell
	if tier >= homeMedium {
		hint := m.text("→ 会话")
		if tier == homeFull {
			hint = "→ " + m.text("进入会话")
		}
		if current.pane != (paneIdentity{}) {
			if tier == homeFull {
				hint = m.text("空格") + " " + m.text("预览") + " · ↵ " + m.text("跳转")
			} else {
				hint = m.text("空格") + " " + m.text("预览")
			}
		}
		right = []homeCell{{text: hint, style: homeFaint}}
	}
	// Keep the count visible even if the task title needs truncation.
	rightW := 0
	for _, c := range right {
		rightW += ansi.StringWidth(c.text)
	}
	countW := ansi.StringWidth(count) + 1
	maxTitle := max(4, gridW-margin-1-rightW-countW-2)
	headerLeft := homePanelTitle.Render(clip(taskName, maxTitle)) + homeMuted.Render(" "+count)
	rightStr := ""
	for _, c := range right {
		rightStr += c.style.Render(c.text)
	}
	if rightStr == "" {
		return marginPad + clip(headerLeft, max(0, gridW-margin-1))
	}
	return homeLine(marginPad+headerLeft, rightStr, gridW)
}

func (v *listView) cropAround(row, height int) {
	if len(v.lines) <= height {
		return
	}
	start := clamp(row-height+1, 0, len(v.lines)-height)
	v.lines = v.lines[start : start+height]
	v.hits = v.hits[start : start+height]
	v.active = clamp(v.active-start, 0, height-1)
}

func (v *listView) appendSelectable(line string, hit listHit, selected bool) {
	if selected {
		v.active = len(v.lines)
	}
	v.append(line, hit)
}

func (m *model) homeUnassignedPanelHeader(tier homeTier, w, gridW int, current boardEntry) string {
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	var count string
	if tier == homeNarrow && w < 42 {
		count = fmt.Sprintf("· %d", len(m.newAgents))
	} else {
		count = fmt.Sprintf("· "+m.text("%d 个会话"), len(m.newAgents))
	}
	headerLeft := homePanelTitle.Render(m.text("未归类")) + homeMuted.Render(" "+count)
	if tier < homeMedium {
		return marginPad + clip(headerLeft, max(0, gridW-margin-1))
	}
	hint := m.text("→ 会话")
	if tier == homeFull {
		hint = "→ " + m.text("进入会话")
	}
	if current.suggest {
		if tier == homeFull {
			hint = m.text("空格") + " " + m.text("预览") + " · ↵ " + m.text("归入任务")
		} else {
			hint = "↵ " + m.text("归入任务")
		}
	}
	return homeLine(marginPad+headerLeft, homeFaint.Render(hint), gridW)
}

func (m *model) appendHomeUnassignedSessions(view *listView, tier homeTier, w, gridW int, current boardEntry) {
	indent := homePaneIndent(w, tier)
	if task := m.taskByID(m.suggestedTask); task != nil {
		suggest := fmt.Sprintf(m.text("建议任务 · %s"), m.taskText(task, task.title))
		view.append(clip(indent+homeMuted.Render(suggest), gridW), listHit{})
	}
	for _, p := range m.homeUnassignedPanes() {
		hit := listHit{pane: identityOf(p), boardSuggest: true}
		m.appendHomePane(view, hit, p, tier, w, gridW, current == (boardEntry{pane: hit.pane, suggest: true}))
	}
}

func (m *model) appendHomeUnassigned(view *listView, tier homeTier, w, gridW, urgent int, current boardEntry) {
	sel := current.offer
	titleStyle := homeSectionHeading
	if sel {
		titleStyle = homeSelectedTitle
	}
	label := m.text("未归类")
	if tier == homeNarrow {
		label = fmt.Sprintf("%s · %d", m.text("未归类"), len(m.newAgents))
	}
	right := m.homePanesTailCells(m.newAgents, tier, w, sel)
	line := renderHomeRow(w, gridW, sel, homeTaskPrefix(tier, sel), label, titleStyle, right)
	view.appendSelectable(line, listHit{boardOffer: true}, sel)
}

func (m *model) appendHomeSessions(view *listView, t *task, tier homeTier, w, gridW int, current boardEntry) {
	indent := homePaneIndent(w, tier)
	if t.note != "" {
		view.append(clip(indent+homeMuted.Render(m.text("下一步")+" · "+m.taskText(t, t.note)), gridW), listHit{})
	}
	panes, hidden := m.homeSessions(t)
	for _, p := range panes {
		hit := listHit{task: t, pane: identityOf(p)}
		m.appendHomePane(view, hit, p, tier, w, gridW, current == (boardEntry{task: t, pane: hit.pane}))
	}
	if hidden != 0 {
		label := fmt.Sprintf(m.text("还有 %d 个会话"), hidden)
		if tier == homeNarrow && w < 30 {
			label = fmt.Sprintf("+%d", hidden)
		}
		if m.boardOpenIdle[t] {
			label = m.text("收起会话")
		}
		sel := current == (boardEntry{task: t, more: true})
		titleStyle := homeFaint
		if sel {
			titleStyle = homeSelectedTitle
		}
		prefix := append(homeCursorPrefix(ansi.StringWidth(indent), sel), homeCell{text: "   ", style: homeFaint})
		line := renderHomeRow(w, gridW, sel, prefix, label, titleStyle, nil)
		view.appendSelectable(line, listHit{task: t, boardMore: true}, sel)
	}
}

func (m *model) appendHomePane(view *listView, hit listHit, p rex.Pane, tier homeTier, w, gridW int, active bool) {
	indent := homePaneIndent(w, tier)
	indentW := ansi.StringWidth(indent)
	icon := homePaneIcon(p)
	leadCell := homeFallbackIconCell(icon)
	if w >= 28 && m.homeGraphics != nil && len(m.homeGraphics.images[icon]) > 0 {
		hit.homeIcon = icon
		leadCell = homeCell{text: "  ", style: homeSessionText}
	}
	prefix := append(homeCursorPrefix(indentW, active), leadCell, homeCell{text: " ", style: homeSessionText})
	titleStyle := homeSessionText
	if active {
		titleStyle = homeSelectedTitle
	}
	right := m.homePaneTailCells(p, tier, w, active)
	line := renderHomeRow(w, gridW, active, prefix, m.paneDisplayTitle(p), titleStyle, right)
	view.appendSelectable(line, hit, active)
	if active && m.peek && !hit.boardSuggest {
		preview := indent + m.paneStatusPreview(p, p.Status)
		view.active = len(view.lines)
		view.append(homeMuted.Render(clip(preview, gridW)), listHit{})
	}
}

func (m *model) homeTaskTailCells(t *task, tier homeTier, w int, selected bool) []homeCell {
	if m.archivedTask(t) {
		mutedStyle := homeMuted
		if selected {
			mutedStyle = homeSelectedMuted
		}
		stateLabel := m.text(stateLabels[t.state])
		if tier == homeNarrow {
			if w < 36 {
				return nil
			}
			return []homeCell{{text: stateLabel, style: mutedStyle}}
		}
		statusW := m.homeStatusColWidth(tier)
		return []homeCell{padRightCell("  "+stateLabel, statusW, mutedStyle)}
	}
	if m.homeOverview && len(t.panes) == 0 && (t.state == paused || t.state == cancelled) {
		mutedStyle := homeMuted
		if selected {
			mutedStyle = homeSelectedMuted
		}
		stateLabel := m.text(stateLabels[t.state])
		if tier == homeNarrow {
			if w < 36 {
				return nil
			}
			return []homeCell{{text: stateLabel, style: mutedStyle}}
		}
		return []homeCell{padRightCell("  "+stateLabel, m.homeStatusColWidth(tier), mutedStyle)}
	}
	return m.homePanesTailCells(m.homePanes(t), tier, w, selected)
}

func (m *model) homePanesTailCells(panes []rex.Pane, tier homeTier, w int, selected bool) []homeCell {
	waiting, failed, running := 0, 0, 0
	hasDone, hasUnavailable := false, false
	for _, p := range panes {
		switch homeStatus(p.Status) {
		case homeWaiting:
			waiting++
		case homeFailed:
			failed++
		case homeRunning:
			running++
		case homeFinished:
			hasDone = true
		default:
			if p.Status == "offline" || m.bindingReason(p) != "" {
				hasUnavailable = true
			}
		}
	}

	mutedStyle := homeMuted
	faintStyle := homeFaint
	if selected {
		mutedStyle = homeSelectedMuted
		faintStyle = homeSelectedMuted
	}

	var statusText string
	var statusStyle lipgloss.Style
	switch {
	case waiting > 0:
		statusStyle = homeWait
		switch {
		case tier >= homeMedium:
			statusText = "● " + fmt.Sprintf(m.text("%d 个等你"), waiting)
		case w >= 36:
			statusText = "● " + m.text("等你")
		default:
			statusText = "●"
		}
	case failed > 0:
		statusStyle = homeError
		if tier >= homeMedium || w >= 36 {
			statusText = "● " + m.text("出错")
		} else {
			statusText = "●"
		}
	case running > 0:
		statusStyle = homeLive
		switch {
		case tier >= homeMedium || w >= 42:
			statusText = "● " + m.text("工作中")
		case w >= 36:
			statusText = "●"
		}
	case hasDone:
		statusStyle = homeWait
		switch {
		case tier >= homeMedium:
			statusText = "● " + m.text("本轮完成")
		case w >= 42:
			statusText = "● " + m.text("待看")
		case w >= 36:
			statusText = "●"
		}
	case hasUnavailable && tier >= homeMedium:
		statusStyle = faintStyle
		statusText = "  " + m.text("不可用")
	case len(panes) > 0 && tier >= homeMedium:
		statusStyle = mutedStyle
		statusText = "  " + m.text("空闲")
	}

	if tier == homeNarrow {
		if statusText == "" {
			return nil
		}
		return []homeCell{{text: statusText, style: statusStyle}}
	}

	statusW := m.homeStatusColWidth(tier)
	statusCell := padRightCell(statusText, statusW, statusStyle)
	if tier == homeMedium && w < 68 {
		return []homeCell{statusCell}
	}

	var count string
	var countW int
	var gap string
	if tier == homeFull {
		count = fmt.Sprintf(m.text("%d 个会话"), len(panes))
		countW = 9
		if m.language == english {
			countW = 12
		}
		gap = "   "
	} else {
		count = fmt.Sprintf(m.text("%d 会话"), len(panes))
		countW = 7
		if m.language == english {
			countW = 8
		}
		gap = "  "
	}
	return []homeCell{
		padLeftCell(count, countW, mutedStyle),
		{text: gap, style: mutedStyle},
		statusCell,
	}
}

func (m *model) homePaneTailCells(p rex.Pane, tier homeTier, w int, selected bool) []homeCell {
	var text string
	var style lipgloss.Style
	switch homeStatus(p.Status) {
	case homeWaiting:
		style = homeWait
		label := "等你"
		if tier >= homeMedium {
			switch p.Status {
			case "permission":
				label = "等你批准"
			case "question":
				label = "等你回答"
			case "auth":
				label = "要登录"
			}
		}
		if tier == homeNarrow && w < 36 {
			text = "●"
		} else {
			text = "● " + m.text(label)
		}
	case homeFailed:
		style = homeError
		if tier == homeNarrow && w < 36 {
			text = "●"
		} else {
			text = "● " + m.text("出错")
		}
	case homeRunning:
		style = homeLive
		switch {
		case tier >= homeMedium || w >= 40:
			text = "● " + m.text("工作中")
		case w >= 34:
			text = "●"
		}
	case homeFinished:
		style = homeWait
		switch {
		case tier >= homeMedium:
			text = "● " + m.text("本轮完成")
		case w >= 40:
			text = "● " + m.text("待看")
		case w >= 34:
			text = "●"
		}
	}
	if text == "" {
		return nil
	}
	if statusW := m.homeStatusColWidth(tier); statusW > 0 {
		return []homeCell{padRightCell(text, statusW, style)}
	}
	return []homeCell{{text: text, style: style}}
}

func homeFallbackIconCell(icon homeIcon) homeCell {
	switch icon {
	case claudeHomeIcon:
		return homeCell{text: "✳", style: lipgloss.NewStyle().Foreground(lipgloss.Color("#d97757"))}
	case codexHomeIcon:
		return homeCell{text: "◎", style: homeMuted}
	case kimiHomeIcon:
		return homeCell{text: "K", style: homeMuted}
	default:
		return homeCell{text: "›", style: homeFaint}
	}
}

func homeLine(left, right string, width int) string {
	right = clip(right, max(0, width-2))
	room := width - 1 - ansi.StringWidth(right)
	if right != "" {
		room--
	}
	left = clip(left, room)
	return left + strings.Repeat(" ", max(0, width-1-ansi.StringWidth(left)-ansi.StringWidth(right))) + right + " "
}

func (m *model) homeConnStatus(compact bool) (string, int) {
	dot := homeFaint.Render("○")
	label := m.text("正在连接 Rex…")
	switch {
	case m.demo:
		dot = homeMuted.Render("●")
		label = m.text("演示")
	case m.feed.HostState("local") == rex.Live:
		dot = homeLive.Render("●")
		label = m.text("Rex 已连接")
		if compact {
			label = "Rex"
		}
	case m.rexContext.SidebarID == "":
		label = m.text("需在 Rex 中启动")
	case !m.rexContext.Inside() || m.feed.HostState("local") == rex.Offline:
		label = m.text("Rex 未连接")
	}
	rendered := dot + " " + homeMuted.Render(label)
	return rendered, 2 + ansi.StringWidth(label)
}

func (m *model) resourceShortcutLabel() string {
	if m.width < 38 {
		return m.text("f 机器")
	}
	return m.text("f 机器与额度")
}

func (m *model) homeFooter(w int, tier homeTier) []string {
	m.homeFooterHits = nil
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	inner := max(0, m.width-margin*2)
	var bottom []string
	if m.notice != "" && m.page != scenePage {
		bottom = append(bottom, pad+clip(accent.Render(m.notice), inner))
	}
	divider := homeRule.Render(strings.Repeat("─", m.width))
	shortcuts := m.homeShortcuts(tier)
	rightLabel := m.resourceShortcutLabel()
	rightW := ansi.StringWidth(rightLabel)
	connStr, connW := m.homeConnStatus(tier == homeNarrow && m.width < 36)

	// On wider screens, merge connection status, shortcuts, and hosts/usage into one quiet bar.
	if m.width >= 72 {
		scStr, scW, scHits := homeShortcutLine(shortcuts, margin+connW+5, inner)
		if connW+5+scW+2+rightW <= inner && len(scHits) >= len(shortcuts) {
			m.mouseResourceTitle = -1
			m.homeFooterHits = append(scHits, homeClickSpan{
				start: max(margin, m.width-margin-rightW),
				end:   m.width - margin,
				cmd:   "f",
			})
			left := connStr + homeFaint.Render("  ·  ") + scStr
			return append(bottom, divider, pad+pair(left, homeMuted.Render(rightLabel), inner))
		}
	}

	// On narrow/compact widths, keep status cleanly above the divider and shortcuts below.
	bottom = append(bottom, pad+pair(connStr, homeMuted.Render(rightLabel), inner))
	m.mouseResourceTitle = m.height - 3
	scStr, _, scHits := homeShortcutLine(shortcuts, margin, inner)
	m.homeFooterHits = scHits
	return append(bottom, divider, pad+clip(scStr, inner))
}

// Use the same measured cells for drawing a shortcut and handling its click.
// A narrower footer omits whole actions; every visible label stays readable.
func homeShortcutLine(shortcuts [][2]string, start, width int) (string, int, []homeClickSpan) {
	var line strings.Builder
	var hits []homeClickSpan
	x := start
	for i, sc := range shortcuts {
		gap := 0
		if i > 0 {
			gap = 2
		}
		key, label := sc[0], sc[1]
		itemWidth := ansi.StringWidth(key) + 1 + ansi.StringWidth(label)
		if x-start+gap+itemWidth > width {
			break
		}
		line.WriteString(strings.Repeat(" ", gap))
		x += gap
		if key == "↑↓" {
			hits = append(hits, homeClickSpan{start: x, end: x + 1, cmd: "up"}, homeClickSpan{start: x + 1, end: x + itemWidth, cmd: "down"})
		} else {
			hits = append(hits, homeClickSpan{start: x, end: x + itemWidth, cmd: homeShortcutCmd(key)})
		}
		line.WriteString(homeKey.Render(key) + " " + homeMuted.Render(label))
		x += itemWidth
	}
	return line.String(), x - start, hits
}

func homeShortcutCmd(key string) string {
	switch key {
	case "↵":
		return "enter"
	case "→":
		return "right"
	case "←":
		return "left"
	case "↑":
		return "up"
	case "空格", "Space":
		return " "
	case "Esc":
		return "esc"
	default:
		return key
	}
}

func (m *model) homeShortcuts(tier homeTier) [][2]string {
	var shortcuts [][2]string
	current := m.homeSelection()
	switch {
	case m.focus == 1 && !m.demo:
		shortcuts = [][2]string{{"↵", "筛选"}, {"↑↓", "选择"}, {"f", "切换"}, {"Esc", "返回"}}
	case m.focus != 0:
		shortcuts = [][2]string{{"↑↓", "滚动"}, {"f", "切换"}, {"Esc", "返回"}}
	case m.attention:
		return m.attentionShortcuts(tier)
	case current.overviewSection, current.secondary:
		shortcuts = [][2]string{{"v", "视图"}, {"↵", "展开／收起"}}
	case m.onCandidates() && m.candidateIndex() >= 0:
		shortcuts = [][2]string{{"↵", "收录"}, {"i", "忽略"}, {"←", "收起"}}
	case m.onCandidates():
		shortcuts = [][2]string{{"↵", "展开／收起"}, {"↑", "返回任务"}}
	case current.offer:
		shortcuts = [][2]string{{"v", "视图"}, {"→", "进入会话"}}
	case current.more && !m.boardOpenIdle[current.task]:
		shortcuts = [][2]string{{"↵", "展开"}}
	case current.more:
		shortcuts = [][2]string{{"↵", "收起"}}
	case current.suggest:
		shortcuts = [][2]string{{"空格", "预览"}, {"↵", "归入任务"}, {"←", "返回"}}
	case current.pane != (paneIdentity{}):
		shortcuts = [][2]string{{"空格", "预览"}, {"↵", "跳转"}, {"m", "移动"}}
	case current.task != nil && m.archivedTask(current.task):
		shortcuts = [][2]string{{"v", "视图"}, {"↵", "详情"}, {"m", "操作"}}
	case current.task != nil && len(current.task.panes) > 0:
		shortcuts = [][2]string{{"v", "视图"}, {"↵", "详情"}, {"→", "会话"}, {"s", "切换状态"}, {"n", "待处理"}}
	case current.task == nil:
		shortcuts = [][2]string{{"v", "视图"}, {"+", "新建任务"}, {"?", "帮助"}}
	default:
		shortcuts = [][2]string{{"v", "视图"}, {"↵", "详情"}, {"s", "切换状态"}, {"n", "待处理"}, {"p", "置顶"}}
	}
	limit := 5
	switch tier {
	case homeMedium:
		if m.width < 72 {
			limit = 3
		} else if m.width < 80 {
			limit = 4
		}
	case homeNarrow:
		limit = 2
		if m.width < 32 {
			limit = 1
		}
	}
	shortcuts = shortcuts[:min(len(shortcuts), limit)]
	for i := range shortcuts {
		shortcuts[i] = [2]string{m.text(shortcuts[i][0]), m.text(shortcuts[i][1])}
	}
	return shortcuts
}

func (m *model) homeMouse(msg tea.MouseMsg, at int) tea.Cmd {
	if msg.Button == tea.MouseButtonLeft {
		switch {
		case msg.Y == 0:
			return m.focusTabClick(msg.X)
		case msg.Y >= 2 && msg.Y < m.mouseBodyStart:
			for _, span := range m.homeTopHits {
				if msg.Y == 2+span.rowOffset && msg.X >= span.start && msg.X < span.end {
					switch span.cmd {
					case "view:tabs":
						m.setHomeOverview(false)
						return nil
					case "view:overview":
						m.setHomeOverview(true)
						return nil
					default:
						return m.listCommand(span.cmd)
					}
				}
			}
			return nil
		case msg.Y == m.height-1:
			for _, span := range m.homeFooterHits {
				if msg.X >= span.start && msg.X < span.end {
					return m.listCommand(span.cmd)
				}
			}
			return nil
		}
	}
	if at < 0 || at >= len(m.mouseRows) {
		return nil
	}
	if m.focus != 0 {
		m.focus = 0
	}
	hit := m.mouseRows[at]
	entry := boardEntry{task: hit.task, pane: hit.pane, more: hit.boardMore, offer: hit.boardOffer, suggest: hit.boardSuggest}
	same := entry == m.homeSelection() && !m.onCandidates()
	if msg.Button == tea.MouseButtonRight && (entry.task != nil || entry.suggest) {
		m.selectHome(entry)
		return m.listCommand("m")
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	switch {
	case hit.overviewSection:
		m.selectHome(boardEntry{overviewSection: true, sectionTab: hit.sectionTab})
		m.overviewFolded[hit.sectionTab] = !m.overviewFolded[hit.sectionTab]
		return nil
	case hit.action == "toggle-secondary":
		m.selectHome(boardEntry{secondary: true})
		return m.listCommand("enter")
	case hit.action != "":
		return m.listCommand(hit.action)
	case entry.offer:
		m.selectHome(entry)
		if same {
			return m.listCommand("enter")
		}
	case entry.more:
		m.selectHome(entry)
		return m.listCommand("enter")
	case entry.suggest || entry.pane != (paneIdentity{}):
		m.selectHome(entry)
		if same {
			return m.listCommand("enter")
		}
	case entry.task != nil:
		m.selectHome(entry)
		if same {
			return m.listCommand("enter")
		}
	default:
		m.clickListRow(hit, msg.X)
	}
	return nil
}
