-- The Raycast td command opens a task-board window for its Rex client.
local home = os.getenv("HOME")
local root = rextd_quick_task_root or rextd_root or (home .. "/.local/opt/rextd")
-- Keep a running board tied to its installation if the current symlink changes.
local physical = rex.exec({ "/bin/pwd", "-P" }, { cwd = root, timeout = 2 })
local binary_root = physical and physical.ok and physical.stdout:match("^(.-)\n$") or root
local binary = binary_root .. "/rextd"
local data = rextd_data_dir or os.getenv("REX_TD_DATA_DIR")
local generic_dirs = rextd_generic_dirs or os.getenv("REX_TD_GENERIC_DIRS")
local language = rextd_language or os.getenv("REX_TD_LANG")
local board_session = rextd_home_session or rextd_attention_session
local terminal = "com.superlogical.terminal"
local board_label = "rextd · Home"

local function text(en, zh)
  return language == "zh" and zh or en
end

local function must(value, problem)
  if value == nil then error(problem or text("Rex returned no result", "Rex 未返回结果"), 0) end
  return value
end

local function app_identity(app, required)
  if not app then return end
  local clients = must(rex.call("client.list", {}))
  local id, label
  for _, client in ipairs(clients.clients or {}) do
    if not required and client.client_id == app and client.connection_state == "connected" and
        client.info and client.info.kind == "cli" then return end
    if client.info and client.info.kind == "app" and client.connection_state ~= "lost" and client.connection_state ~= "disconnected" and
        (client.client_id == app or client.info.label == app) then
      if id then error(text("Rex app label is ambiguous", "Rex 客户端名称重复"), 0) end
      id, label = client.client_id, client.info.label
    end
  end
  if not id then error(text("Rex app is unavailable", "Rex 客户端连不上"), 0) end
  return id, label
end

local function board_pid(program)
  if not program or not program.pid then return end
  if program.path == binary or program.invoked_path == binary then return program.pid end
  if program.path ~= "/usr/bin/login" then return end
  -- On macOS Rex can report login as the terminal child, even for shell=none.
  local children = rex.exec({ "/usr/bin/pgrep", "-P", tostring(program.pid) }, { timeout = 2 })
  if not children or not children.ok then return end
  for pid in children.stdout:gmatch("%d+") do
    local command = rex.exec({ "/bin/ps", "-ww", "-p", pid, "-o", "comm=" }, { timeout = 2 })
    if command and command.ok and command.stdout:match("^%s*(.-)%s*$") == binary then
      return tonumber(pid)
    end
  end
end

local function find_board(session, label)
  local view = must(rex.call("session.view", { session_id = session }))
  for _, window in ipairs(view.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.label == label and block.creator_name == terminal then
          local process = rex.call(terminal .. ".process", { session_id = session, block_id = block.block_id, args = {} })
          if process and not process.last_exit then
            local pid = board_pid(process.foreground) or board_pid(process.child)
            if pid then
              return { session_id = session, window_id = window.window_id, block_id = block.block_id }, pid
            end
          end
        end
      end
    end
  end
end

local function open_home(ctx, requested_app, restore, host_slot, launch_language)
  local session = board_session or ctx.session_id
  if not session then error(text("Open from a Rex workspace", "请在 Rex 工作区内打开"), 0) end
  local app = requested_app or ctx.client_id
  if ctx.server and app then
    local bin_dir = os.getenv("REX_BIN_DIR")
    local rex_cli = bin_dir and (bin_dir .. "/rex") or "rex"
    local argv = {
      rex_cli, "--autostart=false", "--timeout", "6s", "-C", app,
      "do", "-s", session, "rextd_current_task", "attention_client=" .. app,
      "restore_location=" .. tostring(restore == true), "--json",
    }
    if host_slot then argv[#argv+1] = "host_slot=" .. host_slot end
    if launch_language then argv[#argv+1] = "language=" .. launch_language end
    local result = must(rex.exec(argv, { timeout = 8 }))
    if not result.ok then error(result.stderr, 0) end
    return must(rex.json.decode(result.stdout))
  end

  if ctx.session_id ~= session then
    must(rex.call("session.attach", { session_id = session }))
  end
  local client_id, client_label = app_identity(app, requested_app ~= nil)
  local label = board_label .. "/" .. (client_id or "cli")
  local name = text("rextd · Tasks", "rextd · 任务")
  if client_label and client_label ~= "" then name = name .. " · " .. client_label end
  local target, pid = find_board(session, label)
  if target and host_slot then
    must(rex.call(terminal .. ".write", {
      session_id = session, block_id = target.block_id,
      args = { data = string.char(27) .. tostring(host_slot) },
    }))
  elseif target and not restore then
    local signaled = rex.exec({ "/bin/kill", "-USR1", tostring(pid) }, { timeout = 2 })
    if not signaled or not signaled.ok then
      error(text("Could not reopen Tasks", "无法打开已存在的任务页"), 0)
    end
  elseif not target then
    local argv = { "/usr/bin/env", "REX_TD_LANG=" .. (launch_language or text("en", "zh")) }
    if client_id then argv[#argv + 1] = "REX_CLIENT=" .. client_id end
    if client_label and client_label ~= "" then argv[#argv + 1] = "REX_TD_CLIENT_LABEL=" .. client_label end
    if data then argv[#argv + 1] = "REX_TD_DATA_DIR=" .. data end
    if generic_dirs then argv[#argv + 1] = "REX_TD_GENERIC_DIRS=" .. generic_dirs end
    argv[#argv + 1] = binary
    if host_slot then
      argv[#argv + 1] = "--host=" .. host_slot
    elseif not restore then
      argv[#argv + 1] = "--home"
    end
    local window = must(rex.call("session.new_window", {
      session_id = session, window_label = name,
      layout = { block = { flavor = terminal .. ".shell", label = label,
        options = { shell = "none", command = argv },
      } },
    }))
    target = { session_id = session, window_id = window.window_id, block_id = window.block_ids[1] }
  end
  must(rex.call("session.focus_block", { session_id = session, block_id = target.block_id }))
  if client_id and ctx.client_id then
    rex.client.queue("session.select", { session_id = session, window_id = target.window_id })
  end
  return target
end

rex.action{
  name = "rextd_current_task",
  title = text("td / Tasks", "td / 任务首页"),
  description = text("Open the rextd task board.", "打开 rextd 任务首页。"),
  category = "td",
  keywords = "task rextd home 任务 首页",
  -- Raycast already passes open_attention and may pass attention_client.
  args = { open_attention = "boolean?", attention_client = "string?", restore_location = "boolean?", host_slot = "integer?", language = "string?" },
  run = function(ctx, args)
    if args.language and args.language ~= "en" and args.language ~= "zh" then
      error(text("Use en or zh for the board language.", "看板语言请用 en 或 zh。"), 0)
    end
    if args.host_slot and (args.host_slot < 0 or args.host_slot > 9) then
      error(text("Host number must be between 0 and 9.", "主机编号必须在 0 到 9 之间。"), 0)
    end
    return open_home(ctx, args.attention_client, args.restore_location, args.host_slot, args.language)
  end,
}

if rextd_current_task_key then
  rex.bind(rextd_current_task_key, "rextd_current_task")
end
