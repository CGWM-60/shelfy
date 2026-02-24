#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
API_LOG="${API_LOG:-/tmp/shelfy-api.log}"
WORKER_LOG="${WORKER_LOG:-/tmp/shelfy-worker.log}"
FRONT_LOG="${FRONT_LOG:-/tmp/shelfy-front.log}"

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

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Commande manquante: $1"
    exit 1
  fi
}

check_port_free() {
  local port="$1"
  local label="$2"
  local in_use=""
  if command -v ss >/dev/null 2>&1; then
    in_use="$(ss -ltnp 2>/dev/null | awk -v p=":${port}" '$4 ~ p"$" {print $0}' || true)"
  elif command -v lsof >/dev/null 2>&1; then
    in_use="$(lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  fi
  if [ -n "$in_use" ]; then
    echo "Port ${port} déjà utilisé (${label})."
    echo "$in_use"
    return 1
  fi
  return 0
}

is_running() {
  kill -0 "$1" 2>/dev/null
}

wait_for_api() {
  local health_url="$1"
  local retries="${2:-60}"
  local sleep_s="${3:-0.5}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if ! is_running "$api_pid"; then
      echo "API arrêtée prématurément. Voir logs: $API_LOG"
      tail -n 80 "$API_LOG" || true
      return 1
    fi
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS "$health_url" >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep "$sleep_s"
  done
  echo "API non prête sur $health_url après attente."
  tail -n 80 "$API_LOG" || true
  return 1
}

load_dotenv

require_cmd go
require_cmd npm

if [ ! -d frontend/node_modules ]; then
  echo "Installation des dépendances frontend..."
  npm install --prefix frontend
fi

db_driver="${DB_DRIVER:-sqlite}"
if [ "$db_driver" = "sqlite" ]; then
  require_cmd gcc
fi

api_port="${HTTP_ADDR:-:8080}"
api_port="${api_port##*:}"
health_url="http://127.0.0.1:${api_port}/api/health"
front_port="${FRONTEND_PORT}"

check_port_free "$api_port" "api" || exit 1
check_port_free "$front_port" "frontend" || exit 1

echo "Logs API: $API_LOG"
echo "Logs Worker: $WORKER_LOG"
echo "Logs Front: $FRONT_LOG"

echo "Démarrage API..."
go run ./cmd/api >>"$API_LOG" 2>&1 &
api_pid=$!

echo "Attente API sur $health_url ..."
wait_for_api "$health_url"

echo "Démarrage Worker..."
go run ./cmd/worker >>"$WORKER_LOG" 2>&1 &
worker_pid=$!

echo "Démarrage Frontend sur http://${FRONTEND_HOST}:${FRONTEND_PORT} ..."
npm run dev --prefix frontend -- --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" >>"$FRONT_LOG" 2>&1 &
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

set +e
wait -n "$api_pid" "$worker_pid" "$front_pid"
exit_code=$?
set -e

if ! is_running "$api_pid"; then
  echo "API arrêtée. Voir $API_LOG"
  tail -n 80 "$API_LOG" || true
fi
if ! is_running "$worker_pid"; then
  echo "Worker arrêté. Voir $WORKER_LOG"
  tail -n 80 "$WORKER_LOG" || true
fi
if ! is_running "$front_pid"; then
  echo "Frontend arrêté. Voir $FRONT_LOG"
  tail -n 80 "$FRONT_LOG" || true
fi

exit "$exit_code"
