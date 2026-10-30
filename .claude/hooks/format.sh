#!/bin/sh
# Formats a Go file after the agent edits it, so drift never accumulates.
set -eu
path=$(sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
case "$path" in
  *.go) command -v gofmt >/dev/null 2>&1 && gofmt -w "$path" ;;
esac
exit 0
