#!/usr/bin/env bash
# Install FFmpeg 8.x development libraries for CI / Linux builders.
# Ubuntu 24.04 packages are too old (and not 8.x). Prefer Homebrew-on-Linux.
set -euo pipefail

if pkg-config --exists libavcodec 2>/dev/null; then
  ver="$(pkg-config --modversion libavcodec || true)"
  case "$ver" in
    62.*)
      echo "libavcodec $ver already on PKG_CONFIG_PATH"
      exit 0
      ;;
  esac
fi

if [[ -d /opt/homebrew/opt/ffmpeg@8/lib/pkgconfig ]]; then
  echo "PKG_CONFIG_PATH=/opt/homebrew/opt/ffmpeg@8/lib/pkgconfig"
  echo "/opt/homebrew/opt/ffmpeg@8/lib/pkgconfig"
  exit 0
fi

if command -v brew >/dev/null 2>&1; then
  brew install ffmpeg@8 pkg-config
  prefix="$(brew --prefix ffmpeg@8)"
  echo "export PKG_CONFIG_PATH=${prefix}/lib/pkgconfig"
  echo "export LD_LIBRARY_PATH=${prefix}/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  echo "export DYLD_FALLBACK_LIBRARY_PATH=${prefix}/lib${DYLD_FALLBACK_LIBRARY_PATH:+:$DYLD_FALLBACK_LIBRARY_PATH}"
  exit 0
fi

echo "Homebrew is required to install ffmpeg@8 (FFmpeg 8.x) for go-astiav." >&2
echo "Install Homebrew, then: brew install ffmpeg@8 pkg-config" >&2
exit 1
