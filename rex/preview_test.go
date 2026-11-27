package rex

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDecodeSnapshotKeepsPaneIdentityAndWaitStart(t *testing.T) {
	raw := []byte(`{"panes":{"block:b":{"label":"窗格","window":"窗口","records":[
		{"state":"working"},{"state":"blocked","kind":"permission","updated_at":"2026-11-18T02:00:00Z","app":"codex"}
	]},"block:a":{"records":[{"state":"idle","app":"claude-code"}]},"block:shell":{"records":{}}}}`)
	snapshot, ok := decodeSnapshot(raw, nil)
	if !ok || len(snapshot.Panes) != 2 || snapshot.Panes[0].ID != "block:b" ||
		snapshot.Panes[0].Status != "permission" || !snapshot.Panes[0].NeedsAttention() {
		t.Fatalf("bad snapshot: %+v", snapshot)
	}
	last := snapshot.Panes
	snapshot, ok = decodeSnapshot([]byte(`{"partial":true,"panes":{"block:b":{"records":[
		{"state":"blocked","kind":"permission","updated_at":"2026-11-18T02:01:00Z"}
	]}}}`), last)
	if !ok || snapshot.State != Partial || len(snapshot.Panes) != 1 || !snapshot.Panes[0].Since.Equal(last[0].Since) {
		t.Fatalf("bad updated snapshot: %+v", snapshot)
	}
}

func TestWatchCancelsAndReapsRexProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	t.Setenv("REX_TEST_PID_FILE", pidFile)
	const script = `#!/bin/sh
cat >/dev/null
echo $$ > "$REX_TEST_PID_FILE"
echo '{"panes":{}}'
exec sleep 60
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	updates, done := (Context{"session:test", "block:test"}).watchHost(ctx, localHost, logger, nil)
	select {
	case snapshot := <-updates:
		if snapshot.State != Live {
			t.Fatalf("got %s, want live", snapshot.State)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no snapshot")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("watch failed to stop")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("Rex subprocess %d was not reaped: %v", pid, err)
	}
}

func TestReusedPreviewOnlyDeliversTheCurrentRequest(t *testing.T) {
	previews, err := newTerminalPreviews(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer previews.Close()
	host := Host{ID: "host:remote"}
	previous, stopPrevious := previews.Open(Pane{Host: host, SessionID: "session:one", ID: "block:one"})
	current, stopCurrent := previews.Open(Pane{Host: host, SessionID: "session:two", ID: "block:two"})
	defer stopCurrent()
	if _, open := <-previous; open {
		t.Fatal("changing the target should close the previous preview")
	}
	stopPrevious()
	previews.publish(host.ID, []byte(`{"id":1,"content":"old frame"}`))
	previews.publish("host:other", []byte(`{"id":2,"content":"different host"}`))
	select {
	case content := <-current:
		t.Fatalf("unexpected frame: %q", content)
	default:
	}
	previews.publish(host.ID, []byte(`{"id":2,"content":"new frame"}`))
	select {
	case content := <-current:
		if content != "new frame" {
			t.Fatalf("got frame %q", content)
		}
	default:
		t.Fatal("current preview did not receive its frame")
	}
	previews.disconnected(host.ID)
	if _, open := <-current; open {
		t.Fatal("disconnect should close the active preview")
	}
	if _, err := os.Stat(previews.path); !os.IsNotExist(err) {
		t.Fatalf("disconnect did not clear the request: %v", err)
	}
}

func TestFocusUsesTheSameNamedAppOnRemoteServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REX_CLIENT", "")
	logfile := filepath.Join(dir, "calls")
	t.Setenv("REX_TEST_CALLS", logfile)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	const script = `#!/bin/sh
printf '%s\n' "$*" >> "$REX_TEST_CALLS"
case "$*" in
  *"session.view"*"session:home"*)
    printf '%s\n' '{"session_id":"session:home","windows":[{"window_id":"window:here","layers":[{"blocks":[{"block_id":"block:sidebar","owner":"client:here"}]}]}]}';;
  *"-S build do -s session:remote"*"session.focus_block"*"client.perform"*'{"block":"block:target","label":"Device","session":"session:remote"}'*)
    printf '%s\n' '{"window_id":"window:new"}';;
  *"client.list"*)
    printf '%s\n' '{"clients":[{"client_id":"client:here","connection_state":"connected","info":{"kind":"app","label":"Device"}}]}';;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	target := Pane{Host: Host{"host:id", "build", "build"}, SessionID: "session:remote",
		WindowID: "window:old", ID: "block:target"}
	if err := (Context{"session:home", "block:sidebar"}).Focus(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logfile)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	if strings.Count(calls, "-S build") != 1 || !strings.Contains(calls, "session.select") || strings.Contains(calls, "-C client:other") {
		t.Fatalf("expected one scoped remote connection for tab selection:\n%s", calls)
	}
}

func TestPreviewCacheReclaimsOnlyExitedOwners(t *testing.T) {
	cache := t.TempDir()
	child := exec.Command("true")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	stale, err := os.MkdirTemp(cache, "pane-"+strconv.Itoa(child.Process.Pid)+"-")
	if err != nil {
		t.Fatal(err)
	}
	active, err := newTerminalPreviews(cache)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	second, err := newTerminalPreviews(cache)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	for _, dir := range []string{stale, second.dir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("directory was not cleaned: %s (%v)", dir, err)
		}
	}
	if _, err := os.Stat(active.dir); err != nil {
		t.Fatal("live preview was reclaimed", err)
	}
}

func TestSourceAppLabelRejectsDisconnectedOrAmbiguousOrigin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	const cli = `#!/bin/sh
case "$*" in
  *"client.list"*) printf '%s\n' "$CLIENTS";;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(cli), 0700); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"lost", "disconnected", "reconnecting", "connected"} {
		t.Run(state, func(t *testing.T) {
			t.Setenv("CLIENTS", `{"clients":[{"client_id":"client:old","info":{"kind":"app","label":"Phone"},"connection_state":"`+state+`"},{"client_id":"client:new","info":{"kind":"app","label":"Phone"},"connection_state":"connected"}]}`)
			if label, err := sourceAppLabel(context.Background(), "client:old"); err == nil || label != "" {
				t.Fatalf("used stale or ambiguous originating app: %q", label)
			}
		})
	}
}

func TestOldBoardFollowsOnlyItsReconnectedDevice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REX_CLIENT", "client:old")
	t.Setenv("REX_TD_CLIENT_LABEL", "Phone")
	log := filepath.Join(dir, "mutations")
	t.Setenv("FOCUS_LOG", log)
	const cli = `#!/bin/sh
case "$*" in
  *"session.view"*)
    echo '{"session_id":"session:work","windows":[{"window_id":"window:from","layers":[{"blocks":[{"block_id":"block:board","owner":"client:old"}]}]},{"window_id":"window:to","layers":[{"blocks":[{"block_id":"block:target"}]}]}]}';;
  *"block call"*"size") printf '{"owner":"%s"}\n' "$CURRENT_OWNER";;
  *"client.list"*) printf '%s\n' "$CLIENTS";;
  *"session.focus_block"*|*"session.select"*) echo "$*" >> "$FOCUS_LOG"; echo '{}';;
  *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(cli), 0700); err != nil {
		t.Fatal(err)
	}
	clients := `{"clients":[{"client_id":"client:new","info":{"kind":"app","label":"Phone"},"connection_state":"connected"},{"client_id":"client:mac","info":{"kind":"app","label":"Mac"},"connection_state":"connected"}]}`
	target := Pane{Host: localHost, SessionID: "session:work", ID: "block:target"}
	for _, tc := range []struct {
		name, owner, clients string
		allow                bool
	}{
		{"reconnected", "client:new", clients, true},
		{"other device owns board", "client:mac", clients, false},
		{"no owner", "", clients, false},
		{"duplicate label", "client:new", strings.Replace(clients, `"Mac"`, `"Phone"`, 1), false},
		{"original ID still live elsewhere", "client:new", strings.Replace(clients, `"client:mac"`, `"client:old"`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CURRENT_OWNER", tc.owner)
			t.Setenv("CLIENTS", tc.clients)
			if err := os.WriteFile(log, nil, 0600); err != nil {
				t.Fatal(err)
			}
			err := (Context{"session:work", "block:board"}).Focus(context.Background(), target)
			raw, readErr := os.ReadFile(log)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.allow {
				if err != nil || !strings.Contains(string(raw), "-C client:new do session.select") {
					t.Fatalf("did not select the reconnected phone: %s %v", raw, err)
				}
			} else if err == nil || len(raw) != 0 {
				t.Fatalf("unverified owner changed focus: %s %v", raw, err)
			}
		})
	}
}
