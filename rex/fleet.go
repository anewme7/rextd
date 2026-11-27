package rex

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// Host keeps the stable configured ID separate from the label that rex -S accepts.
type Host struct {
	ID, Label, Server string
}

var localHost = Host{ID: "local", Label: "本机"}

func (h Host) command(ctx context.Context, tail ...string) *exec.Cmd {
	return h.commandWithTimeout(ctx, 4*time.Second, tail...)
}

func (h Host) commandWithTimeout(ctx context.Context, timeout time.Duration, tail ...string) *exec.Cmd {
	args := []string{"--autostart=false", "--timeout", timeout.String(), "--color", "never"}
	if h.Server != "" {
		args = append(args, "-S", h.Server)
	}
	return exec.CommandContext(ctx, "rex", append(args, tail...)...)
}

type HostStatus struct {
	Host
	State                     State
	SessionCount, WindowCount int
}

type Fleet struct {
	Hosts     []HostStatus
	Panes     []Pane
	LocalName string
}

func localMachineName(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	clients, err := localHost.appClients(ctx)
	if err == nil {
		var name string
		for _, client := range clients {
			if client.Info.Kind != "app" || client.Principal.Transport != "unix" || client.State == "lost" {
				continue
			}
			label := cleaned(client.Info.Label)
			if label == "" {
				continue
			}
			if name != "" && name != label {
				name = ""
				break
			}
			name = label
		}
		if name != "" {
			return name
		}
	}
	hostname, _ := os.Hostname()
	name, _, _ := strings.Cut(hostname, ".")
	return cleaned(name)
}

func configuredHosts(ctx context.Context) ([]Host, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	raw, err := localHost.command(ctx, "hosts", "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	var reply struct {
		Hosts []struct {
			Host struct{ ID, Label string }
		}
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	seen := map[string]bool{localHost.ID: true}
	hosts := []Host{localHost}
	for _, row := range reply.Hosts {
		host := row.Host
		if host.ID == "" || host.Label == "" || seen[host.ID] {
			continue
		}
		seen[host.ID] = true
		hosts = append(hosts, Host{host.ID, cleaned(host.Label), host.Label})
	}
	return hosts, nil
}

// WatchFleet shares one replaceable aggregate with the UI. Each configured
// server connects and reconnects independently of the others.
func (c Context) WatchFleet(ctx context.Context, logger *slog.Logger, previews *TerminalPreviews) (<-chan Fleet, <-chan struct{}) {
	updates := make(chan Fleet, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(updates)
		c.watchFleet(ctx, logger, previews, updates)
	}()
	return updates, done
}

func (c Context) watchFleet(ctx context.Context, logger *slog.Logger, previews *TerminalPreviews, output chan Fleet) {
	hosts := []Host{localHost}
	latest := map[string]Snapshot{localHost.ID: {State: Connecting}}
	type update struct {
		host Host
		data Snapshot
	}
	incoming := make(chan update, 1)
	var closed []<-chan struct{}
	start := func(host Host) {
		up, done := c.watchHost(ctx, host, logger, previews)
		forwarded := make(chan struct{})
		closed = append(closed, forwarded)
		go func() {
			defer close(forwarded)
			defer func() { <-done }()
			for event := range up {
				select {
				case incoming <- update{host, event}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	defer func() {
		for _, complete := range closed {
			<-complete
		}
	}()
	type hostDirectory struct {
		hosts     []Host
		localName string
	}
	discovered := make(chan hostDirectory, 1)
	discoveryDone := make(chan struct{})
	defer func() { <-discoveryDone }()
	go func() {
		defer close(discoveryDone)
		name := localMachineName(ctx)
		list, err := configuredHosts(ctx)
		if err != nil && ctx.Err() == nil {
			logger.Warn("Rex host directory unavailable")
		}
		discovered <- hostDirectory{list, name}
	}()
	var localName string
	start(localHost)
	for {
		select {
		case <-ctx.Done():
			return
		case directory := <-discovered:
			localName = directory.localName
			if len(directory.hosts) != 0 {
				hosts = directory.hosts
				for _, host := range hosts[1:] {
					latest[host.ID] = Snapshot{State: Connecting}
					start(host)
				}
			}
		case event := <-incoming:
			latest[event.host.ID] = event.data
		}
		select {
		case <-output:
		default:
		}
		fleet := combine(hosts, latest, c.SessionID)
		fleet.LocalName = localName
		output <- fleet
	}
}

func combine(hosts []Host, latest map[string]Snapshot, current string) Fleet {
	var fleet Fleet
	for _, host := range hosts {
		s := latest[host.ID]
		fleet.Hosts = append(fleet.Hosts, HostStatus{host, s.State, s.SessionCount, s.WindowCount})
		fleet.Panes = append(fleet.Panes, s.Panes...)
	}
	slices.SortStableFunc(fleet.Panes, func(a, b Pane) int {
		aOffline, bOffline := latest[a.Host.ID].State == Offline, latest[b.Host.ID].State == Offline
		if aOffline != bOffline {
			if aOffline {
				return 1
			}
			return -1
		}
		if a.NeedsAttention() != b.NeedsAttention() {
			return comparePanes(a, b)
		}
		hereA, hereB := a.Host.ID == localHost.ID && a.SessionID == current, b.Host.ID == localHost.ID && b.SessionID == current
		if hereA != hereB {
			if hereA {
				return -1
			}
			return 1
		}
		return comparePanes(a, b)
	})
	return fleet
}

func (f Fleet) HostState(id string) State {
	for _, h := range f.Hosts {
		if h.ID == id {
			return h.State
		}
	}
	return ""
}

func (h Host) displayError(err error) error {
	if e, ok := err.(*exec.ExitError); ok {
		if line := cleaned(string(e.Stderr)); line != "" {
			return errors.New(line)
		}
	}
	return err
}
