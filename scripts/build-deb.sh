#!/usr/bin/env bash
# Build linux/amd64 and/or linux/arm64 .deb packages from the add-on image.
# Requires Docker. Compiles FFmpeg 8 and SDL3 inside Docker (same as the add-on).
#
# Usage:
#   ./scripts/build-deb.sh                 # snapshot version from config + commit
#   VERSION=0.1.0 ./scripts/build-deb.sh   # release-style version (no snapshot suffix)
#   ARCHS=amd64 ./scripts/build-deb.sh
#
# When VERSION is unset, packages use <config>-dev.<shortsha> (local snapshot).
# When VERSION is set, that exact version is used (must be Debian-safe: digits/dots).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ARCHS="${ARCHS:-amd64 arm64}"
config_version="$(grep '^version:' addon/config.yaml | sed 's/version: *"\?\([^"]*\)"\?/\1/' | tr -d ' ')"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DEBIAN_BASE_VERSION="${DEBIAN_BASE_VERSION:-9.2.0}"

if [[ -n "${VERSION:-}" ]]; then
  PKG_VERSION="${VERSION#v}"
  echo "Packaging release version ${PKG_VERSION}"
else
  PKG_VERSION="${config_version}-dev.${COMMIT}"
  echo "Packaging local snapshot version ${PKG_VERSION}"
fi

build_one() {
  local docker_arch="$1"
  local ha_arch="$2"
  local platform="linux/${docker_arch}"
  local base="ghcr.io/hassio-addons/debian-base/${ha_arch}:${DEBIAN_BASE_VERSION}"
  local image="livestream-viewer-addon:${docker_arch}"

  echo "Building add-on image for ${platform}..."
  docker build \
    --platform "${platform}" \
    -f addon/Dockerfile \
    --build-arg "BUILD_FROM=${base}" \
    --build-arg "BUILD_VERSION=${PKG_VERSION}" \
    --build-arg "BUILD_COMMIT=${COMMIT}" \
    -t "${image}" \
    .

  echo "Smoke-checking image tools..."
  docker run --rm --entrypoint /bin/bash "${image}" -lc '
    set -euo pipefail
    yt-dlp --version
    deno --version
    ffmpeg -version | head -n1
    ffprobe -version | head -n1
    test -e "${SDL3_LIBRARY:-/opt/sdl3/lib/libSDL3.so.0}"
    livestream-viewer --version
  '

  echo "Extracting runtime from ${image}..."
  rm -rf "build/prebuilt/${docker_arch}"
  ./scripts/extract-runtime.sh "${image}" "build/prebuilt/${docker_arch}"

  echo "Packaging ${docker_arch} .deb..."
  VERSION="${PKG_VERSION}" ARCH="${docker_arch}" ./scripts/package-deb.sh
}

mkdir -p dist
for arch in ${ARCHS}; do
  case "${arch}" in
    amd64) build_one amd64 amd64 ;;
    arm64) build_one arm64 aarch64 ;;
    *)
      echo "unknown arch: ${arch}" >&2
      exit 1
      ;;
  esac
done

echo "Debian packages:"
ls -1 dist/*.deb 2>/dev/null || echo "(none found)"
