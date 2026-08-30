#!/bin/sh
# Pre-deploy gate: fail if any known placeholder secret is still live in
# the rendered docker-compose config or in coturn/turnserver.conf (which
# never goes through `docker compose config` — it's COPY'd into the coturn
# image via dockerfile_inline, not interpolated).
#
# Usage: scripts/check-secrets.sh [ENV_DIR] [TURN_CONF]
#   ENV_DIR    a directory containing a file literally named ".env" to
#              render against, via `docker compose --project-directory`.
#              Omit to check the real project .env (production default).
#              NOTE: a plain `--env-file` override is NOT enough here —
#              docker-compose.yml's node services also carry a hardcoded
#              `env_file: - .env` (docker-compose.yml:61), which always
#              re-reads the literal ".env" next to the compose file
#              regardless of --env-file. --project-directory changes
#              *where* that literal ".env" is looked up, which is the only
#              way to fully redirect both interpolation and env_file:.
#   TURN_CONF  defaults to coturn/turnserver.conf
#
# Exit 0: clean, safe to deploy. Exit 1: placeholder found, deploy must stop.

set -e

ENV_DIR="${1:-}"
TURN_CONF="${2:-coturn/turnserver.conf}"

ROOT="$(git rev-parse --show-toplevel)"
COMPOSE_FILE="$ROOT/docker-compose.yml"

BLOCKLIST='CHANGE_THIS_|obscura-secret-CHANGE|obscura-turn-secret-CHANGE|obscura_grafana|obscura-admin'

found=0

if [ -n "$ENV_DIR" ]; then
  render="$(docker compose -f "$COMPOSE_FILE" --project-directory "$ENV_DIR" config 2>/dev/null)"
else
  render="$(docker compose -f "$COMPOSE_FILE" --project-directory "$ROOT" config 2>/dev/null)"
fi

# Exclude `name:` lines (volume/network/project identity, e.g. the
# "<project>_grafana-data" volume compose derives from the project name
# "obscura" + "grafana-data" — that's a structural name, never a secret
# value, but it textually contains our "obscura_grafana" blocklist term).
render_scannable="$(printf '%s\n' "$render" | grep -vE '^[[:space:]]*name:[[:space:]]')"

hits="$(printf '%s\n' "$render_scannable" | grep -inE "$BLOCKLIST" || true)"
if [ -n "$hits" ]; then
  echo "check-secrets: docker compose config render'ında placeholder bulundu:" >&2
  printf '%s\n' "$hits" >&2
  found=1
fi

if [ -f "$TURN_CONF" ]; then
  hits="$(grep -inE "$BLOCKLIST" "$TURN_CONF" || true)"
  if [ -n "$hits" ]; then
    echo "check-secrets: $TURN_CONF içinde placeholder bulundu:" >&2
    printf '%s\n' "$hits" >&2
    found=1
  fi
fi

if [ "$found" -ne 0 ]; then
  echo "check-secrets: FATAL — placeholder secret prod'a sızacaktı, deploy durduruldu" >&2
  exit 1
fi

echo "check-secrets: temiz, placeholder yok"
exit 0
