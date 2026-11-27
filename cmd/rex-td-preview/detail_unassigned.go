package main

import (
	"slices"
	"time"

	"rex-td/rex"
)

func (m *model) idleUnassignedPanes(t *task) []rex.Pane {
	if m.archivedTask(t) {
		return nil
	}
	var panes []rex.Pane
	for _, p := range m.feed.Panes {
		if p.Status == "idle" && p.ID != "" && p.SessionID != "" && p.ID != m.rexContext.SidebarID &&
			m.feed.HostState(p.Host.ID) != rex.Offline && !m.linkedPaneID(identityOf(p)) {
			panes = append(panes, p)
		}
	}
	return panes
}

func (m *model) appendDetailUnassigned(v *listView, t *task, panes []rex.Pane, w int) {
	if len(panes) == 0 {
		return
	}
	v.append("", listHit{})
	v.append(pair(dim.Render(m.text("空闲且未关联")), dim.Render(m.text("↑↓ 选择")), w), listHit{})
	if len(t.panes) == 0 && m.detailIdlePane == (paneIdentity{}) {
		v.active = len(v.lines)
	}
	for _, pane := range panes {
		id := identityOf(pane)
		row := pair("  "+m.paneDisplayTitle(pane), m.status(pane.Status), w)
		if m.detailIdlePane == id {
			v.active = len(v.lines)
			row = focusHighlight.Render(pair("› "+m.paneDisplayTitle(pane), m.text("空闲"), w))
		}
		hit := listHit{pane: id, detailUnassigned: true}
		v.append(row, hit)
		v.append(dim.Render("    "+m.paneLocation(pane)), hit)
	}
}

func (m *model) moveDetailSelection(t *task, delta int) {
	order, idle := m.orderedAssociations(t), m.idleUnassignedPanes(t)
	if len(order)+len(idle) == 0 {
		m.detailIdlePane = paneIdentity{}
		return
	}
	at := slices.Index(order, m.detailSelection)
	if id := m.detailIdlePane; id != (paneIdentity{}) {
		at = len(order) + slices.IndexFunc(idle, func(p rex.Pane) bool { return identityOf(p) == id })
	}
	next := clamp(at+delta, 0, len(order)+len(idle)-1)
	m.detailIdlePane = paneIdentity{}
	if next < len(order) {
		m.detailSelection = order[next]
	} else {
		m.detailIdlePane = identityOf(idle[next-len(order)])
	}
}

func (m *model) linkDetailUnassigned(t *task) {
	if t == nil {
		return
	}
	panes := m.idleUnassignedPanes(t)
	i := slices.IndexFunc(panes, func(p rex.Pane) bool { return identityOf(p) == m.detailIdlePane })
	if i < 0 {
		m.notice = m.text("窗格已变化，请重新选择")
		return
	}
	pane := panes[i]
	binding := paneBinding{source: linkManual, linkedAt: time.Now().UnixMilli()}
	if m.store != nil {
		added, err := m.store.bindNewPane(t.id, pane, binding, false)
		if !m.saved(err) {
			return
		}
		if !added {
			m.notice = m.text(errPaneLinkChanged.Error())
			return
		}
	}
	t.addBinding(pane, binding)
	m.detailSelection, m.detailIdlePane = len(t.panes)-1, paneIdentity{}
	delete(m.removedAgents, identityOf(pane))
	m.touchTask(t)
	m.refreshCandidates()
	m.notice = m.text("已关联 · ") + m.paneDisplayTitle(pane)
}
