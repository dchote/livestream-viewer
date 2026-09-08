#!/usr/bin/env bash
# Export YouTube cookies from a local browser into data/secrets/youtube.cookies.
# Chrome/Safari will prompt for the macOS keychain; run this from Terminal, not
# from a sandboxed agent. Firefox usually does not prompt.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${1:-$ROOT/data/secrets/youtube.cookies}"
BROWSER="${LSV_YOUTUBE_COOKIES_FROM_BROWSER:-chrome}"

if ! command -v yt-dlp >/dev/null 2>&1; then
  echo "yt-dlp is not on PATH" >&2
  exit 1
fi

mkdir -p "$(dirname "$DEST")"
# A cheap extract is enough to dump the jar; the URL need not be playable.
yt-dlp --cookies-from-browser "$BROWSER" --cookies "$DEST" --skip-download --no-warnings \
  "https://www.youtube.com/" || true

if [[ ! -s "$DEST" ]]; then
  echo "cookie export failed (empty $DEST). On macOS, approve the keychain prompt and retry." >&2
  echo "Try BROWSER=chrome:Default or firefox: LSV_YOUTUBE_COOKIES_FROM_BROWSER=firefox $0" >&2
  exit 1
fi

echo "wrote $DEST"
