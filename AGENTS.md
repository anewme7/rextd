# rextd project rules

## Product and integration

- Build around one workflow. Find the task, see what needs a person, jump to the right Rex pane, then return to the task and its next step.
- Prefer behavior and interfaces that Rex already provides. Check current Rex documentation and validate against a real pane before implementing or changing an integration.
- Use Rex's public actions, events and Program Status Protocol for hosts, sessions, pane state and navigation. Use official Agent APIs or local provider interfaces for session details and quota. Reuse existing adapters and supported dependencies before adding a collector, hook, runtime or service.
- Starting a task enables automatic linking for new Agent panes using the tab, project directory and in-progress task rules. Preserve existing links and offer unassigned panes for confirmation.
- Scope live subscriptions to the server or view that needs them. Coalesce updates, cancel work when its view closes and cap concurrency, retries, caches and logs. Keep task data and explicit links durable. Store only the minimum live metadata required to restore user context.
- Keep Tasks and Attention quiet. Lead with the task title and a short next step or blocker. Show host, session, pane and Agent detail as the user drills in. Use spacing and indentation for grouping. Reserve color for selection, meaningful status and user-chosen accents. Make common actions work with both keyboard and pointer input.
- Adopt a reference project's behavior when it removes work from the task flow. Document the native source of truth and measure the change with real Rex panes. Compare key response time, idle CPU, resident memory, subprocess starts, remote calls and disk growth when relevant to the change.

## Release versions

- The project owner chooses the release version. Keep the current version unless the owner explicitly approves a new version number. Bug fixes, documentation updates, builds, and merges do not authorize a version bump.
- Publishing a release or replacing an existing one requires the owner's approval. Use the approved tag for the GitHub release, packaged binaries, and local sidebar installation. Verify the public download and `rextd --version` before reporting the release as complete.
- Group small fixes for the same change into one commit before merging. Rewriting public history or replacing a published tag requires explicit approval.

## 本机开发、预览与安装

- 源码、测试、文档和构建脚本在 `~/dev/rextd/source/`。正式入口 `~/.config/rex/rextd/launch.sh` 使用 `~/dev/rextd/production/releases/current/rextd` 和独立的 `production/data/`。
- 开发预览从源码仓运行 `./scripts/preview.sh`。二进制固定为 `~/dev/rextd/work/preview/rextd`，默认数据在同级 `data/`。示例用 `--demo`，用正式任务调试时用 `--copy-prod-data`，让程序操作隔离副本。
- 预览、验收证据、临时文件和归档分别放在 `~/dev/rextd/work/{preview,evidence,temp,archive}/`。界面或终端改版交付前，必须在真实 Rex 窗格实跑核对。
- `production/releases/current` 只指向经项目所有者确认的正式版。升级前备份 `production/data/tasks.sqlite3` 并验证备份可读。SQLite 表结构须向后兼容，使上一版仍可使用现有数据。
