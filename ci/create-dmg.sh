#!/bin/bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "Usage: $0 <app-bundle> <dmg-path>" >&2
  exit 1
fi

APP="$1"
DMG="$2"
VOL_NAME=$(basename "$APP" .app)

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

cp -R "$APP" "$TMP_DIR"/
ln -s /Applications "$TMP_DIR"/Applications

hdiutil create -volname "$VOL_NAME" -srcfolder "$TMP_DIR" -ov -format UDZO "$DMG"
