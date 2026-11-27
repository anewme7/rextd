package main

import (
	"cmp"
	"slices"
	"strings"

	"rex-td/rex"
)

type grouping int

const (
	bySession grouping = iota
	byAgent
	byHost
	byStatus
)

var groupingLabels = [...]string{"会话", "Agent", "主机", "状态"}
var mixedLabels = [...]string{"跨会话", "跨 Agent", "跨主机", "多状态"}

type groupKey struct{ id, title string }

var (
	pinnedGroup   = groupKey{"pinned", "全局置顶"}
	unlinkedGroup = groupKey{"unlinked", "未关联"}
)

type listGroup struct {
	key      groupKey
	rows     []int
	children []listGroup
	folded   bool
}

func (group listGroup) firstVisibleRow() int {
	for !group.folded && len(group.children) != 0 {
		group = group.children[0]
	}
	return group.rows[0]
}

func addGroup(groups *[]listGroup, key groupKey, row int) *listGroup {
	i := slices.IndexFunc(*groups, func(group listGroup) bool { return group.key.id == key.id })
	if i < 0 {
		i = len(*groups)
		*groups = append(*groups, listGroup{key: key})
	}
	group := &(*groups)[i]
	group.rows = append(group.rows, row)
	return group
}

func orderGroups(groups []listGroup, folded map[string]bool) []listGroup {
	for i := range groups {
		groups[i].folded = folded[groups[i].key.id]
		orderGroups(groups[i].children, folded)
	}
	slices.SortStableFunc(groups, func(a, b listGroup) int {
		switch {
		case a.key == b.key:
			return 0
		case a.key == pinnedGroup:
			return -1
		case b.key == pinnedGroup:
			return 1
		case a.key.id == "host:local":
			return -1
		case b.key.id == "host:local":
			return 1
		case a.key == unlinkedGroup:
			return 1
		case b.key == unlinkedGroup:
			return -1
		}
		if strings.HasPrefix(a.key.id, "status:") && strings.HasPrefix(b.key.id, "status:") {
			return strings.Compare(a.key.id, b.key.id)
		}
		if order := strings.Compare(a.key.title, b.key.title); order != 0 {
			return order
		}
		return strings.Compare(a.key.id, b.key.id)
	})
	return groups
}

func moveInGroups(groups []listGroup, row, delta int) int {
	var order []int
	var collect func([]listGroup)
	collect = func(groups []listGroup) {
		for _, group := range groups {
			switch {
			case group.folded:
				if slices.Contains(group.rows, row) {
					row = group.rows[0]
				}
				order = append(order, group.rows[0])
			case len(group.children) != 0:
				collect(group.children)
			default:
				order = append(order, group.rows...)
			}
		}
	}
	collect(groups)
	if len(order) == 0 {
		return 0
	}
	return order[clamp(slices.Index(order, row)+delta, 0, len(order)-1)]
}

func (m *model) sessionKey(p rex.Pane, underHost bool) groupKey {
	title := m.sampleText(p.Session)
	if title == "" {
		title = m.text("会话名称不可用")
	}
	prefix := "session:"
	if underHost {
		prefix = "host-session:"
	} else {
		title = strings.Join(nonEmpty(title, m.hostLabel(p.Host)), " · ")
	}
	return groupKey{prefix + p.Host.ID + "/" + p.SessionID, title}
}

func (m *model) groupForPane(p rex.Pane) groupKey {
	switch m.grouping {
	case byStatus:
		switch homeStatus(p.Status) {
		case homeWaiting, homeFailed:
			return groupKey{"status:1-waiting", m.text("需回应")}
		case homeFinished:
			return groupKey{"status:2-done", m.text("本轮完成")}
		case homeRunning:
			return groupKey{"status:3-running", m.text("工作中")}
		default:
			return groupKey{"status:4-quiet", m.text("空闲与不可用")}
		}
	case byAgent:
		if p.App == "" {
			return groupKey{"agent:", m.text("其他窗格")}
		}
		return groupKey{"agent:" + p.App, appName(p.App)}
	case byHost:
		title := p.Host.Label
		if p.Host.ID == "local" {
			title = m.text("本机")
			if name := m.feed.LocalName; name != "" && name != "本机" && name != title {
				title = name + "·" + title
			}
		}
		if title == "" {
			title = m.text("未知主机")
		}
		return groupKey{"host:" + p.Host.ID, title}
	default:
		return m.sessionKey(p, false)
	}
}

func (m *model) groupForTask(t *task) groupKey {
	if t.pinned {
		return pinnedGroup
	}
	if len(t.panes) == 0 {
		return unlinkedGroup
	}
	key := m.groupForPane(m.currentPane(t.panes[0]))
	for _, p := range t.panes[1:] {
		if m.groupForPane(m.currentPane(p)).id != key.id {
			return groupKey{"mixed:" + groupingLabels[m.grouping], m.text(mixedLabels[m.grouping])}
		}
	}
	return key
}

func (m *model) taskSession(t *task, parent groupKey) groupKey {
	key := m.sessionKey(m.currentPane(t.panes[0]), true)
	for _, pane := range t.panes[1:] {
		if m.sessionKey(m.currentPane(pane), true).id != key.id {
			return groupKey{"host-mixed-session:" + parent.id, m.text("跨会话")}
		}
	}
	return key
}

func hostGroup(key groupKey) bool { return strings.HasPrefix(key.id, "host:") }

func (m *model) taskGroups() []listGroup {
	var groups []listGroup
	if m.focusPreview {
		return m.homeTaskGroups()
	}
	for i, t := range m.tasks {
		key := m.groupForTask(t)
		parent := addGroup(&groups, key, i)
		if hostGroup(key) {
			addGroup(&parent.children, m.taskSession(t, key), i)
		}
	}
	return orderGroups(groups, m.foldedTasks)
}

func (m *model) homeTaskGroups() []listGroup {
	if m.homeTab == archiveTab {
		archived := listGroup{key: groupKey{"archived", "已归档"}}
		for i, t := range m.tasks {
			parent := m.parentOf(t)
			if t.archived && (parent == nil || !parent.archived) {
				archived.rows = append(archived.rows, i)
			}
		}
		slices.SortStableFunc(archived.rows, func(a, b int) int {
			return cmp.Compare(m.tasks[b].completionOrder, m.tasks[a].completionOrder)
		})
		if len(archived.rows) == 0 {
			return nil
		}
		return []listGroup{archived}
	}
	groups := [...]listGroup{
		working:   {key: groupKey{"working", "进行中"}},
		paused:    {key: groupKey{"paused", "暂缓"}},
		todo:      {key: groupKey{"todo", "待做"}},
		done:      {key: groupKey{"done", "已完成"}},
		cancelled: {key: groupKey{"cancelled", "已取消"}},
	}
	for i, t := range m.tasks {
		if tabForState(t.state) == m.homeTab && !m.archivedTask(t) {
			groups[t.state].rows = append(groups[t.state].rows, i)
		}
	}
	waiting := make(map[int]int, len(groups[working].rows))
	for _, row := range groups[working].rows {
		waiting[row] = m.boardStatus(m.tasks[row]).waiting
	}
	slices.SortStableFunc(groups[working].rows, func(a, b int) int {
		if (waiting[a] > 0) != (waiting[b] > 0) {
			return cmp.Compare(waiting[b], waiting[a])
		}
		return 0
	})
	slices.SortStableFunc(groups[done].rows, func(a, b int) int { return cmp.Compare(m.tasks[b].completionOrder, m.tasks[a].completionOrder) })
	order := []listGroup{groups[working], groups[todo], groups[paused], groups[done], groups[cancelled]}
	return slices.DeleteFunc(order, func(group listGroup) bool { return len(group.rows) == 0 })
}

func (m *model) groupForAttention(p rex.Pane) groupKey {
	if m.panePins[identityOf(p)] {
		return pinnedGroup
	}
	return m.groupForPane(p)
}

func (m *model) paneGroups() []listGroup {
	var groups []listGroup
	for i, p := range m.livePanes {
		key := m.groupForAttention(p)
		parent := addGroup(&groups, key, i)
		if hostGroup(key) {
			addGroup(&parent.children, m.sessionKey(p, true), i)
		}
	}
	if m.focusPreview {
		// Drawing, keyboard navigation and folding share this order.
		sortRows := func(rows []int) {
			slices.SortStableFunc(rows, func(a, b int) int {
				return cmp.Compare(homeStatus(m.livePanes[a].Status), homeStatus(m.livePanes[b].Status))
			})
		}
		for _, group := range groups {
			sortRows(group.rows)
			for _, child := range group.children {
				sortRows(child.rows)
			}
		}
	}
	return orderGroups(groups, m.foldedPanes)
}

func (m *model) selectedGroup(parent bool) (map[string]bool, groupKey) {
	if m.attention {
		if m.paneSelection < len(m.livePanes) {
			p := m.livePanes[m.paneSelection]
			key := m.groupForAttention(p)
			if !parent && hostGroup(key) && !m.foldedPanes[key.id] {
				key = m.sessionKey(p, true)
			}
			return m.foldedPanes, key
		}
	} else if t := m.selected(); t != nil && !m.focusPreview {
		key := m.groupForTask(t)
		if !parent && hostGroup(key) && !m.foldedTasks[key.id] {
			key = m.taskSession(t, key)
		}
		return m.foldedTasks, key
	}
	return nil, groupKey{}
}

func toggleGroup(folded map[string]bool, key groupKey) {
	if folded[key.id] {
		delete(folded, key.id)
	} else {
		folded[key.id] = true
	}
}

func (m *model) taskGroupFolded(t *task) bool {
	if m.focusPreview {
		return false
	}
	key := m.groupForTask(t)
	return m.foldedTasks[key.id] || (hostGroup(key) && m.foldedTasks[m.taskSession(t, key).id])
}
