#!/bin/bash
# run-wrapper.sh — Wrapper script for running gcal-organizer as a service.
# Sources environment from ~/.gcal-organizer/.env and runs the full workflow.

set -euo pipefail

# Source env file if it exists
ENV_FILE="${HOME}/.gcal-organizer/.env"
if [ -f "$ENV_FILE" ]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

# --- Log rotation (FR-016) ---
# Rotate log when it exceeds 5 MB, keeping one backup (.1).
# Max disk usage: ~10 MB (5 MB active + 5 MB rotated).
LOG_FILE="${HOME}/Library/Logs/gcal-organizer.log"
MAX_LOG_BYTES=$((5 * 1024 * 1024))  # 5 MB
if [ -f "$LOG_FILE" ]; then
    LOG_SIZE=$(stat -f%z "$LOG_FILE" 2>/dev/null || stat --format=%s "$LOG_FILE" 2>/dev/null || echo 0)
    if [ "$LOG_SIZE" -gt "$MAX_LOG_BYTES" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.1"
        echo "$(date '+%Y-%m-%d %H:%M:%S') — Log rotated (was ${LOG_SIZE} bytes)"
    fi
fi

# Find the binary
if [ -n "${GCAL_ORGANIZER_BIN:-}" ] && [ -x "$GCAL_ORGANIZER_BIN" ]; then
    BINARY="$GCAL_ORGANIZER_BIN"
elif command -v gcal-organizer &>/dev/null; then
    BINARY="$(command -v gcal-organizer)"
else
    GOPATH_BIN="$(go env GOPATH 2>/dev/null)/bin/gcal-organizer"
    if [ -x "$GOPATH_BIN" ]; then
        BINARY="$GOPATH_BIN"
    else
        echo "ERROR: gcal-organizer binary not found" >&2
        echo "Run 'make install' first, or set GCAL_ORGANIZER_BIN" >&2
        exit 1
    fi
fi

echo "$(date '+%Y-%m-%d %H:%M:%S') — Starting gcal-organizer run"
echo "$BINARY run"
"$BINARY" run
echo "Exit code: $?"
echo "$(date '+%Y-%m-%d %H:%M:%S') — Completed gcal-organizer run"
