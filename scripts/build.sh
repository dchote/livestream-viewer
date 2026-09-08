#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f "$ROOT/scripts/dev-env.sh" ]]; then
  # shellcheck source=/dev/null
  source "$ROOT/scripts/dev-env.sh"
fi

mkdir -p build

SKIP_FRONTEND="${SKIP_FRONTEND:-false}"
TAGS=""

if [[ "${SKIP_FRONTEND}" == "true" || "${SKIP_FRONTEND}" == "1" ]]; then
  echo "Skipping frontend build"
else
  if ! command -v yarn >/dev/null 2>&1; then
    echo "yarn is required for a full build (or set SKIP_FRONTEND=true)" >&2
    exit 1
  fi
  (
    cd frontend
    if [[ -f yarn.lock ]]; then
      yarn install --frozen-lockfile
    else
      yarn install
    fi
    yarn build
  )
  rm -rf cmd/livestream-viewer/frontend-dist
  mkdir -p cmd/livestream-viewer/frontend-dist
  cp -R frontend/dist/. cmd/livestream-viewer/frontend-dist/
  TAGS="embed_frontend"
fi

LDFLAGS="-s -w -X main.version=${VERSION:-dev} -X main.commit=${COMMIT:-unknown} -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ -n "${TAGS}" ]]; then
  CGO_ENABLED=1 go build -tags "${TAGS}" -ldflags "${LDFLAGS}" -o build/livestream-viewer ./cmd/livestream-viewer
else
  CGO_ENABLED=1 go build -ldflags "${LDFLAGS}" -o build/livestream-viewer ./cmd/livestream-viewer
fi

echo "Binary at build/livestream-viewer"
