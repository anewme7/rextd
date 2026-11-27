package main

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"time"

	"rex-td/rex"
)

type paletteNavigation struct {
	TaskID        int64       `json:"task_id"`
	Pane          palettePane `json:"pane"`
	OriginSession string      `json:"origin_session"`
	OriginBlock   string      `json:"origin_block"`
	Client        string      `json:"client"`
	Reason        string      `json:"reason"`
}

type paletteTarget struct {
	palettePane
	Location    string `json:"location"`
	Status      string `json:"status"`
	Unavailable string `json:"unavailable,omitempty"`
}

func handlePaletteNavigation(store *taskStore, encode *json.Encoder, args []string) error {
	if len(args) != 2 || (args[0] != "targets" && args[0] != "jump" && args[0] != "resume") {
		return paletteError("invalid palette command", "无效的命令面板操作")
	}
	var req paletteNavigation
	if err := json.Unmarshal([]byte(args[1]), &req); err != nil {
		return err
	}
	tasks, err := store.load()
	if err != nil {
		return err
	}
	var selected *task
	for _, t := range tasks {
		if t.id == req.TaskID {
			selected = t
			break
		}
	}
	if selected == nil {
		return paletteError("the task is no longer available", "任务已不存在，请重新查找")
	}

	id := paneIdentity{req.Pane.HostID, req.Pane.SessionID, req.Pane.BlockID}
	if args[0] == "resume" {
		if err := store.savePaletteResume(req.Client, req.TaskID, id, req.Reason); err != nil {
			return err
		}
		return encode.Encode(map[string]int64{"task_id": req.TaskID})
	}
	panes := selected.panes
	if args[0] == "jump" {
		i := slices.IndexFunc(panes, func(p rex.Pane) bool { return identityOf(p) == id })
		if i < 0 || id.host == "local" || req.Client == "" {
			return paletteError("could not resolve the selected remote pane", "无法定位选中的远端窗格")
		}
		c := rex.Context{SessionID: req.OriginSession, SidebarID: req.OriginBlock}
		target := c.FocusLinkedPane(context.Background(), panes[i], req.Client)
		return encode.Encode(newPaletteTarget(target))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	targets := rex.LinkedPanes(ctx, panes)
	recent, _ := store.readPaletteResume(req.Client)
	slices.SortStableFunc(targets, func(a, b rex.PaneTarget) int {
		if (a.Unavailable == "") != (b.Unavailable == "") {
			if a.Unavailable == "" {
				return -1
			}
			return 1
		}
		if order := cmp.Compare(b.AttentionPriority(), a.AttentionPriority()); order != 0 {
			return order
		}
		if recent.taskID == selected.id {
			wasA, wasB := identityOf(a.Pane) == recent.pane, identityOf(b.Pane) == recent.pane
			if wasA != wasB {
				if wasA {
					return -1
				}
				return 1
			}
		}
		return b.ReportedAt.Compare(a.ReportedAt)
	})
	result := make([]paletteTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, newPaletteTarget(target))
	}
	return encode.Encode(result)
}

func newPaletteTarget(target rex.PaneTarget) paletteTarget {
	p := target.Pane
	title := reportedPaneTitle(p)
	if title == "" {
		title = paneTitle(p)
	}
	lang := english
	if os.Getenv("REX_TD_LANG") == string(chinese) {
		lang = chinese
	}
	if p.Host.ID == "local" {
		p.Host.Label = lang.text("本机")
	}
	location := []string{p.Host.Label}
	for _, part := range []string{p.Session, p.Window, appName(p.App)} {
		if part != "" && !slices.Contains(location, part) {
			location = append(location, part)
		}
	}
	return paletteTarget{
		palettePane: palettePane{p.Host.ID, p.Host.Label, p.SessionID, p.Session, p.WindowID, p.ID, title, p.App},
		Location:    strings.Join(location, " · "), Status: lang.text(statusText(p.Status)),
		Unavailable: target.Unavailable,
	}
}

type paletteResume struct {
	revision, taskID int64
	pane             paneIdentity
	reason           string
}

func (s *taskStore) readPaletteResume(client string) (paletteResume, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	var r paletteResume
	err := s.db.QueryRowContext(ctx, "SELECT revision,task,host_id,session_id,block_id,reason FROM palette_client_resume WHERE client=?", client).
		Scan(&r.revision, &r.taskID, &r.pane.host, &r.pane.session, &r.pane.block, &r.reason)
	return r, err
}

func (s *taskStore) savePaletteResume(client string, taskID int64, pane paneIdentity, reason string) error {
	switch reason {
	case "", "host_unavailable", "pane_closed", "link_missing", "jump_failed":
	default:
		reason = "jump_failed"
	}
	return s.exec(`INSERT INTO palette_client_resume (client,revision,task,host_id,session_id,block_id,reason)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT(client) DO UPDATE SET revision=max(palette_client_resume.revision+1,excluded.revision),task=excluded.task,
host_id=excluded.host_id,session_id=excluded.session_id,block_id=excluded.block_id,reason=excluded.reason`,
		client, time.Now().UnixMicro(), taskID, pane.host, pane.session, pane.block, reason)
}

func (m *model) applyPaletteResume() {
	r := m.paletteResume
	if r.revision == 0 || r.revision == m.paletteResumeApplied ||
		m.editing || m.taskEdit != nil || m.taskPick != nil ||
		m.page == taskMenuPage || m.page == taskDeletePage || m.page == linkPage || m.jumping || m.terminalPreview != nil ||
		(m.scene != nil && m.scene.editing) {
		return
	}
	m.paletteResumeApplied = r.revision
	i := slices.IndexFunc(m.tasks, func(t *task) bool { return t.id == r.taskID })
	if i < 0 {
		return
	}
	t := m.tasks[i]
	m.homeScrollManual = false
	m.cancelPaneTurn()
	m.scene = nil
	m.selectTask(t)
	m.showDetails()
	if i := slices.IndexFunc(t.panes, func(p rex.Pane) bool { return identityOf(p) == r.pane }); i >= 0 {
		m.detailSelection = i
	}
	switch r.reason {
	case "host_unavailable":
		m.notice = m.text("无法连接目标主机")
	case "pane_closed":
		m.notice = m.text("关联窗格已关闭")
	case "link_missing":
		m.notice = m.text("找不到选中的关联窗格")
	case "jump_failed":
		m.notice = m.text("暂时无法跳转，已打开任务详情")
	}
}
