#!/bin/bash
# Packages build/bin/Heka.app into a drag-to-Applications .dmg (SPEC-18 §3.2).
# Uses hdiutil (built into macOS) rather than a third-party tool like
# create-dmg — nothing extra to install for a one-folder-plus-a-symlink DMG.
set -euo pipefail

VERSION="${1:-$(go run . --version 2>/dev/null | awk '{print $3}')}"; VERSION="${VERSION:-dev}"
BIN_DIR="build/bin"
APP_PATH="$BIN_DIR/Heka.app"
DMG_PATH="$BIN_DIR/heka-$VERSION.dmg"

if [ ! -d "$APP_PATH" ]; then
    echo "error: $APP_PATH not found — build it first (make build)" >&2
    exit 1
fi

STAGING_DIR=$(mktemp -d)
trap 'rm -rf "$STAGING_DIR"' EXIT

cp -R "$APP_PATH" "$STAGING_DIR/"
ln -s /Applications "$STAGING_DIR/Applications"

rm -f "$DMG_PATH"
hdiutil create -volname "Heka" -srcfolder "$STAGING_DIR" -ov -format UDZO "$DMG_PATH"

echo "Created $DMG_PATH"
