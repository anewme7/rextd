-- Copy this block into Rex init.lua and set the absolute installation directory.
rextd_root = "/absolute/path/to/rextd"
-- rextd_language = "zh"
-- rextd_data_dir = "/absolute/path/to/task-data"
-- rextd_generic_dirs = "/work:/srv/shared"

dofile(rextd_root .. "/rex/action.lua")
dofile(rextd_root .. "/rex/palette.lua")
dofile(rextd_root .. "/rex/current-task.lua")

-- Optional shortcut for host 1.
-- rex.bind("alt+shift+[Digit1]", "rextd_show_host", { slot = 1 })

-- Optional read-only Agent details on this Rex host.
-- dofile(rextd_root .. "/rex/agent-details/action.lua")

-- Optional host CPU readings. On macOS this uses rextd_root .. "/rextd".
-- dofile(rextd_root .. "/rex/metrics.lua")
