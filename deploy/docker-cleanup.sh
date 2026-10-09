#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RETENTION="${1:-168h}"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml")

if [[ ! "$RETENTION" =~ ^[1-9][0-9]*(s|m|h)$ ]]; then
  echo "Invalid retention period: $RETENTION (examples: 24h, 168h, 720h)" >&2
  exit 2
fi

PROJECT="$("${COMPOSE[@]}" config --format json | sed -n 's/^[[:space:]]*"name": "\([^"]*\)",*$/\1/p' | head -n 1)"
if [[ -z "$PROJECT" ]]; then
  echo "Could not resolve the production Compose project name." >&2
  exit 1
fi

printf 'Docker disk usage before cleanup:\n'
docker system df
printf '\nRemoving stopped containers and dangling images for project %s older than %s...\n' "$PROJECT" "$RETENTION"
docker container prune --force \
  --filter "label=com.docker.compose.project=$PROJECT" \
  --filter "until=$RETENTION"
docker image prune --force \
  --filter "label=com.docker.compose.project=$PROJECT" \
  --filter "until=$RETENTION"

printf '\nRemoving unused Docker build cache older than %s...\n' "$RETENTION"
printf 'Build cache is shared by the Docker daemon; removing it can only make a later build slower.\n'
docker builder prune --force --filter "until=$RETENTION"

printf '\nDocker disk usage after cleanup:\n'
docker system df
printf '\nCleanup complete. Running containers, tagged images, networks and all volumes were preserved.\n'
