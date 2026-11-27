// Package rex exposes the Rex event feed to the Go sidebar preview.
package rex

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"
)

//go:embed activity.lua
var activityScript string

type State string

const (
	Connecting State = "connecting"
	Live       State = "live"
	Partial    State = "partial"
	Offline    State = "offline"
)

type Pane struct {
	ID, Label, Window, App, Status, Message string
	AgentTitle, TerminalTitle, ReportID     string
	SessionID, Session, WindowID            string
	Host                                    Host
	Since, ReportedAt, ActivityAt           time.Time
	Progress                                *int
}

func (p Pane) NeedsAttention() bool {
	return p.Status != "" && p.Status != "idle" && p.Status != "working"
}

func (p Pane) AttentionPriority() int {
	switch p.Status {
	case "permission", "question", "auth":
		return 3
	case "error", "waiting":
		return 2
	case "done":
		return 1
	default:
		return 0
	}
}

type Snapshot struct {
	State                     State
	Panes                     []Pane
	SessionCount, WindowCount int
}

type Context struct {
	SessionID, SidebarID string
}

func Current() Context {
	return Context{os.Getenv("REX_SESSION"), os.Getenv("REX_BLOCK")}
}

func (c Context) Inside() bool {
	return c.SessionID != "" && c.SidebarID != ""
}

// watchHost keeps one connection and the most recent snapshot for this server.
func (c Context) watchHost(ctx context.Context, host Host, logger *slog.Logger, previews *TerminalPreviews) (updates <-chan Snapshot, done <-chan struct{}) {
	output := make(chan Snapshot, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer close(output)
		send := func(s Snapshot) {
			select {
			case <-output:
			default:
			}
			output <- s
		}
		c.watch(ctx, host, logger.With("host", host.Label), previews, send)
	}()
	return output, finished
}

func (c Context) watch(ctx context.Context, host Host, logger *slog.Logger, previews *TerminalPreviews, send func(Snapshot)) {
	delay := time.Second
	failed := false
	var latest Snapshot
	for ctx.Err() == nil {
		c.read(ctx, host, previews, func(s Snapshot) {
			if latest.State != Live && latest.State != Partial {
				logger.Info("Rex status feed connected")
			}
			failed = false
			delay = time.Second
			latest = s
			send(s)
		})
		if ctx.Err() != nil {
			return
		}
		if !failed {
			logger.Warn("Rex status feed disconnected")
			latest.State = Offline
			send(latest)
			failed = true
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func (c Context) read(ctx context.Context, host Host, previews *TerminalPreviews, publish func(Snapshot)) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	firstSnapshot := time.AfterFunc(15*time.Second, cancel)
	defer firstSnapshot.Stop()
	args := []string{"do", "--all", "--fail-fast", "-e", activityScript, "fleet=true"}
	if host.Server == "" {
		args = append(args, "sidebar_id="+c.SidebarID)
	} else if previews != nil {
		payload, _ := json.Marshal(map[string]any{
			"fleet": true, "preview_path": previews.path, "preview_host": host.ID,
		})
		args = []string{"do", "--all", "--fail-fast", "-e",
			ReadTerminalScreenLua + activityScript + watchTerminalPreviewLua, "--args", string(payload)}
		defer previews.disconnected(host.ID)
	}
	cmd := host.command(child, args...)
	cmd.WaitDelay = time.Second
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	defer out.Close()

	reader := bufio.NewScanner(out)
	reader.Buffer(make([]byte, 8192), 1<<20)
	var previous []byte
	var last []Pane
	for reader.Scan() {
		raw := reader.Bytes()
		if frame, ok := bytes.CutPrefix(raw, []byte("rextd-preview ")); ok && previews != nil {
			previews.publish(host.ID, frame)
			continue
		}
		if bytes.Equal(raw, previous) {
			continue
		}
		if snapshot, ok := decodeSnapshot(raw, last); ok {
			firstSnapshot.Stop()
			previous = append(previous[:0], raw...)
			last = snapshot.Panes
			for i := range snapshot.Panes {
				snapshot.Panes[i].Host = host
			}
			publish(snapshot)
		}
	}
	cancel()
	_ = cmd.Wait()
}

type report struct {
	ID           string
	State        string
	Kind         string
	App          string
	EffectiveApp string `json:"effective_app"`
	Title        string
	Msg          string
	Message      string
	UpdatedAt    string `json:"updated_at"`
	Progress     *int
}

func decodeSnapshot(raw []byte, last []Pane) (Snapshot, bool) {
	var payload struct {
		Panes        json.RawMessage
		Partial      bool
		SessionCount int `json:"session_count"`
		WindowCount  int `json:"window_count"`
	}
	if json.Unmarshal(raw, &payload) != nil || len(payload.Panes) == 0 {
		return Snapshot{}, false
	}
	var blocks map[string]struct {
		Label, Window, Session string
		TerminalTitle          string `json:"terminal_title"`
		ActivityAt             string `json:"activity_at"`
		SessionID              string `json:"session_id"`
		WindowID               string `json:"window_id"`
		Records                json.RawMessage
	}
	if string(payload.Panes) != "[]" && json.Unmarshal(payload.Panes, &blocks) != nil {
		return Snapshot{}, false
	}

	byID := make(map[string]Pane, len(last))
	for _, pane := range last {
		byID[pane.ID] = pane
	}
	snapshot := Snapshot{State: Live, Panes: make([]Pane, 0, len(blocks)),
		SessionCount: payload.SessionCount, WindowCount: payload.WindowCount}
	if payload.Partial {
		snapshot.State = Partial
	}
	for id, block := range blocks {
		var records []report
		if len(block.Records) != 0 {
			_ = json.Unmarshal(block.Records, &records)
		}
		base := Pane{ID: id, Label: cleaned(block.Label), Window: cleaned(block.Window),
			TerminalTitle: cleaned(block.TerminalTitle),
			Session:       cleaned(block.Session), SessionID: block.SessionID, WindowID: block.WindowID}
		base.ActivityAt, _ = time.Parse(time.RFC3339Nano, block.ActivityAt)
		best := paneWithStatus(base, records)
		if best.Status != "" || best.SessionID != "" {
			if old, ok := byID[id]; ok && old.Status == best.Status && !old.Since.IsZero() {
				best.Since = old.Since
			}
			snapshot.Panes = append(snapshot.Panes, best)
		}
	}
	slices.SortFunc(snapshot.Panes, comparePanes)
	return snapshot, true
}

func paneWithStatus(base Pane, records []report) Pane {
	best := base
	for _, record := range records {
		status := decodeStatus(record)
		if status == "" {
			continue
		}
		since, _ := time.Parse(time.RFC3339Nano, record.UpdatedAt)
		p := base
		p.App, p.Status = cleaned(first(record.EffectiveApp, record.App)), status
		p.AgentTitle, p.ReportID = cleaned(record.Title), record.ID
		p.Message, p.Since = cleaned(first(record.Msg, record.Message)), since
		p.ReportedAt = since
		if record.Progress != nil && *record.Progress >= 0 && *record.Progress <= 100 &&
			(record.State == "working" || record.State == "blocked") {
			p.Progress = record.Progress
		}
		if best.Status == "" || comparePanes(p, best) < 0 {
			best = p
		}
	}
	return best
}

func decodeStatus(r report) string {
	switch r.State {
	case "blocked":
		switch r.Kind {
		case "permission", "question", "auth":
			return r.Kind
		default:
			return "waiting"
		}
	case "working", "idle", "done", "error":
		return r.State
	default:
		return ""
	}
}

func comparePanes(a, b Pane) int {
	if a.NeedsAttention() != b.NeedsAttention() {
		if a.NeedsAttention() {
			return -1
		}
		return 1
	}
	if (a.Status == "") != (b.Status == "") {
		if a.Status == "" {
			return 1
		}
		return -1
	}
	if (a.Status == "idle") != (b.Status == "idle") {
		if a.Status == "idle" {
			return 1
		}
		return -1
	}
	if a.Since.IsZero() != b.Since.IsZero() {
		if a.Since.IsZero() {
			return 1
		}
		return -1
	}
	if order := a.Since.Compare(b.Since); order != 0 {
		return order
	}
	if order := strings.Compare(a.Label, b.Label); order != 0 {
		return order
	}
	return strings.Compare(a.ID, b.ID)
}

func first(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func cleaned(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
			if b.Len() >= 512 {
				break
			}
		}
	}
	return strings.TrimSpace(b.String())
}
