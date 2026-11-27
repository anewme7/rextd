package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

const (
	homeProbe            = "\x1b_Ga=q,i=3188020333,s=1,v=1,t=d,f=24;AAAA\x1b\\"
	homeProbeReply       = "\x1b_Gi=3188020333;"
	osc11Prefix          = "\x1b]11;rgb:"
	colorSchemeQuery     = "\x1b[?996n"
	osc11BackgroundQuery = "\x1b]11;?\x1b\\"
	colorSchemeNotifyOn  = "\x1b[?2031h"
	colorSchemeNotifyOff = "\x1b[?2031l"
)

var (
	colorSchemeDarkNotify  = []byte("\x1b[?997;1n")
	colorSchemeLightNotify = []byte("\x1b[?997;2n")
)

type homeTerminalProbe struct {
	graphics bool
	dark     bool
	hasTheme bool
	pending  []byte
}

type terminalThemeMsg struct {
	dark bool
}

type homeInput struct {
	*os.File
	pending  []byte
	fragment []byte
	program  atomic.Pointer[tea.Program]
}

// Keep homeInput compatible with Bubble Tea raw-mode setup (term.File)
// and macOS kqueue cancelreader (File with Fd/Name/Read/Close).
var _ term.File = (*homeInput)(nil)
var _ interface {
	Fd() uintptr
	Name() string
	Read([]byte) (int, error)
	Close() error
} = (*homeInput)(nil)

func (in *homeInput) attachProgram(p *tea.Program) {
	in.program.Store(p)
}

// Read at most one keyboard batch per call. Returning an empty batch after
// a terminal reply lets Bubble Tea wait on its cancellable file descriptor.
func (in *homeInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n, err := in.readRaw(p)
	data := p[:n]
	if len(in.fragment) > 0 {
		data = append(in.fragment, data...)
		in.fragment = nil
	}
	// Once the introducer identifies a terminal report, keep it across reads.
	// A lone Esc or Alt+[ is also a key, so bound only this ambiguous lookahead.
	deadline := time.Now().Add(25 * time.Millisecond)
	for {
		start := partialReplyStart(data)
		if start < 0 {
			break
		}
		if len(data)-start > 2 {
			in.fragment = bytes.Clone(data[start:])
			data = data[:start]
			break
		}
		if err != nil || (len(in.pending) == 0 && !inputReady(in.File, deadline)) {
			break
		}
		var one [1]byte
		var size int
		size, err = in.readRaw(one[:])
		if size == 0 {
			break
		}
		data = append(data, one[0])
	}
	_, _, data = stripGraphicsReply(data)
	oscDark, osc, data := extractOSC11Background(data)
	data, dark, changed := stripThemeNotifications(data)
	if osc || changed {
		if !changed {
			dark = oscDark
		}
		if prog := in.program.Load(); prog != nil {
			prog.Send(terminalThemeMsg{dark: dark})
		}
	}
	n = copy(p, data)
	if n < len(data) {
		in.pending = append(bytes.Clone(data[n:]), in.pending...)
	}
	return n, err
}

func (in *homeInput) readRaw(p []byte) (int, error) {
	if len(in.pending) > 0 {
		n := copy(p, in.pending)
		in.pending = in.pending[n:]
		return n, nil
	}
	return in.File.Read(p)
}

func partialReplyStart(data []byte) int {
	// A trailing Esc may itself be half of an OSC/APC string terminator.
	end := len(data)
	if end > 1 && data[end-1] == '\x1b' {
		end--
	}
	pos := bytes.LastIndexByte(data[:end], '\x1b')
	if pos >= 0 {
		tail := string(data[pos:])
		for _, seq := range []string{string(colorSchemeDarkNotify), string(colorSchemeLightNotify), osc11Prefix, homeProbeReply} {
			if len(tail) < len(seq) && strings.HasPrefix(seq, tail) {
				return pos
			}
		}
		if len(tail) < 64 && (strings.HasPrefix(tail, osc11Prefix) || strings.HasPrefix(tail, homeProbeReply)) {
			terminal, _ := findOSCTerminator(data[pos:])
			if terminal < 0 {
				return pos
			}
		}
	}
	if len(data) > 0 && data[len(data)-1] == '\x1b' {
		return len(data) - 1
	}
	return -1
}

func stripGraphicsReply(data []byte) (graphics, ok bool, rest []byte) {
	start := bytes.Index(data, []byte(homeProbeReply))
	if start < 0 {
		return false, false, data
	}
	body := data[start+len(homeProbeReply):]
	end := bytes.Index(body, []byte("\x1b\\"))
	if end < 0 {
		return false, false, data
	}
	graphics = string(body[:end]) == "OK"
	return graphics, true, append(data[:start], body[end+2:]...)
}

// extractOSC11Background parses an OSC 11 background color reply (rgb:R/G/B)
// from a startup probe buffer, returning whether the background is dark along
// with the buffer stripped of the reply sequence.
func extractOSC11Background(data []byte) (dark bool, ok bool, rest []byte) {
	const prefix = osc11Prefix
	start := bytes.Index(data, []byte(prefix))
	if start < 0 {
		return false, false, data
	}
	body := data[start+len(prefix):]
	end, termLen := findOSCTerminator(body)
	if end < 0 {
		return false, false, data
	}
	r, g, b, valid := parseOSCColorChannels(string(body[:end]))
	stripped := make([]byte, 0, len(data)-(len(prefix)+end+termLen))
	stripped = append(stripped, data[:start]...)
	stripped = append(stripped, body[end+termLen:]...)
	if !valid {
		return false, false, stripped
	}
	luminance := 0.2126*r + 0.7152*g + 0.0722*b
	return luminance < 0.5, true, stripped
}

func findOSCTerminator(body []byte) (index, length int) {
	for i := 0; i < len(body); i++ {
		if body[i] == '\x07' {
			return i, 1
		}
		if body[i] == '\x1b' && i+1 < len(body) && body[i+1] == '\\' {
			return i, 2
		}
	}
	return -1, 0
}

func parseOSCColorChannels(spec string) (r, g, b float64, ok bool) {
	parts := strings.Split(spec, "/")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	vals := [3]float64{}
	for i, part := range parts {
		if len(part) == 0 || len(part) > 4 {
			return 0, 0, 0, false
		}
		v, err := strconv.ParseUint(part, 16, 16)
		if err != nil {
			return 0, 0, 0, false
		}
		maxVal := float64((uint32(1) << (4 * len(part))) - 1)
		vals[i] = float64(v) / maxVal
	}
	return vals[0], vals[1], vals[2], true
}

func stripThemeNotifications(data []byte) (rest []byte, dark bool, changed bool) {
	if !bytes.Contains(data, colorSchemeDarkNotify) && !bytes.Contains(data, colorSchemeLightNotify) {
		return data, false, false
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if bytes.HasPrefix(data[i:], colorSchemeDarkNotify) {
			dark, changed = true, true
			i += len(colorSchemeDarkNotify)
			continue
		}
		if bytes.HasPrefix(data[i:], colorSchemeLightNotify) {
			dark, changed = false, true
			i += len(colorSchemeLightNotify)
			continue
		}
		out = append(out, data[i])
		i++
	}
	return out, dark, changed
}
