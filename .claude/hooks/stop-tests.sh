#!/usr/bin/env bash
# Stop: run unit tests once Go code exists. Exit 2 sends failures back to Claude.
active=$(jq -r '.stop_hook_active // false')
[[ "$active" == "true" ]] && exit 0   # avoid infinite loops
cd "$CLAUDE_PROJECT_DIR" || exit 0
[[ -f go.mod ]] && command -v go >/dev/null || exit 0
out=$(go test ./... 2>&1) || { echo "Unit tests failing:"$'\n'"$out" >&2; exit 2; }
exit 0
