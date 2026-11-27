package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

type paneAgentResult struct {
	scene   *paneScene
	request uint64
	detail  agentDetail
	turn    rex.PaneTurn
	binding agentBinding
	save    bool
	err     error
}

func (s *taskStore) paneAgentBinding(p rex.Pane) (agentBinding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var binding agentBinding
	err := s.db.QueryRowContext(ctx, "SELECT agent_id, pid FROM pane_agent_binding WHERE host_id=? AND session_id=? AND block_id=? AND app=?",
		p.Host.ID, p.SessionID, p.ID, p.App).Scan(&binding.id, &binding.pid)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return binding, err
}

func (s *taskStore) savePaneAgentBinding(p rex.Pane, binding agentBinding) error {
	if binding.id == "" {
		return s.exec("DELETE FROM pane_agent_binding WHERE host_id=? AND session_id=? AND block_id=? AND app=?",
			p.Host.ID, p.SessionID, p.ID, p.App)
	}
	return s.exec(`INSERT INTO pane_agent_binding(host_id,session_id,block_id,app,agent_id,pid)
VALUES(?,?,?,?,?,?) ON CONFLICT(host_id,session_id,block_id,app)
DO UPDATE SET agent_id=excluded.agent_id,pid=excluded.pid`,
		p.Host.ID, p.SessionID, p.ID, p.App, binding.id, binding.pid)
}

func requestPaneAgent(ctx context.Context, pane rex.Pane, binding agentBinding) (agentDetail, agentBinding, error) {
	if pane.App != "codex" && pane.App != "claude-code" {
		return agentDetail{}, binding, nil
	}
	if pane.App == "codex" && binding.id == "" {
		return agentDetail{}, binding, nil
	}
	pid, err := pane.ForegroundPID(ctx)
	if err != nil {
		return agentDetail{}, binding, err
	}
	if pane.App == "codex" && binding.pid != 0 && binding.pid != pid {
		return agentDetail{}, binding, errors.New("the pane foreground process changed")
	}
	binding.pid = pid
	var detail agentDetail
	if pane.Host.Server != "" {
		var raw []byte
		raw, err = pane.RequestAgentDetail(ctx, binding.id, pid)
		if err == nil {
			err = json.Unmarshal(raw, &detail)
		}
	} else if pane.App == "codex" {
		detail, err = codexAgentDetail(ctx, binding.id)
	} else {
		detail, err = claudeAgentDetail(ctx, pid)
	}
	if err != nil {
		return agentDetail{}, binding, err
	}
	currentPID, err := pane.ForegroundPID(ctx)
	if err != nil || currentPID != pid {
		return agentDetail{}, binding, errors.New("the pane changed during the lookup")
	}
	if !agentIDPattern.MatchString(detail.SessionID) || (binding.id != "" && detail.SessionID != binding.id) {
		return agentDetail{}, binding, errors.New("the provider returned a different session")
	}
	return detail, binding, nil
}

func (m *model) readPaneAgent(id string, save bool) tea.Cmd {
	scene := m.scene
	pane := m.currentPane(scene.pane)
	if m.demo || m.ctx == nil || m.feed.HostState(pane.Host.ID) == rex.Offline ||
		(pane.App != "codex" && pane.App != "claude-code") {
		return nil
	}
	if !save && pane.App == "codex" && scene.binding.id == "" && pane.Status != "done" {
		return nil
	}
	m.cancelPaneTurn()
	scene.request++
	request := scene.request
	ctx, cancel := context.WithTimeout(m.ctx, 4*time.Second)
	scene.cancel = cancel
	binding := scene.binding
	if save {
		binding = agentBinding{id: id}
	}
	return func() tea.Msg {
		defer cancel()
		detail, resolved, err := requestPaneAgent(ctx, pane, binding)
		result := paneAgentResult{scene: scene, request: request, detail: detail, binding: resolved, save: save, err: err}
		if !save && detail.Result == "" && pane.App == "codex" && pane.Status == "done" && ctx.Err() == nil {
			result.turn = pane.RecentCodexTurn(ctx)
		}
		return result
	}
}

func (m *model) receivePaneAgent(msg paneAgentResult) {
	if m.page != scenePage || m.scene != msg.scene || m.scene.request != msg.request {
		return
	}
	scene := m.scene
	scene.cancel = nil
	if !msg.save {
		scene.detail, scene.turn = msg.detail, msg.turn
		return
	}
	if msg.err != nil || msg.detail.SessionID == "" {
		m.notice = m.text("无法验证这个会话 ID")
		return
	}
	if m.store != nil && !m.saved(m.store.savePaneAgentBinding(scene.pane, msg.binding)) {
		return
	}
	scene.binding, scene.detail, scene.turn = msg.binding, msg.detail, msg.turn
	scene.more, scene.offset = false, 0
	m.notice = m.text("已关联 Codex 会话")
}

func (m *model) editPaneAgent(msg tea.KeyMsg) tea.Cmd {
	scene := m.scene
	switch msg.Type {
	case tea.KeyEsc:
		scene.editing = false
		m.notice = ""
	case tea.KeyEnter:
		id := strings.TrimSpace(scene.input.value)
		if id != "" && !agentIDPattern.MatchString(id) {
			m.notice = m.text("请输入完整的会话 ID")
			return nil
		}
		scene.editing = false
		if id != "" {
			m.notice = ""
			return m.readPaneAgent(id, true)
		}
		m.cancelPaneTurn()
		scene.request++
		if m.store != nil && !m.saved(m.store.savePaneAgentBinding(scene.pane, agentBinding{})) {
			return nil
		}
		scene.binding, scene.detail, scene.turn = agentBinding{}, agentDetail{}, rex.PaneTurn{}
		return m.readPaneAgent("", false)
	default:
		scene.input.update(msg)
	}
	return nil
}

func (m *model) taskNextForPane(pane rex.Pane) string {
	if t := m.selected(); t != nil && !m.attention && t.note != "" {
		return m.taskText(t, t.note)
	}
	for _, t := range m.tasks {
		if !t.archived && t.note != "" {
			for _, linked := range t.panes {
				if samePane(linked, pane) {
					return m.taskText(t, t.note)
				}
			}
		}
	}
	return ""
}

func (m *model) urgentTaskPane(t *task) (rex.Pane, bool) {
	priority := 0
	var pane rex.Pane
	choose := func(current *task) {
		for _, linked := range current.panes {
			p := m.currentPane(linked)
			if !m.isAlert(p) {
				continue
			}
			rank := p.AttentionPriority()
			if rank > priority {
				priority, pane = rank, p
			}
		}
	}
	choose(t)
	for _, i := range m.childRows()[t.id] {
		if child := m.tasks[i]; !child.archived {
			choose(child)
		}
	}
	return pane, priority > 0
}
