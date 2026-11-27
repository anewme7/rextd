package main

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

type listHit struct {
	detailUnassigned bool
	boardSuggest     bool
	boardOffer       bool
	boardMore        bool
	homeIcon         homeIcon
	overviewSection  bool
	sectionTab       taskTab
	action           string
	groupID          string
	task             *task
	pane             paneIdentity
	taskArrowEnd     int
	taskCheckX       int
	taskMore         bool
	candidateHeader  bool
	candidate        paneIdentity
}

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if m.focusPreview {
		return m.focusMouse(msg)
	}
	if msg.Action != tea.MouseActionPress || m.width < 22 || m.height < 8 ||
		msg.X < 1 || msg.X >= m.width-1 || msg.Y < 0 || msg.Y >= m.height {
		return nil
	}
	if m.editing {
		if m.candidateDraft != nil && msg.Button == tea.MouseButtonLeft && msg.Y == m.height-1 {
			key := tea.KeyEsc
			if m.shortcutAt(msg.X, 2) == 0 {
				key = tea.KeyEnter
			}
			m.edit(tea.KeyMsg{Type: key})
		}
		return nil
	}
	row := msg.Y - m.mouseBodyStart
	inList := m.page == listPage && row >= 0 && row < len(m.mouseRows)
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		if inList || (m.page == scenePage && msg.Y >= m.mouseBodyStart && msg.Y < m.height-2) {
			m.focus = 0
			key := "down"
			if msg.Button == tea.MouseButtonWheelUp {
				key = "up"
			}
			return m.listCommand(key)
		}
	case tea.MouseButtonRight:
		if inList && m.attention && m.mouseRows[row].pane != (paneIdentity{}) {
			m.clickListRow(m.mouseRows[row], msg.X)
			return m.listCommand("o")
		}
	case tea.MouseButtonLeft:
		if msg.Y == 2 {
			m.cancelPaneTurn()
			m.page = listPage
			m.attention = msg.X >= 1+(m.width-2)/2
			m.focus, m.peek = 0, false
			return nil
		}
		if m.page == scenePage && msg.Y == m.height-1 {
			return m.listCommand([...]string{"enter", "d", "w", "esc"}[m.shortcutAt(msg.X, 4)])
		}
		if m.page == scenePage && row >= 0 && row < len(m.mouseRows) && m.mouseRows[row].action == "y" {
			return m.listCommand("y")
		}
		if m.page != listPage {
			return nil
		}
		switch {
		case msg.Y == 0 && msg.X < 7:
			return m.listCommand("+")
		case msg.Y == 4:
			if msg.X < 1+(m.width-2)/2 {
				return m.listCommand("v")
			}
			if m.attention {
				return m.listCommand("a")
			}
		case msg.Y == m.mouseResourceTitle:
			width := m.width - 2
			split := width / 2
			if width >= resourceSplitWidth {
				split, _ = resourceWidths(width)
			}
			focus := 1
			if msg.X >= 1+split {
				focus = 2
			}
			if m.focus == focus {
				focus = 0
			}
			m.focus, m.peek = focus, false
		case msg.Y == m.height-1 && m.focus == 0:
			if m.onCandidates() {
				if m.candidateIndex() >= 0 {
					return m.listCommand([...]string{"enter", "i", "left"}[m.shortcutAt(msg.X, 3)])
				}
				return m.listCommand([...]string{"enter", "up"}[m.shortcutAt(msg.X, 2)])
			}
			keys := [...]string{"o", "enter", " ", "p"}
			return m.listCommand(keys[m.shortcutAt(msg.X, len(keys))])
		case inList:
			m.clickListRow(m.mouseRows[row], msg.X)
		}
	}
	return nil
}

func (m *model) shortcutAt(x, count int) int {
	for i := 0; i < count-1; i++ {
		if x < 1+(i+1)*(m.width-2)/count {
			return i
		}
	}
	return count - 1
}

func (m *model) clickListRow(hit listHit, x int) {
	switch {
	case hit.candidateHeader:
		m.candidatesFocused, m.focus, m.peek = true, 0, false
		m.candidatesOpen, m.candidatePane = !m.candidatesOpen, paneIdentity{}
	case hit.candidate != (paneIdentity{}):
		m.candidatesFocused, m.candidatePane, m.focus, m.peek = true, hit.candidate, 0, false
	case hit.groupID != "":
		if !m.attention {
			m.candidatesFocused = false
		}
		groups, selection, folded := m.taskGroups, &m.taskSelection, m.foldedTasks
		if m.attention {
			groups, selection, folded = m.paneGroups, &m.paneSelection, m.foldedPanes
		}
		if group := findGroup(groups(), hit.groupID); group != nil {
			if !slices.Contains(group.rows, *selection) {
				*selection = group.firstVisibleRow()
			}
			toggleGroup(folded, group.key)
			m.focus, m.peek = 0, false
		}
	case !m.attention && hit.task != nil:
		if i := slices.Index(m.tasks, hit.task); i >= 0 {
			m.candidatesFocused = false
			m.taskSelection, m.focus, m.peek = i, 0, false
			if x < hit.taskArrowEnd {
				m.expanded[hit.task] = !m.expanded[hit.task]
			}
		}
	case m.attention && hit.pane != (paneIdentity{}):
		for i, pane := range m.livePanes {
			if identityOf(pane) == hit.pane {
				m.paneSelection, m.focus, m.peek = i, 0, false
				break
			}
		}
	}
}

func findGroup(groups []listGroup, id string) *listGroup {
	for i := range groups {
		if groups[i].key.id == id {
			return &groups[i]
		}
		if group := findGroup(groups[i].children, id); group != nil {
			return group
		}
	}
	return nil
}
