-- Load on a monitored Rex server. Linux uses the kernel's aggregate counters.
-- macOS uses the same installation as the task board unless explicitly overridden.
local binary = rex_td_preview_binary or (rextd_root and (rextd_root .. "/rextd"))
local language = rextd_language or os.getenv("REX_TD_LANG")
local function text(en, zh)
  return language == "zh" and zh or en
end
rex.action({
  name = "rex_td_cpu_snapshot",
  title = text("rextd CPU counters", "rextd 处理器计数"),
  run = function()
    local proc = io.open("/proc/stat", "r")
    if proc then
      local line = proc:read("*l") or ""
      proc:close()
      local ticks = {}
      for value in line:gmatch("%d+") do
        if #ticks == 8 then break end
        ticks[#ticks + 1] = tonumber(value)
      end
      if line:match("^cpu%s") and #ticks == 8 then
        return {
          Busy = ticks[1] + ticks[2] + ticks[3] + ticks[6] + ticks[7] + ticks[8],
          Idle = ticks[4] + ticks[5],
        }
      end
      error(text("Could not read aggregate /proc/stat CPU counters", "无法从 /proc/stat 读取处理器计数"), 0)
    end
    if not binary then
      error(text("Set rextd_root or rex_td_preview_binary on this host", "请为这台主机设置 rextd_root 或 rex_td_preview_binary"), 0)
    end
    local result, err = rex.exec({ binary, "--cpu-snapshot" }, { timeout = 2 })
    if not result or result.code ~= 0 then error(err or text("Could not read native CPU counters", "读取处理器计数失败"), 0) end
    local counters, decode_error = rex.json.decode(result.stdout)
    if not counters then error(decode_error, 0) end
    return counters
  end,
})
