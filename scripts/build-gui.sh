#!/usr/bin/env bash
# Build the EXALTED Terminal Fyne GUI with Homebrew-provided GL/Wayland/X11 deps.
set -eu
cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
. scripts/env.sh
exec go build -o /tmp/opencode/exalted-gui "$@" ./cmd/exalted-gui/