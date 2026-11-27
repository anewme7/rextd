-- This read-only action is optional on a Rex host that provides agent detail.
local home = os.getenv("HOME")
local binary = (rextd_root or (home .. "/.local/opt/rextd")) .. "/rextd"
local language = rextd_language or os.getenv("REX_TD_LANG")
local function text(en, zh)
  return language == "zh" and zh or en
end
local terminal = "com.superlogical.terminal"

rex.action{
  name = "rextd_agent_detail",
  title = text("Agent session details", "助手会话详情"),
  description = text("Read a matching agent session in the current Rex pane", "读取当前 Rex 窗格的助手会话详情"),
  args = { provider = "string", thread_id = "string?", pid = "integer" },
  run = function(ctx, args)
    if not ctx.session_id or not ctx.block_id or (args.provider ~= "codex" and args.provider ~= "claude-code") then
      error(text("No supported Rex pane", "当前窗格不支持会话详情"), 0)
    end
    local ref = { session_id = ctx.session_id, block_id = ctx.block_id, args = {} }
    local status = rex.call(terminal .. ".program_status", ref)
    local matched = false
    for _, record in ipairs(status and status.records or {}) do
      if (record.effective_app or record.app) == args.provider then matched = true end
    end
    local process = rex.call(terminal .. ".process", ref)
    if not matched or not process or not process.foreground or process.foreground.pid ~= args.pid then
      error(text("Rex pane changed before the lookup", "查询前窗格中的进程已改变"), 0)
    end
    local environment = table.concat({ home .. "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", os.getenv("PATH") or "/usr/bin:/bin" }, ":")
    local result, err = rex.exec({
      "/usr/bin/env", "PATH=" .. environment,
      binary, "agent-details", args.provider, args.thread_id or "", tostring(args.pid)
    }, { timeout = 4 })
    if not result or not result.ok then
      error(err or text("Agent session details unavailable", "暂时无法读取助手会话详情"), 0)
    end
    return rex.json.decode(result.stdout)
  end,
}
