#!/bin/sh
set -e

if command -v systemctl > /dev/null 2>&1; then
  systemctl stop livestream-viewer.service || true
  systemctl disable livestream-viewer.service || true
fi
