#!/bin/bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <dmg-path>" >&2
  exit 1
fi

DMG="$1"

# Ensure required environment variables are present
: "${AC_API_KEY_ID:?Need AC_API_KEY_ID}"
: "${AC_API_ISSUER_ID:?Need AC_API_ISSUER_ID}"
: "${AC_API_P8:?Need AC_API_P8}"  # base64-encoded API key

P8_FILE="$(mktemp)"
# Decode the base64-encoded key into a temporary file
printf '%s' "$AC_API_P8" | base64 --decode > "$P8_FILE"

# Submit for notarization and wait for completion
xcrun notarytool submit "$DMG" \
  --key "$P8_FILE" \
  --key-id "$AC_API_KEY_ID" \
  --issuer "$AC_API_ISSUER_ID" \
  --wait

rm -f "$P8_FILE"
