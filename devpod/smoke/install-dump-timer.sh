#!/usr/bin/env bash
set -euo pipefail

# Install the nightly dump for the magiklead smoke devpod: copies
# dump.sh + systemd user units out of the worktree (so they survive
# worktree removal), enables the timer + linger, and runs one dump
# immediately to prove the pipeline.
#
# Usage: ./install-dump-timer.sh

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BIN_DIR="$HOME/.local/share/magiklead-smoke/bin"
UNIT_DIR="$HOME/.config/systemd/user"

echo "==> Installing dump.sh to ${BIN_DIR}"
mkdir -p "$BIN_DIR" "$UNIT_DIR"
install -m 0755 "$HERE/dump.sh" "$BIN_DIR/dump.sh"

echo "==> Installing systemd user units"
install -m 0644 "$HERE/systemd/magiklead-smoke-dump.service" "$UNIT_DIR/"
install -m 0644 "$HERE/systemd/magiklead-smoke-dump.timer" "$UNIT_DIR/"
systemctl --user daemon-reload

echo "==> Enabling timer (nightly 03:17, Persistent=true)"
systemctl --user enable --now magiklead-smoke-dump.timer

echo "==> Enabling linger so user units run without a login session"
loginctl enable-linger "$USER" \
  || echo "WARN: enable-linger failed — run: sudo loginctl enable-linger $USER"

echo "==> Running first dump now"
systemctl --user start magiklead-smoke-dump.service

echo "==> Timer state:"
systemctl --user list-timers magiklead-smoke-dump.timer --no-pager
echo "==> Dumps:"
ls -lh "$HOME/.local/share/magiklead-smoke/dumps/"
