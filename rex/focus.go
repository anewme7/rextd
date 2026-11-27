package rex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var errPaneClosed = errors.New("目标窗格已关闭或移出会话")

type layout struct {
	SessionID string `json:"session_id"`
	Label     string
	Windows   []struct {
		ID     string `json:"window_id"`
		Label  string
		Layers []struct {
			Blocks []struct {
				Owner   string
				Label   string
				Creator string `json:"creator_name"`
				BlockID string `json:"block_id"`
			}
		}
	}
}

func (v layout) locate(block string) (window, owner string) {
	for _, win := range v.Windows {
		for _, layer := range win.Layers {
			for _, b := range layer.Blocks {
				if b.BlockID == block {
					return win.ID, b.Owner
				}
			}
		}
	}
	return "", ""
}

func (h Host) call(ctx context.Context, method string, args map[string]string, result any) error {
	if args == nil {
		args = map[string]string{}
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	command := []string{"api", "call"}
	if session := args["session_id"]; session != "" {
		command = append(command, "-s", session)
	}
	output, err := h.command(ctx, append(command, method, string(payload))...).Output()
	if err != nil {
		return h.displayError(err)
	}
	if result != nil {
		return json.Unmarshal(output, result)
	}
	return nil
}

func (h Host) layout(ctx context.Context, id string) (layout, error) {
	var v layout
	err := h.call(ctx, "session.view", map[string]string{"session_id": id}, &v)
	if err == nil && v.SessionID != id {
		err = fmt.Errorf("Rex 返回了不同的会话")
	}
	return v, err
}

type appClient struct {
	ID        string `json:"client_id"`
	State     string `json:"connection_state"`
	Info      struct{ Kind, Label string }
	Principal struct{ Transport string }
}

func (h Host) appClients(ctx context.Context) ([]appClient, error) {
	var result struct{ Clients []appClient }
	if err := h.call(ctx, "client.list", nil, &result); err != nil {
		return nil, err
	}
	return result.Clients, nil
}

func sourceAppLabel(ctx context.Context, sourceID string) (string, error) {
	source, err := localHost.appClients(ctx)
	if err != nil {
		return "", err
	}
	return connectedAppLabel(source, sourceID)
}

func connectedAppLabel(source []appClient, sourceID string) (string, error) {
	var label string
	for _, c := range source {
		if c.ID == sourceID && c.Info.Kind == "app" && c.State == "connected" {
			label = c.Info.Label
			break
		}
	}
	if label == "" {
		return "", fmt.Errorf("当前 Rex 应用需要客户端标签才能匹配远端设备")
	}
	for _, c := range source {
		if c.ID != sourceID && c.Info.Kind == "app" && c.Info.Label == label && c.State != "lost" && c.State != "disconnected" {
			return "", fmt.Errorf("本机有多个同名 Rex 应用，请给当前设备使用独立标签")
		}
	}
	return label, nil
}

// Locate the current terminal and its matching app, focus the block and select
// its current window on one remote connection. Never retry a partial jump.
const focusRemoteLua = `
local sid, bid = rex.args.session, rex.args.block
local view, err = rex.call("session.view", { session_id = sid })
if not view then error(err, 0) end
local win
for _, window in ipairs(view.windows or {}) do
  for _, layer in ipairs(window.layers or {}) do
    for _, block in ipairs(layer.blocks or {}) do
      if block.block_id == bid and block.creator_name == "com.superlogical.terminal" then win = window.window_id end
    end
  end
end
if not win then return { unavailable = "pane_closed" } end
local clients, problem = rex.call("client.list", { kinds = { "app" } })
if not clients then error(problem, 0) end
local chosen, seen
for _, c in ipairs(clients.clients or {}) do
  if c.info and c.info.label == rex.args.label and c.connection_state ~= "lost" and c.connection_state ~= "disconnected" then
    if seen then return { unavailable = "ambiguous_client" } end
    seen = true
    if c.connection_state == "connected" then chosen = c.client_id end
  end
end
if not chosen then return { unavailable = "missing_client" } end
local focused, why = rex.call("session.focus_block", { session_id = sid, block_id = bid })
if not focused then error(why, 0) end
local _, select_error = rex.call("client.perform", { client_id = chosen, action = "session.select",
  args = { session_id = sid, window_id = win }, timeout_ms = 6000 })
if select_error then error(select_error, 0) end
return { window_id = win }
`

func (h Host) focusRemote(ctx context.Context, target Pane, client string) (string, error) {
	label, err := sourceAppLabel(ctx, client)
	if err != nil {
		return "", err
	}
	params, err := json.Marshal(map[string]string{"session": target.SessionID, "block": target.ID, "label": label})
	if err != nil {
		return "", err
	}
	raw, err := h.commandWithTimeout(ctx, 16*time.Second, "do", "-s", target.SessionID,
		"--fail-fast", "-e", focusRemoteLua, "--args", string(params), "--json").Output()
	if err != nil {
		return "", h.displayError(err)
	}
	var result struct {
		Window      string `json:"window_id"`
		Unavailable string
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	switch result.Unavailable {
	case "ambiguous_client":
		return "", fmt.Errorf("目标主机有多个同名 Rex 应用，请给当前设备使用独立标签")
	case "missing_client":
		return "", fmt.Errorf("当前 Rex 应用未连接目标主机")
	}
	if result.Window == "" {
		return "", errPaneClosed
	}
	return result.Window, nil
}

// Focus locates the current placement before acting. It focuses the exact
// block on its Rex server, then selects the tab on the initiating application.
func (c Context) Focus(parent context.Context, target Pane) error {
	client := os.Getenv("REX_CLIENT")
	if label := os.Getenv("REX_TD_CLIENT_LABEL"); client != "" && label != "" {
		id, err := c.boardClient(parent, client, label)
		if err != nil {
			return err
		}
		client = id
	}
	return c.FocusFrom(parent, target, client)
}

// A reconnected app has a new ID. Trust it only when it now owns this
// terminal, has the launch device's unique label, and the old ID has left.
func (c Context) boardClient(ctx context.Context, initialID, label string) (string, error) {
	raw, err := localHost.command(ctx, "block", "call", "-s", c.SessionID, "-b", c.SidebarID, "size").Output()
	if err != nil {
		return "", localHost.displayError(err)
	}
	var size struct{ Owner string }
	if err := json.Unmarshal(raw, &size); err != nil {
		return "", err
	}
	clients, err := localHost.appClients(ctx)
	if err != nil {
		return "", err
	}
	if size.Owner != "" && size.Owner != initialID {
		for _, app := range clients {
			if app.ID == initialID && app.State != "lost" && app.State != "disconnected" {
				return "", fmt.Errorf("当前看板属于另一个 Rex 客户端")
			}
		}
	}
	id := first(size.Owner, initialID)
	actualLabel, err := connectedAppLabel(clients, id)
	if err != nil {
		return "", err
	}
	if actualLabel != label {
		return "", fmt.Errorf("当前看板属于另一个 Rex 客户端")
	}
	return id, nil
}

// FocusFrom uses the Rex client that initiated a palette action as the origin.
func (c Context) FocusFrom(parent context.Context, target Pane, sourceClient string) error {
	_, err := c.focusFrom(parent, target, sourceClient)
	return err
}

// FocusLinkedPane resolves the saved host ID before jumping. Looking up status
// again here would reconnect to the same remote server before the actual jump.
func (c Context) FocusLinkedPane(parent context.Context, target Pane, client string) PaneTarget {
	result := PaneTarget{Pane: target, Unavailable: "host_unavailable"}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	hosts, err := configuredHosts(ctx)
	if err != nil {
		return result
	}
	for _, host := range hosts {
		if host.ID == target.Host.ID && host.Server != "" {
			target.Host = host
			window, err := c.focusFrom(ctx, target, client)
			if err != nil {
				result.Unavailable = "jump_failed"
				if errors.Is(err, errPaneClosed) || strings.Contains(err.Error(), "code=session_not_found") {
					result.Unavailable = "pane_closed"
				}
			} else {
				target.WindowID = window
				result = PaneTarget{Pane: target}
			}
			break
		}
	}
	return result
}

func (c Context) focusFrom(parent context.Context, target Pane, sourceClient string) (string, error) {
	if !c.Inside() || target.ID == "" || target.SessionID == "" || target.Host.ID == "" {
		return "", fmt.Errorf("缺少 Rex 窗格位置")
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	origin, err := localHost.layout(ctx, c.SessionID)
	if err != nil {
		return "", err
	}
	originWindow, owner := origin.locate(c.SidebarID)
	if originWindow == "" {
		return "", fmt.Errorf("Rex 找不到侧栏所在的窗格")
	}
	client := first(sourceClient, owner)
	if target.Host.Server != "" {
		return target.Host.focusRemote(ctx, target, client)
	}
	view := origin
	sameSession := target.SessionID == c.SessionID
	if !sameSession {
		view, err = target.Host.layout(ctx, target.SessionID)
		if err != nil {
			return "", err
		}
	}
	window, _ := view.locate(target.ID)
	if window == "" || (sameSession && target.ID == c.SidebarID) {
		return "", errPaneClosed
	}

	needsSelect := !sameSession || window != originWindow
	if needsSelect && client == "" {
		return "", fmt.Errorf("Rex 未提供当前应用的客户端标识")
	}
	if err := target.Host.call(ctx, "session.focus_block", map[string]string{
		"session_id": target.SessionID, "block_id": target.ID,
	}, nil); err != nil {
		return "", err
	}
	if !needsSelect {
		return window, nil
	}
	_, err = target.Host.command(ctx, "-C", client, "do", "session.select",
		"session_id="+target.SessionID, "window_id="+window).Output()
	if err != nil {
		return "", target.Host.displayError(err)
	}
	return window, nil
}
