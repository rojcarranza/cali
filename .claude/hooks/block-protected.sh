#!/usr/bin/env bash
# PreToolUse (Edit|Write|MultiEdit): block edits to protected files.
f=$(jq -r '.tool_input.file_path // empty')
case "$(basename "$f")" in
  .env|.env.*) echo "Blocked: $f is protected (.env files). Ask the user." >&2; exit 2 ;;
esac
exit 0
