#!/usr/bin/env bash
# Package a .deb from build/prebuilt/<arch> using nFPM (OSS).
#
# Usage:
#   VERSION=0.1.0 ARCH=amd64 ./scripts/package-deb.sh
#   VERSION=0.1.0-dev.12 ARCH=arm64 ./scripts/package-deb.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ARCH="${ARCH:?ARCH=amd64|arm64 required}"
VERSION="${VERSION:?VERSION required}"
VERSION="${VERSION#v}"
NFPM_IMAGE="${NFPM_IMAGE:-goreleaser/nfpm:v2.47.0}"

case "${ARCH}" in
  amd64) config=packaging/nfpm-amd64.yaml ;;
  arm64) config=packaging/nfpm-arm64.yaml ;;
  *)
    echo "unsupported ARCH=${ARCH}" >&2
    exit 1
    ;;
esac

if [[ ! -x "build/prebuilt/${ARCH}/livestream-viewer" ]]; then
  echo "missing build/prebuilt/${ARCH}/livestream-viewer — run extract-runtime.sh first" >&2
  exit 1
fi
if [[ ! -d "build/prebuilt/${ARCH}/lib" ]]; then
  echo "missing build/prebuilt/${ARCH}/lib — run extract-runtime.sh first" >&2
  exit 1
fi

mkdir -p dist
out="dist/livestream-viewer_${VERSION}_${ARCH}.deb"

docker run --rm \
  -e VERSION="${VERSION}" \
  -v "${ROOT}:/work" \
  -w /work \
  "${NFPM_IMAGE}" \
  package -f "${config}" -p deb -t "${out}"

echo "Wrote ${out}"
