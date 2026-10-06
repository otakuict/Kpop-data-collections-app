#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Refresh on a temporary database, then replace the checked-in seed only after verification.
# Preserve existing images and provider state. Fetch new media only through the running API.
task_temp_dir=$(mktemp -d)
trap 'rm -rf "$task_temp_dir"' EXIT
curl -fL --max-time 60 'https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/export?format=csv&gid=0' -o "$task_temp_dir/source.csv"
cp data/seed.sqlite "$task_temp_dir/seed.sqlite"
(cd backend && go run ./cmd/server -db "$task_temp_dir/seed.sqlite" -import "$task_temp_dir/source.csv")
python3 scripts/verify-seed.py "$task_temp_dir/source.csv" "$task_temp_dir/seed.sqlite"
cp "$task_temp_dir/source.csv" data/source.csv
cp "$task_temp_dir/seed.sqlite" data/seed.sqlite
python3 scripts/verify-seed.py
