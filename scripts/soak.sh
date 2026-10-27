#!/usr/bin/env bash
# 502Drive soak test script (reliability & memory leak probe)
# DO NOT RUN IN CI: this script is designed for manual / soak testing.
set -euo pipefail

DURATION="${SOAK_DURATION:-30s}"
INTERVAL="${SOAK_INTERVAL:-2s}"
MAX_RSS_KB="${SOAK_MAX_RSS_KB:-150000}" # 150 MB

echo "=== 502Drive Soak Test ==="
echo "Duration: ${DURATION}"
echo "Check Interval: ${INTERVAL}"
echo "Max RSS Threshold: ${MAX_RSS_KB} KB"

TEMP_DIR=$(mktemp -d /tmp/502drive-soak-XXXXXX)
trap 'rm -rf "${TEMP_DIR}"' EXIT

export DRIVE502_CONFIG="${TEMP_DIR}/config.json"
export DRIVE502_DATA_DIR="${TEMP_DIR}/data"
export DRIVE502_TOKEN_FILE="${TEMP_DIR}/token.json"

# Build local binary if not found
BIN="./502drive"
if [ ! -f "${BIN}" ]; then
    echo "Building ./502drive..."
    go build -o "${BIN}" ./cmd/502drive
fi

echo "Setting up initial private config..."
mkdir -p "${TEMP_DIR}/data"
echo "soak-secret-token" > "${DRIVE502_TOKEN_FILE}"
chmod 0600 "${DRIVE502_TOKEN_FILE}"

cat <<EOF > "${DRIVE502_CONFIG}"
{
  "listen": "127.0.0.1:0",
  "data_dir": "${DRIVE502_DATA_DIR}",
  "token_file": "${DRIVE502_TOKEN_FILE}"
}
EOF

echo "Starting 502drive serve in background..."
"${BIN}" serve --maintenance &
DAEMON_PID=$!

cleanup_daemon() {
    if kill -0 "${DAEMON_PID}" 2>/dev/null; then
        echo "Stopping daemon PID ${DAEMON_PID}..."
        kill -TERM "${DAEMON_PID}" 2>/dev/null || true
        wait "${DAEMON_PID}" 2>/dev/null || true
    fi
}
trap cleanup_daemon EXIT

sleep 1

# Check process is alive
if ! kill -0 "${DAEMON_PID}" 2>/dev/null; then
    echo "Daemon failed to start."
    exit 1
fi

START_TIME=$(date +%s)
# Parse duration in seconds
case "${DURATION}" in
    *s) DURATION_SECS="${DURATION%s}" ;;
    *m) DURATION_SECS=$(( ${DURATION%m} * 60 )) ;;
    *h) DURATION_SECS=$(( ${DURATION%h} * 3600 )) ;;
    *)  DURATION_SECS="${DURATION}" ;;
esac

echo "Running soak test for ${DURATION_SECS} seconds..."
ITERATION=0
PEAK_RSS=0

while true; do
    NOW=$(date +%s)
    ELAPSED=$(( NOW - START_TIME ))
    if [ "${ELAPSED}" -ge "${DURATION_SECS}" ]; then
        break
    fi

    ITERATION=$(( ITERATION + 1 ))

    # Sample RSS from /proc/$PID/status
    if [ -f "/proc/${DAEMON_PID}/status" ]; then
        RSS_KB=$(grep VmRSS "/proc/${DAEMON_PID}/status" | awk '{print $2}')
        if [ -n "${RSS_KB}" ]; then
            if [ "${RSS_KB}" -gt "${PEAK_RSS}" ]; then
                PEAK_RSS="${RSS_KB}"
            fi
            if [ "${RSS_KB}" -gt "${MAX_RSS_KB}" ]; then
                echo "FAIL: RSS exceeded limit (${RSS_KB} KB > ${MAX_RSS_KB} KB) at iteration ${ITERATION}"
                exit 1
            fi
        fi
    fi

    # Run quick client operations
    "${BIN}" status >/dev/null 2>&1 || true
    "${BIN}" doctor >/dev/null 2>&1 || true

    sleep "${INTERVAL}"
done

echo "Soak test completed successfully."
echo "Iterations: ${ITERATION}"
echo "Peak RSS: ${PEAK_RSS} KB"
echo "SOAK-TEST-PASSED"
