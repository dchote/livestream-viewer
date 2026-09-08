# Source before go build/test/run on macOS so cgo links FFmpeg 8.x
# (Homebrew ffmpeg/ffmpeg-full may be 9.x; go-astiav requires n8.0).
#   source ./scripts/dev-env.sh
if [ -d /opt/homebrew/opt/ffmpeg@8/lib/pkgconfig ]; then
  export PKG_CONFIG_PATH="/opt/homebrew/opt/ffmpeg@8/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
  export DYLD_FALLBACK_LIBRARY_PATH="/opt/homebrew/opt/ffmpeg@8/lib${DYLD_FALLBACK_LIBRARY_PATH:+:$DYLD_FALLBACK_LIBRARY_PATH}"
fi
export CGO_ENABLED=1
