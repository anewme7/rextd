package rex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// ReadTerminalScreenLua is shared by direct and already-connected previews.
const ReadTerminalScreenLua = `
local function read_terminal_screen(session_id, block_id)
  local terminal = "com.superlogical.terminal."
  local target = { session_id = session_id, block_id = block_id }
  local limit = 65536
  target.args = { format = "text", trim = true, max_bytes = 1024 }
  local snapshot = rex.call(terminal .. "format", target)
  if not snapshot then return nil end
  local content = snapshot.content
  if snapshot.id then
    local offset = math.max(0, snapshot.total - limit)
    -- The reply limit counts JSON escapes as well as the original bytes.
    target.args = { id = snapshot.id, offset = offset, max_bytes = limit * 6 }
    local chunk = rex.call(terminal .. "blob_read", target)
    if not chunk then return nil end
    content = chunk.data
    if content and offset > 0 then
      content = content:match("^[^\n]*\n(.*)$") or ""
    end
  end
  return content or ""
end
`

// The CLI runs Lua locally, even when its Rex connection is remote. The
// request file contains only the selected pane IDs. Frames stay on the pipe.
const watchTerminalPreviewLua = `
local preview_request, preview_timer, last_screen
local function read_preview_request()
  local file = io.open(rex.args.preview_path, "rb")
  if not file then return nil end
  local request = rex.json.decode(file:read("*a"))
  file:close()
  if type(request) == "table" and request.host == rex.args.preview_host then return request end
end

local function refresh_terminal_preview(request)
  local current = read_preview_request()
  if not current or current.id ~= request.id or request ~= preview_request then return end
  local screen = read_terminal_screen(request.session, request.block)
  if screen == nil or screen ~= last_screen then
    print("rextd-preview " .. rex.json.encode({
      id = request.id, content = screen, unavailable = screen == nil,
    }))
    last_screen = screen
  end
  if screen ~= nil then
    preview_timer = rex.after(0.5, function() refresh_terminal_preview(request) end)
  end
end

rex.every(0.15, function()
  local request = read_preview_request()
  if (request and request.id) == (preview_request and preview_request.id) then return end
  if preview_timer then preview_timer:cancel() end
  preview_request, preview_timer, last_screen = request, nil, nil
  if request then refresh_terminal_preview(request) end
end)
`

type terminalPreviewRequest struct {
	ID      uint64 `json:"id"`
	Host    string `json:"host"`
	Session string `json:"session"`
	Block   string `json:"block"`
}

type TerminalPreviews struct {
	dir, path string
	mu        sync.Mutex
	next      uint64
	active    terminalPreviewRequest
	frames    chan string
}

func NewTerminalPreviews() (*TerminalPreviews, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return newTerminalPreviews(filepath.Join(cache, "rex-td", "preview"))
}

func newTerminalPreviews(cache string) (*TerminalPreviews, error) {
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		return nil, err
	}
	// The PID is published atomically with the directory. A concurrent startup
	// can never mistake an owner that is still initializing for an exited one.
	for _, entry := range entries {
		name, ok := strings.CutPrefix(entry.Name(), "pane-")
		if !ok || !entry.IsDir() {
			continue
		}
		owner, _, ok := strings.Cut(name, "-")
		pid, err := strconv.Atoi(owner)
		if ok && err == nil && pid > 0 && syscall.Kill(pid, 0) == syscall.ESRCH {
			_ = os.RemoveAll(filepath.Join(cache, entry.Name()))
		}
	}
	dir, err := os.MkdirTemp(cache, "pane-"+strconv.Itoa(os.Getpid())+"-")
	if err != nil {
		return nil, err
	}
	return &TerminalPreviews{dir: dir, path: filepath.Join(dir, "target.json")}, nil
}

func (p *TerminalPreviews) Open(pane Pane) (<-chan string, func()) {
	if p == nil {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishLocked()
	p.next++
	request := terminalPreviewRequest{p.next, pane.Host.ID, pane.SessionID, pane.ID}
	payload, _ := json.Marshal(request)
	stage := p.path + ".new"
	if err := os.WriteFile(stage, payload, 0600); err != nil {
		return nil, nil
	}
	if err := os.Rename(stage, p.path); err != nil {
		return nil, nil
	}
	p.active = request
	p.frames = make(chan string, 1)
	return p.frames, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.active.ID == request.ID {
			p.finishLocked()
		}
	}
}

func (p *TerminalPreviews) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishLocked()
	_ = os.RemoveAll(p.dir)
}

func (p *TerminalPreviews) finishLocked() {
	if p.frames != nil {
		close(p.frames)
		p.frames = nil
		p.active = terminalPreviewRequest{}
		_ = os.Remove(p.path)
	}
}

func (p *TerminalPreviews) disconnected(host string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active.Host == host {
		p.finishLocked()
	}
}

func (p *TerminalPreviews) publish(host string, raw []byte) {
	var frame struct {
		ID          uint64 `json:"id"`
		Content     string `json:"content"`
		Unavailable bool   `json:"unavailable"`
	}
	if json.Unmarshal(raw, &frame) != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.frames == nil || p.active.Host != host || p.active.ID != frame.ID {
		return
	}
	if frame.Unavailable {
		p.finishLocked()
		return
	}
	select {
	case <-p.frames:
	default:
	}
	p.frames <- frame.Content
}
