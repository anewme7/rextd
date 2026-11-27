package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type taskTab int

const (
	nowTab taskTab = iota
	todoTab
	doneTab
	archiveTab
)

func tabForState(state taskState) taskTab {
	switch state {
	case working:
		return nowTab
	case done, cancelled:
		return doneTab
	default:
		return todoTab
	}
}

func (m *model) tabForTask(t *task) taskTab {
	if m.archivedTask(t) {
		return archiveTab
	}
	return tabForState(t.state)
}

func (m *model) followTask(t *task) {
	if m.archivedTask(t) {
		m.homeTab = archiveTab
		m.archivedView = true
	} else {
		m.homeTab = tabForState(t.state)
		m.archivedView = false
	}
	if t.state == paused || t.state == cancelled {
		m.secondaryOpen[m.homeTab] = true
	}
	if m.homeOverview {
		m.overviewFolded[m.tabForTask(t)] = false
	}
	m.overviewSectionSelected = false
	m.secondarySelected = false
	m.boardOfferSelected = false
}

func (m *model) setHomeOverview(overview bool) {
	if m.homeOverview == overview {
		return
	}
	current := m.homeSelection()
	m.homeOverview = overview
	m.attention, m.focus = false, 0
	if overview {
		m.archivedView = false
		switch {
		case current.task != nil:
			m.overviewFolded[m.tabForTask(current.task)] = false
			m.selectHome(boardEntry{task: current.task})
		case current.offer || current.suggest:
			m.overviewFolded[nowTab] = false
			m.selectHome(boardEntry{offer: true})
		default:
			m.jumpOverviewSection(m.homeTab)
		}
		return
	}
	switch {
	case current.task != nil:
		m.followTask(current.task)
		m.selectHome(boardEntry{task: current.task})
	case current.overviewSection:
		m.switchHomeTab(current.sectionTab)
	case current.offer || current.suggest:
		m.switchHomeTab(nowTab)
		if len(m.newAgents) > 0 {
			m.selectHome(boardEntry{offer: true})
		}
	default:
		m.switchHomeTab(m.homeTab)
	}
}

func (m *model) currentOverviewTab() taskTab {
	if m.overviewSectionSelected {
		return m.overviewSectionTab
	}
	if entry := m.homeSelection(); entry.task != nil {
		return m.tabForTask(entry.task)
	}
	return nowTab
}

func (m *model) jumpOverviewSection(tab taskTab) {
	m.overviewFolded[tab] = false
	rows := m.overviewTasksFor(tab)
	if len(rows) > 0 {
		m.selectHome(boardEntry{task: m.tasks[rows[0]]})
		return
	}
	if tab == nowTab && len(m.newAgents) > 0 {
		m.selectHome(boardEntry{offer: true})
		return
	}
	m.selectHome(boardEntry{overviewSection: true, sectionTab: tab})
}

func (m *model) overviewTasksFor(tab taskTab) []int {
	switch tab {
	case nowTab:
		var rows []int
		for i, t := range m.tasks {
			if !m.archivedTask(t) && t.state == working {
				rows = append(rows, i)
			}
		}
		waiting := make(map[int]int, len(rows))
		for _, row := range rows {
			waiting[row] = m.boardStatus(m.tasks[row]).waiting
		}
		slices.SortStableFunc(rows, func(a, b int) int {
			if (waiting[a] > 0) != (waiting[b] > 0) {
				return cmp.Compare(waiting[b], waiting[a])
			}
			return 0
		})
		return rows
	case todoTab:
		var primary, secondary []int
		for i, t := range m.tasks {
			if m.archivedTask(t) {
				continue
			}
			if t.state == todo {
				primary = append(primary, i)
			} else if t.state == paused {
				secondary = append(secondary, i)
			}
		}
		return append(primary, secondary...)
	case doneTab:
		var primary, secondary []int
		for i, t := range m.tasks {
			if m.archivedTask(t) {
				continue
			}
			if t.state == done {
				primary = append(primary, i)
			} else if t.state == cancelled {
				secondary = append(secondary, i)
			}
		}
		slices.SortStableFunc(primary, func(a, b int) int {
			return cmp.Compare(m.tasks[b].completionOrder, m.tasks[a].completionOrder)
		})
		return append(primary, secondary...)
	case archiveTab:
		var rows []int
		for i, t := range m.tasks {
			parent := m.parentOf(t)
			if t.archived && (parent == nil || !parent.archived) {
				rows = append(rows, i)
			}
		}
		slices.SortStableFunc(rows, func(a, b int) int {
			return cmp.Compare(m.tasks[b].completionOrder, m.tasks[a].completionOrder)
		})
		return rows
	}
	return nil
}

func (m *model) sessionPanelTask() *task {
	entry := m.homeSelection()
	if m.onCandidates() || entry.task == nil || entry.overviewSection {
		return nil
	}
	if m.homeOverview {
		return entry.task
	}
	if m.homeTab == archiveTab {
		if !m.archivedTask(entry.task) {
			return nil
		}
		return entry.task
	}
	if m.archivedTask(entry.task) || tabForState(entry.task.state) != m.homeTab {
		return nil
	}
	return entry.task
}

func (m *model) isSecondary(group listGroup) bool {
	return group.key.id == "paused" || group.key.id == "cancelled"
}

func (m *model) switchHomeTab(tab taskTab) {
	m.homeScrollManual = false
	if tab != archiveTab {
		m.lastActiveTab = tab
	}
	m.homeTab = tab
	m.archivedView = tab == archiveTab
	m.attention, m.focus = false, 0
	m.selectHome(boardEntry{})
	m.selectVisibleTask()
	if m.selected() == nil {
		groups := m.taskGroups()
		if len(groups) > 0 {
			m.selectHome(boardEntry{secondary: true})
		} else if tab == nowTab && len(m.newAgents) > 0 {
			m.selectHome(boardEntry{offer: true})
		} else if tab == nowTab && len(m.candidates) > 0 {
			m.selectHome(boardEntry{candidate: true})
		}
	}
}

var homeTabNames = [...]string{"现在", "待办", "已完成", "归档"}

func (m *model) tabLabel(i int) string {
	name := m.text(homeTabNames[i])
	if m.width < 38 {
		return name
	}
	return fmt.Sprintf("%d %s", i+1, name)
}

func (m *model) homeMargin() int {
	if homeWidth(m.width) == homeNarrow {
		return 1
	}
	return 2
}

func (m *model) homeTabGap() int {
	if homeWidth(m.width) == homeNarrow {
		if m.width < 34 {
			return 1
		}
		return 2
	}
	return 4
}

func (m *model) homeTabs() string {
	var items []string
	for i := range homeTabNames {
		label := m.tabLabel(i)
		if taskTab(i) == m.homeTab && !m.attention {
			label = homeBlue.Bold(true).Render(label)
		} else {
			label = homeMuted.Render(label)
		}
		items = append(items, label)
	}
	return strings.Join(items, strings.Repeat(" ", m.homeTabGap()))
}

func (m *model) clickedTab(x int) (taskTab, bool) {
	start := m.homeMargin()
	gap := m.homeTabGap()
	for i := range homeTabNames {
		end := start + ansi.StringWidth(m.tabLabel(i))
		if x >= start && x < end {
			return taskTab(i), true
		}
		start = end + gap
	}
	return 0, false
}

func (m *model) alertSummary() string {
	return m.homeAlertSummary(false)
}

func (m *model) homeAlertSummary(compact bool) string {
	waiting, results := 0, 0
	for _, p := range m.feed.Panes {
		if !m.isAlert(p) {
			continue
		}
		if p.Status == "done" {
			results++
		} else {
			waiting++
		}
	}
	var parts []string
	if waiting > 0 {
		if compact {
			parts = append(parts, fmt.Sprintf(m.text("%d 需回应"), waiting))
		} else {
			parts = append(parts, fmt.Sprintf(m.text("%d 个需回应"), waiting))
		}
	}
	if results > 0 {
		if compact {
			parts = append(parts, fmt.Sprintf(m.text("%d 待看"), results))
		} else {
			parts = append(parts, fmt.Sprintf(m.text("%d 个结果待看"), results))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "● " + strings.Join(parts, " · ") + " (n)"
}
