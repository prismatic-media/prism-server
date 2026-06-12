#!/usr/bin/env bash
# scripts/watch-go.sh
# Safely watches Go source files, rebuilds the server, and restarts it on changes.

set -e

CMD="./tmp/prism-dev"
PID=""

# Ensure tmp/ directory exists
mkdir -p tmp

cleanup() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    echo "[watcher] Stopping server (PID: $PID)..."
    kill "$PID" 2>/dev/null
    wait "$PID" 2>/dev/null
  fi
  exit 0
}

# Run cleanup on script interruption/termination
trap cleanup SIGINT SIGTERM EXIT

build_and_restart() {
  echo "[watcher] Rebuilding server..."
  # Build to a temporary binary in the git-ignored tmp/ directory
  if go build -o "$CMD" ./cmd/server; then
    echo "[watcher] Build successful."
    # Kill the previously running server if it exists
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
      echo "[watcher] Stopping previous server instance (PID: $PID)..."
      kill "$PID" 2>/dev/null
      wait "$PID" 2>/dev/null
    fi
    # Start the new server build
    "$CMD" &
    PID=$!
    echo "[watcher] Server started with PID: $PID"
  else
    echo "[watcher] Build failed! Keeping the existing server running."
  fi
}

# Function to calculate hash of Go source files (captures paths, additions, deletions, and mod times)
get_hash() {
  find cmd internal pkg -name "*.go" -type f -exec stat -c "%Y %n" {} + 2>/dev/null | md5sum
}

# Initial build and start
build_and_restart

LAST_HASH=$(get_hash)

while true; do
  sleep 1
  CURRENT_HASH=$(get_hash)
  if [ "$CURRENT_HASH" != "$LAST_HASH" ]; then
    echo "[watcher] Go files changed."
    LAST_HASH="$CURRENT_HASH"
    build_and_restart
  fi
done
