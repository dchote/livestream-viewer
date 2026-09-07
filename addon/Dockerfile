# Build from the repository root:
#   docker build -f addon/Dockerfile -t livestream-viewer-addon .
#
# Home Assistant Supervisor uses BUILD_FROM from build.yaml when it builds
# locally; published installs pull the image named in config.yaml.

ARG BUILD_FROM=debian:bookworm-slim
ARG GO_VERSION=1.25.5
ARG NODE_VERSION=20

FROM node:${NODE_VERSION}-bookworm AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile
COPY frontend/ ./
RUN yarn build

FROM golang:${GO_VERSION}-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist /src/cmd/livestream-viewer/frontend-dist
RUN CGO_ENABLED=1 go build -tags embed_frontend -ldflags "-s -w" -o /out/livestream-viewer ./cmd/livestream-viewer

FROM ${BUILD_FROM}
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=backend /out/livestream-viewer /usr/bin/livestream-viewer
COPY addon/rootfs /
RUN chmod a+x /etc/s6-overlay/s6-rc.d/livestream-viewer/run /etc/s6-overlay/s6-rc.d/livestream-viewer/finish 2>/dev/null || true \
  && chmod a+x /etc/services.d/livestream-viewer/run /etc/services.d/livestream-viewer/finish 2>/dev/null || true
ENV LSV_DATABASE_PATH=/data/livestream-viewer.sqlite \
    LSV_DATA_DIR=/data \
    LSV_HTTP_PORT=8099 \
    LSV_HTTP_BIND=0.0.0.0 \
    LSV_DISPLAY_ENABLED=false
EXPOSE 8099
CMD ["/usr/bin/livestream-viewer", "-display=false"]
