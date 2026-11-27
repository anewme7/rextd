local home = os.getenv("HOME")
local runtime = rextd_root or (home .. "/.local/opt/rextd")
local binary = runtime .. "/rextd"
local language = rextd_language or os.getenv("REX_TD_LANG")
local terminal = "com.superlogical.terminal"
local data = rextd_data_dir or os.getenv("REX_TD_DATA_DIR")
local generic_dirs = rextd_generic_dirs or os.getenv("REX_TD_GENERIC_DIRS")
local environment = { "/usr/bin/env", "REX_TD_LANG=" .. (language == "zh" and "zh" or "en") }
if data then environment[#environment + 1] = "REX_TD_DATA_DIR=" .. data end
if generic_dirs then environment[#environment + 1] = "REX_TD_GENERIC_DIRS=" .. generic_dirs end

local function text(en, zh)
  return language == "zh" and zh or en
end

local function command(...)
  local argv = {}
  for _, value in ipairs(environment) do argv[#argv + 1] = value end
  argv[#argv + 1] = binary
  for _, value in ipairs({ ... }) do argv[#argv + 1] = value end
  return argv
end

local states = {
  ["To do"] = "待做", ["In progress"] = "进行中", Paused = "暂缓", Done = "已完成", Cancelled = "已取消",
}

local function must(value, problem)
  if value == nil then error(problem or text("Rex returned no result", "Rex 未返回结果"), 0) end
  return value
end

local function store(operation, request, timeout)
  local argv = command("palette", operation)
  if request then argv[#argv + 1] = rex.json.encode(request) end
  local result = must(rex.exec(argv, { timeout = timeout or 5 }))
  if not result.ok then
    error((result.stderr or ""):gsub("%s+$", ""), 0)
  end
  return must(rex.json.decode(result.stdout))
end

local function by_block(view, id)
  if not view or not id then return nil end
  for _, window in ipairs(view.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.block_id == id then return window, block end
      end
    end
  end
end

local function view_of(session_id)
  if not session_id then return nil end
  return rex.call("session.view", { session_id = session_id })
end

local function focus(ctx, target, select_window)
  local origin = by_block(view_of(ctx.session_id), ctx.block_id)
  must(rex.call("session.focus_block", { session_id = target.session_id, block_id = target.block_id }))
  if ctx.client_id and (select_window or ctx.session_id ~= target.session_id or not origin or origin.window_id ~= target.window_id) then
    rex.client.queue("session.select", { session_id = target.session_id, window_id = target.window_id })
  end
  return target
end

local function open_board(client)
  return must(rex.invoke("rextd_current_task", { restore_location = true, attention_client = client }))
end

local function client_scope(ctx)
  if not ctx.client_id then return "" end
  local connected = must(rex.call("client.list", { kinds = { "app" } }))
  for _, app in ipairs(connected.clients or {}) do
    if app.client_id == ctx.client_id then return ctx.client_id end
  end
  return ""
end

local function task_items(tasks)
  local items, indexed = {}, {}
  for _, task in ipairs(tasks) do
    local id = tostring(task.id)
    local state = task.state == "Blocked" and "Paused" or task.state
    items[#items + 1] = { id = id, title = task.title, subtitle = task.archived and text("Archived", "已归档") or text(state, states[state] or state),
      detail = text(tostring(#task.panes) .. (#task.panes == 1 and " pane" or " panes"), #task.panes .. " 个窗格"), icon = "document" }
    indexed[id] = task
  end
  return items, indexed
end

local reasons = {
  host_unavailable = text("Host unreachable", "主机连不上"),
  pane_closed = text("Pane closed", "窗格已关闭"),
}

local function remember(scope, task, target, reason)
  store("resume", { client = scope, task_id = task.id, pane = target, reason = reason })
end

local function show_task(scope, task, target, reason)
  remember(scope, task, target, reason)
  local opened = open_board(scope ~= "" and scope or nil)
  opened.task_id = task.id
  opened.unavailable = reason
  return opened
end

local function find_task(ctx, args)
  local items, indexed = task_items(store("find-list"))
  if args.inspect and not args.task_id then return { items = items } end
  if #items == 0 then return open_board() end
  local id = args.task_id and tostring(args.task_id) or rex.ui.pick{
    title = text("Find Task", "查找任务"), placeholder = text("Task name", "任务名称"), items = items,
  }
  if not id then return end
  local task = indexed[id]
  if not task then error(text("The task is no longer available", "任务已不存在，请重新查找"), 0) end
  local scope = client_scope(ctx)
  if task.archived and not args.inspect then return show_task(scope, task) end
  local targets = store("targets", { client = scope, task_id = task.id }, 21)
  for _, target in ipairs(targets) do
    local status = reasons[target.unavailable] or target.status
    if status and status ~= "" then target.location = target.location .. " · " .. status end
  end
  if args.inspect then return { title = task.title, targets = targets } end
  if #targets == 0 then return show_task(scope, task, nil, args.block_id and "link_missing" or nil) end

  local chosen
  if args.block_id then
    for _, target in ipairs(targets) do
      if target.block_id == args.block_id and (not args.host_id or target.host_id == args.host_id) then
        if chosen then return show_task(scope, task, nil, "link_missing") end
        chosen = target
      end
    end
    if not chosen then return show_task(scope, task, nil, "link_missing") end
  elseif #targets == 1 then
    chosen = targets[1]
  else
    local options = {}
    for index, target in ipairs(targets) do
      options[#options + 1] = { id = tostring(index), title = target.title, subtitle = target.location, icon = "terminal" }
    end
    local selection = rex.ui.pick{ title = task.title, placeholder = text("Choose a pane", "选择一个窗格"), items = options }
    if not selection then return end
    chosen = targets[tonumber(selection)]
  end
  if not chosen then return end
  if chosen.unavailable then return show_task(scope, task, chosen, chosen.unavailable) end

  remember(scope, task, chosen)
  local result
  if chosen.host_id == "local" then
    local ok, focused = pcall(focus, ctx, chosen, args.select_window)
    if not ok then return show_task(scope, task, chosen, "jump_failed") end
    result = focused
  else
    local ok, jumped = pcall(store, "jump", {
      task_id = task.id, pane = chosen,
      origin_session = ctx.session_id, origin_block = ctx.block_id, client = ctx.client_id,
    }, 24)
    if not ok then return show_task(scope, task, chosen, "jump_failed") end
    if jumped.unavailable then return show_task(scope, task, chosen, jumped.unavailable) end
    result = jumped
  end
  result.task_id = task.id
  return result
end

local function current_target(ctx)
  local view = view_of(ctx.session_id)
  local window, block = by_block(view, ctx.block_id)
  if not window or block.creator_name ~= terminal or (block.label == "rextd" or (block.label or ""):match("^rextd · Home/")) then
    error(text("Run this action from the pane you want to link", "请在要收录的工作窗格里运行"), 0)
  end
  local report = rex.call(terminal .. ".program_status", { session_id = ctx.session_id, block_id = ctx.block_id, args = {} })
  local latest
  for _, row in ipairs((report and report.records) or {}) do
    if (row.effective_app or row.app) and (not latest or row.updated_at > latest.updated_at) then latest = row end
  end
  local agent = latest and (latest.effective_app or latest.app) or ""
  local title = block.label
  if not title or title == "" then title = latest and latest.title or "" end
  if title == "" then title = window.label or "" end
  return {
    host_id = "local", host = text("Local", "本机"),
    session_id = ctx.session_id, session = view.label or "", window_id = window.window_id,
    block_id = ctx.block_id, title = title, agent = agent,
  }
end

local function capture(ctx, args)
  local pane = current_target(ctx)
  if args.inspect then return { title = pane.title } end
  local id, title = args.task_id, args.title
  if not id and not title then
    local entries = task_items(store("list"))
    table.insert(entries, 1, { id = "new", title = text("New Task…", "新建任务…"), icon = "plus" })
    local choice = rex.ui.pick{ title = text("Capture Current Pane", "收录当前窗格"), placeholder = text("Choose a task or create one", "选择现有任务或新建"), items = entries }
    if not choice then return end
    if choice == "new" then
      title = rex.ui.input{ title = text("Task title", "任务标题"), value = pane.title, confirm = text("Create and link", "新建并关联") }
      if not title then return end
    else
      id = tonumber(choice)
    end
  end
  pane = current_target(ctx)
  return store("capture", { task_id = id, title = title, pane = pane })
end

rex.action{
  name = "rextd_preview_open",
  title = text("td / Open Task Board", "td / 打开任务看板"),
  category = "td",
  keywords = "rextd tasks board focus 任务 看板 回到侧栏",
  description = text("Return to the task board for this client.", "打开此客户端的任务看板。"),
  args = { select_window = "boolean?" },
  run = function() return open_board() end,
}

rex.action{
  name = "rextd_preview_find",
  title = text("td / Find Task…", "td / 查找任务…"),
  category = "td",
  keywords = "rextd task find search jump 找任务 跳转",
  description = text("Find a task and jump to a linked Rex pane.", "按任务名称查找并跳转到关联窗格。"),
  args = { task_id = "integer?", block_id = "string?", host_id = "string?", inspect = "boolean?", select_window = "boolean?" },
  run = find_task,
}

rex.action{
  name = "rextd_preview_capture",
  title = text("td / Capture Current Pane…", "td / 收录当前窗格…"),
  category = "td",
  keywords = "rextd task capture link 新建 任务 关联 收录",
  description = text("Link the current pane to a task.", "把当前窗格添到任务库。"),
  args = { task_id = "integer?", title = "string?", inspect = "boolean?" },
  run = capture,
}
