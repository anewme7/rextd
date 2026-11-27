package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

const (
	resourceSplitWidth = 36
	quotaShortMinutes  = 5 * 60
	quotaWeekMinutes   = 7 * 24 * 60
)

func resourceWidths(w int) (int, int) {
	machines := (w - 3) * 3 / 5
	return machines, w - machines - 3
}

func resourceHeight(height int) int {
	if height < 18 {
		return 2
	}
	return 4
}

func (m *model) resources(w, height int) []string {
	leftStyle, rightStyle := ink, ink
	if m.focus == 1 {
		leftStyle = accent
	}
	if m.focus == 2 {
		rightStyle = accent
	}
	now := time.Now()
	limit := resourceHeight(height)
	sources := m.quotaSources(now)
	start := min(m.quotaOffset, max(0, len(sources)-limit/2))
	sources = sources[start:min(start+limit/2, len(sources))]
	standardPeriods := standardQuotaPeriods(sources)
	leftTitle := leftStyle.Render(m.text("机器"))
	rightTitle := rightStyle.Render(m.text("额度"))
	if w >= 22 || m.focus != 2 {
		leftTitle += dim.Render(" · CPU")
	}
	if w >= 22 || m.focus == 2 {
		if standardPeriods {
			rightTitle += dim.Render(" 5h/wk")
		} else {
			rightTitle += dim.Render(m.text(" · 已用"))
		}
	}
	hostWidth, quotaWidth := resourceWidths(w)
	combine := func(left, right string) string {
		return fill(left, hostWidth) + "   " + clip(right, quotaWidth)
	}
	heading := pair(leftTitle, rightTitle, w)
	if w >= resourceSplitWidth {
		heading = combine(leftTitle, rightTitle)
	}
	rows := []string{inset(heading)}
	switch {
	case m.focus == 2:
		for _, line := range m.quotaRows(sources, now, standardPeriods, w) {
			rows = append(rows, inset(line))
		}
	case m.focus == 1 || w < resourceSplitWidth:
		for _, line := range m.hostRows(w, limit, m.focus == 1) {
			rows = append(rows, inset(line))
		}
	default:
		machines := m.hostRows(hostWidth, limit, false)
		usage := m.quotaRows(sources, now, standardPeriods, quotaWidth)
		for i := 0; i < max(len(machines), len(usage)); i++ {
			var left, right string
			if i < len(machines) {
				left = machines[i]
			}
			if i < len(usage) {
				right = usage[i]
			}
			rows = append(rows, inset(combine(left, right)))
		}
	}
	return rows
}

func (m *model) hostRows(width, limit int, exact bool) []string {
	var rows []string
	if m.demo {
		start := min(m.hostOffset, max(0, len(hostExamples)-limit))
		for i, host := range hostExamples[start:min(start+limit, len(hostExamples))] {
			label := fmt.Sprintf("%d %s", start+i+1, m.text(host.label))
			cpu := fmt.Sprintf("%d%%", host.cpu)
			value := cpuAndReminders(cpu, host.badge, 4)
			if exact {
				value = cpu
				if host.badge != "" {
					value += " · " + host.badge + m.text("条提醒")
				}
			}
			rows = append(rows, pair(label, value, width))
		}
		return rows
	}
	if len(m.feed.Hosts) == 0 {
		if m.rexContext.Inside() {
			return []string{dim.Render(m.text("连接中…"))}
		}
		if m.rexContext.SidebarID != "" {
			return []string{dim.Render(m.text("Rex 未连接"))}
		}
		return []string{dim.Render(m.text("需在 Rex 中启动"))}
	}
	start := min(m.hostOffset, max(0, len(m.feed.Hosts)-limit))
	visible := m.feed.Hosts[start:min(start+limit, len(m.feed.Hosts))]
	reminders := make(map[string]int)
	badgeWidth := 4
	if !exact {
		for _, pane := range m.feed.Panes {
			if m.isAlert(pane) {
				reminders[pane.Host.ID]++
			}
		}
		for _, host := range visible {
			badgeWidth = max(badgeWidth, 3+len(fmt.Sprint(reminders[host.ID])))
		}
	}
	for i, host := range visible {
		value := dim.Render(m.text("连接中"))
		cpu := m.hostCPU[host.ID]
		switch host.State {
		case rex.Live:
			number := dim.Render("—")
			if cpu != nil {
				number = fmt.Sprintf("%d%%", *cpu)
			}
			if exact {
				value = fmt.Sprintf(m.text("%d会话 %d窗"), host.SessionCount, host.WindowCount)
				if cpu != nil {
					value = number + " " + value
				}
			} else {
				badge := ""
				if count := reminders[host.ID]; count != 0 {
					badge = fmt.Sprint(count)
				}
				value = cpuAndReminders(number, badge, badgeWidth)
			}
		case rex.Partial:
			value = warn.Render(m.text("部分可用"))
		case rex.Offline:
			value = failure.Render(m.text("失联"))
			if exact {
				value = failure.Render(m.text("失联 · 重连"))
			}
		}
		name := fmt.Sprintf("%d %s", start+i+1, m.hostLabel(host.Host))
		if exact && i+start == m.hostSelection {
			name = accent.Render("› " + m.hostLabel(host.Host))
		}
		rows = append(rows, pair(name, value, width))
	}
	return rows
}

func cpuAndReminders(cpu, reminder string, badgeWidth int) string {
	badge := ""
	if reminder != "" {
		badge = warn.Render(" · " + reminder)
	}
	return strings.Repeat(" ", max(0, 4-ansi.StringWidth(cpu))) + cpu + fill(badge, badgeWidth)
}

type quotaSource struct {
	name, missing, sampleReset string
	rate                       quotaReading
	stale                      bool
}

func (m *model) quotaSources(now time.Time) []quotaSource {
	claude := quotaSource{name: "Claude", rate: m.claudeUsage, missing: m.text("待配置"),
		stale: now.Sub(m.claudeUsage.Received) > 15*time.Minute}
	weekly := claude.rate.Windows[1]
	if limit := m.claudeLimit.Windows[1]; limit.Reset > now.Unix() &&
		(!weekly.Present || m.claudeLimit.Received.After(claude.rate.Received)) {
		claude.rate.Windows[1] = limit
		if short := &claude.rate.Windows[0]; short.Reset <= now.Unix() {
			*short = quotaWindow{}
		}
		claude.stale = false
	}
	sources := []quotaSource{
		claude,
		{name: "Codex", rate: m.codexUsage, missing: m.text("读取中"), stale: m.codexFailed},
	}
	if m.codexFailed {
		sources[1].missing = m.text("不可用")
	}
	if m.demo {
		sources = nil
		for _, q := range quotaExamples {
			sources = append(sources, quotaSource{
				name: q.label, sampleReset: q.reset,
				rate: quotaReading{Windows: [2]quotaWindow{
					{Minutes: quotaShortMinutes, Used: float64(q.short), Present: true},
					{Minutes: quotaWeekMinutes, Used: float64(q.week), Present: true},
				}},
			})
		}
	}
	return sources
}

func standardQuotaPeriods(sources []quotaSource) bool {
	for _, source := range sources {
		var short, weekly bool
		for _, window := range source.rate.Windows {
			if !window.Present {
				continue
			}
			switch {
			case window.Minutes == quotaShortMinutes && !short:
				short = true
			case window.Minutes == quotaWeekMinutes && !weekly:
				weekly = true
			default:
				return false
			}
		}
	}
	return true
}

func (m *model) quotaRows(sources []quotaSource, now time.Time, standard bool, width int) []string {
	var rows []string
	for _, source := range sources {
		if !source.rate.available() {
			rows = append(rows, pair(source.name, dim.Render(source.missing), width), "")
			continue
		}
		header := source.sampleReset
		soonest := quotaWindow{}
		short, weekly := "—", "—"
		var actual []string
		var peak float64
		for _, window := range source.rate.Windows {
			if !window.Present {
				continue
			}
			if window.Reset > now.Unix() && (!soonest.Present || window.Reset < soonest.Reset) {
				soonest = window
			}
			peak = max(peak, window.Used)
			if standard {
				value := fmt.Sprintf("%.0f%%", window.Used)
				if window.Minutes == quotaShortMinutes {
					short = value
				} else {
					weekly = value
				}
			} else {
				actual = append(actual, strings.TrimSpace(quotaValue(window)))
			}
		}
		if header == "" {
			header = resetIn(soonest, now)
		}
		style := dim
		if source.stale {
			header = m.text("旧值")
		} else if peak >= 80 {
			style = warn
		}
		value := strings.Join(actual, " / ")
		if standard {
			value = fmt.Sprintf("%4s / %4s", short, weekly)
		}
		if ansi.StringWidth(value)+1 <= width {
			value = " " + value
		}
		rows = append(rows, pair(source.name, dim.Render(header), width), clip(style.Render(value), width))
	}
	return rows
}

// homeResourcesDrawer renders Hosts and Usage quietly at the bottom of the
// Home / Sessions screen without switching the main page.
func (m *model) homeResourcesDrawer(w int, tier homeTier, height int) []string {
	margin := m.homeMargin()
	pad := strings.Repeat(" ", margin)
	gridW := min(max(20, w-margin*2), max(20, homeGridWidth(w, tier)-margin*2))
	if tier != homeFull {
		gridW = max(20, w-margin*2)
	}
	leftStyle, rightStyle := homeMuted, homeMuted
	if m.focus == 1 {
		leftStyle = homeBlue.Bold(true)
	}
	if m.focus == 2 {
		rightStyle = homeBlue.Bold(true)
	}
	now := time.Now()
	limit := resourceHeight(height)
	sources := m.quotaSources(now)
	start := min(m.quotaOffset, max(0, len(sources)-limit/2))
	sources = sources[start:min(start+limit/2, len(sources))]
	standardPeriods := standardQuotaPeriods(sources)

	leftTitle := leftStyle.Render(m.text("机器")) + homeFaint.Render(" · CPU")
	rightTitle := rightStyle.Render(m.text("额度"))
	if standardPeriods {
		rightTitle += homeFaint.Render(" · " + m.text("5小时/周"))
	} else {
		rightTitle += homeFaint.Render(m.text(" · 已用"))
	}
	hostWidth, quotaWidth := resourceWidths(gridW)
	if gridW < resourceSplitWidth && ansi.StringWidth(leftTitle)+2+ansi.StringWidth(rightTitle) > gridW {
		leftTitle = leftStyle.Render(m.text("机器"))
	}
	if (gridW < resourceSplitWidth && ansi.StringWidth(leftTitle)+2+ansi.StringWidth(rightTitle) > gridW) ||
		(gridW >= resourceSplitWidth && quotaWidth < ansi.StringWidth(rightTitle)) {
		rightTitle = rightStyle.Render(m.text("额度"))
	}
	combine := func(left, right string) string {
		return fill(left, hostWidth) + "   " + clip(right, quotaWidth)
	}
	heading := pair(leftTitle, rightTitle, gridW)
	usageX := margin + gridW - ansi.StringWidth(rightTitle)
	if gridW >= resourceSplitWidth {
		heading = combine(leftTitle, rightTitle)
		usageX = margin + hostWidth + 3
	}
	m.homeDrawerHits = []homeDrawerHit{
		{start: margin, end: margin + ansi.StringWidth(leftTitle), focus: 1},
		{start: usageX, end: min(margin+gridW, usageX+ansi.StringWidth(rightTitle)), focus: 2},
	}
	rows := []string{homeRule.Render(strings.Repeat("─", m.width)), pad + heading}
	switch {
	case m.focus == 2:
		for _, line := range m.quotaRows(sources, now, standardPeriods, gridW) {
			rows = append(rows, pad+line)
		}
	default:
		start := min(m.hostOffset, max(0, len(m.feed.Hosts)-limit))
		for i, line := range m.homeHostRows(w, tier, gridW, limit, m.focus == 1) {
			rows = append(rows, line)
			if !m.demo && start+i < len(m.feed.Hosts) {
				m.homeDrawerHits = append(m.homeDrawerHits, homeDrawerHit{row: i + 1, start: margin, end: margin + gridW, focus: 1, hostSlot: start + i + 1})
			}
		}
	}
	return rows
}

func (m *model) homeHostRows(w int, tier homeTier, gridW, limit int, exact bool) []string {
	margin := m.homeMargin()
	fullGridW := gridW + margin*2
	var rows []string
	if m.demo {
		start := min(m.hostOffset, max(0, len(hostExamples)-limit))
		for i, host := range hostExamples[start:min(start+limit, len(hostExamples))] {
			label := fmt.Sprintf("%d %s", start+i+1, m.text(host.label))
			value := fmt.Sprintf("%d%%", host.cpu)
			if host.badge != "" {
				value += " · " + host.badge + m.text("条提醒")
			}
			sel := exact && i+start == m.hostSelection
			titleStyle := homeTaskText
			valStyle := homeMuted
			if sel {
				titleStyle, valStyle = homeSelectedTitle, homeSelectedMuted
			}
			rows = append(rows, renderHomeRow(w, fullGridW, sel, homeTaskPrefix(tier, sel), label, titleStyle, []homeCell{{text: value, style: valStyle}}))
		}
		return rows
	}
	if len(m.feed.Hosts) == 0 {
		msg := m.text("需在 Rex 中启动")
		if m.rexContext.Inside() {
			msg = m.text("连接中…")
		} else if m.rexContext.SidebarID != "" {
			msg = m.text("Rex 未连接")
		}
		return []string{strings.Repeat(" ", margin) + homeMuted.Render(msg)}
	}
	start := min(m.hostOffset, max(0, len(m.feed.Hosts)-limit))
	visible := m.feed.Hosts[start:min(start+limit, len(m.feed.Hosts))]
	for i, host := range visible {
		sel := exact && i+start == m.hostSelection
		titleStyle := homeTaskText
		valStyle := homeMuted
		if sel {
			titleStyle, valStyle = homeSelectedTitle, homeSelectedMuted
		}
		value := m.text("连接中")
		cpu := m.hostCPU[host.ID]
		switch host.State {
		case rex.Live:
			value = fmt.Sprintf(m.text("%d会话 %d窗"), host.SessionCount, host.WindowCount)
			if cpu != nil {
				value = fmt.Sprintf("%d%%  %s", *cpu, value)
			}
		case rex.Partial:
			value = m.text("部分可用")
			valStyle = homeWait
		case rex.Offline:
			value = m.text("失联 · 重连")
			valStyle = homeError
		}
		label := fmt.Sprintf("%d %s", start+i+1, m.hostLabel(host.Host))
		rows = append(rows, renderHomeRow(w, fullGridW, sel, homeTaskPrefix(tier, sel), label, titleStyle, []homeCell{{text: value, style: valStyle}}))
	}
	return rows
}
