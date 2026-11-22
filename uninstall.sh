#!/bin/sh
set -e

BINARY="flow"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY_PATH="${INSTALL_DIR}/${BINARY}"

if [ ! -f "$BINARY_PATH" ]; then
  echo "${BINARY} is not installed at ${BINARY_PATH}"
  exit 0
fi

rm "$BINARY_PATH"
echo "${BINARY} has been removed from ${BINARY_PATH}"

# macOS: the helper app flow builds to show its icon on notifications.
NOTIFIER_APP="$HOME/Library/Application Support/flow/Flow.app"
if [ -d "$NOTIFIER_APP" ]; then
  rm -rf "$NOTIFIER_APP"
  echo "Removed ${NOTIFIER_APP}"
fi
