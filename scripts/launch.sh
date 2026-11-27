#!/bin/bash
# Installed as ~/.config/rex/rextd/launch.sh for the managed macOS layout.
set -euo pipefail
runtime="$HOME/dev/rextd/production/releases/current/rextd"
export REX_TD_DATA_DIR="$HOME/dev/rextd/production/data"
export REX_TD_LANG="${REX_TD_LANG:-zh}"
export REX_TD_GENERIC_DIRS="${REX_TD_GENERIC_DIRS:-$HOME/dev/AgentWork}"

for arg in "$@"; do
  case "$arg" in
    --home|-home|--lang|-lang|--lang=*|-lang=*|en|zh) ;;
    *) exec "$runtime" "$@" ;;
  esac
done
if [[ -n ${REX_BLOCK:-} ]]; then
  exec "$runtime" --home "$@"
fi

# A new window starts inside the Rex server, outside this shell's environment.
language=$REX_TD_LANG
while (( $# )); do
  case "$1" in
    --lang|-lang)
      if (( $# < 2 )); then echo '--lang requires en or zh' >&2; exit 2; fi
      language=$2
      shift ;;
    --lang=*|-lang=*) language=${1#*=} ;;
    --home|-home) ;;
    *) echo 'Usage: rextd [--home] [--lang en|zh]' >&2; exit 2 ;;
  esac
  shift
done
case "$language" in en|zh) ;; *) echo '--lang requires en or zh' >&2; exit 2 ;; esac

rex_cli="$HOME/Library/Application Support/com.superlogical.rex/RexServer/Versions/Current/Rex Server.app/Contents/MacOS/rex"
requested=${REX_CLIENT:-}
unset REX_SERVER REX_HOME REX_SESSION REX_WINDOW REX_BLOCK REX_CLIENT
# Raycast runs outside a pane. Choose the app connected locally to this Mac.
local_app='
local clients, err = rex.call("client.list", { kinds = { "app" } })
if not clients then error(err, 0) end
local id
for _, client in ipairs(clients.clients or {}) do
  if client.connection_state == "connected" and client.principal and client.principal.transport == "unix" then
    if id then error("Multiple local Rex apps. Set REX_CLIENT.", 0) end
    id = client.client_id
  end
end
if id then print(id) end'
for attempt in 1 2 3 4 5; do
  app="$requested"
  if [[ -z $app ]]; then
    app=$("$rex_cli" --autostart=false --timeout=3s do -e "$local_app") || app=''
  fi
  if [[ -n $app ]] && "$rex_cli" --autostart=false --timeout=6s -C "$app" actions >/dev/null 2>&1; then
    target=$("$rex_cli" --autostart=false --timeout=18s -C "$app" do rextd_current_task open_attention=true "attention_client=$app" "language=$language" --json)
    session=$(printf '%s' "$target" | /usr/bin/plutil -extract session_id raw -o - -- -)
    window=$(printf '%s' "$target" | /usr/bin/plutil -extract window_id raw -o - -- -)
    exec "$rex_cli" --autostart=false --timeout=12s -C "$app" do session.select "session_id=$session" "window_id=$window" >/dev/null
  fi
  if [[ $attempt == 1 && -z $requested ]]; then /usr/bin/open -b com.superlogical.rex; fi
  /bin/sleep .3
done
if [[ $REX_TD_LANG == zh ]]; then
  echo '未连上指定的 Rex 客户端，请在该设备上打开 Rex。' >&2
else
  echo 'Could not reach the selected Rex client. Open Rex on that device.' >&2
fi
exit 1
