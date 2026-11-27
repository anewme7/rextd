local terminal = "com.superlogical.terminal"
local sessions = {}
local function locations(sid)
  local attached, err = rex.call("session.attach", { session_id = sid })
  local result, layout = {}
  if attached then layout, err = rex.call("session.view", { session_id = sid }) end
  if not layout then
    result.problem = err and err:find("code=session_not_found", 1, true) and "pane_closed" or "host_unavailable"
    return result
  end
  for _, window in ipairs(layout.windows or {}) do
    for _, layer in ipairs(window.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.creator_name == terminal then
          result[block.block_id] = {
            block_id = block.block_id, session_id = sid, window_id = window.window_id,
            label = block.label, session = layout.label, window = window.label,
          }
        end
      end
    end
  end
  return result
end

local replies = {}
for i, link in ipairs(rex.args.links) do
  local sid = link.session_id
  if not sessions[sid] then sessions[sid] = locations(sid) end
  local view = sessions[sid]
  local reply = view[link.block_id]
  if reply then
    local status = rex.call(terminal .. ".program_status", {
      session_id = sid, block_id = link.block_id, args = {},
    })
    reply.records = status and status.records
  else
    reply = { block_id = link.block_id, unavailable = view.problem or "pane_closed" }
  end
  replies[i] = reply
end
return { targets = replies }
