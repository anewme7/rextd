package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

func TestPaletteKeepsTheSelectedTaskAndPaneOnReturn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	board := openTaskBoard(t, path)
	boardKeys(board, "+", "First task", tea.KeyEnter, tea.KeyEsc)
	boardKeys(board, "+", "Release review", tea.KeyEnter)
	panes := []rex.Pane{
		{Host: rex.Host{ID: "local", Label: "Local"}, SessionID: "session:local", ID: "block:local", Label: "Local work"},
		{Host: rex.Host{ID: "host:remote", Label: "Build"}, SessionID: "session:remote", ID: "block:remote", Label: "Remote review"},
	}
	other, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.db.Close()
	for _, pane := range panes {
		if err := other.exec(insertPaneBinding, paneBindingValues(2, pane)...); err != nil {
			t.Fatal(err)
		}
	}
	board.Update(paletteStoreTick{})
	boardShows(t, board, "› Local work")
	if err := handlePaletteNavigation(other, json.NewEncoder(io.Discard), []string{"resume",
		`{"task_id":2,"pane":{"host_id":"host:remote","session_id":"session:remote","block_id":"block:remote"},"reason":"host_unavailable"}`}); err != nil {
		t.Fatal(err)
	}
	board.Update(paletteStoreTick{})
	boardShows(t, board, "› Remote review")
	boardShows(t, board, "无法连接目标主机")
}

func TestPaletteHomeWinsOverEarlierPendingRestore(t *testing.T) {
	t.Setenv("REX_CLIENT", "client:desktop")
	path := filepath.Join(t.TempDir(), "tasks.sqlite3")
	m := openTaskBoard(t, path)
	boardKeys(m, "+", "Home race", tea.KeyEnter, tea.KeyEsc)
	other, err := openTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.db.Close()
	if err := other.savePaletteResume("client:desktop", 1, paneIdentity{}, ""); err != nil {
		t.Fatal(err)
	}
	m.Update(openHome{})
	m.Update(paletteStoreTick{})
	if m.page != listPage || m.homeTab != nowTab {
		t.Fatal("an earlier Find request took the board away from Home")
	}
	if err := other.savePaletteResume("client:phone", 1, paneIdentity{}, ""); err != nil {
		t.Fatal(err)
	}
	m.Update(paletteStoreTick{})
	if m.page != listPage {
		t.Fatal("another client's Find changed this board")
	}
	if err := other.savePaletteResume("client:desktop", 1, paneIdentity{}, ""); err != nil {
		t.Fatal(err)
	}
	m.Update(paletteStoreTick{})
	if m.page != detailPage {
		t.Fatal("a newer Find request did not open details")
	}
}

func TestPaletteResumeBoundsDisconnectedClientHistory(t *testing.T) {
	m := openTaskBoard(t, filepath.Join(t.TempDir(), "tasks.sqlite3"))
	boardKeys(m, "+", "History", tea.KeyEnter)
	if err := m.store.savePaletteResume("", 1, paneIdentity{}, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 130; i++ {
		if err := m.store.savePaletteResume(fmt.Sprintf("client:%d", i), 1, paneIdentity{}, ""); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := m.store.db.QueryRow("SELECT count(*) FROM palette_client_resume").Scan(&count); err != nil || count != 129 {
		t.Fatalf("expected CLI and 128 client positions, got %d (%v)", count, err)
	}
	old, err := m.store.readPaletteResume("")
	if err != nil || old.taskID != 1 {
		t.Fatal("lost the CLI restore position")
	}
}
