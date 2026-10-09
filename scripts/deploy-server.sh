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
exec 9>"$lock_dir/$project.deploy.lock"
flock -w 600 9

compose=(docker compose --env-file /dev/null -p "$project" -f "$repo_root/compose.release.yaml")
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
