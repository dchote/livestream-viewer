# Build from the repository root. BUILD_FROM is required (arch-specific HA base):
#   docker build -f addon/Dockerfile \
#     --build-arg BUILD_FROM=ghcr.io/hassio-addons/debian-base/amd64:9.2.0 \
#     -t livestream-viewer-addon .
#   # arm64: .../debian-base/aarch64:9.2.0
#
# Home Assistant Supervisor pulls the pre-built GHCR image named in config.yaml.
# GitHub Actions always passes BUILD_FROM for each architecture.

ARG BUILD_FROM
ARG GO_VERSION=1.25.5
ARG NODE_VERSION=20
ARG FFMPEG_VERSION=8.0
ARG SDL_VERSION=3.4.0
# Pin Deno for reproducible YouTube/EJS support (yt-dlp requires Deno ≥ 2.3).
ARG DENO_VERSION=2.9.6
# BgUtils PO token provider (yt-dlp plugin + Deno HTTP server).
ARG BGUTIL_POT_VERSION=2.0.0

FROM node:${NODE_VERSION}-bookworm AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile
COPY frontend/ ./
RUN yarn build

FROM debian:bookworm AS ffmpeg
ARG FFMPEG_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl xz-utils build-essential pkg-config nasm yasm \
    libssl-dev libdrm-dev libva-dev libv4l-dev libsrt-openssl-dev \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /tmp
# Bookworm ships OpenSSL 3 (Apache-2.0). FFmpeg will not enable openssl under GPLv2.
RUN curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" | tar xJ \
  && cd "ffmpeg-${FFMPEG_VERSION}" \
  && ./configure --prefix=/opt/ffmpeg-8 --enable-shared --disable-static \
       --disable-doc --disable-debug --enable-gpl --enable-version3 \
       --enable-openssl \
       --enable-libdrm --enable-vaapi --enable-libv4l2 --enable-libsrt \
  && make -j"$(nproc)" \
  && make install

FROM debian:bookworm AS sdl3
ARG SDL_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl cmake ninja-build build-essential pkg-config \
    libdrm-dev libgbm-dev libegl-dev libgles-dev libgl-dev \
    libx11-dev libxext-dev libxcursor-dev libxi-dev libxrandr-dev libxfixes-dev \
    libxss-dev libxtst-dev \
    libwayland-dev wayland-protocols libxkbcommon-dev \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /tmp
RUN curl -fsSL "https://github.com/libsdl-org/SDL/releases/download/release-${SDL_VERSION}/SDL3-${SDL_VERSION}.tar.gz" | tar xz \
  && cmake -S "SDL3-${SDL_VERSION}" -B build -G Ninja \
       -DCMAKE_BUILD_TYPE=Release \
       -DCMAKE_INSTALL_PREFIX=/opt/sdl3 \
       -DSDL_KMSDRM=ON \
       -DSDL_X11=ON \
       -DSDL_WAYLAND=ON \
       -DSDL_TESTS=OFF \
       -DSDL_INSTALL_TESTS=OFF \
  && cmake --build build -j"$(nproc)" \
  && cmake --install build

FROM debian:bookworm AS pot
ARG DENO_VERSION
ARG BGUTIL_POT_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl unzip git python3 make g++ pkg-config \
    libcairo2-dev libpango1.0-dev libjpeg-dev libgif-dev librsvg2-dev \
  && rm -rf /var/lib/apt/lists/*
RUN set -eux; \
    arch="$(uname -m)"; \
    case "${arch}" in \
      x86_64) deno_zip=deno-x86_64-unknown-linux-gnu.zip ;; \
      aarch64) deno_zip=deno-aarch64-unknown-linux-gnu.zip ;; \
      *) echo "unsupported arch ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/denoland/deno/releases/download/v${DENO_VERSION}/${deno_zip}" -o /tmp/deno.zip; \
    unzip -o /tmp/deno.zip -d /usr/local/bin; \
    chmod 0755 /usr/local/bin/deno; \
    rm -f /tmp/deno.zip; \
    deno --version
RUN git clone --depth 1 --branch "${BGUTIL_POT_VERSION}" \
      https://github.com/Brainicism/bgutil-ytdlp-pot-provider.git /tmp/bgutil \
  && mv /tmp/bgutil/server /opt/bgutil-pot \
  && rm -rf /tmp/bgutil
WORKDIR /opt/bgutil-pot
ENV DENO_DIR=/opt/bgutil-pot/.deno
RUN deno install --allow-scripts=npm:canvas --frozen

FROM golang:${GO_VERSION}-bookworm AS backend
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
# FFmpeg was built with VA-API/DRM/V4L2/SRT/OpenSSL; cgo must link those too.
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libssl-dev libdrm-dev libva-dev libv4l-dev libsrt-openssl-dev \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY --from=ffmpeg /opt/ffmpeg-8 /opt/ffmpeg-8
ENV PKG_CONFIG_PATH=/opt/ffmpeg-8/lib/pkgconfig \
    LD_LIBRARY_PATH=/opt/ffmpeg-8/lib \
    CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist /src/cmd/livestream-viewer/frontend-dist
RUN go build -tags embed_frontend \
    -ldflags "-s -w -X main.version=${BUILD_VERSION} -X main.commit=${BUILD_COMMIT} -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -extldflags '-Wl,-rpath,/usr/lib/livestream-viewer -Wl,-rpath,/opt/ffmpeg-8/lib -Wl,-rpath,/opt/sdl3/lib'" \
    -o /out/livestream-viewer ./cmd/livestream-viewer

# BUILD_FROM must be set by the caller (amd64 or aarch64 HA debian-base).
FROM ${BUILD_FROM}
COPY --from=ffmpeg /opt/ffmpeg-8 /opt/ffmpeg-8
COPY --from=sdl3 /opt/sdl3 /opt/sdl3
COPY --from=backend /out/livestream-viewer /usr/bin/livestream-viewer
COPY --from=pot /usr/local/bin/deno /usr/local/bin/deno
COPY --from=pot /opt/bgutil-pot /opt/bgutil-pot
COPY addon/rootfs /
RUN apt-get update \
  && apt-get install -y --no-install-recommends \
       ca-certificates python3 python3-pip python3-setuptools python3-wheel \
       libssl3 \
       libdrm2 libgbm1 libegl1 libgles2 libgl1 \
       libx11-6 libxext6 libxcursor1 libxi6 libxrandr2 libxfixes3 \
       libxss1 libxtst6 \
       libwayland-client0 libxkbcommon0 \
       libva2 libva-drm2 libv4l-0 \
       libsrt1.5-openssl \
       libgl1-mesa-dri \
       libcairo2 libpango-1.0-0 libjpeg62-turbo libgif7 librsvg2-2 \
  && rm -rf /var/lib/apt/lists/* \
  && ln -sf /opt/ffmpeg-8/bin/ffmpeg /usr/local/bin/ffmpeg \
  && ln -sf /opt/ffmpeg-8/bin/ffprobe /usr/local/bin/ffprobe \
  && python3 -m pip install --break-system-packages --no-cache-dir 'yt-dlp[default]' bgutil-ytdlp-pot-provider \
  && chmod a+x /usr/local/bin/deno \
  && chmod a+x /etc/services.d/livestream-viewer/run /etc/services.d/livestream-viewer/finish \
  && chmod a+x /etc/s6-overlay/s6-rc.d/livestream-viewer/run /etc/s6-overlay/s6-rc.d/livestream-viewer/finish 2>/dev/null || true \
  && command -v yt-dlp \
  && command -v ffmpeg \
  && command -v ffprobe \
  && command -v deno \
  && test -e /opt/bgutil-pot/src/main.ts \
  && test -e /opt/sdl3/lib/libSDL3.so.0 \
  && /usr/bin/livestream-viewer --version
ENV LSV_DATABASE_PATH=/data/livestream-viewer.sqlite \
    LSV_DATA_DIR=/data \
    LSV_HTTP_PORT=8099 \
    LSV_HTTP_BIND=0.0.0.0 \
    LSV_DISPLAY_ENABLED=false \
    LSV_YOUTUBE_POT_SERVER_DIR=/opt/bgutil-pot \
    PATH="/opt/ffmpeg-8/bin:/usr/local/bin:/usr/bin:/bin" \
    LD_LIBRARY_PATH=/opt/ffmpeg-8/lib:/opt/sdl3/lib \
    SDL3_LIBRARY=/opt/sdl3/lib/libSDL3.so.0 \
    DENO_DIR=/opt/bgutil-pot/.deno
EXPOSE 8099
CMD ["/usr/bin/livestream-viewer", "-display=false"]
