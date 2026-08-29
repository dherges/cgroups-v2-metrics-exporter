#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"
EXPORTER_PID=""
cleanup() {
  if [ -n "${EXPORTER_PID:-}" ]; then
    if kill -0 "$EXPORTER_PID" 2>/dev/null; then
      echo -e "\n=== [Shutdown Hook] Cleaning Up ==="
      echo "Killing background exporter process (PID: $EXPORTER_PID)..."
      kill "$EXPORTER_PID"
      wait "$EXPORTER_PID" 2>/dev/null || true
    fi
  fi
}
trap cleanup EXIT INT TERM

echo "=== Building Exporter ==="
make build
echo "Build success!"

echo "=== Launching Exporter ==="
export CGROUP_BASE_PATH="${PROJECT_ROOT}/fake-units"
export METRICS_PORT="9100"
./bin/cgroups-v2-metrics-exporter &
EXPORTER_PID=$!
sleep 0.5

echo "=== Querying Metrics ==="
local_url="http://127.0.0.1:${METRICS_PORT}/metrics"
echo "Executing: curl ${local_url}"
echo "------------------------------------------------"
curl --verbose "${local_url}"
echo "------------------------------------------------"
