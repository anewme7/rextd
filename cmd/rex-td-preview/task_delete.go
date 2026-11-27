package main

import (
	"context"
	"errors"
	"slices"
	"time"
)

const taskDeletePage page = taskPickPage + 1

type taskDeletion struct {
	id          int64
	title       string
	hasChildren bool
}

func (m *model) taskDeleteView(w int) listView {
	var view listView
	view.append(warn.Render(m.text("永久删除任务？")), listHit{})
	view.append(ink.Render(clip(m.taskDelete.title, w)), listHit{})
	view.append("", listHit{})
	for _, explanation := range []string{"任务记录将被永久删除。", "会话会解除关联。"} {
		for _, line := range wrap(m.text(explanation), w) {
			view.append(line, listHit{})
		}
	}
	if m.taskDelete.hasChildren {
		for _, line := range wrap(m.text("子任务保留为独立任务。"), w) {
			view.append(line, listHit{})
		}
	}
	return view
}

func (m *model) taskDeleteKey(key string) {
	switch key {
	case "esc", "left":
		m.page = taskMenuPage
		m.taskDelete = taskDeletion{}
	case "y":
		draft := m.taskDelete
		index := slices.IndexFunc(m.tasks, func(t *task) bool { return t.id == draft.id && t.title == draft.title })
		if index < 0 {
			m.notice = m.text("任务已变化，请重新选择")
			m.page = listPage
			m.taskDelete = taskDeletion{}
			return
		}
		if m.store != nil && !m.saved(m.store.deleteTask(draft)) {
			return
		}
		t := m.tasks[index]
		for _, pane := range t.panes {
			m.removedAgents[identityOf(pane)] = true
		}
		for _, child := range m.tasks {
			if child.parentID == t.id {
				child.parentID = 0
			}
		}
		delete(m.expanded, t)
		delete(m.boardOpenIdle, t)
		delete(m.taskTouched, t.id)
		m.tasks = slices.Delete(m.tasks, index, index+1)
		m.taskSelection = min(index, len(m.tasks)-1)
		m.taskDelete = taskDeletion{}
		m.page, m.boardPane, m.boardMore = listPage, paneIdentity{}, false
		m.sortTaskBoard(m.selected())
		m.selectVisibleTask()
		m.refreshCandidates()
		m.notice = m.text("任务已删除")
	}
}

func (s *taskStore) deleteTask(draft taskDeletion) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// The other task-owned tables cascade through their foreign keys.
	for _, statement := range []string{
		"UPDATE task SET parent_id=NULL WHERE parent_id=?",
		"DELETE FROM task_pin WHERE task=?",
		"DELETE FROM task_color WHERE task=?",
		"DELETE FROM result WHERE task=?",
		"DELETE FROM pane_link WHERE task=?",
	} {
		if _, err := tx.ExecContext(ctx, statement, draft.id); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM task WHERE id=? AND title=?", draft.id, draft.title)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return errors.New("任务已变化，请重新选择")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_id_floor(slot,deleted_id) VALUES(1,?)
ON CONFLICT(slot) DO UPDATE SET deleted_id=max(deleted_id,excluded.deleted_id)`, draft.id); err != nil {
		return err
	}
	return tx.Commit()
}
