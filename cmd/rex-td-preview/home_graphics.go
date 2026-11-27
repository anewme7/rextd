package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"rex-td/rex"
)

type homeIcon uint8

const (
	noHomeIcon homeIcon = iota
	claudeHomeIcon
	codexHomeIcon
	terminalHomeIcon
	kimiHomeIcon
	homeIconCount
)

func homePaneIcon(pane rex.Pane) homeIcon {
	if pane.Status == "" || pane.Status == "offline" {
		return terminalHomeIcon
	}
	switch pane.App {
	case "claude-code":
		return claudeHomeIcon
	case "codex":
		return codexHomeIcon
	case "kimi-code":
		return kimiHomeIcon
	default:
		return terminalHomeIcon
	}
}

type homePlacement struct {
	row, column int
	icon        homeIcon
}

const homeFramePrefix = "\x1b]777;rextd-icons;"

// homeGraphics keeps the icon uploads in the terminal and replaces their placements
// as a frame scrolls, changes width, or leaves Home. Only the renderer writes to it.
type homeGraphics struct {
	*os.File
	images [homeIconCount][]byte
	baseID uint32
	loaded [homeIconCount]bool
	frame  string
}

func (m *model) homeTerminalOptions() ([]tea.ProgramOption, *homeInput) {
	probe := probeHomeTerminal(os.Stdin, os.Stdout)
	if probe.hasTheme {
		lipgloss.SetHasDarkBackground(probe.dark)
	} else {
		// Resolve terminal background before Bubble Tea claims stdin.
		lipgloss.SetHasDarkBackground(lipgloss.HasDarkBackground())
	}
	var options []tea.ProgramOption
	var input *homeInput
	if term.IsTerminal(os.Stdin.Fd()) || len(probe.pending) > 0 {
		input = &homeInput{File: os.Stdin, pending: probe.pending}
		options = append(options, tea.WithInput(input))
	}
	if !probe.graphics {
		return options, input
	}
	database, err := taskDBPath()
	if err != nil {
		return options, input
	}
	ctx, cancel := context.WithTimeout(m.ctx, 1500*time.Millisecond)
	defer cancel()
	cache := filepath.Join(filepath.Dir(database), "icons")
	screen := &homeGraphics{File: os.Stdout, baseID: rand.Uint32N(1<<31) + 1}
	screen.images[claudeHomeIcon] = homeAppIcon(ctx, "/Applications/Claude.app", cache, "claude.png")
	screen.images[codexHomeIcon] = homeAppIcon(ctx, "/Applications/ChatGPT.app", cache, "codex.png")
	screen.images[terminalHomeIcon] = homeAppIcon(ctx, "/System/Applications/Utilities/Terminal.app", cache, "terminal.png")
	screen.images[kimiHomeIcon] = homeRexKimiIcon(ctx, cache)
	for _, image := range screen.images {
		if len(image) != 0 {
			m.homeGraphics = screen
			return append(options, tea.WithOutput(screen)), input
		}
	}
	return options, input
}

func homeAppIcon(ctx context.Context, app, cache, name string) []byte {
	contents := filepath.Join(app, "Contents")
	plist := filepath.Join(contents, "Info.plist")
	appInfo, err := os.Stat(plist)
	if err != nil {
		return nil
	}
	return cachedHomeIcon(cache, name, appInfo.ModTime(), func(output string) bool {
		entry, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", "CFBundleIconFile", "raw", "-o", "-", plist).Output()
		if err != nil {
			return false
		}
		resource := strings.TrimSpace(string(entry))
		if resource == "" || filepath.Base(resource) != resource {
			return false
		}
		if filepath.Ext(resource) == "" {
			resource += ".icns"
		}
		source := filepath.Join(contents, "Resources", resource)
		return exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "png", "-z", "64", "64",
			source, "--out", output).Run() == nil
	})
}

func cachedHomeIcon(cache, name string, sourceModified time.Time, writePNG func(string) bool) []byte {
	path := filepath.Join(cache, name)
	if info, err := os.Stat(path); err == nil && !info.ModTime().Before(sourceModified) {
		if image := readHomePNG(path); image != nil {
			return image
		}
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil
	}
	temporary, err := os.CreateTemp(cache, ".icon-*.png")
	if err != nil {
		return nil
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Close(); err != nil {
		return nil
	}
	if !writePNG(temporary.Name()) {
		return nil
	}
	image := readHomePNG(temporary.Name())
	if image != nil {
		_ = os.Rename(temporary.Name(), path)
	}
	return image
}

func readHomePNG(path string) []byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	const maxPNG = 128 << 10
	data, err := io.ReadAll(io.LimitReader(file, maxPNG+1))
	if err != nil || len(data) > maxPNG {
		return nil
	}
	info, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || info.Width != 64 || info.Height != 64 {
		return nil
	}
	return data
}

func homeFrame(width, height int, placements []homePlacement) string {
	var frame strings.Builder
	fmt.Fprintf(&frame, "%s%d,%d", homeFramePrefix, width, height)
	for _, place := range placements {
		fmt.Fprintf(&frame, ";%d,%d,%d", place.row, place.column, place.icon)
	}
	frame.WriteByte('\a')
	return frame.String()
}

// Bubble Tea uses io.WriteString for screen resets and leaving the alt screen.
func (out *homeGraphics) WriteString(s string) (int, error) {
	return out.Write([]byte(s))
}

func (out *homeGraphics) Write(data []byte) (int, error) {
	var result bytes.Buffer
	if bytes.Contains(data, []byte("\x1b[?1049l")) {
		out.deleteImages(&result, true)
		out.frame = ""
	}
	if bytes.Contains(data, []byte("\x1b[2J")) {
		out.frame = ""
	}
	// Serialize image placements from the exact frame Bubble Tea flushed. Its
	// differential renderer may skip unchanged text rows or coalesce model frames.
	start := bytes.Index(data, []byte(homeFramePrefix))
	if start >= 0 {
		end := bytes.IndexByte(data[start:], '\a')
		if end >= 0 {
			end += start
			result.Write(data[:start])
			result.Write(data[end+1:])
			frame := string(data[start+len(homeFramePrefix) : end])
			if frame != out.frame {
				out.drawFrame(&result, frame)
				out.frame = frame
			}
			if _, err := out.File.Write(result.Bytes()); err != nil {
				return 0, err
			}
			return len(data), nil
		}
	}
	if result.Len() == 0 {
		return out.File.Write(data)
	}
	result.Write(data)
	if _, err := out.File.Write(result.Bytes()); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (out *homeGraphics) deleteImages(to *bytes.Buffer, free bool) {
	mode := 'i'
	if free {
		mode = 'I'
	}
	for icon := claudeHomeIcon; icon < homeIconCount; icon++ {
		if out.loaded[icon] {
			fmt.Fprintf(to, "\x1b_Ga=d,d=%c,i=%d,q=2;\x1b\\", mode, out.baseID+uint32(icon))
			out.loaded[icon] = !free
		}
	}
}

func (out *homeGraphics) drawFrame(to *bytes.Buffer, frame string) {
	out.deleteImages(to, false)
	_, positions, ok := strings.Cut(frame, ";")
	if !ok {
		return
	}
	to.WriteString("\x1b7")
	for _, position := range strings.Split(positions, ";") {
		parts := strings.Split(position, ",")
		if len(parts) != 3 {
			continue
		}
		row, _ := strconv.Atoi(parts[0])
		column, _ := strconv.Atoi(parts[1])
		id, _ := strconv.Atoi(parts[2])
		if row < 1 || column < 1 || id < int(claudeHomeIcon) || id >= int(homeIconCount) {
			continue
		}
		icon := homeIcon(id)
		if image := out.images[icon]; !out.loaded[icon] && len(image) > 0 {
			out.upload(to, icon, image)
			out.loaded[icon] = true
		}
		if out.loaded[icon] {
			fmt.Fprintf(to, "\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,c=2,r=1,C=1,q=2;\x1b\\",
				row, column, out.baseID+uint32(icon), row)
		}
	}
	to.WriteString("\x1b8")
}

func (out *homeGraphics) upload(to *bytes.Buffer, icon homeIcon, image []byte) {
	data := base64.StdEncoding.EncodeToString(image)
	header := fmt.Sprintf("a=t,f=100,i=%d,q=2,", out.baseID+uint32(icon))
	for len(data) > 0 {
		size := min(4096, len(data))
		more := 0
		if size < len(data) {
			more = 1
		}
		fmt.Fprintf(to, "\x1b_G%sm=%d;%s\x1b\\", header, more, data[:size])
		data, header = data[size:], ""
	}
}
