#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

load_dotenv() {
  [ -f .env ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ""|\#*) continue
      ;;
    esac
    case "$line" in
      *=*)
      ;;
      *)
        continue
      ;;
    esac
    key="${line%%=*}"
    val="${line#*=}"
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ "$val" =~ ^\".*\"$ ]]; then
      val="${val:1:${#val}-2}"
    fi
    if [[ "$val" =~ ^\'.*\'$ ]]; then
      val="${val:1:${#val}-2}"
    fi
    export "$key=$val"
  done < .env
}

if [ ! -d frontend/node_modules ]; then
  echo "Installation des dépendances frontend..."
  npm install --prefix frontend
fi

load_dotenv

echo "Démarrage API..."
go run ./cmd/api &
api_pid=$!

echo "Démarrage Worker..."
go run ./cmd/worker &
worker_pid=$!

echo "Démarrage Frontend sur http://${FRONTEND_HOST}:${FRONTEND_PORT} ..."
npm run dev --prefix frontend -- --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" &
front_pid=$!

cleanup_done=0
cleanup() {
  [ "$cleanup_done" -eq 0 ] || return 0
  cleanup_done=1
  echo ""
  echo "Arrêt des services..."
  kill "$api_pid" "$worker_pid" "$front_pid" 2>/dev/null || true
  wait "$api_pid" "$worker_pid" "$front_pid" 2>/dev/null || true
}

trap "cleanup; exit 0" INT TERM
trap cleanup EXIT

wait "$api_pid" "$worker_pid" "$front_pid"
