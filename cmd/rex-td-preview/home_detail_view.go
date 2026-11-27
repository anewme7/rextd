package main

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func (m *model) homeDetailView() string {
	w, h := m.width-1, m.height
	m.mouseRows, m.mouseResourceTitle = nil, -1
	if w < 20 || h < 8 {
		return clip(m.text("请扩大 Rex 侧栏  ·  q 退出"), w)
	}
	tier := homeWidth(m.width)
	lines := m.homeDetailTop()
	m.mouseBodyStart = len(lines)
	footer := m.homeDetailFooter(w, tier)
	footer = footer[max(0, len(lines)+len(footer)-h):]
	height := h - len(lines) - len(footer)
	body := m.homeDetailRows(w, tier, height)
	return m.homeViewport(lines, body, footer, height, w, tier)
}

func (m *model) homeDetailTop() []string {
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	inner := max(0, m.width-margin*2)
	back := m.text("‹ 任务")
	more := m.text("m 更多")
	backW := ansi.StringWidth(back)
	moreW := ansi.StringWidth(more)
	m.homeHeaderHits = []homeClickSpan{
		{start: margin, end: margin + backW, cmd: "esc"},
		{start: max(margin, m.width-margin-moreW), end: m.width - margin, cmd: "m"},
	}
	return []string{pad + pair(homeMuted.Render(back), homeMuted.Render(more), inner), ""}
}

func detailTaskStateStyle(state taskState, archived bool) lipgloss.Style {
	if archived {
		return homeMuted
	}
	switch state {
	case working:
		return homeLive
	case paused:
		return homeWait
	case done:
		return homeLive
	default:
		return homeMuted
	}
}

func (m *model) homeDetailRows(w int, tier homeTier, height int) listView {
	var v listView
	t := m.selected()
	if t == nil {
		return v
	}
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	gridW := homeGridWidth(w, tier)
	innerW := max(12, gridW-margin-1)
	add := func(s string) { v.append(s, listHit{}) }

	children := m.childRows()[t.id]
	stateText := m.text(stateLabels[t.state])
	if m.archivedTask(t) {
		stateText = m.text("已归档") + " · " + stateText
	}
	if progress := m.progressLabel(t, children); progress != "" {
		stateText += " · " + progress
	}
	checkGlyph := "○"
	checkStyle := homeMuted
	if t.state == done {
		checkGlyph = "✓"
		checkStyle = homeLive
	}
	title := m.taskText(t, t.title)
	titleLeft := checkStyle.Render(checkGlyph) + " " + homeSelectedTitle.Render(title)
	stateRight := detailTaskStateStyle(t.state, m.archivedTask(t)).Render(stateText)
	checkHit := listHit{task: t, taskCheckX: margin}
	if ansi.StringWidth(pad+titleLeft)+ansi.StringWidth(stateRight)+2 <= gridW {
		v.append(homeLine(pad+titleLeft, stateRight, gridW), checkHit)
	} else {
		// The list abbreviates long names. Details use the full width to read them.
		parts := wrap(title, gridW-margin-3)
		v.append(pad+checkStyle.Render(checkGlyph)+" "+homeSelectedTitle.Render(parts[0]), checkHit)
		for _, part := range parts[1:] {
			add(pad + "  " + homeSelectedTitle.Render(part))
		}
		add(pad + "  " + stateRight)
	}

	var metaBits []string
	if parent := m.parentOf(t); parent != nil {
		v.append(pad+"  "+homeMuted.Render(m.text("主任务")+" · "+m.taskText(parent, parent.title)), listHit{task: parent})
	}
	if t.kind != uncategorized {
		metaBits = append(metaBits, m.text("t 类型  ")+m.text(kindLabels[t.kind]))
	}
	if len(metaBits) > 0 {
		add(pad + "  " + homeFaint.Render(strings.Join(metaBits, "  ·  ")))
	}

	next := m.taskText(t, t.note)
	if next == "" && t.completed < len(t.steps) {
		next = m.taskText(t, t.steps[t.completed])
	}
	if next != "" {
		add("")
		add(pad + homeSectionHeading.Render(m.text("下一步")))
		for _, line := range wrap(next, max(10, innerW-2)) {
			add(pad + "  " + homeTaskText.Render(line))
		}
	}

	if pending, ok := m.urgentTaskPane(t); ok {
		add("")
		pendingStatus := m.homePaneTailCells(pending, tier, w, false)
		statusStr := ""
		for _, c := range pendingStatus {
			statusStr += c.style.Render(c.text)
		}
		left := homeWait.Render("●") + " " + homeMuted.Render("w "+m.text("待处理")) + homeFaint.Render(" · ") + homeTaskText.Render(m.paneDisplayTitle(pending)) + homeMuted.Render(" · "+m.hostLabel(pending.Host))
		v.append(homeLine(pad+left, statusStr, gridW), listHit{action: "w"})
	}

	if len(children) != 0 {
		add("")
		add(pad + homeSectionHeading.Render(fmt.Sprintf("%s · %d", m.text("子任务"), len(children))))
		for _, index := range children {
			child := m.tasks[index]
			label := m.text(stateLabels[child.state])
			if child.archived {
				label = m.text("已归档")
			}
			if len(child.panes) == 1 {
				label += " · " + m.hostLabel(child.panes[0].Host)
			}
			cGlyph := "○"
			if child.state == done {
				cGlyph = "✓"
			}
			v.append(homeLine(pad+"  "+homeMuted.Render(cGlyph)+" "+homeTaskText.Render(m.taskText(child, child.title)), homeMuted.Render(label), gridW),
				listHit{task: child, taskCheckX: margin + 2})
		}
	}

	add("")
	add(homeRule.Render(strings.Repeat("─", m.width)))

	groups := m.associatedSessions(t)
	idle := m.idleUnassignedPanes(t)
	if len(groups) == 0 {
		add(pad + homeMuted.Render(m.text("还没有关联会话")))
		if len(idle) == 0 && !m.archivedTask(t) {
			v.append(pad+homeBlue.Render(m.text("l 关联 Rex 窗格")), listHit{action: "l"})
		}
	} else {
		add(pad + homeSectionHeading.Render(fmt.Sprintf("%s · %d", m.text("关联会话"), len(groups))))
		indent := homePaneIndent(w, tier)
		indentW := ansi.StringWidth(indent)
		for gIdx, g := range groups {
			if gIdx > 0 {
				add("")
			}
			first := m.currentPane(t.panes[g.rows[0]])
			expanded := slices.Contains(g.rows, m.detailSelection)
			marker := "▸ "
			if expanded {
				marker = "▾ "
			}
			v.append(homeLine(pad+homePanelTitle.Render(marker+g.key.title), homeFaint.Render(m.hostLabel(first.Host)), gridW),
				listHit{groupID: g.key.id})
			if !expanded {
				continue
			}
			for _, i := range g.rows {
				p := m.currentPane(t.panes[i])
				reason := m.bindingReason(p)
				selected := i == m.detailSelection && m.detailIdlePane == (paneIdentity{})
				icon := homePaneIcon(p)
				leadCell := homeFallbackIconCell(icon)
				hit := listHit{pane: identityOf(p)}
				if w >= 28 && m.homeGraphics != nil && len(m.homeGraphics.images[icon]) > 0 {
					hit.homeIcon = icon
					leadCell = homeCell{text: "  ", style: homeSessionText}
				}
				prefix := append(homeCursorPrefix(indentW, selected), leadCell, homeCell{text: " ", style: homeSessionText})
				titleStyle := homeSessionText
				if selected {
					titleStyle = homeSelectedTitle
				}
				right := m.detailPaneStatusCells(p, reason, tier, w, selected)
				row := renderHomeRow(w, gridW, selected, prefix, m.paneDisplayTitle(p), titleStyle, right)
				v.appendSelectable(row, hit, selected)

				meta := nonEmpty(appName(p.App), m.linkLabel(t.bindings[identityOf(p)].source))
				if p.Window != "" && p.Window != m.paneDisplayTitle(p) && !generatedWindowLabel(p.Window, p.WindowID) {
					meta = append(meta, m.sampleText(p.Window))
				}
				if len(meta) != 0 {
					add(indent + "   " + homeFaint.Render(strings.Join(meta, " · ")))
				}
				if selected && reason != "" {
					v.append(indent+"   "+homeBlue.Render(m.text("r 重新关联")), listHit{action: "r"})
					v.append(indent+"   "+homeMuted.Render("u "+m.text("解除关联")), listHit{action: "u"})
				}
			}
		}
	}

	if len(idle) > 0 {
		add("")
		v.append(homeLine(pad+homeSectionHeading.Render(fmt.Sprintf("%s · %d", m.text("空闲且未关联"), len(idle))), homeFaint.Render(m.text("↑↓ 选择")), gridW), listHit{})
		if len(t.panes) == 0 && m.detailIdlePane == (paneIdentity{}) {
			v.active = len(v.lines)
		}
		indent := homePaneIndent(w, tier)
		indentW := ansi.StringWidth(indent)
		for _, pane := range idle {
			id := identityOf(pane)
			selected := m.detailIdlePane == id
			icon := homePaneIcon(pane)
			leadCell := homeFallbackIconCell(icon)
			hit := listHit{pane: id, detailUnassigned: true}
			if w >= 28 && m.homeGraphics != nil && len(m.homeGraphics.images[icon]) > 0 {
				hit.homeIcon = icon
				leadCell = homeCell{text: "  ", style: homeSessionText}
			}
			prefix := append(homeCursorPrefix(indentW, selected), leadCell, homeCell{text: " ", style: homeSessionText})
			titleStyle := homeSessionText
			if selected {
				titleStyle = homeSelectedTitle
			}
			right := m.detailPaneStatusCells(pane, "", tier, w, selected)
			row := renderHomeRow(w, gridW, selected, prefix, m.paneDisplayTitle(pane), titleStyle, right)
			v.appendSelectable(row, hit, selected)
			v.append(indent+"   "+homeFaint.Render(m.paneLocation(pane)), hit)
		}
	}

	if len(t.steps) > 0 {
		add("")
		add(pad + homeSectionHeading.Render(fmt.Sprintf(m.text("子任务 %d/%d"), t.completed, len(t.steps))))
		for i, step := range t.steps {
			check := "○ "
			style := homeMuted
			if i < t.completed {
				check = "✓ "
				style = homeLive
			}
			add(pad + "  " + style.Render(check) + homeTaskText.Render(m.taskText(t, step)))
		}
	}

	if t.result != "" {
		add("")
		add(pad + homeSectionHeading.Render(m.text("结果")))
		for _, line := range wrap(m.taskText(t, t.result), max(10, innerW-2)) {
			add(pad + "  " + homeTaskText.Render(line))
		}
	}

	return v
}

func (m *model) detailPaneStatusCells(p rex.Pane, reason string, tier homeTier, w int, selected bool) []homeCell {
	if reason != "" {
		label := m.text("不可用")
		if tier == homeNarrow {
			return []homeCell{{text: label, style: homeWait}}
		}
		return []homeCell{padRightCell("  "+label, m.homeStatusColWidth(tier), homeWait)}
	}
	return m.attentionStatusCells(p, tier, w, selected)
}

func (m *model) homeDetailFooter(w int, tier homeTier) []string {
	m.homeFooterHits = nil
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	inner := max(0, m.width-margin*2)
	var bottom []string
	if m.notice != "" {
		bottom = append(bottom, pad+clip(homeBlue.Render(m.notice), inner))
	}
	divider := homeRule.Render(strings.Repeat("─", m.width))
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
	shortcuts = append(shortcuts, [2]string{"l", m.text(picker)})

	line, _, hits := homeShortcutLine(shortcuts, margin, inner)
	m.homeFooterHits = hits
	return append(bottom, divider, pad+line)
}

func (m *model) homeDetailMouse(msg tea.MouseMsg, at int) tea.Cmd {
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	switch {
	case msg.Y == 0:
		for _, span := range m.homeHeaderHits {
			if msg.X >= span.start && msg.X < span.end {
				return m.listCommand(span.cmd)
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
	if at < 0 || at >= len(m.mouseRows) {
		return nil
	}
	hit := m.mouseRows[at]
	if hit.action != "" {
		return m.listCommand(hit.action)
	}
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
	return nil
}
