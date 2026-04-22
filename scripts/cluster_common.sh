#!/usr/bin/env bash
# Helper condivisi per l'orchestrazione del cluster locale SDCC via Docker Compose.
# Il file centralizza naming, percorsi e funzioni di utilità per mantenere gli script
# idempotenti, con errori leggibili e cleanup esplicito anche in ambiente sporco.

set -euo pipefail

# Calcola la root della repository in modo robusto partendo dalla directory degli script.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE_INPUT="${SDCC_COMPOSE_FILE:-docker-compose.yml}"
if [[ "${COMPOSE_FILE_INPUT}" = /* ]]; then
  COMPOSE_FILE="${COMPOSE_FILE_INPUT}"
else
  COMPOSE_FILE="${REPO_ROOT}/${COMPOSE_FILE_INPUT}"
fi
ARTIFACTS_DIR="${REPO_ROOT}/artifacts/cluster"
PROJECT_NAME="${SDCC_PROJECT_NAME:-sdcc-bootstrap}"
SERVICES_CONFIG_FILE_DEFAULT="${REPO_ROOT}/deploy/compose_services.env"

# trim_spaces rimuove spazi iniziali/finali da una stringa senza usare tool esterni.
trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

# parse_services_list converte una lista separata da virgole/spazi in un array di servizi.
parse_services_list() {
  local raw="$1"
  local -n parsed_ref="$2"
  local normalized token

  normalized="${raw//,/ }"
  parsed_ref=()
  for token in ${normalized}; do
    token="$(trim_spaces "${token}")"
    [[ -z "${token}" ]] && continue
    parsed_ref+=("${token}")
  done
}

# load_services_from_file carica SDCC_SERVICES da file env esterno (se presente).
load_services_from_file() {
  local config_file="${SDCC_SERVICES_FILE:-${SERVICES_CONFIG_FILE_DEFAULT}}"
  local file_value

  if [[ ! -f "${config_file}" ]]; then
    return 1
  fi

  # shellcheck disable=SC1090
  source "${config_file}"
  file_value="${SDCC_SERVICES:-}"
  if [[ -z "$(trim_spaces "${file_value}")" ]]; then
    return 1
  fi

  parse_services_list "${file_value}" SERVICES
  return 0
}

# load_services imposta SERVICES con precedenza env -> file -> default canonico.
load_services() {
  local env_value="${SDCC_SERVICES:-}"

  if [[ -n "$(trim_spaces "${env_value}")" ]]; then
    parse_services_list "${env_value}" SERVICES
  elif ! load_services_from_file; then
    SERVICES=(node1 node2 node3)
  fi

  if [[ "${#SERVICES[@]}" -eq 0 ]]; then
    fail "lista servizi vuota: impostare SDCC_SERVICES o il file servizi esterno"
  fi
}

load_services

# Crea la directory artefatti usata per log, report e snapshot intermedi.
ensure_artifacts_dir() {
  mkdir -p "${ARTIFACTS_DIR}"
}

# Esegue il comando Compose canonico con gli argomenti aggiuntivi richiesti.
# L'implementazione evita `mapfile` per mantenere compatibilità con bash 3.2+.
run_compose() {
  docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" "$@"
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
