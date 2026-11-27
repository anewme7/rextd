local terminal = "com.superlogical.terminal"
local chinese = rextd_language == "zh" or (rextd_language == nil and os.getenv("REX_TD_LANG") == "zh")

local function text(english, translated)
  return chinese and translated or english
end

local function sidebar_in_window(ctx, view)
  for _, window in ipairs(view.windows or {}) do
    local candidates, contains_origin = {}, false
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        contains_origin = contains_origin or block.block_id == ctx.block_id
        if block.creator_name == terminal then
          candidates[#candidates + 1] = block
        end
      end
    end
    if contains_origin or (not ctx.block_id and window.window_id == view.active_window_id) then
      for _, block in ipairs(candidates) do
        -- Dedicated Home windows belong to one client. The Home action below
        -- resolves their client label and verifies the exact running binary.
        if not (block.label or ""):match("^rextd · Home/") then
          local id = block.block_id
          local process = rex.call(terminal .. ".process", { session_id = ctx.session_id, block_id = id, args = {} })
          local program = process and not process.last_exit and process.foreground
          if program and program.path then
            if program.path:match("/rextd$") then return id end
            if program.path == "/usr/bin/login" and block.label == "rextd" and program.pid then
              local child = rex.exec({ "/usr/bin/pgrep", "-P", tostring(program.pid), "-x", "rextd" }, { timeout = 2 })
              if child and child.ok then return id end
            end
          end
        end
      end
      return nil
    end
  end
end

rex.action({
  name = "rextd_show_host",
  title = text("td / Show Host 1", "td / 显示 1 号主机"),
  description = text("Show Attention for this host in your task board", "打开此客户端的看板，按主机编号筛选待处理会话"),
  category = "td",
  keywords = "rextd tasks sidebar host filter 任务 侧栏 主机 筛选",
  args = { slot = "integer?" },
  run = function(ctx, args)
    local slot = args.slot or 1
    if slot < 0 or slot > 9 or slot % 1 ~= 0 then
      error(text("Host number must be between 0 and 9.", "主机编号必须在 0 到 9 之间。"), 0)
    end
    local view, view_error = rex.call("session.view", { session_id = ctx.session_id })
    if not view then error(view_error, 0) end
    local target = sidebar_in_window(ctx, view)
    local session = ctx.session_id
    if not target then
      local board, err = rex.invoke("rextd_current_task", { host_slot = slot })
      if not board then error(err, 0) end
      return { block_id = board.block_id }
    end
    local result, write_error = rex.call(terminal .. ".write", {
      session_id = session,
      block_id = target,
      args = { data = string.char(27) .. tostring(slot) },
    })
    if not result then error(write_error, 0) end
    local focused, focus_error = rex.call("session.focus_block", { session_id = session, block_id = target })
    if not focused then error(focus_error, 0) end
    return { block_id = target }
  end,
})
