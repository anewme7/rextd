package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"rex-td/rex"
)

type paneScene struct {
	pane    rex.Pane
	binding agentBinding
	detail  agentDetail
	turn    rex.PaneTurn
	offset  int
	cancel  context.CancelFunc
	request uint64
	more    bool
	editing bool
	input   lineInput
}

func (m *model) cancelPaneTurn() {
	if m.scene != nil && m.scene.cancel != nil {
		m.scene.cancel()
		m.scene.cancel = nil
	}
}

func (m *model) paneSceneView(width int) listView {
	var v listView
	scene := m.scene
	add := func(line string) { v.append(line, listHit{}) }
	if scene.editing {
		add(ink.Render(m.text("关联 Codex 会话")))
		add("")
		add(dim.Render(m.text("会话 ID")))
		add(scene.input.view(width))
		if m.notice != "" {
			add(dim.Render(m.notice))
		}
		return v
	}
	pane := m.currentPane(scene.pane)
	state := pane.Status
	offline := m.feed.HostState(pane.Host.ID) == rex.Offline
	unavailable := !m.demo && !offline && m.bindingReason(pane) != ""
	if offline {
		state = "offline"
	} else if unavailable {
		state, pane.Message = "", ""
	}
	label := m.text("Rex 窗格详情")
	if m.demo {
		label = m.text("演示窗格")
	}
	add(accent.Render(label))
	add("")
	add(ink.Render(clip(m.paneDisplayTitle(pane), width)))
	add(pair(dim.Render(m.paneLocation(pane)), m.status(state), width))
	var taskNames []string
	for _, t := range m.tasks {
		if slices.ContainsFunc(t.panes, func(bound rex.Pane) bool { return samePane(bound, pane) }) {
			taskNames = append(taskNames, m.taskText(t, t.title))
		}
	}
	association := m.text("未关联任务")
	if len(taskNames) != 0 {
		association = m.text("关联任务") + " · " + strings.Join(taskNames, " / ")
	}
	add(dim.Render(clip(association, width)))
	if pane, ok := m.sceneResult(); ok {
		action := "标为已看"
		if m.seenResults[identityOf(pane)].matches(pane) {
			action = "撤销已看"
		}
		v.append(clip(accent.Render("y")+" "+m.text(action), width), listHit{action: "y"})
	}
	detail := scene.detail
	if pane.App != scene.pane.App || offline || unavailable {
		detail = agentDetail{}
	}
	if scene.more {
		m.appendPaneMetadata(&v, pane, detail, offline || unavailable, width)
		return v
	}
	if m.notice != "" {
		add(dim.Render(clip(m.notice, width)))
	}
	add("")
	add(ink.Render(m.text("关键节点")))
	node, next := m.paneMilestone(state)
	reported := pane.ReportedAt
	if reported.IsZero() {
		reported = pane.Since
	}
	if !reported.IsZero() && state != "offline" {
		node = reported.In(time.Local).Format("01-02 15:04") + " · " + node
	}
	for _, line := range briefLines(node, width, 2) {
		add(line)
	}
	if state != "offline" {
		wait := m.sampleText(pane.Message)
		if wait == "" && (state == "waiting" || state == "permission" || state == "question") {
			wait = m.waitingReason(detail.Waiting)
		}
		for _, line := range briefLines(wait, width, 2) {
			add(dim.Render(line))
		}
	}
	result := detail.Result
	if result == "" && !offline && !unavailable && state == "done" {
		result = scene.turn.Result
	}
	if result != "" {
		add("")
		add(ink.Render(m.text("最近结果")))
		for _, line := range briefLines(result, width, 3) {
			add(line)
		}
	}
	if !m.demo {
		if taskNext := m.taskNextForPane(pane); taskNext != "" {
			next = taskNext
		}
		add("")
		add(ink.Render(m.text("下一步")))
		for _, line := range briefLines(next, width, 2) {
			add(line)
		}
	}

	if scene.cancel != nil {
		add("")
		add(dim.Render(m.text("正在读取会话详情…")))
	}
	return v
}

func (m *model) appendPaneMetadata(v *listView, pane rex.Pane, detail agentDetail, offline bool, width int) {
	scene := m.scene
	add := func(line string) { v.append(line, listHit{}) }
	add("")
	add(dim.Render(m.text("更多详情")))
	if m.notice != "" {
		add(dim.Render(clip(m.notice, width)))
	}
	if scene.cancel != nil {
		add(dim.Render(m.text("正在读取会话详情…")))
	}
	if pane.Progress != nil && !offline {
		add(dim.Render(fmt.Sprintf(m.text("已上报进度 %d%%"), *pane.Progress)))
	}
	for _, row := range [][2]string{{"项目", detail.CWD}, {"模型", detail.Model}, {"推理档位", detail.Effort}, {"会话 ID", detail.SessionID}} {
		if row[1] != "" {
			add(clip(dim.Render(m.text(row[0])+" · ")+row[1], width))
		}
	}
	if detail.Updated > 0 {
		stamp := detail.Updated
		if stamp > 100_000_000_000 {
			stamp /= 1000
		}
		add(dim.Render(m.text("会话更新") + " · " + time.Unix(stamp, 0).Format("01-02 15:04")))
	}
	if scene.turn.Input != "" && !offline && pane.Status == "done" {
		add("")
		add(dim.Render(m.text("本轮输入")))
		for _, line := range briefLines(scene.turn.Input, width, 4) {
			add(line)
		}
		add(dim.Render(m.text("来源 · 所选窗格的终端摘录")))
	}
	if pane.App == "codex" && !m.demo && !offline {
		v.append(accent.Render(m.text("i 关联 Codex 会话")), listHit{action: "i"})
	}
	if !m.demo && !offline {
		v.append(dim.Render(m.text("u 刷新详情")), listHit{action: "u"})
	}
}

func briefLines(text string, width, maxLines int) []string {
	if text == "" {
		return nil
	}
	lines := wrap(text, width)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = clip(lines[maxLines-1], width-1) + "…"
	}
	return lines
}

func (m *model) waitingReason(reason string) string {
	switch reason {
	case "permission prompt":
		return m.text("等待批准")
	case "input needed":
		return m.text("等待回答")
	case "sandbox request":
		return m.text("等待沙盒确认")
	case "worker request", "dialog open":
		return m.text("等待处理")
	}
	return ""
}

func (m *model) paneMilestone(state string) (string, string) {
	switch state {
	case "done":
		return m.text("本轮结束"), m.text("回车跳转，核对结果")
	case "permission":
		return m.text("请求批准"), m.text("回车跳转，查看批准请求")
	case "question":
		return m.text("等待回答"), m.text("回车跳转，查看 Agent 的问题")
	case "auth":
		return m.text("需要登录"), m.text("回车跳转，查看登录提示")
	case "error":
		return m.text("报告错误"), m.text("回车跳转，查看报错上下文")
	case "waiting":
		return m.text("等待处理"), m.text("回车跳转，查看需要处理的事项")
	case "working":
		return m.text("正在执行"), m.text("回车跳转，查看实时进展")
	case "idle":
		return m.text("等候下一条指令"), m.text("回车跳转，继续工作")
	case "offline":
		return m.text("主机失联"), m.text("等 Rex 恢复连接后再跳转")
	default:
		return m.text("窗格已发现，状态未上报"), m.text("回车跳转，查看当前内容")
	}
}
