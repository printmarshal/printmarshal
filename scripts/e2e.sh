#!/usr/bin/env bash
# End-to-end smoke test: simulator + server + discovery + poll + alerts, no hardware.
# Usage: scripts/e2e.sh [port-base]   (default 19000; uses port-base .. port-base+2)
set -euo pipefail
BASE=${1:-19000}
HTTP=$BASE; SNMP=$((BASE+1)); IPP=$((BASE+2))
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d)
cleanup() { kill "${SIM_PID:-}" "${SRV_PID:-}" 2>/dev/null || true; rm -rf "$TMP"; }
trap cleanup EXIT

cd "$ROOT"
go build -o "$TMP/printmarshal" ./cmd/printmarshal
go build -o "$TMP/pm-sim" ./cmd/pm-sim

"$TMP/pm-sim" -fixture testdata/fixtures/generic-laser.snmp -snmp 127.0.0.1:$SNMP -ipp 127.0.0.1:$IPP >"$TMP/sim.log" 2>&1 &
SIM_PID=$!
PM_LISTEN=127.0.0.1:$HTTP PM_DATA_DIR="$TMP/data" PM_ADMIN_PASSWORD=e2e-pass-123 PM_SNMP_PORT=$SNMP \
  PM_IPP_ENDPOINTS="http://%s:$IPP/ipp/print" PM_TRAP_ENABLED=false PM_LOG_LEVEL=warn \
  "$TMP/printmarshal" >"$TMP/server.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 50); do curl -sf "http://127.0.0.1:$HTTP/healthz" >/dev/null && break; sleep 0.2; done
curl -sf "http://127.0.0.1:$HTTP/healthz" >/dev/null || { echo "server did not start"; cat "$TMP/server.log"; exit 1; }

J="$TMP/cookies"
api() { curl -sf -b "$J" -c "$J" -H 'Content-Type: application/json' "$@"; }
api -d '{"username":"admin","password":"e2e-pass-123"}' "http://127.0.0.1:$HTTP/api/auth/login" >/dev/null
JOB=$(api -d '{"zeroconf":false,"targets":["127.0.0.1"]}' "http://127.0.0.1:$HTTP/api/discovery/runs" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
for _ in $(seq 1 60); do
  ST=$(api "http://127.0.0.1:$HTTP/api/jobs/$JOB" | python3 -c 'import sys,json;print(json.load(sys.stdin)["job"]["status"])')
  [ "$ST" != "queued" ] && [ "$ST" != "running" ] && break; sleep 0.5
done
[ "$ST" = "succeeded" ] || { echo "discovery job status: $ST"; api "http://127.0.0.1:$HTTP/api/jobs/$JOB"; exit 1; }

api "http://127.0.0.1:$HTTP/api/devices" | python3 -c '
import sys, json
d = json.load(sys.stdin)["devices"]
assert len(d) == 1, d
x = d[0]
assert x["serial"] == "ACM4200X0917", x["serial"]
assert x["status"]["state"] == "idle" and x["status"]["severity"] == "warning", x["status"]
assert [s["percent"] for s in x["supplies"]] == [12, 71], x["supplies"]
assert x["counters"]["total"] == 184233
print("device ok:", x["model"], x["identifyMethods"])'
api "http://127.0.0.1:$HTTP/api/alerts" | python3 -c '
import sys, json
a = json.load(sys.stdin)
keys = sorted(x["key"] for x in a)
assert "supply:1" in keys, keys
print("alerts ok:", keys)'
api "http://127.0.0.1:$HTTP/api/devices.csv" | head -2 | tail -1 | grep -q ACM4200X0917 && echo "csv ok"
echo "E2E PASSED"
