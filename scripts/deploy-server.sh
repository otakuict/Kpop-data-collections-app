#!/usr/bin/env bash
# Deploy all three published images from one commit; keep the SQLite volume.
# Runtime configuration comes from exported environment variables, not the checkout.
set -euo pipefail

sha="${1:?usage: deploy-server.sh <full-commit-sha>}"
[[ "$sha" =~ ^[a-f0-9]{40}$ ]] || { echo 'Expected a full commit SHA' >&2; exit 1; }
: "${DOCKERHUB_USERNAME:?Set DOCKERHUB_USERNAME}"
: "${ADMIN_TOKEN:?Set ADMIN_TOKEN}"
[[ ${#ADMIN_TOKEN} -ge 24 ]] || { echo 'ADMIN_TOKEN must contain at least 24 characters' >&2; exit 1; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
export DOCKERHUB_USERNAME ADMIN_TOKEN
export IMAGE_TAG="$sha"

compose=(docker compose --env-file /dev/null -p "${DEPLOY_PROJECT_NAME:-otakuict-data-gallery}" -f "$repo_root/compose.release.yaml")
"${compose[@]}" pull
"${compose[@]}" up -d --wait --wait-timeout 180
"${compose[@]}" ps
