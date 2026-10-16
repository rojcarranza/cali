#!/usr/bin/env bash
# PostToolUse (Edit|Write): format and vet edited Go files.
f=$(jq -r '.tool_input.file_path // empty')
[[ "$f" == *.go && -f "$f" ]] || exit 0
command -v gofmt >/dev/null && gofmt -w "$f"
if command -v go >/dev/null && [[ -f "$CLAUDE_PROJECT_DIR/go.mod" ]]; then
  cd "$CLAUDE_PROJECT_DIR" || exit 0
  out=$(go vet "./$(dirname "${f#$CLAUDE_PROJECT_DIR/}")/..." 2>&1) || { echo "$out" >&2; exit 2; }
fi
exit 0
