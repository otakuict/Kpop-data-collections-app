#!/bin/sh
set -eu
trap 'rm -f data/seed.sqlite.next' EXIT
cd "$(dirname "$0")/.."
if docker compose version >/dev/null 2>&1; then
 compose() { docker compose "$@"; }
else
 compose() { docker-compose "$@"; }
fi
# Snapshot through the backend's SQLite connection, then verify before replacing the seed.
compose exec -T backend sh -c 'task_snapshot_dir=$(mktemp -d /tmp/gallery-seed-XXXXXX); task_snapshot=$task_snapshot_dir/seed.sqlite; if [ -x /tmp/bias-air/server ]; then task_server=/tmp/bias-air/server; else task_server=/server; fi; "$task_server" -db /data/gallery.sqlite -backup "$task_snapshot" >/dev/null && cat "$task_snapshot"; task_result=$?; rm -rf "$task_snapshot_dir"; exit "$task_result"' > data/seed.sqlite.next
python3 scripts/verify-seed.py data/source.csv data/seed.sqlite.next
mv data/seed.sqlite.next data/seed.sqlite
