package main

import "rex-td/rex"

func sampleData() (rex.Fleet, []*task) {
	home := rex.Host{ID: "local", Label: "本机"}
	build := rex.Host{ID: "host:build", Label: "Build"}
	qa := rex.Host{ID: "host:qa", Label: "QA"}
	archive := rex.Host{ID: "host:archive", Label: "Archive"}
	fleet := rex.Fleet{LocalName: "Workstation"}
	for _, h := range []rex.Host{home, build, qa, archive} {
		fleet.Hosts = append(fleet.Hosts, rex.HostStatus{Host: h, State: rex.Live})
	}
	pane := func(host rex.Host, session, agent, title, status, message string) rex.Pane {
		p := rex.Pane{Host: host, SessionID: session, Session: session, ID: title,
			Window: title, App: agent, Status: status, Message: message}
		fleet.Panes = append(fleet.Panes, p)
		return p
	}
	csv := pane(home, "报表交付", "codex", "CSV 导出开发", "question", "导出时要包含隐藏列吗？")
	csvTest := pane(build, "报表交付", "claude-code", "CSV 导出测试", "working", "正在核对日期格式。")
	check := pane(build, "导入修复", "claude-code", "导入检查", "error", "第三次重试后仍超时。")
	release := pane(build, "发布准备", "codex", "发布前检查", "permission", "等待批准运行检查。")
	docs := pane(home, "发布准备", "claude-code", "安装指引", "done", "安装示例已补齐。")
	backup := pane(archive, "备份巡检", "codex", "备份目录权限", "auth", "等待重新登录。")
	logs := pane(qa, "机器巡检", "claude-code", "查日志轮换", "done", "已记录轮换结果。")
	notes := pane(home, "发布准备", "codex", "整理发布清单", "idle", "")
	service := pane(qa, "机器巡检", "codex", "服务健康检查", "working", "")
	return fleet, []*task{
		{
			title: "给报表加 CSV 导出", note: "只导出用户看得见的列", state: working, pinned: true, color: 1, kind: feature,
			steps: []string{"选定导出列", "生成 CSV 样例", "确认日期格式"}, completed: 2,
			panes:  []rex.Pane{csv, csvTest},
			result: "CSV 样例已生成。列为日期、商品、数量。",
		},
		{title: "核对导入样例", note: "查看第三次重试的错误", state: paused, kind: repair, color: 2, panes: []rex.Pane{check}},
		{title: "检查发布说明", state: todo, kind: releaseKind, pinned: true, panes: []rex.Pane{release}},
		{title: "更新安装指引", state: done, kind: docsKind, panes: []rex.Pane{docs}},
		{title: "确认备份目录权限", state: paused, kind: operations, panes: []rex.Pane{backup}},
		{title: "查日志轮换", state: done, kind: operations, panes: []rex.Pane{logs}},
		{title: "整理发布清单", state: todo, kind: releaseKind, panes: []rex.Pane{notes}},
		{title: "核对两台机器健康状态", state: working, kind: operations, panes: []rex.Pane{check, service}},
		{title: "整理帮助页快捷键", note: "整理常用操作并附上说明", state: working, kind: docsKind},
		{title: "补 Linux 安装截图", state: todo, kind: docsKind},
		{title: "准备下周巡检计划", state: todo, kind: operations},
	}
}

type host struct {
	label string
	cpu   int
	badge string
}

var hostExamples = []host{
	{"本机", 23, "2"}, {"Build", 87, "2"}, {"QA", 3, "1"}, {"Archive", 9, "1"}, {"Lab", 41, ""},
}

type quota struct {
	label, reset string
	short, week  int
}

var quotaExamples = []quota{
	{"Claude Max", "1h20", 61, 34}, {"Codex Pro", "4h10", 12, 100}, {"Claude Team", "40m", 88, 71},
}
