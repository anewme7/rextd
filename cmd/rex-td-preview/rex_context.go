package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"rex-td/rex"
)

func currentRexContext(parent context.Context) (rex.Context, error) {
	legacy := rex.Current()
	current := rex.Context{SidebarID: legacy.SidebarID}
	if current.SidebarID == "" {
		return current, nil
	}

	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "rex", "--autostart=false", "--timeout=3s", "whoami", "--json").Output()
	if err != nil {
		if legacy.Inside() {
			return legacy, nil
		}
		return current, err
	}

	var location struct {
		BlockID   string `json:"block_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &location); err != nil {
		return current, err
	}
	if location.BlockID != current.SidebarID || location.SessionID == "" {
		return current, fmt.Errorf("Rex did not resolve the current pane to a session")
	}
	current.SessionID = location.SessionID
	return current, nil
}
