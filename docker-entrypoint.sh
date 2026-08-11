#!/bin/sh
set -e
DATA_DIR="${DATA_DIR:-/app/data}"
mkdir -p "$DATA_DIR"
chown -R apps:apps "$DATA_DIR" 2>/dev/null || true
exec su-exec apps /usr/local/bin/lampa-sync-server
