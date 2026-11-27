package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

const taskMenuPage page = linkPage + 1

func taskCheckbox(t *task) string {
	if t.state == done {
		return "☑"
	}
	return "☐"
}

func (m *model) setTaskState(t *task, next taskState) {
	if m.archivedTask(t) || t.state == next {
		return
	}
	previous := t.previousState
	if t.state < done && next >= done {
		previous = t.state
	}
	if m.store != nil && !m.saved(m.store.updateTask(
		"UPDATE task SET state=?,previous_state=? WHERE id=? AND archived=0 AND state=?",
		storedStates[next], storedStates[previous], t.id, storedStates[t.state],
	)) {
		return
	}
	t.state, t.previousState = next, previous
	if next == done {
		var latest int64
		for _, other := range m.tasks {
			latest = max(latest, other.completionOrder)
		}
		t.completionOrder = latest + 1
	}
	if m.selected() == t {
		m.followTask(t)
	}
	m.touchTask(t)
}

func (m *model) cycleTaskState(t *task) {
	next := (t.state + 1) % (done + 1)
	if t.state == cancelled {
		next = t.previousState
	}
	m.setTaskState(t, next)
}

func (m *model) taskCount() int {
	count := 0
	for _, t := range m.tasks {
		if m.archivedView {
			if parent := m.parentOf(t); t.archived && (parent == nil || !parent.archived) {
				count++
			}
		} else if t.parentID == 0 && !t.archived {
			count++
		}
	}
	return count
}

func (m *model) selectVisibleTask() {
	groups := m.taskGroups()
	if m.focusPreview && !m.archivedView && !m.secondaryOpen[m.homeTab] {
		groups = slices.DeleteFunc(groups, m.isSecondary)
	}
	for _, group := range groups {
		if slices.Contains(group.rows, m.taskSelection) {
			return
		}
	}
	if len(groups) == 0 {
		m.taskSelection = -1
	} else {
		m.taskSelection = groups[0].firstVisibleRow()
	}
}

func (m *model) changeArchive(t *task) {
	if !t.archived && t.state < done {
		return
	}
	archived := !t.archived
	if m.store != nil && !m.saved(m.store.updateTask("UPDATE task SET archived=? WHERE id=? AND archived=? AND state=?", archived, t.id, t.archived, storedStates[t.state])) {
		return
	}
	t.archived = archived
	m.touchTask(t)
	m.page = listPage
	if !archived {
		m.selectTask(t)
	}
	m.selectVisibleTask()
	if archived {
		m.notice = m.text("任务已归档")
	} else {
		m.notice = m.text("任务已还原")
	}
}

func (m *model) taskBoardKey(key string) bool {
	t := m.selected()
	switch key {
	case "A":
		if m.page != listPage {
			return false
		}
		if m.focusPreview {
			if m.homeTab == archiveTab && !m.attention {
				m.switchHomeTab(m.lastActiveTab)
			} else {
				m.switchHomeTab(archiveTab)
			}
			m.notice = ""
			return true
		}
		m.archivedView, m.candidatesFocused, m.peek = !m.archivedView, false, false
		m.notice = ""
		m.selectVisibleTask()
	case "/":
		m.startTaskPick(nil)
	case "m":
		if t == nil || m.onCandidates() {
			return false
		}
		m.menuBack, m.menuSelection, m.page = m.page, 0, taskMenuPage
		m.notice = ""
	case "s", "e":
		if t == nil || m.onCandidates() {
			return false
		}
		if key == "s" {
			m.cycleTaskState(t)
		} else if m.archivedTask(t) {
			m.notice = m.text("请先还原任务")
		} else {
			m.startTaskEdit(t, taskNextField)
		}
	case "x":
		if t == nil || m.onCandidates() {
			return false
		}
		m.toggleTaskCheck(t)
	default:
		return false
	}
	return true
}

func (m *model) toggleTaskCheck(t *task) {
	m.notice = ""
	if m.archivedTask(t) {
		m.notice = m.text("请先还原任务")
	} else if t.state >= done {
		m.setTaskState(t, t.previousState)
	} else {
		m.setTaskState(t, done)
	}
}

type taskAction struct {
	key, label string
}

var deleteTaskAction = taskAction{"D", "删除任务"}

func (m *model) taskActions(t *task) []taskAction {
	if m.archivedTask(t) {
		if t.archived {
			return []taskAction{{"a", "还原任务"}, deleteTaskAction}
		}
		return []taskAction{{"h", "打开主任务"}, deleteTaskAction}
	}
	actions := []taskAction{{"x", "标记完成"}, {"c", "取消任务"}}
	if t.state >= done {
		actions[0] = taskAction{"x", "重开任务"}
		actions = append(actions, taskAction{"a", "归档任务"})
	}
	if t.state == cancelled {
		actions = actions[1:]
		actions[0] = taskAction{"c", "重开任务"}
	}
	switch t.state {
	case working:
		actions = append(actions, taskAction{"s", "暂缓任务"})
	case paused:
		actions = append(actions, taskAction{"s", "恢复进行"})
	}
	actions = append(actions, taskAction{"u", "上移"}, taskAction{"d", "下移"},
		taskAction{"e", "改标题"}, taskAction{"n", "编辑下一步"})
	if t.parentID == 0 {
		return append(actions, taskAction{"+", "新建子任务"}, taskAction{"l", "关联已有任务"}, deleteTaskAction)
	}
	return append(actions, taskAction{"h", "打开主任务"}, taskAction{"l", "移出主任务"}, deleteTaskAction)
}

func (m *model) taskMenu(w int) listView {
	var view listView
	t := m.selected()
	if t == nil {
		return view
	}
	view.append(ink.Render(m.taskText(t, t.title)), listHit{})
	view.append("", listHit{})
	for i, entry := range m.taskActions(t) {
		line := pair("  "+m.text(entry.label), dim.Render(entry.key), w)
		if i == m.menuSelection {
			view.active = len(view.lines)
			line = focusHighlight.Render(pair("› "+m.text(entry.label), entry.key, w))
		}
		view.append(line, listHit{action: entry.key})
	}
	return view
}

func (m *model) taskMenuKey(key string) {
	t := m.selected()
	if t == nil || key == "esc" || key == "left" || key == "m" {
		m.page = m.menuBack
		return
	}
	actions := m.taskActions(t)
	switch key {
	case "up", "k":
		m.menuSelection = max(0, m.menuSelection-1)
		return
	case "down", "j":
		m.menuSelection = min(len(actions)-1, m.menuSelection+1)
		return
	case "enter":
		key = actions[clamp(m.menuSelection, 0, len(actions)-1)].key
	}
	if !slices.ContainsFunc(actions, func(entry taskAction) bool { return entry.key == key }) {
		return
	}
	m.page = m.menuBack
	switch key {
	case "x":
		m.taskBoardKey("x")
	case "c":
		next := cancelled
		if t.state == cancelled {
			next = t.previousState
		}
		m.setTaskState(t, next)
	case "s":
		if t.state == working {
			m.setTaskState(t, paused)
		} else {
			m.setTaskState(t, working)
		}
	case "a":
		m.changeArchive(t)
	case "D":
		m.taskDelete = taskDeletion{id: t.id, title: t.title, hasChildren: len(m.childRows()[t.id]) != 0}
		m.page = taskDeletePage
	case "u":
		m.moveTask(t, -1)
	case "d":
		m.moveTask(t, 1)
	case "e":
		m.startTaskEdit(t, taskTitleField)
	case "n":
		m.startTaskEdit(t, taskNextField)
	case "+":
		m.startTaskEdit(t, taskChildField)
	case "h":
		if parent := m.parentOf(t); parent != nil {
			m.selectTask(parent)
			m.showDetails()
		}
	case "l":
		if t.parentID == 0 {
			m.startTaskPick(t)
			return
		}
		if m.store != nil && !m.saved(m.store.changeParent(t.id, t.parentID, 0)) {
			return
		}
		t.parentID = 0
		m.sortTaskBoard(t)
	}
}

func taskProgress(done, total int) string {
	return fmt.Sprintf("%d/%d", done, total)
}

func (t *task) order() int64 {
	if t.sortOrder != 0 {
		return t.sortOrder
	}
	return t.id
}

func (t *task) inPinnedSection() bool {
	return t.pinned && t.parentID == 0
}

func (m *model) sortTaskBoard(selected *task) {
	slices.SortStableFunc(m.tasks, func(a, b *task) int {
		if a.inPinnedSection() != b.inPinnedSection() {
			if a.inPinnedSection() {
				return -1
			}
			return 1
		}
		if order := cmp.Compare(a.order(), b.order()); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})
	m.taskSelection = slices.Index(m.tasks, selected)
}

func (m *model) moveTask(t *task, delta int) {
	waiting := t.state == working && m.boardStatus(t).waiting > 0
	for i := m.taskSelection + delta; i >= 0 && i < len(m.tasks); i += delta {
		other := m.tasks[i]
		if other.archived || t.parentID != other.parentID || t.inPinnedSection() != other.inPinnedSection() {
			continue
		}
		if (t.state == working) != (other.state == working) ||
			(t.parentID == 0 && (t.state == todo) != (other.state == todo)) ||
			(t.state == working && waiting != (m.boardStatus(other).waiting > 0)) {
			continue
		}
		first, second := t.order(), other.order()
		if m.store != nil && !m.saved(m.store.swapTaskOrder(t, other)) {
			return
		}
		t.sortOrder, other.sortOrder = second, first
		m.sortTaskBoard(t)
		return
	}
	m.notice = m.text("已到此区边缘")
}

func (s *taskStore) updateTask(statement string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed != 1 {
		return errors.New("任务已变化，请重试")
	}
	return nil
}

func (s *taskStore) swapTaskOrder(first, second *task) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, change := range []struct{ id, expected, next int64 }{
		{first.id, first.order(), second.order()},
		{second.id, second.order(), first.order()},
	} {
		result, err := tx.ExecContext(ctx, "UPDATE task SET sort_order=? WHERE id=? AND archived=0 AND COALESCE(NULLIF(sort_order,0),id)=?", change.next, change.id, change.expected)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil {
			return err
		} else if changed != 1 {
			return errors.New("任务顺序已变化，请重试")
		}
	}
	return tx.Commit()
}
