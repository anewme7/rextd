package rex

import (
	"context"
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
)

//go:embed targets.lua
var targetsScript string

type PaneTarget struct {
	Pane
	Unavailable string
}

// LinkedPanes resolves saved links against the configured Rex hosts and their
// current layouts. It only queries sessions attached to this task.
func LinkedPanes(ctx context.Context, panes []Pane) []PaneTarget {
	hosts := map[string]Host{localHost.ID: localHost}
	if slices.ContainsFunc(panes, func(p Pane) bool { return p.Host.ID != localHost.ID }) {
		directory, _ := configuredHosts(ctx)
		for _, host := range directory {
			hosts[host.ID] = host
		}
	}

	groups := make(map[string][]int)
	targets := make([]PaneTarget, len(panes))
	for i, pane := range panes {
		targets[i] = PaneTarget{Pane: pane, Unavailable: "host_unavailable"}
		host, ok := hosts[pane.Host.ID]
		if ok {
			targets[i].Host = host
			groups[host.ID] = append(groups[host.ID], i)
		}
	}

	slots := make(chan struct{}, 4)
	var workers sync.WaitGroup
	for hostID, indices := range groups {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			workers.Wait()
			return targets
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			host := hosts[hostID]
			if !host.resolveLinks(ctx, targets, indices) && ctx.Err() == nil {
				host.resolveLinksFromLayout(ctx, targets, indices)
			}
		}()
	}
	workers.Wait()
	return targets
}

func (h Host) resolveLinks(ctx context.Context, targets []PaneTarget, indices []int) bool {
	type link struct {
		BlockID   string `json:"block_id"`
		SessionID string `json:"session_id"`
	}
	links := make([]link, 0, len(indices))
	for _, i := range indices {
		target := targets[i]
		links = append(links, link{target.ID, target.SessionID})
	}
	args, err := json.Marshal(map[string][]link{"links": links})
	if err != nil {
		return false
	}
	raw, err := h.commandWithTimeout(ctx, 12*time.Second, "do", "--fail-fast", "-e", targetsScript, "--args", string(args), "--json").Output()
	if err != nil {
		return false
	}
	var reply struct {
		Targets []struct {
			BlockID                             string `json:"block_id"`
			SessionID                           string `json:"session_id"`
			WindowID                            string `json:"window_id"`
			Label, Window, Session, Unavailable string
			Records                             json.RawMessage
		}
	}
	if json.Unmarshal(raw, &reply) != nil || len(reply.Targets) != len(indices) {
		return false
	}
	for offset, row := range reply.Targets {
		target := targets[indices[offset]]
		if row.BlockID != target.ID || (row.Unavailable == "" &&
			(row.SessionID != target.SessionID || row.WindowID == "")) {
			return false
		}
	}
	for offset, row := range reply.Targets {
		target := &targets[indices[offset]]
		target.Unavailable = row.Unavailable
		if target.Unavailable == "" {
			target.Label = first(cleaned(row.Label), target.Label)
			target.Session, target.Window, target.WindowID = cleaned(row.Session), cleaned(row.Window), row.WindowID
			// Rex Lua encodes an empty table as {}. Optional status must not
			// invalidate a terminal whose current location is already known.
			var records []report
			_ = json.Unmarshal(row.Records, &records)
			target.Pane = paneWithStatus(target.Pane, records)
		}
	}
	return true
}

func (h Host) resolveLinksFromLayout(ctx context.Context, targets []PaneTarget, indices []int) {
	type result struct {
		view layout
		err  error
	}
	views := make(map[string]result)
	for _, i := range indices {
		target := &targets[i]
		cached, ok := views[target.SessionID]
		if !ok {
			cached.view, cached.err = h.layout(ctx, target.SessionID)
			views[target.SessionID] = cached
		}
		if cached.err != nil {
			if strings.Contains(cached.err.Error(), "code=session_not_found") {
				target.Unavailable = "pane_closed"
			}
			continue
		}
		target.Unavailable = "pane_closed"
		target.resolve(ctx, cached.view)
	}
}

func (p *PaneTarget) resolve(ctx context.Context, view layout) {
	for _, window := range view.Windows {
		for _, layer := range window.Layers {
			for _, block := range layer.Blocks {
				if block.BlockID != p.ID || block.Creator != "com.superlogical.terminal" {
					continue
				}
				p.Unavailable = ""
				p.Session, p.Window, p.WindowID = cleaned(view.Label), cleaned(window.Label), window.ID
				p.Label = first(cleaned(block.Label), p.Label)
				raw, err := p.Host.command(ctx, "block", "call", "-s", p.SessionID, "-b", p.ID, "program_status").Output()
				var status struct{ Records []report }
				if err == nil && json.Unmarshal(raw, &status) == nil {
					p.Pane = paneWithStatus(p.Pane, status.Records)
				}
				return
			}
		}
	}
}
