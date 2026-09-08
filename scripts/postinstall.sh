#!/bin/sh
set -e

DATA_DIR="/var/lib/livestream-viewer"
CONFIG_DIR="/etc/livestream-viewer"
LOG_DIR="/var/log/livestream-viewer"

mkdir -p "${DATA_DIR}" "${DATA_DIR}/uploads" "${DATA_DIR}/thumbnails" "${CONFIG_DIR}" "${LOG_DIR}"

if ! getent group livestream-viewer > /dev/null 2>&1; then
  groupadd --system livestream-viewer
fi

if ! getent passwd livestream-viewer > /dev/null 2>&1; then
  useradd --system --gid livestream-viewer --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin livestream-viewer
fi

# DRM/KMS and render nodes on typical Linux hosts.
if getent group video > /dev/null 2>&1; then
  usermod -aG video livestream-viewer || true
fi
if getent group render > /dev/null 2>&1; then
  usermod -aG render livestream-viewer || true
fi

chown livestream-viewer:livestream-viewer "${DATA_DIR}" "${LOG_DIR}"
chmod 750 "${DATA_DIR}" "${LOG_DIR}"
chmod 750 "${CONFIG_DIR}"

if [ ! -f "${CONFIG_DIR}/livestream-viewer.toml" ]; then
  if [ -f "/usr/share/livestream-viewer/livestream-viewer.toml" ]; then
    cp "/usr/share/livestream-viewer/livestream-viewer.toml" "${CONFIG_DIR}/livestream-viewer.toml"
    chown root:livestream-viewer "${CONFIG_DIR}/livestream-viewer.toml"
    chmod 640 "${CONFIG_DIR}/livestream-viewer.toml"
  fi
fi

if command -v systemctl > /dev/null 2>&1; then
  systemctl daemon-reload
  systemctl enable livestream-viewer.service || true
fi
