#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
LOG_DIR="${LOG_DIR:-$ROOT_DIR/logs}"
API_LOG="${API_LOG:-$LOG_DIR/shelfy-api.log}"
WORKER_LOG="${WORKER_LOG:-$LOG_DIR/shelfy-worker.log}"
FRONT_LOG="${FRONT_LOG:-$LOG_DIR/shelfy-front.log}"
API_READY_TIMEOUT_SECONDS="${API_READY_TIMEOUT_SECONDS:-180}"

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
    val="${val%$'\r'}"
    val="${val#"${val%%[![:space:]]*}"}"
    val="${val%"${val##*[![:space:]]}"}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ "$val" =~ ^\".*\"$ ]]; then
      val="${val:1:${#val}-2}"
    elif [[ "$val" =~ ^\'.*\'$ ]]; then
      val="${val:1:${#val}-2}"
    else
      val="${val%%#*}"
      val="${val#"${val%%[![:space:]]*}"}"
      val="${val%"${val##*[![:space:]]}"}"
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

ensure_sqlite_writable() {
  local raw_dsn="$1"
  if [ -z "$raw_dsn" ]; then
    raw_dsn="shelfy.db"
  fi
  if [[ "$raw_dsn" == file:* ]]; then
    raw_dsn="${raw_dsn#file:}"
    raw_dsn="${raw_dsn%%\?*}"
  fi
  if [[ "$raw_dsn" == *"mode=ro"* ]]; then
    echo "DB_DSN est en mode lecture seule (mode=ro), impossible d'écrire."
    exit 1
  fi
  local db_path="$raw_dsn"
  if [[ "$db_path" != /* ]]; then
    db_path="$ROOT_DIR/$db_path"
  fi
  local db_dir
  db_dir="$(dirname "$db_path")"
  mkdir -p "$db_dir"
  touch "$db_path" 2>/dev/null || true
  if [ ! -w "$db_dir" ] || [ ! -w "$db_path" ]; then
    echo "SQLite non inscriptible: $db_path"
    echo "Utilisateur courant: $(id -un)"
    echo "Corrige avec:"
    echo "  sudo chown -R $(id -un):$(id -gn) \"$ROOT_DIR\""
    echo "  sudo chmod -R u+rwX \"$ROOT_DIR\""
    exit 1
  fi
  export DB_DSN="$db_path"
  echo "DB SQLite: $DB_DSN"
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

is_port_listening() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn 2>/dev/null | awk -v p=":${port}" '$4 ~ p"$" {found=1} END {exit(found?0:1)}'
    return $?
  fi
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  return 1
}

wait_for_api() {
  local api_port="$1"
  local primary_health_url="$2"
  local retries="$3"
  local sleep_s="${4:-0.5}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if ! is_running "$api_pid"; then
      echo "API arrêtée prématurément. Voir logs: $API_LOG"
      tail -n 80 "$API_LOG" || true
      return 1
    fi
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS "$primary_health_url" >/dev/null 2>&1; then
        return 0
      fi
    fi
    if is_port_listening "$api_port"; then
      return 0
    fi
    sleep "$sleep_s"
  done
  echo "API non prête sur $primary_health_url après attente."
  if is_port_listening "$api_port"; then
    echo "Le port ${api_port} écoute, mais /api/health n'a pas répondu."
  fi
  tail -n 80 "$API_LOG" || true
  return 1
}

kill_tree() {
  local pid="$1"
  kill "$pid" 2>/dev/null || true
  pkill -TERM -P "$pid" 2>/dev/null || true
  sleep 1
  kill -9 "$pid" 2>/dev/null || true
  pkill -KILL -P "$pid" 2>/dev/null || true
}

load_dotenv

echo "Config chargée: AUTH_ENABLED=${AUTH_ENABLED:-<vide>} HTTP_ADDR=${HTTP_ADDR:-:8080}"

require_cmd go
require_cmd npm

need_frontend_install=0
if [ ! -d frontend/node_modules ]; then
  need_frontend_install=1
fi
if [ ! -d frontend/node_modules/@xterm/xterm ] || [ ! -d frontend/node_modules/@xterm/addon-fit ]; then
  need_frontend_install=1
fi
if [ "$need_frontend_install" -eq 1 ]; then
  echo "Installation des dépendances frontend..."
  npm install --prefix frontend
fi

db_driver="${DB_DRIVER:-sqlite}"
if [ "$db_driver" = "sqlite" ]; then
  require_cmd gcc
  ensure_sqlite_writable "${DB_DSN:-shelfy.db}"
fi

api_port="${HTTP_ADDR:-:8080}"
api_host="127.0.0.1"
if [[ "$api_port" == \[*\]:* ]]; then
  api_host="${api_port%%]*}"
  api_host="${api_host#[}"
  api_port="${api_port##*:}"
elif [[ "$api_port" == *:* && "$api_port" != :* ]]; then
  api_host="${api_port%:*}"
  api_port="${api_port##*:}"
elif [[ "$api_port" == :* ]]; then
  api_port="${api_port##*:}"
fi
if [ -z "$api_host" ] || [ "$api_host" = "0.0.0.0" ] || [ "$api_host" = "::" ] || [ "$api_host" = "[::]" ]; then
  api_host="127.0.0.1"
fi
health_url="http://${api_host}:${api_port}/api/health"
export VITE_PROXY_TARGET="http://${api_host}:${api_port}"
echo "Proxy frontend API: ${VITE_PROXY_TARGET}"
front_port="${FRONTEND_PORT}"
api_retries=$((API_READY_TIMEOUT_SECONDS * 2))

check_port_free "$api_port" "api" || exit 1
check_port_free "$front_port" "frontend" || exit 1

mkdir -p "$LOG_DIR"
: >"$API_LOG"
: >"$WORKER_LOG"
: >"$FRONT_LOG"

echo "Logs API: $API_LOG"
echo "Logs Worker: $WORKER_LOG"
echo "Logs Front: $FRONT_LOG"

echo "Préchargement des dépendances Go..."
go mod download >>"$API_LOG" 2>&1

echo "Démarrage API..."
go run ./cmd/api >>"$API_LOG" 2>&1 &
api_pid=$!

echo "Attente API sur $health_url ..."
wait_for_api "$api_port" "$health_url" "$api_retries"

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
  kill_tree "$front_pid"
  kill_tree "$worker_pid"
  kill_tree "$api_pid"
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
