#!/usr/bin/env bash
# Extract the add-on binary and vendored FFmpeg 8 / SDL3 shared libraries
# from a built image into a staging directory for nfpm.
#
# Usage: ./scripts/extract-runtime.sh <image> <dest-dir>
set -euo pipefail

IMAGE="${1:?image tag required}"
OUT="${2:?destination directory required}"

mkdir -p "${OUT}/lib"
cid="$(docker create "${IMAGE}")"
cleanup() {
  docker rm -f "${cid}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker cp "${cid}:/usr/bin/livestream-viewer" "${OUT}/livestream-viewer"
docker cp "${cid}:/opt/ffmpeg-8/lib/." "${OUT}/lib/"
docker cp "${cid}:/opt/sdl3/lib/." "${OUT}/lib/"
mkdir -p "${OUT}/bin" "${OUT}/bgutil-pot"
docker cp "${cid}:/usr/local/bin/deno" "${OUT}/bin/deno"
docker cp "${cid}:/opt/bgutil-pot/." "${OUT}/bgutil-pot/"
rm -rf "${OUT}/lib/pkgconfig" "${OUT}/lib/cmake"
find "${OUT}/lib" -name '*.a' -delete
chmod 0755 "${OUT}/livestream-viewer" "${OUT}/bin/deno"
