#!/usr/bin/env bash
# Helper condivisi per l'orchestrazione del cluster locale SDCC via Docker Compose.
# Il file centralizza naming, percorsi e funzioni di utilità per mantenere gli script
# idempotenti, con errori leggibili e cleanup esplicito anche in ambiente sporco.

set -euo pipefail

# Calcola la root della repository in modo robusto partendo dalla directory degli script.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/docker-compose.yml"
ARTIFACTS_DIR="${REPO_ROOT}/artifacts/cluster"
PROJECT_NAME="sdcc-bootstrap"
SERVICES=(node1 node2 node3)

# Crea la directory artefatti usata per log, report e snapshot intermedi.
ensure_artifacts_dir() {
  mkdir -p "${ARTIFACTS_DIR}"
}

# Restituisce il comando Compose canonico come array shell-safe.
compose_cmd() {
  printf '%s\0' docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}"
}

# Esegue il comando Compose ricevuto come argomenti aggiuntivi.
run_compose() {
  local -a base_cmd
  mapfile -d '' base_cmd < <(compose_cmd)
  "${base_cmd[@]}" "$@"
}

# Stampa un errore uniforme e interrompe lo script chiamante.
fail() {
  printf 'ERRORE: %s\n' "$*" >&2
  exit 1
}

# Verifica che Docker sia disponibile prima di tentare l'orchestrazione del cluster.
require_docker() {
  command -v docker >/dev/null 2>&1 || fail "docker non trovato nel PATH"
  docker info >/dev/null 2>&1 || fail "docker non raggiungibile; avviare il daemon Docker e riprovare"
}

# Restituisce l'id container del servizio richiesto, oppure stringa vuota se assente.
container_id_for() {
  local service="$1"
  run_compose ps -q "${service}" 2>/dev/null | tr -d '[:space:]'
}

# Verifica se il servizio è attualmente in esecuzione.
service_is_running() {
  local service="$1"
  local container_id
  container_id="$(container_id_for "${service}")"
  if [[ -z "${container_id}" ]]; then
    return 1
  fi
  [[ "$(docker inspect -f '{{.State.Status}}' "${container_id}" 2>/dev/null || true)" == "running" ]]
}
