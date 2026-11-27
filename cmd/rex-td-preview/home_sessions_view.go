package main

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

func (m *model) homeAttentionSubBar(margin, inner int, tier homeTier) string {
	pad := strings.Repeat(" ", margin)
	m.homeTopHits = nil

	alertTotal := m.alertCount()
	allTotal := 0
	for _, p := range m.feed.Panes {
		if m.inHostScope(p) {
			allTotal++
		}
	}

	if tier == homeNarrow {
		groupName := groupingLabels[m.grouping]
		if m.grouping == byAgent {
			groupName = "工具"
		}
		groupLabel := "v " + m.text("按"+groupName)
		scopeLabel := fmt.Sprintf("a %s %d", m.text("待看"), alertTotal)
		if m.allPanes {
			scopeLabel = fmt.Sprintf("a %s %d", m.text("全部"), allTotal)
		}
		sep := "  ·  "
		gW := ansi.StringWidth(groupLabel)
		sW := ansi.StringWidth(scopeLabel)
		sepW := ansi.StringWidth(sep)
		m.homeTopHits = []homeClickSpan{
			{start: margin, end: margin + gW, cmd: "v"},
			{start: margin + gW + sepW, end: margin + gW + sepW + sW, cmd: "a"},
		}
		return pad + clip(homeBlue.Bold(true).Render(groupLabel)+homeFaint.Render(sep)+homeMuted.Render(scopeLabel), inner)
	}

	// Medium & Full: clickable grouping switcher + scope toggle.
	options := []struct {
		mode  grouping
		label string
		cmd   string
	}{
		{byStatus, m.text("状态"), "group:status"},
		{bySession, "Session", "group:session"},
		{byAgent, "Agent", "group:agent"},
		{byHost, "Host", "group:host"},
	}
	if m.language == chinese {
		options[1].label = "会话"
		options[2].label = "工具"
		options[3].label = "主机"
	}

	var sb strings.Builder
	x := margin
	gap := "   "
	gapW := 3
	if tier == homeMedium {
		gap = "  "
		gapW = 2
	}
	for i, opt := range options {
		if i > 0 {
			sb.WriteString(gap)
			x += gapW
		}
		w := ansi.StringWidth(opt.label)
		m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + w, cmd: opt.cmd})
		if m.grouping == opt.mode {
			sb.WriteString(homeBlue.Bold(true).Render(opt.label))
		} else {
			sb.WriteString(homeMuted.Render(opt.label))
		}
		x += w
	}

	sep := "   ·   "
	if tier == homeMedium {
		sep = "  ·  "
	}
	sb.WriteString(homeFaint.Render(sep))
	x += ansi.StringWidth(sep)

	alertText := fmt.Sprintf("%s %d", m.text("待看"), alertTotal)
	allText := fmt.Sprintf("%s %d", m.text("全部"), allTotal)

	alertW := ansi.StringWidth(alertText)
	m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + alertW, cmd: "scope:alerts"})
	if !m.allPanes {
		sb.WriteString(homeSelectedTitle.Render(alertText))
	} else {
		sb.WriteString(homeMuted.Render(alertText))
	}
	x += alertW + gapW
	sb.WriteString(gap)

	allW := ansi.StringWidth(allText)
	m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + allW, cmd: "scope:all"})
	if m.allPanes {
		sb.WriteString(homeSelectedTitle.Render(allText))
	} else {
		sb.WriteString(homeMuted.Render(allText))
	}
	x += allW

	if m.hostFilter != "" {
		hostName := m.hostFilter
		for _, h := range m.feed.Hosts {
			if h.ID == m.hostFilter {
				hostName = m.hostLabel(h.Host)
				break
			}
		}
		chip := fmt.Sprintf("%s (g)", hostName)
		sb.WriteString(homeFaint.Render(sep))
		x += ansi.StringWidth(sep)
		chipW := ansi.StringWidth(chip)
		m.homeTopHits = append(m.homeTopHits, homeClickSpan{start: x, end: x + chipW, cmd: "g"})
		sb.WriteString(homeMuted.Render(chip))
	}

	return pad + clip(sb.String(), inner)
}

func (m *model) homeAttentionRows(w int, tier homeTier, height int) listView {
	var view listView
	gridW := homeGridWidth(w, tier)
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	if height >= 12 {
		view.append("", listHit{})
	}
	if len(m.livePanes) == 0 {
		text := m.text("没有需要处理的窗格 · a 查看全部")
		if m.allPanes {
			text = m.text("暂无 Rex 窗格")
		}
		if !m.rexContext.Inside() && !m.demo {
			text = m.text("在 Rex 窗格内打开可查看实时状态")
		} else if len(m.feed.Hosts) == 0 {
			text = m.text("正在连接 Rex…")
		}
		view.append(marginPad+homeMuted.Render(text), listHit{})
		return view
	}

	groups := m.paneGroups()
	for i, group := range groups {
		if i > 0 {
			view.append("", listHit{})
		}
		m.appendAttentionGroup(&view, group, margin, w, gridW, tier)
	}

	if m.paneSelection >= 0 && m.paneSelection < len(m.livePanes) {
		selectedPane := m.livePanes[m.paneSelection]
		if m.feed.HostState(selectedPane.Host.ID) == rex.Offline {
			selectedPane.Status = "offline"
		}
		var inspector listView
		inspector.append(m.attentionInspectorHeader(selectedPane, tier, gridW), listHit{})
		if msg := strings.TrimSpace(m.sampleText(selectedPane.Message)); msg != "" {
			inspector.append(clip(homePaneIndent(w, tier)+homeMuted.Render(msg), gridW), listHit{})
		}
		if height >= 6 {
			sepLines := 1
			if len(view.lines)+2+len(inspector.lines) <= height {
				sepLines = 2
			}
			view.cropAround(view.active, max(1, height-len(inspector.lines)-sepLines))
			if sepLines == 2 {
				view.append("", listHit{})
			}
			view.append(homeRule.Render(strings.Repeat("─", m.width)), listHit{})
			view.lines = append(view.lines, inspector.lines...)
			view.hits = append(view.hits, inspector.hits...)
		}
	}
	return view
}

func (m *model) appendAttentionGroup(view *listView, group listGroup, indent, w, gridW int, tier homeTier) {
	title := group.key.title
	if group.key == pinnedGroup {
		title = "★ " + m.text(title)
	}
	marker := "▾"
	if group.folded {
		marker = "▸"
	}
	heading := fmt.Sprintf("%s %s · %d", marker, title, len(group.rows))
	selected := group.folded && slices.Contains(group.rows, m.paneSelection)
	style := homeSectionHeading
	if selected {
		style = homeSelectedTitle
	}
	row := renderHomeRow(w, gridW, selected, homeCursorPrefix(indent, selected), heading, style, nil)
	view.appendSelectable(row, listHit{groupID: group.key.id}, selected)
	if group.folded {
		return
	}
	if len(group.children) > 0 {
		for _, child := range group.children {
			m.appendAttentionGroup(view, child, indent+1, w, gridW, tier)
		}
		return
	}
	for _, i := range group.rows {
		pane := m.livePanes[i]
		if m.feed.HostState(pane.Host.ID) == rex.Offline {
			pane.Status = "offline"
		}
		m.appendAttentionPaneRow(view, pane, tier, w, gridW, i == m.paneSelection)
	}
}

func (m *model) appendAttentionPaneRow(view *listView, p rex.Pane, tier homeTier, w, gridW int, active bool) {
	indent := homePaneIndent(w, tier)
	indentW := ansi.StringWidth(indent)
	icon := homePaneIcon(p)
	leadCell := homeFallbackIconCell(icon)
	hit := listHit{pane: identityOf(p)}
	if w >= 28 && m.homeGraphics != nil && len(m.homeGraphics.images[icon]) > 0 {
		hit.homeIcon = icon
		leadCell = homeCell{text: "  ", style: homeSessionText}
	}
	prefix := append(homeCursorPrefix(indentW, active), leadCell, homeCell{text: " ", style: homeSessionText})
	title := m.paneDisplayTitle(p)
	if m.panePins[identityOf(p)] {
		title = "★ " + title
	}
	titleStyle := homeTaskText
	if active {
		titleStyle = homeSelectedTitle
	}
	right := m.attentionPaneTailCells(p, tier, w, active)
	line := renderHomeRow(w, gridW, active, prefix, title, titleStyle, right)
	view.appendSelectable(line, hit, active)
}

func (m *model) attentionPaneTailCells(p rex.Pane, tier homeTier, w int, selected bool) []homeCell {
	statusCells := m.attentionStatusCells(p, tier, w, selected)
	if tier < homeMedium || w < 68 {
		return statusCells
	}
	context := m.attentionPaneContext(p)
	if context == "" {
		return statusCells
	}
	mutedStyle := homeMuted
	if selected {
		mutedStyle = homeSelectedMuted
	}
	ctxW := 18
	gap := "   "
	if tier == homeMedium {
		ctxW = 14
		gap = "  "
	}
	return append([]homeCell{
		padLeftCell(context, ctxW, mutedStyle),
		{text: gap, style: mutedStyle},
	}, statusCells...)
}

func (m *model) attentionStatusCells(p rex.Pane, tier homeTier, w int, selected bool) []homeCell {
	if cells := m.homePaneTailCells(p, tier, w, selected); len(cells) > 0 {
		return cells
	}
	if tier == homeNarrow {
		return nil
	}
	mutedStyle := homeMuted
	faintStyle := homeFaint
	if selected {
		mutedStyle = homeSelectedMuted
		faintStyle = homeSelectedMuted
	}
	label := "  " + m.text("空闲")
	style := mutedStyle
	if p.Status == "offline" || m.bindingReason(p) != "" {
		label = "  " + m.text("不可用")
		style = faintStyle
	}
	return []homeCell{padRightCell(label, m.homeStatusColWidth(tier), style)}
}

func (m *model) attentionPaneContext(p rex.Pane) string {
	switch m.grouping {
	case byStatus, bySession:
		if t := m.linkedTaskForPane(p); t != nil {
			return m.taskText(t, t.title)
		}
		return m.text("未归类")
	case byAgent:
		return strings.Join(nonEmpty(m.sampleText(p.Session), m.hostLabel(p.Host)), " · ")
	case byHost:
		if s := m.sampleText(p.Session); s != "" {
			return s
		}
		return appName(p.App)
	}
	return ""
}

func (m *model) linkedTaskForPane(p rex.Pane) *task {
	for _, t := range m.tasks {
		if t.archived {
			continue
		}
		for _, linked := range t.panes {
			if samePane(linked, p) {
				return t
			}
		}
	}
	return nil
}

func (m *model) attentionInspectorHeader(p rex.Pane, tier homeTier, gridW int) string {
	margin := m.homeMargin()
	marginPad := strings.Repeat(" ", margin)
	meta := nonEmpty(m.hostLabel(p.Host), m.sampleText(p.Session), appName(p.App))
	if t := m.linkedTaskForPane(p); t != nil {
		meta = append(meta, m.taskText(t, t.title))
	} else {
		meta = append(meta, m.text("未归类"))
	}
	left := homePanelTitle.Render(m.paneDisplayTitle(p)) + homeMuted.Render(" · "+strings.Join(meta, " · "))
	return marginPad + clip(left, max(0, gridW-margin-1))
}

func (m *model) attentionShortcuts(tier homeTier) [][2]string {
	shortcuts := [][2]string{
		{m.text("空格"), m.text("预览")},
		{"↵", m.text("跳转")},
		{"v", m.text("分组")},
		{"a", m.text("范围")},
		{"Esc", m.text("返回")},
	}
	limit := len(shortcuts)
	switch tier {
	case homeMedium:
		if m.width < 72 {
			limit = 3
		}
	case homeNarrow:
		limit = 2
		if m.width < 32 {
			limit = 1
		}
	}
	return shortcuts[:min(len(shortcuts), limit)]
}

func (m *model) homeAttentionMouse(msg tea.MouseMsg, at int) tea.Cmd {
	if msg.Button == tea.MouseButtonLeft {
		switch {
		case msg.Y == 0:
			return m.focusTabClick(msg.X)
		case msg.Y == 2:
			for _, span := range m.homeTopHits {
				if msg.X >= span.start && msg.X < span.end {
					return m.runAttentionBarCmd(span.cmd)
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
	if msg.Button == tea.MouseButtonRight && hit.pane != (paneIdentity{}) {
		m.clickListRow(hit, msg.X)
		return m.listCommand("o")
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if hit.pane != (paneIdentity{}) {
		same := m.paneSelection >= 0 && m.paneSelection < len(m.livePanes) && identityOf(m.livePanes[m.paneSelection]) == hit.pane
		m.clickListRow(hit, msg.X)
		if same {
			return m.listCommand("enter")
		}
		return nil
	}
	m.clickListRow(hit, msg.X)
	return nil
}

func (m *model) runAttentionBarCmd(cmd string) tea.Cmd {
	switch cmd {
	case "group:status":
		m.grouping = byStatus
		return nil
	case "group:session":
		m.grouping = bySession
		return nil
	case "group:agent":
		m.grouping = byAgent
		return nil
	case "group:host":
		m.grouping = byHost
		return nil
	case "scope:alerts":
		m.allPanes = false
		m.refreshPanes()
		return nil
	case "scope:all":
		m.allPanes = true
		m.refreshPanes()
		return nil
	default:
		return m.listCommand(cmd)
	}
}
