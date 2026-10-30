#!/bin/sh
# Approval gate for shell commands (docs/sdlc.md, Deploy stage). Exit 2 blocks the action and
# sends the message to the agent. Input: the hook JSON on stdin.
set -eu
cmd=$(sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p' | head -1)
[ -n "$cmd" ] || exit 0
block() { echo "$1" >&2; exit 2; }
case "$cmd" in
  *"git push"*" main"*|*"git push"*":main"*|*"git push"*"HEAD:main"*)
    block "Pushing to main is blocked: open a PR from a branch; the owner merges." ;;
  *"git push"*"--force"*|*"git push"*" -f "*|*"git push"*"--force-with-lease"*)
    echo "$cmd" | grep -q " main" && block "Force-pushing main is blocked." ;;
esac
case "$cmd" in
  *"git filter-repo"*|*"git filter-branch"*)
    block "History rewriting is blocked: it needs the owner's explicit go-ahead and a disabled ruleset." ;;
  *"gh pr merge"*|*"gh api"*"/merge"*)
    block "Merging is the owner's gate; give them the PR link instead." ;;
esac
# Device writes: only the simulator on loopback may receive SNMP SETs, IPP operations or
# firmware posts. The lab printer is read-only.
case "$cmd" in
  *snmpset*|*ipptool*|*"/fw-sim"*|*"Set-Printer-Attributes"*)
    echo "$cmd" | grep -Eq '127\.0\.0\.1|localhost|\[::1\]' || block "Device writes are allowed only against the simulator on loopback (docs/sdlc.md, device-write-safety skill)." ;;
esac
case "$cmd" in
  *"10.113.1.205"*)
    echo "$cmd" | grep -Eq 'snmpset|ipptool|curl[^|]* -X *(POST|PUT|DELETE)|-d ' && block "The lab printer at 10.113.1.205 is read-only." ;;
esac
exit 0
