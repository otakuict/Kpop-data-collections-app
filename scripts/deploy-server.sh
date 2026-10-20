#!/usr/bin/env bash
# Deploy one published service; keep other containers and the SQLite volume.
# Runtime configuration comes from exported environment variables, not the checkout.
set -euo pipefail

service="${1:?usage: deploy-server.sh <frontend|backoffice|backend> <full-commit-sha>}"
sha="${2:?usage: deploy-server.sh <service> <full-commit-sha>}"
case "$service" in
  frontend|backoffice|backend) ;;
  *) echo 'Unknown service' >&2; exit 1 ;;
esac
[[ "$sha" =~ ^[a-f0-9]{40}$ ]] || { echo 'Expected a full commit SHA' >&2; exit 1; }
: "${DOCKERHUB_USERNAME:?Set DOCKERHUB_USERNAME}"
: "${ADMIN_TOKEN:?Set ADMIN_TOKEN}"
[[ ${#ADMIN_TOKEN} -ge 24 ]] || { echo 'ADMIN_TOKEN must contain at least 24 characters' >&2; exit 1; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
project="${DEPLOY_PROJECT_NAME:-otakuict-data-gallery}"
[[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || { echo 'Invalid Compose project name' >&2; exit 1; }
export DOCKERHUB_USERNAME ADMIN_TOKEN
case "$service" in
  frontend) export FRONTEND_IMAGE_TAG="$sha" ;;
  backoffice) export BACKOFFICE_IMAGE_TAG="$sha" ;;
  backend) export BACKEND_IMAGE_TAG="$sha" ;;
esac

lock_dir="${XDG_STATE_HOME:-$HOME/.local/state}/bias-archive"
mkdir -p "$lock_dir"
chmod 700 "$lock_dir"
exec 9>"$lock_dir/$project.deploy.lock"
flock -w 600 9

compose=(docker compose --env-file /dev/null -p "$project" -f "$repo_root/compose.release.yaml")
sheet_key_file="${GOOGLE_SHEETS_HOST_KEY_FILE:-$lock_dir/$project.google-sheets.json}"
if [[ -n "${GOOGLE_SHEETS_SERVICE_ACCOUNT_B64:-}" && "$service" == backend ]]; then
  [[ "$sheet_key_file" == /* ]] || { echo 'Google Sheets key path must be absolute' >&2; exit 1; }
  key_tmp="$(mktemp "$(dirname "$sheet_key_file")/$project.google-sheets.XXXXXX")"
  # The backend container runs as UID 10001. Its bind mount needs read access;
  # the private host directory prevents other host users from reaching the file.
  chmod 644 "$key_tmp"
  if ! printf '%s' "$GOOGLE_SHEETS_SERVICE_ACCOUNT_B64" | base64 --decode > "$key_tmp"; then
    rm -f "$key_tmp"
    echo 'Google Sheets secret is not valid base64' >&2
    exit 1
  fi
  mv -f "$key_tmp" "$sheet_key_file"
  unset GOOGLE_SHEETS_SERVICE_ACCOUNT_B64
fi
if [[ -f "$sheet_key_file" ]]; then
  export GOOGLE_SHEETS_HOST_KEY_FILE="$sheet_key_file"
  compose+=(-f "$repo_root/compose.sheet-sync.yaml")
fi
if [[ "$service" != backend ]]; then
  backend_id="$("${compose[@]}" ps --status running -q backend)"
  if [[ -z "$backend_id" ]] || [[ "$(docker inspect --format '{{.State.Health.Status}}' "$backend_id")" != healthy ]]; then
    echo 'Deploy a healthy backend before deploying a web service' >&2
    exit 1
  fi
fi
"${compose[@]}" pull "$service"
"${compose[@]}" up -d --no-deps --wait --wait-timeout 180 "$service"
"${compose[@]}" ps "$service"
