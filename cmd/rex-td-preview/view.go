package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

var (
	dim         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626C77", Dark: "#8595A4"})
	ink         = lipgloss.NewStyle().Bold(true)
	accent      = ink.Foreground(lipgloss.AdaptiveColor{Light: "#007580", Dark: "#42D9E1"})
	warn        = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#875200", Dark: "#F3C86C"})
	success     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#22633F", Dark: "#A8DCB8"})
	failure     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9C382A", Dark: "#FFB9A8"})
	selected    = ink.Reverse(true)
	rules       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C6CFD6", Dark: "#35414C"})
	taskAccents = [...]lipgloss.AdaptiveColor{
		{}, {Light: "#325A77", Dark: "#A7C5DD"}, {Light: "#346453", Dark: "#A4C9B7"},
		{Light: "#795D30", Dark: "#D4BF99"}, {Light: "#625474", Dark: "#C4B5D1"},
	}
)

func groupHeadingStyle(key groupKey) lipgloss.Style {
	if strings.HasPrefix(key.id, "host-session:") || strings.HasPrefix(key.id, "host-mixed-session:") {
		return dim.Bold(true)
	}
	return ink.Foreground(lipgloss.AdaptiveColor{Light: "#485966", Dark: "#B7C3CE"})
}

func (m *model) View() string {
	if m.focusPreview {
		m.homePlacements = m.homePlacements[:0]
		view := m.focusView()
		if m.homeGraphics != nil {
			view += homeFrame(m.width, m.height, m.homePlacements)
		}
		return view
	}
	w, h := m.width, m.height
	m.mouseRows = nil
	m.mouseResourceTitle = -1
	if w == 0 || h == 0 {
		return ""
	}
	if w < 22 || h < 8 {
		return clip(m.text("请扩大 Rex 侧栏  ·  q 退出"), w)
	}

	focus := [...]string{m.text("任务"), m.text("机器"), m.text("额度")}[m.focus]
	if m.focus == 0 && m.attention {
		focus = m.text("需要处理")
	}
	density := dim.Render("z") + " " + ink.Render(m.text(densityLabels[m.density]))
	if m.demo {
		density = dim.Render(m.text("演示")) + "  " + density
	}
	row := rules.Render(strings.Repeat("─", w))
	top := []string{
		inset(pair(accent.Render(m.text("+ 新建"))+"   "+dim.Render("f")+" "+focus, density, w-2)),
		row, inset(m.tabs(w - 2)), row,
	}
	if m.page == listPage && !m.editing {
		toggle := ""
		if m.attention {
			toggle = m.text("a 全部窗格")
			if m.allPanes {
				toggle = m.text("a 仅提醒")
			}
		}
		top = append(top, inset(pair(accent.Render("v")+m.text(" 按")+m.text(groupingLabels[m.grouping]), dim.Render(toggle), w-2)))
	}
	hints := inset(clip(m.hints(w-2), w-2))
	bottom := []string{row, hints}
	if m.page == detailPage && m.notice != "" {
		bottom = []string{row, inset(clip(accent.Render(m.notice), w-2)), hints}
	}
	if m.page == listPage && h >= 13 {
		bottom = []string{inset(m.summary(w - 2)), row}
		bottom = append(bottom, m.resources(w-2, h)...)
		bottom = append(bottom, row, hints)
		m.mouseResourceTitle = h - len(bottom) + 2
	}

	m.mouseBodyStart = len(top)
	body := m.body(w-2, max(1, h-len(top)-len(bottom)))
	return strings.Join(append(append(top, body...), bottom...), "\n")
}

func (m *model) tabRow(title string, count, width int, active bool) string {
	text := clip(fmt.Sprintf("%s %d", title, count), width)
	left := max(0, (width-ansi.StringWidth(text))/2)
	style := dim
	if active {
		style = selected
	}
	return strings.Repeat(" ", left) + style.Render(text) + strings.Repeat(" ", max(0, width-left-ansi.StringWidth(text)))
}

func (m *model) tabs(w int) string {
	title, count := m.text("任务"), len(m.tasks)
	if m.focusPreview {
		count = m.taskCount()
		if m.archivedView && !m.attention {
			title = m.text("归档")
		}
	}
	return m.tabRow(title, count, w/2, !m.attention) +
		m.tabRow(m.text("需要处理"), m.alertCount(), w-w/2, m.attention)
}

func (m *model) body(w, h int) []string {
	var screen listView
	switch m.page {
	case detailPage:
		if m.focusPreview {
			screen = m.focusDetails(w)
		} else {
			screen.lines, screen.active = m.details(w)
		}
	case taskMenuPage:
		screen = m.taskMenu(w)
	case taskDeletePage:
		screen = m.taskDeleteView(w)
	case taskPickPage:
		screen = m.taskPicker(w)
	case linkPage:
		screen = m.focusPicker(w)
	case scenePage:
		screen = m.paneSceneView(w)
	case helpPage:
		screen.lines = m.help(w)
	default:
		switch {
		case m.editing:
			screen.lines = []string{accent.Render(m.text("新建任务")), "", m.input.view(w), "", dim.Render(m.text("回车 保存  ·  Esc 取消"))}
			if m.focusPreview && m.candidateDraft == nil {
				title, prompt := m.editLabels()
				screen.lines = []string{"", ink.Render(title), "", dim.Render(prompt), m.input.view(w)}
				if m.notice != "" {
					screen.lines = append(screen.lines, dim.Render(m.notice))
				}
			}
			if c := m.candidateDraft; c != nil {
				screen.lines[0] = accent.Render(m.text("收录候选任务"))
				screen.lines[4] = dim.Render(m.text(c.source) + " · " + m.paneLocation(c.pane))
				screen.lines = append(screen.lines, dim.Render(m.notice))
			}
		case m.attention:
			screen = m.panes(w)
		default:
			if m.focusPreview {
				screen = m.focusTasks(w)
			} else {
				screen = m.taskRows(w)
			}
		}
	}
	offset := max(0, screen.active-h+1)
	if m.onCandidates() && h > 1 && m.candidateIndex() >= 0 {
		offset = max(offset, screen.active-h+2)
	}
	if m.page == scenePage {
		m.scene.offset = clamp(m.scene.offset, 0, max(0, len(screen.lines)-h))
		offset = m.scene.offset
	} else if m.page == helpPage {
		m.helpOffset = clamp(m.helpOffset, 0, max(0, len(screen.lines)-h))
		offset = m.helpOffset
	}
	limit := min(offset+h, len(screen.lines))
	if len(screen.hits) != 0 {
		m.mouseRows = screen.hits[offset:limit]
	}
	result := make([]string, h)
	for i, line := range screen.lines[offset:limit] {
		result[i] = inset(clip(line, w))
	}
	return result
}

func (m *model) taskRows(w int) listView {
	var screen listView
	if len(m.tasks) == 0 {
		screen.append("", listHit{})
		screen.append(dim.Render(m.text("暂无任务")), listHit{})
		screen.append(dim.Render(m.text("按 + 新建任务")), listHit{})
	} else {
		screen = m.listRows(m.taskGroups(), m.taskSelection, w, func(i, width, depth int, group groupKey) []string {
			return m.taskRow(i, width)
		})
	}
	m.appendCandidateRows(&screen, w)
	return screen
}

type listView struct {
	lines  []string
	hits   []listHit
	active int
}

func (v *listView) append(line string, hit listHit) {
	v.lines = append(v.lines, line)
	v.hits = append(v.hits, hit)
}

func (m *model) listRows(groups []listGroup, selection, w int, renderRow func(int, int, int, groupKey) []string) listView {
	var screen listView
	if m.attention && m.density > 0 && len(groups) > 0 {
		screen.append("", listHit{})
	}
	var renderGroups func([]listGroup, int)
	renderGroups = func(groups []listGroup, depth int) {
		indent := strings.Repeat(" ", depth*2)
		for i, group := range groups {
			if len(screen.lines) > 0 && screen.lines[len(screen.lines)-1] != "" && (depth == 0 || i > 0) {
				screen.append("", listHit{})
			}
			atHeader := group.folded && slices.Contains(group.rows, selection)
			if atHeader {
				screen.active = len(screen.lines)
			}
			screen.append(indent+m.groupHeading(group, atHeader && m.focus == 0 && !m.onCandidates(), w-len(indent)), listHit{groupID: group.key.id})
			switch {
			case group.folded:
			case len(group.children) != 0:
				if m.attention && m.density > 0 {
					screen.append("", listHit{})
				}
				renderGroups(group.children, depth+1)
			default:
				rowIndent := indent
				if m.attention {
					rowIndent += "    "
				}
				for _, row := range group.rows {
					if row == selection {
						screen.active = len(screen.lines)
					}
					hit := listHit{}
					if m.attention {
						hit.pane = identityOf(m.livePanes[row])
					} else {
						hit.task = m.tasks[row]
					}
					for i, line := range renderRow(row, w-len(rowIndent), depth, group.key) {
						if line != "" {
							line = rowIndent + line
						}
						lineHit := hit
						if line == "" {
							lineHit = listHit{}
						} else if i == 0 && hit.task != nil {
							lineHit.taskArrowEnd = 5 + len(indent)
						}
						screen.append(line, lineHit)
					}
				}
			}
		}
	}
	renderGroups(groups, 0)
	return screen
}

func (m *model) groupHeading(group listGroup, here bool, w int) string {
	marker := "▾"
	if group.folded {
		marker = "▸"
	}
	caret := " "
	if here {
		caret = "›"
	}
	title := group.key.title
	if group.key == pinnedGroup || group.key == unlinkedGroup {
		title = m.text(title)
	}
	if group.key == pinnedGroup {
		title = "★ " + title
	}
	style := groupHeadingStyle(group.key)
	if here {
		style = accent
	}
	return pair(style.Render(caret+" "+marker+" "+title), dim.Render(fmt.Sprint(len(group.rows))), w)
}

func (m *model) taskRow(i int, w int) []string {
	return m.taskRowWithProgress(i, w, "")
}

func (m *model) taskRowWithProgress(i, w int, progress string) []string {
	t := m.tasks[i]
	home := m.focusPreview && m.page == listPage && !m.attention && !m.archivedView
	here := i == m.taskSelection && !m.onCandidates() && (!home || (!m.boardSuggest && m.boardPane == (paneIdentity{})))
	caret, marker, pin := " ", "▸ ", ""
	if here && m.focus == 0 {
		caret = "›"
	}
	if m.expanded[t] {
		marker = "▾ "
	}
	if t.inPinnedSection() {
		pin = "★ "
	}
	right := m.text(stateLabels[t.state])
	if home && t.state == working {
		status := m.boardStatus(t)
		switch {
		case status.waiting > 0:
			right = fmt.Sprintf(m.text("%d 个等你"), status.waiting)
		case status.running:
			right = m.text("在跑")
		case status.idle > 0 && status.idle == status.total:
			right = fmt.Sprintf(m.text("%d 个会话 · 都空闲"), status.idle)
		}
	}
	if len(t.steps) > 0 && !(here && m.peek) && (!m.focusPreview || m.expanded[t]) {
		right = fmt.Sprintf("%d/%d", t.completed, len(t.steps))
	}
	check := ""
	if m.focusPreview {
		check = taskCheckbox(t) + " "
	}
	if progress != "" {
		right += " · " + progress
	}
	left := caret + " " + marker + check + pin + m.taskText(t, t.title)
	category := ""
	if t.kind != uncategorized && w >= 32 && (!m.focusPreview || m.density == 2) {
		category = m.text(kindLabels[t.kind])
	}
	var line string
	if here && m.focus == 0 {
		style := selected
		if m.focusPreview {
			style = focusHighlight
		}
		line = style.Render(pair(left, strings.Join(nonEmpty(category, right), "  "), w))
	} else {
		if t.color != 0 {
			left = lipgloss.NewStyle().Foreground(taskAccents[t.color]).Render(left)
		}
		meta := statusColor(stateLabels[t.state]).Render(right)
		if category != "" {
			meta = dim.Render(category) + "  " + meta
		}
		line = pair(left, meta, w)
	}
	lines := []string{line}
	if m.focusPreview {
		if home && (t.state == working || !m.expanded[t]) {
			return lines
		}
		if m.expanded[t] {
			lines = append(lines, m.focusSummary(t, w)...)
		} else if m.density == 2 {
			lines = append(lines, dim.Render(clip("    "+m.taskCaption(t), w)))
		}
		if m.density > 0 {
			lines = append(lines, "")
		}
		return lines
	}
	if m.expanded[t] {
		lines = append(lines, m.taskInline(t, w)...)
	} else if m.density > 0 {
		caption := m.taskText(t, t.note)
		if caption == "" {
			caption = m.taskCaption(t)
		}
		lines = append(lines, dim.Render(clip("    "+caption, w)))
	}
	if m.density == 2 {
		lines = append(lines, "")
	}
	return lines
}

func (m *model) taskInline(t *task, w int) []string {
	var lines []string
	if t.note != "" {
		lines = append(lines, dim.Render("    "+clip(m.taskText(t, t.note), w-4)))
	}
	if len(t.steps) != 0 {
		lines = append(lines, dim.Render(m.text("    子任务")))
	}
	for i, step := range t.steps {
		marker := "◐ "
		if i < t.completed {
			marker = "✔ "
		}
		lines = append(lines, dim.Render("    ├ "+marker+clip(m.taskText(t, step), w-8)))
	}
	if len(t.panes) != 0 {
		lines = append(lines, dim.Render(m.text("    已关联窗格")))
	}
	for _, pane := range t.panes {
		pane = m.currentPane(pane)
		lines = append(lines, pair("    ├ ↗ "+m.paneDisplayTitle(pane), dim.Render(m.hostLabel(pane.Host)), w))
	}
	if len(lines) > 8 {
		return append(lines[:7], dim.Render(m.text("     o 看全部")))
	}
	return lines
}

func (m *model) panes(w int) listView {
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
		return listView{lines: []string{"", dim.Render(text)}}
	}
	return m.listRows(m.paneGroups(), m.paneSelection, w, m.paneRow)
}

func (m *model) paneRow(i, w, depth int, group groupKey) []string {
	p := m.livePanes[i]
	title, state := m.paneDisplayTitle(p), p.Status
	if group == pinnedGroup {
		title = "★ " + title
	}
	if m.feed.HostState(p.Host.ID) == rex.Offline {
		state = "offline"
	}
	name := statusText(state)
	var lines []string
	if i == m.paneSelection && m.focus == 0 {
		style := selected
		if m.focusPreview {
			style = focusHighlight
		}
		lines = append(lines, style.Render(pair("› "+title, m.text(name), w)))
	} else {
		lines = append(lines, pair("  "+title, statusColor(name).Render(m.text(name)), w))
	}
	location := m.paneLocation(p)
	if i == m.paneSelection && m.peek {
		return append(lines, dim.Render(clip("    "+m.paneStatusPreview(p, state), w)))
	}
	if depth > 0 {
		location = appName(p.App)
	}
	if m.focusPreview && m.density < 2 {
		return lines
	}
	if m.density == 1 && p.Message != "" {
		lines = append(lines, dim.Render(clip("    "+m.sampleText(p.Message), w)))
	} else if m.density > 0 && location != "" {
		lines = append(lines, dim.Render(clip("    "+location, w)))
	}
	if m.density == 2 && p.Message != "" {
		lines = append(lines, dim.Render(clip("    "+m.sampleText(p.Message), w)))
	}
	return lines
}

func (m *model) details(w int) ([]string, int) {
	t := m.selected()
	if t == nil {
		return nil, 0
	}
	lines := []string{ink.Render(clip(m.taskText(t, t.title), w)), pair(dim.Render(m.taskCaption(t)), statusColor(stateLabels[t.state]).Render(m.text(stateLabels[t.state])), w), ""}
	lines = append(lines, dim.Render(m.text("t 类型  ")+m.text(kindLabels[t.kind])))
	if t.note != "" {
		lines = append(lines, wrap(m.taskText(t, t.note), w)...)
		lines = append(lines, "")
	}
	if len(t.steps) != 0 {
		lines = append(lines, accent.Render(m.text("子任务")))
		for i, step := range t.steps {
			marker := "◐ "
			if i < t.completed {
				marker = "✔ "
			}
			lines = append(lines, "  "+marker+clip(m.taskText(t, step), w-4))
		}
	}
	active := 0
	addLink := func(index int, title, location string) {
		header := "  ↗ " + title
		if index == m.detailSelection {
			active = len(lines)
			header = selected.Render(fill("› ↗ "+title, w))
		}
		lines = append(lines, header, dim.Render("    "+clip(location, w-4)))
	}
	if len(t.panes) != 0 {
		lines = append(lines, accent.Render(m.text("已关联窗格")))
		for i, pane := range t.panes {
			pane = m.currentPane(pane)
			addLink(i, m.paneDisplayTitle(pane), m.paneLocation(pane))
		}
	}
	if t.result != "" {
		lines = append(lines, accent.Render(m.text("结果")))
		lines = append(lines, wrap(m.taskText(t, t.result), w)...)
	}
	lines = append(lines, "", dim.Render(m.text("标题色  ")+m.text(colorLabels[t.color])))
	return lines, active
}

func (m *model) help(w int) []string {
	groups := [][]string{
		{"任务",
			"+ 新建 · / 搜索 · 4 或 A 查看归档",
			"s 切换状态 · e 编辑下一步 · N 以任务名开 Rex 标签",
			"x 完成或重开 · m 任务菜单 · p 置顶或取消置顶",
			"在任务菜单选择删除，再按 y 确认"},
		{"关联会话",
			"Enter 跳到窗格 · → 看状态和详情 · 空格 预览终端",
			"在预览中滚动查看前文，空格或 Esc 返回，Enter 进入窗格",
			"在任务详情选中空闲且未关联的会话，按 Enter 关联",
			"m 移到其他任务或移出任务",
			"在任务详情选中不可用会话，按 u 解除关联",
			"在「还有 N 个会话」行按 Enter 展开或收起",
			"在待处理页按 v 切换按状态／会话／工具／主机分组，a 切换待看与全部"},
		{"未归类",
			"选中未归类行后，下方直接显示未归类会话，按 → 或 Enter 进入",
			"选中会话后，空格预览，Enter 选择任务并归入"},
		{"全局",
			"1/2/3/4 或 Tab 切换现在、待办、已完成、归档 · v 切换分页与总览 · n 查看待处理",
			"↑↓ 选任务 · → 选会话 · Enter 看详情",
			"f 切换机器与额度 · Shift+L 切换语言",
			"? 帮助 · Esc 返回 · q 退出"},
	}
	lines := []string{accent.Render(m.text("rextd · 帮助"))}
	for _, group := range groups {
		lines = append(lines, "", ink.Render(m.text(group[0])))
		for _, line := range group[1:] {
			for _, part := range strings.Split(ansi.Wrap(m.text(line), w-2, ""), "\n") {
				lines = append(lines, "  "+part)
			}
		}
	}
	return lines
}

func (m *model) paneStatusPreview(p rex.Pane, status string) string {
	detail := m.sampleText(p.Message)
	if detail == "" {
		detail = m.paneLocation(p)
	}
	return strings.Join(nonEmpty(m.text("状态 ")+m.text(statusText(status)), detail), " · ")
}

func (m *model) summary(w int) string {
	if m.notice != "" {
		return clip(accent.Render(m.notice), w)
	}
	if m.attention {
		label := m.text("全部机器")
		for _, host := range m.feed.Hosts {
			if host.ID == m.hostFilter {
				label = m.hostLabel(host.Host)
				break
			}
		}
		scope := m.text("提醒")
		if m.allPanes {
			scope = m.text("全部窗格")
		}
		return pair(dim.Render(label), dim.Render(fmt.Sprintf("%s %d", scope, len(m.livePanes))), w)
	}
	if t := m.selected(); t != nil {
		return pair(dim.Render(m.taskCaption(t)), statusColor(stateLabels[t.state]).Render(m.text(stateLabels[t.state])), w)
	}
	return clip(dim.Render(m.storageLabel()), w)
}

func (m *model) storageLabel() string {
	if m.store != nil {
		return m.text("任务保存在本机")
	}
	return m.text("任务只在本次运行中保留")
}

func (m *model) taskCaption(t *task) string {
	results := t.resultCount
	if t.result != "" {
		results = max(1, results)
	}
	return fmt.Sprintf(m.text("%d 个窗格 · %d 份结果"), len(t.panes), results)
}

func (m *model) hints(width int) string {
	if m.editing {
		if m.candidateDraft != nil {
			return renderShortcuts(width, [][2]string{{"↵", m.text("收录")}, {"Esc", m.text("取消")}})
		}
		return dim.Render(m.text("回车 保存  ·  Esc 取消"))
	}
	if m.page == detailPage {
		if t := m.selected(); t != nil && len(t.panes) != 0 {
			return accent.Render("↑↓") + m.text(" 选窗格   ") + accent.Render("↵") + m.text(" 跳转   ") + dim.Render(m.text("Esc 返回"))
		}
		return accent.Render("s") + m.text(" 状态   ") + accent.Render("b") + m.text(" 标题色   ") + accent.Render("p") + m.text(" 置顶   ") + dim.Render(m.text("Esc 返回"))
	}
	if m.page == scenePage {
		if m.scene.editing {
			return renderShortcuts(width, [][2]string{{"Esc", m.text("取消")}, {"↵", m.text("保存")}})
		}
		watch := m.text("观察")
		if m.candidateWatches[identityOf(m.scene.pane)] {
			watch = m.text("停止观察")
		}
		return renderShortcuts(width, [][2]string{{"↵", m.text("跳转")}, {"d", m.text("发现")}, {"w", watch}, {"Esc", m.text("返回")}})
	}
	if m.page == helpPage {
		return renderShortcuts(width, [][2]string{{"↑↓", m.text("滚动")}, {"Esc", m.text("返回")}})
	}
	if m.page != listPage {
		return accent.Render("Esc") + m.text(" 返回   ") + dim.Render(m.text("q 退出"))
	}
	if m.focus == 1 && !m.demo {
		return accent.Render("↵") + m.text(" 筛选   ") + accent.Render("↑↓") + m.text(" 选择   ") + dim.Render(m.text("Esc 返回"))
	}
	if m.focus != 0 {
		return accent.Render("↑↓") + m.text(" 滚动   ") + accent.Render("f") + m.text(" 切区   ") + dim.Render(m.text("Esc 返回"))
	}
	if m.onCandidates() {
		if m.candidateIndex() >= 0 {
			return renderShortcuts(width, [][2]string{{"↵", m.text("收录")}, {"i", m.text("忽略")}, {"←", m.text("收起")}})
		}
		return renderShortcuts(width, [][2]string{{"↵", m.text("展开／收起")}, {"↑", m.text("返回任务")}})
	}
	return m.listShortcuts(width)
}

func (m *model) listShortcuts(width int) string {
	shortcuts := [][2]string{{"→", m.text("详情")}, {"↵", m.text("跳转")}, {m.text("空格"), m.text("状态")}, {"p", m.text("置顶")}}
	if m.focusPreview && m.attention {
		shortcuts[2][1] = m.text("预览")
	}
	return renderShortcuts(width, shortcuts)
}

func renderShortcuts(width int, shortcuts [][2]string) string {
	center := lipgloss.NewStyle().Align(lipgloss.Center)
	var row strings.Builder
	for i, shortcut := range shortcuts {
		columns := (i+1)*width/len(shortcuts) - i*width/len(shortcuts)
		key, label := shortcut[0], shortcut[1]
		if (key == "空格" || key == "Space") && columns < ansi.StringWidth(key+" "+label) {
			key = "␣"
		}
		gap := " "
		if ansi.StringWidth(key+gap+label) > columns {
			gap = ""
		}
		row.WriteString(center.Width(columns).Render(accent.Render(key) + gap + label))
	}
	return row.String()
}

func statusText(s string) string {
	switch s {
	case "working":
		return "进行中"
	case "idle":
		return "空闲"
	case "error":
		return "出错"
	case "permission":
		return "待批准"
	case "question":
		return "待回答"
	case "auth":
		return "需登录"
	case "waiting":
		return "等待"
	case "done":
		return "本轮结束"
	case "offline":
		return "失联"
	default:
		return "未上报"
	}
}

func (m *model) status(s string) string {
	text := statusText(s)
	return statusColor(text).Render(m.text(text))
}

func statusColor(s string) lipgloss.Style {
	switch s {
	case "出错":
		return failure
	case "待批准", "待回答", "需登录", "等待":
		return warn
	case "完成", "已完成", "本轮结束":
		return success
	case "进行中":
		return accent
	default:
		return dim
	}
}

func clip(text string, width int) string {
	return ansi.Truncate(text, max(0, width), "…")
}

func fill(text string, width int) string {
	text = clip(text, width)
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func pair(left, right string, width int) string {
	right = clip(right, width)
	size := ansi.StringWidth(right)
	gap := max(0, min(2, width-size))
	return fill(left, width-size-gap) + strings.Repeat(" ", gap) + right
}

func inset(s string) string {
	return " " + s
}

func wrap(s string, width int) []string {
	return strings.Split(ansi.Hardwrap(s, width, false), "\n")
}
