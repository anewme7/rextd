package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"path"
	"strconv"
	"strings"
	"time"

	"rex-td/rex"
)

type linkSource string

const (
	linkManual linkSource = "manual"
	linkRexTD  linkSource = "rextd"
	linkTab    linkSource = "tab"
	linkDir    linkSource = "dir"
	linkOnly   linkSource = "only"
)

var errPaneLinkChanged = errors.New("会话归属有变化，请重新选择")

type paneBinding struct {
	source   linkSource
	linkedAt int64
}

func (t *task) addBinding(p rex.Pane, binding paneBinding) {
	t.panes = append(t.panes, p)
	t.setBinding(p, binding)
}

func (t *task) setBinding(p rex.Pane, binding paneBinding) {
	if t.bindings == nil {
		t.bindings = make(map[paneIdentity]paneBinding)
	}
	t.bindings[identityOf(p)] = binding
}

type paneWindow struct{ host, session, window string }
type paneDirectory struct{ host, dir string }

func windowOf(p rex.Pane) paneWindow {
	return paneWindow{p.Host.ID, p.SessionID, p.WindowID}
}

type linkHints struct {
	owners    map[paneIdentity]bool
	open      map[int64]bool
	tabs      map[paneWindow]int64
	dirs      map[paneDirectory]int64
	only      int64
	recent    int64
	latestUse int64
}

func buildLinkHints(tasks []*task, live map[paneIdentity]rex.Pane, dirs map[paneIdentity]string, touched map[int64]int64) linkHints {
	hints := linkHints{
		owners: make(map[paneIdentity]bool), open: make(map[int64]bool),
		tabs: make(map[paneWindow]int64), dirs: make(map[paneDirectory]int64),
	}
	archived := make(map[int64]bool)
	for _, t := range tasks {
		archived[t.id] = t.archived
	}
	var latestBinding, linkedTask int64
	for _, t := range tasks {
		active := t.state == working && !t.archived && !archived[t.parentID]
		if active {
			if len(hints.open) == 0 {
				hints.only = t.id
			} else {
				hints.only = 0
			}
			hints.open[t.id] = true
			last := touched[t.id]
			if hints.recent == 0 || last > hints.latestUse {
				hints.recent, hints.latestUse = t.id, last
			}
			for _, binding := range t.bindings {
				if binding.linkedAt > latestBinding {
					latestBinding, linkedTask = binding.linkedAt, t.id
				}
			}
		}
		for _, saved := range t.panes {
			id := identityOf(saved)
			hints.owners[id] = true
			p, available := live[id]
			if available && p.WindowID != "" {
				noteLinkOwner(hints.tabs, windowOf(p), t.id)
			} else if !available && saved.WindowID != "" && t.bindings[id].source == linkRexTD {
				// The originating shell may have closed while its tab is still open.
				noteLinkOwner(hints.tabs, windowOf(saved), t.id)
			}
			if active && available && p.App != "" && dirs[id] != "" {
				noteLinkOwner(hints.dirs, paneDirectory{p.Host.ID, dirs[id]}, t.id)
			}
		}
	}
	if hints.latestUse == 0 && linkedTask != 0 {
		hints.recent = linkedTask
	}
	return hints
}

func noteLinkOwner[K comparable](owners map[K]int64, key K, taskID int64) {
	if owner, found := owners[key]; !found {
		owners[key] = taskID
	} else if taskID != owner {
		owners[key] = 0
	}
}

// A nonzero task with an empty source is a suggestion requiring confirmation.
func (h linkHints) match(p rex.Pane, dir string, fresh bool) (int64, linkSource) {
	if p.App == "" || h.owners[identityOf(p)] {
		return 0, ""
	}
	if fresh {
		if id := h.tabs[windowOf(p)]; h.open[id] {
			return id, linkTab
		}
		if id := h.dirs[paneDirectory{p.Host.ID, dir}]; id != 0 {
			return id, linkDir
		}
		if h.only != 0 {
			return h.only, linkOnly
		}
	}
	return h.recent, ""
}

func paneOpenedAt(id string) time.Time {
	raw, ok := strings.CutPrefix(id, "block:")
	if !ok || len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[14] != '7' {
		return time.Time{}
	}
	millis, err := strconv.ParseInt(raw[:8]+raw[9:13], 16, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(millis)
}

func newAgentPane(p rex.Pane, started time.Time, sawShell bool) bool {
	// A pane created before the board opened can start its Agent later.
	if sawShell {
		return true
	}
	opened := paneOpenedAt(p.ID)
	return !opened.IsZero() && !opened.Before(started.Truncate(time.Millisecond))
}

func genericDirectories(value string) map[string]bool {
	dirs := make(map[string]bool)
	for _, dir := range strings.Split(value, ":") {
		if path.IsAbs(dir) {
			dirs[path.Clean(dir)] = true
		}
	}
	return dirs
}

func projectDirectory(cwd, home string, generic map[string]bool) string {
	if !path.IsAbs(cwd) {
		return ""
	}
	dir := path.Clean(cwd)
	if dir == "/" || dir == path.Clean(home) || generic[dir] {
		return ""
	}
	return dir
}

type agentObservation struct {
	app             string
	fresh, reviewed bool
	dir             string
	directoryReadAt time.Time
}

type paneDirectoryResult map[paneIdentity]agentObservation

type directoryReply struct {
	id          paneIdentity
	observation agentObservation
}

func (m *model) syncAutolink() tea.Cmd {
	m.autolinkDirty = false
	if m.store == nil || !m.rexContext.Inside() {
		return nil
	}

	live := make(map[paneIdentity]rex.Pane, len(m.feed.Panes))
	dirs := make(map[paneIdentity]string)
	for _, p := range m.feed.Panes {
		id := identityOf(p)
		if m.feed.HostState(p.Host.ID) == rex.Offline {
			continue
		}
		live[id] = p
		previous, seen := m.observedAgents[id]
		if !seen || previous.app != p.App {
			m.observedAgents[id] = agentObservation{
				app: p.App, fresh: newAgentPane(p, m.autolinkStarted, seen && previous.app == ""),
			}
		}
		dirs[id] = m.observedAgents[id].dir
	}
	for id := range m.observedAgents {
		if _, available := live[id]; !available && m.feed.HostState(id.host) == rex.Live {
			delete(m.observedAgents, id)
			delete(m.removedAgents, id)
		}
	}

	hints := buildLinkHints(m.tasks, live, dirs, m.taskTouched)
	tasks := make(map[int64]*task, len(m.tasks))
	anchors := make(map[string][]rex.Pane)
	for _, t := range m.tasks {
		tasks[t.id] = t
		for _, saved := range t.panes {
			id := identityOf(saved)
			if p, ok := live[id]; ok {
				observed := m.observedAgents[id]
				observed.reviewed = true
				m.observedAgents[id] = observed
				if hints.open[t.id] && p.App != "" {
					anchors[p.Host.ID] = append(anchors[p.Host.ID], p)
				}
			}
		}
	}

	now := time.Now()
	const directoryCacheAge = time.Minute
	reads := make(map[paneIdentity]rex.Pane)
	queue := func(p rex.Pane) {
		if len(reads) < 16 {
			reads[identityOf(p)] = p
		}
	}
	m.newAgents = m.newAgents[:0]
	m.suggestedTask = hints.recent
	for _, p := range m.feed.Panes {
		id := identityOf(p)
		if p.App == "" || hints.owners[id] || live[id].ID == "" || m.removedAgents[id] {
			continue
		}
		observation := m.observedAgents[id]
		if !observation.reviewed && observation.fresh && len(hints.open) > 0 && m.openingTaskTab == 0 {
			target, source := hints.match(p, "", true)
			if source != linkTab && len(anchors[id.host]) > 0 {
				if observation.directoryReadAt.IsZero() {
					queue(p)
					continue
				}
				if observation.dir != "" {
					ready := true
					for _, anchor := range anchors[id.host] {
						if now.Sub(m.observedAgents[identityOf(anchor)].directoryReadAt) > directoryCacheAge {
							queue(anchor)
							ready = false
						}
					}
					if !ready {
						continue
					}
					target, source = hints.match(p, observation.dir, true)
				}
			}
			if source != "" {
				binding := paneBinding{source: source, linkedAt: now.UnixMilli()}
				added, err := m.store.bindNewPane(target, p, binding, true)
				if err != nil {
					m.saved(err)
					continue
				}
				if added {
					tasks[target].addBinding(p, binding)
					hints.owners[id] = true
					noteLinkOwner(hints.tabs, windowOf(p), target)
					if observation.dir != "" {
						noteLinkOwner(hints.dirs, paneDirectory{id.host, observation.dir}, target)
					}
					m.refreshCandidates()
				}
				observation.reviewed = true
				m.observedAgents[id] = observation
				continue
			}
			observation.reviewed = true
			m.observedAgents[id] = observation
		}
		m.newAgents = append(m.newAgents, p)
	}
	slices.SortStableFunc(m.newAgents, func(a, b rex.Pane) int {
		freshA, freshB := m.observedAgents[identityOf(a)].fresh, m.observedAgents[identityOf(b)].fresh
		if freshA != freshB {
			if freshA {
				return -1
			}
			return 1
		}
		return strings.Compare(b.ID, a.ID)
	})
	if m.boardSuggest {
		if _, available := m.suggestionPane(); !available {
			m.boardSuggest, m.boardSuggestionPane = false, paneIdentity{}
		}
	}
	if m.boardPane != (paneIdentity{}) && m.homeSelection().pane == (paneIdentity{}) {
		m.boardPane = paneIdentity{}
	}
	if m.readingDirectories || len(reads) == 0 {
		return nil
	}
	m.readingDirectories = true
	ctx, generic := m.ctx, m.genericDirs
	return func() tea.Msg { return readPaneDirectories(ctx, reads, generic) }
}

func readPaneDirectories(ctx context.Context, panes map[paneIdentity]rex.Pane, generic map[string]bool) paneDirectoryResult {
	result := make(paneDirectoryResult, len(panes))
	replies := make(chan directoryReply, len(panes))
	jobs := make(chan rex.Pane, len(panes))
	for _, p := range panes {
		jobs <- p
	}
	close(jobs)
	for range min(4, len(panes)) {
		go func() {
			for p := range jobs {
				var process struct {
					Foreground struct{ CWD string }
					Home       string
				}
				started := time.Now()
				args := map[string]any{"session_id": p.SessionID, "block_id": p.ID, "args": map[string]any{}}
				_ = callRexAPI(ctx, p.Host, p.SessionID, "com.superlogical.terminal.process", args, &process)
				replies <- directoryReply{identityOf(p), agentObservation{app: p.App, dir: projectDirectory(process.Foreground.CWD, process.Home, generic), directoryReadAt: started}}
			}
		}()
	}
	for range panes {
		reply := <-replies
		result[reply.id] = reply.observation
	}
	return result
}

func callRexAPI(ctx context.Context, host rex.Host, session, method string, input, output any) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	args := []string{"--autostart=false", "--timeout", "1800ms", "--color", "never"}
	if host.Server != "" {
		args = append(args, "-S", host.Server)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	args = append(args, "api", "call", "-s", session, method, string(body))
	raw, err := exec.CommandContext(ctx, "rex", args...).Output()
	if exit, ok := err.(*exec.ExitError); ok {
		if problem := strings.TrimSpace(storedDisplay(string(exit.Stderr))); problem != "" {
			return errors.New(problem)
		}
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, output)
}
