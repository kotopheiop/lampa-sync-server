#!/bin/sh
set -e
DATA_DIR="${DATA_DIR:-/app/data}"
mkdir -p "$DATA_DIR"
# volume mount часто приходит с root-владельцем с хоста
chown -R apps:apps "$DATA_DIR" 2>/dev/null || true
exec su-exec apps node index.js
