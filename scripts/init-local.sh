#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f .env ]; then
  umask 077
  task_token=$(openssl rand -hex 24)
  printf 'ADMIN_TOKEN=%s\nGALLERY_PORT=5173\nBACKOFFICE_PORT=5174\nAPI_PORT=8080\n' "$task_token" > .env
  echo 'Created .env with a random administrator token.'
else
  echo '.env already exists; preserved existing settings.'
fi
