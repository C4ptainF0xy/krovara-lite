#!/usr/bin/env bash
# Krovara — production deploy (run ON the VPS, from the repo checkout).
#
# Pulls the latest main, rebuilds the images, applies migrations (the `migrate`
# compose service runs once on `up`), and restarts the stack with zero manual
# steps. Invoked by the GitHub Actions deploy job over SSH, or by hand:
#
#   ssh deploy@<vps>  'cd /opt/krovara && ./scripts/deploy.sh'
#
# Assumes: Docker + compose v2 installed, the repo cloned at $KROVARA_DIR with a
# populated `.env` next to docker-compose.yml. Idempotent and safe to re-run.
set -euo pipefail

KROVARA_DIR="${KROVARA_DIR:-/opt/krovara}"
BRANCH="${KROVARA_BRANCH:-main}"
COMPOSE="docker compose"

cd "$KROVARA_DIR"

echo "==> Fetching $BRANCH"
git fetch --prune origin "$BRANCH"
git reset --hard "origin/$BRANCH"

if [[ ! -f .env ]]; then
  echo "!! .env is missing next to docker-compose.yml — copy .env.production.example and fill it." >&2
  exit 1
fi

echo "==> Building images"
$COMPOSE build

echo "==> Starting stack (migrate runs as a one-shot dependency)"
# --remove-orphans drops services deleted from the compose file between releases.
$COMPOSE up -d --remove-orphans

echo "==> Waiting for the API to report healthy"
# The api is distroless (no in-container shell for a compose healthcheck); poll
# the public /healthz route (Caddy → api) instead. Non-fatal: log and continue.
for i in $(seq 1 30); do
  if curl -fsS -o /dev/null "https://${KROVARA_DOMAIN:-localhost}/healthz" 2>/dev/null; then
    echo "    API healthy after ${i}0s"
    break
  fi
  sleep 10
done

echo "==> Pruning dangling images"
docker image prune -f >/dev/null || true

echo "==> Deploy complete: $(git rev-parse --short HEAD)"
$COMPOSE ps
