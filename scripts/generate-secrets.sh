#!/bin/sh
# Generate real secrets for every unrotated CHANGE_THIS_*/known-placeholder
# value in .env, and keep coturn/turnserver.conf's static-auth-secret in
# sync with whatever TURN secret ends up effective (secrets.Require's
# TURN_SECRET -> TURN_SHARED_SECRET alias order).
#
# Idempotent: a value that is already real (rotated) is left untouched.
# Only the fixed set of known secret-bearing keys below is ever considered
# — unrelated CHANGE_THIS_-looking strings elsewhere in .env are not touched.
#
# Usage: scripts/generate-secrets.sh [ENV_FILE] [TURN_CONF]
#   ENV_FILE   defaults to .env
#   TURN_CONF  defaults to coturn/turnserver.conf

set -e

ENV_FILE="${1:-.env}"
TURN_CONF="${2:-coturn/turnserver.conf}"

if [ ! -f "$ENV_FILE" ]; then
  echo "generate-secrets: $ENV_FILE bulunamadı" >&2
  exit 1
fi

cp "$ENV_FILE" "$ENV_FILE.bak"

# Known secret-bearing keys this script is allowed to rotate. TURN_SECRET is
# intentionally excluded: its default is a legitimately empty alias slot
# (see .env.example TURN/coturn comment), not a placeholder to replace.
KNOWN_KEYS="NODE_INTERNAL_SECRET INTERNAL_SECRET JWT_SECRET OBSCURA_PHONE_PEPPER OBSCURA_MESSAGE_OWNER_PEPPER MINIO_ACCESS_KEY MINIO_SECRET_KEY TURN_SHARED_SECRET POSTGRES_PASSWORD GF_SECURITY_ADMIN_PASSWORD"

is_known_key() {
  key="$1"
  for k in $KNOWN_KEYS; do
    [ "$k" = "$key" ] && return 0
  done
  return 1
}

is_placeholder_value() {
  v="$1"
  case "$v" in
    CHANGE_THIS_*) return 0 ;;
    obscura-secret-CHANGE-IN-PRODUCTION) return 0 ;;
    obscura-turn-secret-CHANGE-IN-PRODUCTION) return 0 ;;
    obscura_grafana) return 0 ;;
    obscura-admin) return 0 ;;
  esac
  return 1
}

generated=0
skipped=0
turn_secret_val=""
turn_shared_secret_val=""

tmp_file="$(mktemp)"

while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    [A-Za-z_]*=*)
      key="${line%%=*}"
      value="${line#*=}"
      case "$key" in
        *[!A-Za-z0-9_]*) is_key=0 ;;
        *) is_key=1 ;;
      esac
      if [ "$is_key" -eq 1 ] && is_known_key "$key"; then
        if is_placeholder_value "$value"; then
          value="$(openssl rand -hex 32)"
          line="${key}=${value}"
          generated=$((generated + 1))
        else
          skipped=$((skipped + 1))
        fi
      fi
      [ "$key" = "TURN_SECRET" ] && turn_secret_val="$value"
      [ "$key" = "TURN_SHARED_SECRET" ] && turn_shared_secret_val="$value"
      ;;
  esac
  printf '%s\n' "$line" >>"$tmp_file"
done <"$ENV_FILE"

mv "$tmp_file" "$ENV_FILE"

# Effective TURN secret mirrors secrets.Require("TURN_SECRET","TURN_SHARED_SECRET"):
# TURN_SECRET wins if non-empty, else fall back to TURN_SHARED_SECRET.
if [ -n "$turn_secret_val" ]; then
  effective_turn_secret="$turn_secret_val"
else
  effective_turn_secret="$turn_shared_secret_val"
fi

if [ -f "$TURN_CONF" ] && [ -n "$effective_turn_secret" ]; then
  turn_tmp="$(mktemp)"
  awk -v val="$effective_turn_secret" '
    /^static-auth-secret=/ { print "static-auth-secret=" val; next }
    { print }
  ' "$TURN_CONF" >"$turn_tmp"
  mv "$turn_tmp" "$TURN_CONF"
fi

echo "generate-secrets: ${generated} secret üretildi, ${skipped} zaten-gerçekti-atlandı"
