# rextd user guide

rextd adds a task list and an Alerts view to Rex. Use it to keep track of work, see which agents need input, and jump back to a pane.

## Install and open the sidebar

1. Install Rex and make its `rex` command available on the machine running your terminal. Download the Apple Silicon or Linux x86-64 package and matching checksum from [GitHub Releases](https://github.com/anewme7/rextd/releases). The archive contains the `rextd` executable.
2. Follow the [checksum and extraction steps](../README.md#install-and-open-the-sidebar) for your platform. The package bundles SQLite.
3. In a Rex terminal, change to the extracted directory and run:

   ```sh
   rex run --split=right --ratio 35 --shell=none --label rextd -- "$PWD/rextd"
   ```

The standalone sidebar defaults to English. The managed Mac launcher and development preview default to Chinese. Press **Shift+L** to switch between English and Simplified Chinese. To start in Chinese, append `--lang zh` after the executable in the launch command.

To try sample tasks and alerts first, run `./rextd --demo`. The demo uses fictional data in memory. `--demo` shows `演示` or **Demo**.

For a board on one machine and agents on another, follow the [Mac and Linux walkthrough](../README.md#use-one-board-across-hosts). It covers adding a host, checking its connection, and jumping from the board to a remote pane.

## Start a task and jump to its agent sessions

1. Open **Now** and press `+`. Type a title such as “Review the release notes” and press Enter to save it.
2. Select the task and press `s` to set it to **In progress**. Press `N` to open a Rex tab for the task, then start your agent in that tab. Press `e` on the task to write its next step.
3. Select an agent row and press Enter to jump to it. Press `m` on the row to move the pane to another task or detach it.

Use **1 Now**, **2 To Do**, **3 Done**, or **4 Archive**. Tab and Shift+Tab cycle forward and back. In **Tabs** mode, open Paused under To Do or Cancelled under Done. Newly completed tasks appear first. Press `v` to switch to **All**. The same four lists appear as sections, and the number keys jump to them. Done and Archive start collapsed. Select a section heading and press Enter to fold or open it.

Select a task to see its sessions in the panel below. Press Right to select a session, Space to preview, Enter to jump, or Left to return to the task. The single **Unassigned** row uses the same panel. Select it and press Right or Enter, choose a session, then press Enter to pick and confirm a task. Panes needing attention bring that row to the top of Now.

For a new agent pane, rextd looks for a task in the same tab, then a single in-progress task with an agent in the same project directory on the same host, then a single in-progress task overall.

The home directory and filesystem root are skipped during directory matching. To skip more directories for matching, set `REX_TD_GENERIC_DIRS=/work:/srv/shared` before launching rextd. Use colon-separated absolute paths. Each path excludes that directory. Projects in its subdirectories can still match. For Rex actions, set `rextd_generic_dirs = "/work:/srv/shared"` in Rex `init.lua`.

To link a pane manually, press `o` on a task to open details, then press lowercase `l` and pick the pane.

When a linked session is unavailable, select it in task details and choose **Unlink** (`u`). This removes its link from the task. Choose **Relink** (`r`) to replace it with a different pane.

## Finish and organize tasks

Click the circle in task details or press `x` to mark a task done or reopen it. Open the task menu with `m` to cancel, archive, reorder, or split work into subtasks. Press `4` to see the archive and `/` to search.

To permanently delete an active or archived task, open its menu with `m`, choose **Delete task**, then press `y` or click **Confirm delete**. Any linked sessions are detached. Subtasks become standalone tasks.

Load the [bundled Rex actions](../README.md#add-rextd-to-the-rex-command-palette). On macOS, press Shift+Command+P and search for **td** to open the board, find a task, or capture the current pane. Find Task includes archived tasks and opens their details. Capture Current Pane offers active tasks. After jumping, use **Open Task Board** to return to your client’s board. **Tasks** and the Raycast `td` shortcut return to Now.

## Review suggested tasks

Open a pane's details from **Alerts**, then press `d` to discover a candidate or `w` to watch the pane for suggestions. The sidebar uses Rex status updates from the Codex and Claude Code panes you choose.

On **Now**, expand **Candidates** and select a suggestion. Press Enter to review it and edit the title. Press Enter again to create a task with a link to the original pane. Press `i` to ignore a candidate's topic.

## Review agent activity

Select a numbered host with Alt+1 through Alt+9, or press Alt+0 to show all hosts. See the [Rex command palette setup](../README.md#add-rextd-to-the-rex-command-palette) for the optional action.

Press `n` for **Alerts**, or click its reminder count. With no reminders, click **n Alerts**. It opens with panes that need a reply or have a result ready, grouped by status. Press `a` for all panes, including working, idle and unavailable panes. Select a pane to read the latest message beneath the list. Press Esc to return to the task board. Open [Agent session details](#agent-session-details) for the latest activity and available session metadata.

| To do this | Press |
| --- | --- |
| Cycle through Status, Session, Agent and Host | `v` |
| Fold or expand the current group | `c` |
| Fold or expand a whole host in Host view | Shift+C |
| Inspect a pane's key events | Right |
| Mark a completed result as seen or undo the mark | `y` in pane details |
| Jump to the selected pane | Enter |
| Preview the selected pane or return from its preview | Space |
| Pin a task or pane | `p` |
| See the shortcut list | `?` |

To undo a seen mark, show all panes and reopen its details. Approvals stay in Alerts until Rex reports a change.

Select a linked pane in Tasks or a pane in Alerts and click **Preview** to read its live terminal output. Scroll up to read earlier output and press End to resume following it. Press Space, Esc, or **Back** to return to the selected row. Press Enter or click **Enter pane** when you want to type in the terminal.

In **Unassigned**, select a pane and press Enter to choose a task. Review the highlighted task and press Enter again to link it. In task details, the **Idle and unlinked** section lists panes you can link directly. Select one and press Enter. Use `l` for the full pane picker.

Task state and pane activity are independent. A task can be **To do**, **In progress**, **Paused**, **Done**, or **Cancelled**. Use the task menu to pause or resume it. Archiving keeps the task's state and links. An agent finishing a turn or waiting for an answer updates its pane activity.

Saved task pins persist. Alerts pins last until you close the sidebar. For cross-host jumps, connect the Rex app to both servers, give the device a unique Rex client label, and enable Rex Remote Control for tab switches.

## Read machine and usage data

- **Hosts · CPU** shows the number of sessions and windows on each connected host, with CPU usage when available. Local CPU appears automatically on supported platforms. Follow [remote CPU setup](#enable-remote-cpu) to enable a remote host.
- **Usage** shows each provider's reported quota windows. When the heading is **5h/wk**, read the left value as five-hour usage and the right value as weekly usage. A dash means a reading is unavailable.
- For **Codex**, put `codex` on the sidebar machine's PATH and sign in. For **Claude Code**, follow the [status-line setup](#enable-claude-code-usage).

From the board or Alerts, press `f` for Hosts at the bottom of the current page. Press `f` again for Usage, and once more to close the drawer. Esc also closes it. In Hosts, use Up/Down to select a machine and Enter to filter Alerts to it. You can also tap a machine to select it and tap again to apply the filter.

Swipe up on the board, Alerts or task details to read further down. Swipe down to go back. Scrolling keeps the current selection. Tap a visible row to select it. Open details to read a long task title in full. On a 28–55 column pane, tab numbers may disappear and the footer shows fewer shortcuts. Use `1` through `4` at any width. The 56–103 column layout adds status labels, and full-width panes (104 columns and up) have more room for session context. Click or tap a visible tab, section or action in Rex clients that forward terminal mouse events.

## Optional integrations

### Agent session details

Press Right on an Alerts row, then `m` to expand the Agent metadata. Claude Code matches a local session to the pane's foreground process with `claude agents`. For Codex, press `i` in pane details and enter the agent's session ID to verify and link it using the local Codex App Server.

To make the same lookup available for a remote pane, uncomment the Agent details line in the Rex example config on that pane's host. Agent tools must be installed on the host with the pane.

Claude Code's latest reply can be read with the optional bundled reader. Install Node.js, copy `rex/agent-details/read.mjs` to `~/.config/rex/rextd/agent-details/claude-reader/` on the pane's host, then run `npm install --save-exact @anthropic-ai/claude-agent-sdk@0.3.329` from that directory. Use the matching path under `$XDG_CONFIG_HOME` if you set it.

### Enable remote CPU

1. Copy [rex/metrics.lua](../rex/metrics.lua) to the target Rex server.
2. Add the following line to that server's Rex `init.lua`. Use the absolute path to the copied file. Rex normally reads `~/.config/rex/init.lua`, or `$XDG_CONFIG_HOME/rex/init.lua` when set. For a shared server, use the [service account's configuration](https://www.superlogical.com/rex/docs/linux/shared-server).

   ```lua
   dofile("/absolute/path/to/metrics.lua")
   ```

3. For a macOS server, copy the matching `rextd` binary to the server. The action uses `rextd_root .. "/rextd"`. To use a separate binary, set `rex_td_preview_binary = "/absolute/path/to/rextd"` above the `dofile` line. On Linux, the Lua action reads `/proc/stat` directly.
4. On the target server, run `rex config check` and `rex config reload`. Reopen the sidebar to discover the action. Reachability and pane counts already work through Rex. [Rex server configuration](https://www.superlogical.com/rex/docs/customize/config)

To remove remote CPU collection, remove the lines you added to the server's `init.lua` and reload Rex on that server.

### Enable Claude Code usage

Merge [rex/claude-statusline.example.json](../rex/claude-statusline.example.json) into the settings for the Claude Code profile you want to display. Set its command to your executable's absolute path followed by `--claude-statusline`. If you already have a status-line script, forward a copy of its JSON input to that command and keep your script in place. Claude Code sends supported five-hour and weekly windows after the first response in a session. The sidebar replaces one local cache with the latest reading. [Claude Code status line](https://code.claude.com/docs/en/statusline)

If a local Claude Code pane reports that its weekly quota is exhausted, rextd uses that status to show 100% weekly usage until the reported reset. This works before Claude Code sends its first status-line usage reading.

### Raycast on a managed Mac

For the managed macOS installation under `~/dev/rextd/production/`, install the archive’s `rex/launch.sh` at `~/.config/rex/rextd/launch.sh` when upgrading. Raycast’s `td` script calls that launcher through `~/.local/bin/rextd`. It selects the local Mac app or the explicitly requested `REX_CLIENT`. A new window uses the launcher’s `--lang en|zh` or `REX_TD_LANG` (default `zh`). A reused window keeps its current language. Load the bundled actions once and remove any older local action that redefines `rextd_preview_open`, so returning to the board preserves its location.

## Back up, update, or remove

For the managed installation, the launcher points to `~/dev/rextd/production/data/tasks.sqlite3`. A development preview uses its own copy. For other installations, tasks and pane links are saved to `~/.local/share/rex-td/tasks.sqlite3` by default, on the machine running the sidebar. If you set `REX_TD_DATA_DIR` or `XDG_DATA_HOME`, use the [data location rules](../README.md#data-updates-and-removal) to find it.

The managed installation can take a consistent backup while the board is open. Run these commands before switching releases. The final check should print `ok`.

```sh
umask 077
archive="$HOME/dev/rextd/work/archive"
mkdir -p "$archive"
backup="$archive/tasks-$(date +%Y%m%d-%H%M%S).sqlite3"
sqlite3 -readonly "$HOME/dev/rextd/production/data/tasks.sqlite3" ".backup '$backup'"
sqlite3 -readonly "$backup" 'PRAGMA quick_check;'
```

For other installations, use the same backup command with the location of your database.
- **Update** by quitting with `q` and launching the binary from the newer archive. If you set up Rex actions, point `rextd_root` to the new directory and reload Rex.
- **Remove** by quitting with `q`, deleting the extracted program, and removing any optional Rex and Claude settings you added. Keep the database if you plan to use your tasks again.
