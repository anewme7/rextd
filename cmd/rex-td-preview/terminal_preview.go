package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"rex-td/rex"
)

// Take bounded text snapshots through Rex's terminal API. Keep one connection
// for the selected pane, and only send a frame when its content changes.
const terminalPreviewScript = rex.ReadTerminalScreenLua + `
local previous
while true do
  local content = read_terminal_screen(rex.args.session, rex.args.block)
  if content == nil then return end
  if content ~= previous then
    print(rex.json.encode({ content = content }))
    previous = content
  end
  rex.sleep(0.5)
end
`

type terminalPreview struct {
	pane        rex.Pane
	ctx         context.Context
	cancel      context.CancelFunc
	frames      <-chan string
	content     string
	lines       []string
	top         int
	follow      bool
	unavailable bool
}

type terminalPreviewUpdate struct {
	preview *terminalPreview
	content string
	closed  bool
}

func (m *model) openTerminalPreview(pane rex.Pane) tea.Cmd {
	m.closeTerminalPreview()
	preview := &terminalPreview{pane: pane, follow: true}
	m.terminalPreview = preview
	if m.demo || !m.rexContext.Inside() || pane.SessionID == "" || pane.ID == "" ||
		m.feed.HostState(pane.Host.ID) == rex.Offline {
		preview.unavailable = true
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	preview.ctx, preview.cancel = ctx, cancel
	if state := m.feed.HostState(pane.Host.ID); pane.Host.Server != "" && (state == rex.Live || state == rex.Partial) {
		if frames, stop := m.previews.Open(pane); frames != nil {
			preview.frames = frames
			preview.cancel = func() {
				stop()
				cancel()
			}
			return preview.watch()
		}
	}
	args := []string{"--autostart=false", "--color", "never"}
	if pane.Host.Server != "" {
		args = append(args, "-S", pane.Host.Server)
	}
	target, _ := json.Marshal(map[string]string{"session": pane.SessionID, "block": pane.ID})
	cmd := exec.CommandContext(ctx, "rex", append(args, "do", "-s", pane.SessionID, "-b", pane.ID,
		"--args", string(target), "-e", terminalPreviewScript)...)
	frames := make(chan string, 1)
	preview.frames = frames
	go readTerminalPreview(ctx, cmd, frames)
	return preview.watch()
}

func readTerminalPreview(ctx context.Context, cmd *exec.Cmd, frames chan<- string) {
	defer close(frames)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	// Also stop the producer if its output becomes unreadable.
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	decoder := json.NewDecoder(out)
	for {
		var frame struct{ Content string }
		if err := decoder.Decode(&frame); err != nil {
			return
		}
		select {
		case frames <- frame.Content:
		case <-ctx.Done():
			return
		}
	}
}

func (p *terminalPreview) watch() tea.Cmd {
	return func() tea.Msg {
		select {
		case content, ok := <-p.frames:
			if ok {
				content = strings.Map(func(r rune) rune {
					if unicode.IsControl(r) && r != '\n' && r != '\t' {
						return -1
					}
					return r
				}, ansi.Strip(content))
				content = strings.ReplaceAll(content, "\t", "    ")
			}
			return terminalPreviewUpdate{p, content, !ok}
		case <-p.ctx.Done():
			return nil
		}
	}
}

func (m *model) receiveTerminalPreview(msg terminalPreviewUpdate) tea.Cmd {
	p := m.terminalPreview
	if p != msg.preview {
		return nil
	}
	if msg.closed {
		p.unavailable = true
		return nil
	}
	p.content = msg.content
	p.layout(m.width)
	return p.watch()
}

func (p *terminalPreview) layout(width int) {
	lines := strings.Split(ansi.Hardwrap(p.content, max(1, width-2), true), "\n")
	if !p.follow && p.top < len(p.lines) {
		// Preserve the visible text if the bounded scrollback has rotated.
		anchor := p.lines[p.top:min(p.top+3, len(p.lines))]
		if p.top+len(anchor) > len(lines) || !slices.Equal(anchor, lines[p.top:p.top+len(anchor)]) {
			for i := 0; i+len(anchor) <= len(lines); i++ {
				if slices.Equal(anchor, lines[i:i+len(anchor)]) {
					p.top = i
					break
				}
			}
		}
	}
	p.lines = lines
}

func (m *model) closeTerminalPreview() {
	if p := m.terminalPreview; p != nil && p.cancel != nil {
		p.cancel()
	}
	m.terminalPreview = nil
}

func (m *model) terminalPreviewKey(key string) tea.Cmd {
	p := m.terminalPreview
	height := max(1, m.height-5)
	bottom := max(0, len(p.lines)-height)
	top := p.viewTop(bottom)
	switch key {
	case " ", "esc":
		m.closeTerminalPreview()
		return nil
	case "enter":
		if !m.demo && !p.unavailable {
			m.closeTerminalPreview()
			return m.jump(p.pane)
		}
		return nil
	case "up":
		top--
	case "down":
		top++
	case "pgup":
		top -= height
	case "pgdown":
		top += height
	case "home":
		top = 0
	case "end":
		top = bottom
	default:
		return nil
	}
	p.top = clamp(top, 0, bottom)
	p.follow = p.top == bottom
	return nil
}

func (p *terminalPreview) viewTop(bottom int) int {
	if p.follow {
		return bottom
	}
	return clamp(p.top, 0, bottom)
}

func (m *model) terminalPreviewMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress || msg.X < 1 || msg.X >= m.width-1 ||
		msg.Y < 0 || msg.Y >= m.height {
		return nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.terminalPreviewKey("up")
	case tea.MouseButtonWheelDown:
		return m.terminalPreviewKey("down")
	case tea.MouseButtonLeft:
		if msg.Y == m.height-1 {
			if m.terminalPreview.unavailable {
				return m.terminalPreviewKey("esc")
			}
			return m.terminalPreviewKey([...]string{"esc", "enter"}[m.shortcutAt(msg.X, 2)])
		}
	}
	return nil
}

func (m *model) terminalPreviewView() string {
	p := m.terminalPreview
	w, h := m.width, m.height
	if w < 3 || h < 1 {
		return ""
	}
	inner := w - 2
	shortcuts := [][2]string{{"Esc", m.text("返回")}}
	if !p.unavailable {
		shortcuts = append(shortcuts, [2]string{"↵", m.text("进入窗格")})
	}
	footer := inset(clip(renderShortcuts(inner, shortcuts), inner))
	if h < 6 {
		return strings.Repeat("\n", h-1) + footer
	}
	status := m.paneLocation(p.pane)
	if p.unavailable {
		status = m.text("窗格暂不可用") + " · " + status
	} else if p.lines == nil {
		status = m.text("连接中") + " · " + status
	}
	rule := rules.Render(strings.Repeat("─", w))
	lines := []string{
		inset(clip(accent.Render(m.text("窗格预览")+" · "+m.paneDisplayTitle(p.pane)), inner)),
		inset(clip(dim.Render(status), inner)),
		rule,
	}
	height := h - 5
	top := p.viewTop(max(0, len(p.lines)-height))
	for i := 0; i < height; i++ {
		line := ""
		if top+i < len(p.lines) {
			line = p.lines[top+i]
		}
		lines = append(lines, inset(clip(line, inner)))
	}
	return strings.Join(append(lines, rule, footer), "\n")
}
