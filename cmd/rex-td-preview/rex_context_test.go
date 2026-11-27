package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentRexContextLocatesTheCurrentPane(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REX_BLOCK", "block:board")
	t.Setenv("REX_SESSION", "")

	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(`#!/bin/sh
case "$*" in
  *'whoami --json') test -n "$REX_TEST_LOCATION" && printf '%s\n' "$REX_TEST_LOCATION" ;;
  *) exit 1 ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("current Rex has a block and no session variable", func(t *testing.T) {
		t.Setenv("REX_TEST_LOCATION", `{"block_id":"block:board","session_id":"session:live"}`)
		got, err := currentRexContext(context.Background())
		if err != nil || !got.Inside() || got.SidebarID != "block:board" || got.SessionID != "session:live" {
			t.Fatalf("context = %+v, error = %v", got, err)
		}
	})
	t.Run("Rex reports a different pane", func(t *testing.T) {
		t.Setenv("REX_SESSION", "session:stale")
		t.Setenv("REX_TEST_LOCATION", `{"block_id":"block:other","session_id":"session:other"}`)
		got, err := currentRexContext(context.Background())
		if err == nil || got.Inside() || got.SidebarID != "block:board" {
			t.Fatalf("context = %+v, error = %v", got, err)
		}
	})
	t.Run("an older launcher can still supply both IDs", func(t *testing.T) {
		t.Setenv("REX_SESSION", "session:legacy")
		t.Setenv("REX_TEST_LOCATION", "")
		got, err := currentRexContext(context.Background())
		if err != nil || !got.Inside() || got.SessionID != "session:legacy" {
			t.Fatalf("context = %+v, error = %v", got, err)
		}
	})
	t.Run("a shell outside Rex must not adopt a default session", func(t *testing.T) {
		t.Setenv("REX_BLOCK", "")
		t.Setenv("REX_TEST_LOCATION", `{"block_id":"block:other","session_id":"session:other"}`)
		got, err := currentRexContext(context.Background())
		if err != nil || got.Inside() || got.SidebarID != "" {
			t.Fatalf("context = %+v, error = %v", got, err)
		}
	})
}
