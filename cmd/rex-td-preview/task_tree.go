package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const taskPickPage page = taskMenuPage + 1

type taskField int

const (
	taskTitleField taskField = iota
	taskNextField
	taskChildField
)

type taskEdit struct {
	target *task
	field  taskField
	back   page
}

type taskPick struct {
	paneMove  *paneMove
	parent    *task
	query     lineInput
	selection int
	back      page
}

func (m *model) parentOf(t *task) *task {
	if t.parentID != 0 {
		if i := slices.IndexFunc(m.tasks, func(candidate *task) bool { return candidate.id == t.parentID }); i >= 0 {
			return m.tasks[i]
		}
	}
	return nil
}

func (m *model) archivedTask(t *task) bool {
	parent := m.parentOf(t)
	return t.archived || (parent != nil && parent.archived)
}

func (m *model) childRows() map[int64][]int {
	children := make(map[int64][]int)
	for i, t := range m.tasks {
		if t.parentID != 0 {
			children[t.parentID] = append(children[t.parentID], i)
		}
	}
	return children
}

func (m *model) progressLabel(t *task, children []int) string {
	completed, total := t.completed, len(t.steps)
	if len(children) != 0 {
		completed, total = 0, 0
		for _, row := range children {
			switch m.tasks[row].state {
			case cancelled:
			case done:
				completed++
				total++
			default:
				total++
			}
		}
	}
	if total > 0 {
		return taskProgress(completed, total)
	}
	return ""
}

func (m *model) startTaskEdit(t *task, field taskField) {
	m.taskEdit = &taskEdit{target: t, field: field, back: m.page}
	m.input.set("")
	if field == taskTitleField {
		m.input.set(t.title)
	} else if field == taskNextField {
		m.input.set(t.note)
	}
	m.editing, m.page = true, listPage
}

func (m *model) editLabels() (title, prompt string) {
	if m.taskEdit != nil {
		switch m.taskEdit.field {
		case taskTitleField:
			return m.text("改标题"), m.text("任务标题")
		case taskNextField:
			return m.text("编辑下一步"), m.text("下一步")
		case taskChildField:
			return m.text("新建子任务"), m.text("任务标题")
		}
	}
	return m.text("新建任务"), m.text("任务标题")
}

func (m *model) finishTaskEdit(value string) {
	draft := m.taskEdit
	t := draft.target
	if m.archivedTask(t) {
		m.notice = m.text("请先还原任务")
		return
	}
	if draft.field != taskNextField && value == "" {
		return
	}
	switch draft.field {
	case taskTitleField:
		if m.store != nil && !m.saved(m.store.updateTask("UPDATE task SET title=? WHERE id=? AND archived=0", value, t.id)) {
			return
		}
		t.title = value
	case taskNextField:
		if m.store != nil && !m.saved(m.store.updateTask("UPDATE task SET note=? WHERE id=? AND archived=0", value, t.id)) {
			return
		}
		t.note = value
	case taskChildField:
		if t.parentID != 0 {
			return
		}
		child := &task{id: m.nextTaskID(), title: value, parentID: t.id}
		if m.store != nil {
			var err error
			child.id, err = m.store.addChild(t.id, value)
			if !m.saved(err) {
				return
			}
		}
		m.tasks = append(m.tasks, child)
		m.expanded[t] = true
		m.sortTaskBoard(child)
		m.followTask(child)
		m.showDetails()
		m.taskEdit, m.editing, m.input = nil, false, lineInput{}
		return
	}
	m.touchTask(t)
	m.page, m.taskEdit, m.editing, m.input = draft.back, nil, false, lineInput{}
}

func (m *model) nextTaskID() int64 {
	var last int64
	for _, t := range m.tasks {
		last = max(last, t.id)
	}
	return last + 1
}

func (s *taskStore) addChild(parent int64, title string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(ctx, "INSERT INTO task(id,title,state,parent_id) SELECT "+nextStoredTaskID+",?,?,id FROM task WHERE id=? AND parent_id IS NULL AND archived=0",
		title, storedStates[todo], parent)
	if err != nil {
		return 0, err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return 0, err
	} else if changed != 1 {
		return 0, errors.New("主任务已变化，请重新选择")
	}
	return result.LastInsertId()
}

func (s *taskStore) changeParent(child int64, from, to int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `UPDATE task SET parent_id=NULLIF(?,0)
WHERE id=? AND id<>? AND COALESCE(parent_id,0)=? AND archived=0
AND NOT EXISTS(SELECT 1 FROM task AS nested WHERE nested.parent_id=?)
AND (?=0 OR EXISTS(SELECT 1 FROM task AS parent WHERE parent.id=? AND parent.parent_id IS NULL AND parent.archived=0))`,
		to, child, to, from, child, to, to)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed != 1 {
		return errors.New("任务关系已变化，请重新选择")
	}
	return nil
}

func (m *model) pickTasks() []*task {
	var out []*task
	children := m.childRows()
	pick := m.taskPick
	query := strings.ToLower(strings.TrimSpace(pick.query.value))
	if move := pick.paneMove; move != nil && move.from != 0 && query == "" {
		out = append(out, nil)
	}
	for _, t := range m.tasks {
		if move := pick.paneMove; move != nil && (t.id == move.from || m.archivedTask(t)) {
			continue
		}
		if parent := pick.parent; parent != nil && (t == parent || t.parentID != 0 || t.archived || len(children[t.id]) != 0) {
			continue
		}
		title := m.taskText(t, t.title)
		if parent := m.parentOf(t); parent != nil {
			title += " " + m.taskText(parent, parent.title)
		}
		if strings.Contains(strings.ToLower(title), query) {
			out = append(out, t)
		}
	}
	return out
}

func (m *model) startTaskPick(parent *task) {
	m.taskPick, m.page = &taskPick{parent: parent, back: m.page}, taskPickPage
	m.notice = ""
}

func (m *model) chooseTask(t *task) {
	pick := m.taskPick
	if pick == nil {
		return
	}
	if move := pick.paneMove; move != nil {
		m.finishPaneMove(*move, t)
		return
	}
	if t == nil {
		return
	}
	if parent := pick.parent; parent != nil {
		if m.store != nil && !m.saved(m.store.changeParent(t.id, 0, parent.id)) {
			return
		}
		t.parentID = parent.id
		m.expanded[parent] = true
		m.sortTaskBoard(parent)
		m.taskPick = nil
		m.showDetails()
		return
	}
	m.selectTask(t)
	m.taskPick = nil
	m.showDetails()
}

func (m *model) selectTask(t *task) {
	m.boardSuggest, m.boardPane, m.boardSuggestionPane = false, paneIdentity{}, paneIdentity{}
	m.followTask(t)
	m.taskSelection = slices.Index(m.tasks, t)
	m.archivedView, m.candidatesFocused, m.attention, m.focus = m.archivedTask(t), false, false, 0
	if parent := m.parentOf(t); parent != nil {
		m.expanded[parent] = true
	}
}

func (m *model) taskPickKey(msg tea.KeyMsg) {
	pick := m.taskPick
	switch msg.Type {
	case tea.KeyEsc:
		m.page, m.taskPick = pick.back, nil
	case tea.KeyUp:
		pick.selection = max(0, pick.selection-1)
	case tea.KeyDown:
		pick.selection = min(pick.selection+1, max(0, len(m.pickTasks())-1))
	case tea.KeyEnter:
		m.confirmTaskPick()
	default:
		if pick.query.update(msg) {
			pick.selection = 0
		}
	}
}

func (m *model) confirmTaskPick() {
	if tasks := m.pickTasks(); len(tasks) > 0 {
		m.chooseTask(tasks[clamp(m.taskPick.selection, 0, len(tasks)-1)])
	}
}

func (m *model) taskPicker(w int) listView {
	var v listView
	title := "搜索任务"
	if m.taskPick.parent != nil {
		title = "关联已有任务"
	}
	if move := m.taskPick.paneMove; move != nil {
		title = "移动会话"
		if move.from == 0 {
			title = "归入任务"
		}
	}
	v.append(ink.Render(m.text(title)), listHit{})
	if move := m.taskPick.paneMove; move != nil {
		label := strings.Join(nonEmpty(m.paneDisplayTitle(move.pane), appName(move.pane.App)), " · ")
		v.append(dim.Render(clip(label, w)), listHit{})
	}
	v.append("/ "+m.taskPick.query.view(w-2), listHit{})
	v.append("", listHit{})
	tasks := m.pickTasks()
	if len(tasks) == 0 {
		v.append(dim.Render(m.text("没有匹配的任务")), listHit{})
	}
	for i, t := range tasks {
		name, state := m.text("移出任务"), ""
		hit := listHit{task: t}
		if t == nil {
			hit.pane = identityOf(m.taskPick.paneMove.pane)
		} else {
			name = m.taskText(t, t.title)
			if parent := m.parentOf(t); parent != nil {
				name = m.taskText(parent, parent.title) + " / " + name
			}
			state = m.text(stateLabels[t.state])
			if m.archivedTask(t) {
				state += " · " + m.text("归档")
			}
		}
		row := pair("  "+name, dim.Render(state), w)
		if i == m.taskPick.selection {
			v.active = len(v.lines)
			row = focusHighlight.Render(pair("› "+name, state, w))
		}
		v.append(row, hit)
	}
	return v
}
