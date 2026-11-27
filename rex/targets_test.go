package rex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLinkedPanesUsesConfiguredHostAndLivePane(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	const cli = `#!/bin/sh
case "$*" in
  *"hosts list"*)
    echo '{"hosts":[{"host":{"id":"host:remote","label":"renamed"}}]}';;
  *"-S renamed"*"session.view"*"session:ready"*)
    echo '{"session_id":"session:ready","label":"Build","windows":[{"window_id":"window:current","layers":[{"blocks":[{"block_id":"block:ready","label":"Compile","creator_name":"com.superlogical.terminal"}]}]}]}';;
  *"-S renamed"*"block call"*"block:ready"*"program_status"*)
    echo '{"records":[{"app":"codex","state":"blocked","kind":"permission"}]}';;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(cli), 0700); err != nil {
		t.Fatal(err)
	}

	host := Host{ID: "host:remote", Label: "old", Server: "old"}
	targets := LinkedPanes(context.Background(), []Pane{
		{Host: host, SessionID: "session:ready", WindowID: "window:old", ID: "block:ready"},
		{Host: host, SessionID: "session:ready", ID: "block:gone"},
		{Host: Host{ID: "host:removed", Server: "renamed"}, SessionID: "session:ready", ID: "block:ready"},
	})
	if len(targets) != 3 {
		t.Fatalf("expected a result for each link, got %v", targets)
	}
	if got := targets[0]; got.Unavailable != "" || got.WindowID != "window:current" || got.Status != "permission" {
		t.Fatalf("live link did not resolve to the current location and status: %+v", got)
	}
	if targets[1].Unavailable != "pane_closed" || targets[2].Unavailable != "host_unavailable" {
		t.Fatalf("closed panes and removed hosts must be distinguishable: %+v", targets)
	}
}

func TestLinkedPanesBatchesAcrossSessionsOnOneHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	const cli = `#!/bin/sh
case "$*" in
  *"do --fail-fast"*'{"links":[{"block_id":"block:first","session_id":"session:one"},{"block_id":"block:second","session_id":"session:two"},{"block_id":"block:gone","session_id":"session:one"}]}'*)
    echo '{"targets":[{"block_id":"block:first","session_id":"session:one","window_id":"window:new","session":"Build","records":[{"state":"done"}]},{"block_id":"block:second","session_id":"session:two","window_id":"window:next","label":"Check","records":{}},{"block_id":"block:gone","unavailable":"pane_closed"}]}';;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(cli), 0700); err != nil {
		t.Fatal(err)
	}
	targets := LinkedPanes(context.Background(), []Pane{
		{Host: localHost, SessionID: "session:one", WindowID: "window:old", ID: "block:first"},
		{Host: localHost, SessionID: "session:two", ID: "block:second"},
		{Host: localHost, SessionID: "session:one", ID: "block:gone"},
	})
	if targets[0].WindowID != "window:new" || targets[0].Status != "done" ||
		targets[1].Label != "Check" || targets[1].Status != "" || targets[2].Unavailable != "pane_closed" {
		t.Fatalf("native batch did not retain live locations and status: %+v", targets)
	}
}
