#!/usr/bin/env bash
set -euo pipefail

# Inputs (env):
#   DEB_VERSION   (required)  Debian version for this build (e.g., 1.4.0-1 or 1.4.0~gitYYYYMMDD+sha-1)
#   BUILD_KIND    (required)  "release" | "snapshot"
#   DESCRIBE                  e.g., output of `git describe` (snapshot only)
#   BUILD_SHA                 short commit sha (snapshot only)
#   BUILD_DATE                ISO UTC (snapshot only)
#   DEB_PACKAGE              default: infero
#   DEB_DISTRIBUTION         default: unstable
#   RELEASE_TITLE             (release) optional if not parsing event JSON
#   RELEASE_BODY              (release) optional if not parsing event JSON
#   RELEASE_URL               (release) optional if not parsing event JSON
#   GITHUB_EVENT_NAME, GITHUB_EVENT_PATH, GITHUB_SERVER_URL, GITHUB_REPOSITORY (for release JSON)

: "${DEB_VERSION:?DEB_VERSION is required}"
: "${BUILD_KIND:?BUILD_KIND is required}"
DEB_PACKAGE="${DEB_PACKAGE:-infero}"
DEB_DISTRIBUTION="${DEB_DISTRIBUTION:-unstable}"

# Ensure base changelog state (create or bump to DEB_VERSION)
if [[ ! -f debian/changelog ]]; then
  dch --create -v "$DEB_VERSION" --package "$DEB_PACKAGE" --distribution "$DEB_DISTRIBUTION" "Initialize changelog"
elif ! dpkg-parsechangelog -S Version | grep -qx "$DEB_VERSION"; then
  dch --newversion "$DEB_VERSION" --distribution "$DEB_DISTRIBUTION" " "
fi

if [[ "$BUILD_KIND" == "release" ]]; then
  # Prefer provided RELEASE_* vars; otherwise read from the GitHub release event.
  if [[ -z "${RELEASE_TITLE:-}" || -z "${RELEASE_URL:-}" ]]; then
    if [[ "${GITHUB_EVENT_NAME:-}" == "release" && -f "${GITHUB_EVENT_PATH:-}" ]]; then
      RAW_TAG="$(jq -r '.release.tag_name' "$GITHUB_EVENT_PATH")"
      RELEASE_TITLE="$(jq -r '.release.name // .release.tag_name' "$GITHUB_EVENT_PATH")"
      RELEASE_BODY="$(jq -r '.release.body // ""' "$GITHUB_EVENT_PATH")"
      RELEASE_URL="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/releases/tag/${RAW_TAG}"
    fi
  fi

  dch --no-auto-nmu "Release: ${RELEASE_TITLE:-Untitled} — ${RELEASE_URL:-}"
  # Append each line of body; blank lines -> '.' to keep paragraph breaks for dch
  while IFS= read -r line; do
    if [[ -z "$line" ]]; then
      dch --no-auto-nmu "."
    else
      dch --no-auto-nmu "$line"
    fi
  done <<< "${RELEASE_BODY:-}"

else
  # Snapshot entry
  SNAP_MSG="Snapshot build: ${DESCRIBE:-} (${BUILD_SHA:-} @ ${BUILD_DATE:-})"
  dch --no-auto-nmu "$SNAP_MSG"
fi
