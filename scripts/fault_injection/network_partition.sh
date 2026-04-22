#!/usr/bin/env bash
# Simula una partizione di rete temporanea disconnettendo/riconnettendo un servizio
# dalla rete Docker Compose canonica. Lo script è riusabile sia in manuale sia nei test.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

require_docker
ensure_artifacts_dir
ensure_fault_artifacts_dir

ACTION="${ACTION:-${1:-partition}}"
SERVICE="${SERVICE:-${2:-${DEFAULT_SERVICE}}}"
PARTITION_SECONDS="${PARTITION_SECONDS:-8}"
RECONNECT_WAIT_SECONDS="${RECONNECT_WAIT_SECONDS:-30}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-1}"

# resolve_compose_network_name produce il nome rete runtime basato sul project Compose.
resolve_compose_network_name() {
  printf '%s_default' "${PROJECT_NAME}"
}

# require_container_id assicura che il servizio abbia un container risolto prima di operare.
require_container_id() {
  local service="$1"
  local container_id

  container_id="$(container_id_for "${service}")"
  [[ -n "${container_id}" ]] || fail "container assente per servizio ${service}"
  printf '%s' "${container_id}"
}

# wait_network_attachment attende lo stato di attach/detach del container rispetto alla rete target.
wait_network_attachment() {
  local container_id="$1"
  local network_name="$2"
  local expected="$3"
  local timeout_seconds="$4"
  local deadline_epoch="$(( $(date +%s) + timeout_seconds ))"
  local current

  while (( $(date +%s) <= deadline_epoch )); do
    current="$(docker inspect -f '{{if index .NetworkSettings.Networks "'"${network_name}"'"}}attached{{else}}detached{{end}}' "${container_id}" 2>/dev/null || true)"
    if [[ "${current}" == "${expected}" ]]; then
      return 0
    fi
    sleep "${POLL_INTERVAL_SECONDS}"
  done

  fail "timeout in attesa stato rete ${expected} per container ${container_id} su rete ${network_name}"
}

require_known_service "${SERVICE}"
CONTAINER_ID="$(require_container_id "${SERVICE}")"
NETWORK_NAME="$(resolve_compose_network_name)"

case "${ACTION}" in
  disconnect)
    printf '==> disconnessione rete temporanea: service=%s container=%s network=%s\n' "${SERVICE}" "${CONTAINER_ID}" "${NETWORK_NAME}"
    docker network disconnect "${NETWORK_NAME}" "${CONTAINER_ID}" >/dev/null
    wait_network_attachment "${CONTAINER_ID}" "${NETWORK_NAME}" detached "${RECONNECT_WAIT_SECONDS}"
    ;;
  connect)
    printf '==> riconnessione rete: service=%s container=%s network=%s\n' "${SERVICE}" "${CONTAINER_ID}" "${NETWORK_NAME}"
    docker network connect "${NETWORK_NAME}" "${CONTAINER_ID}" >/dev/null
    wait_network_attachment "${CONTAINER_ID}" "${NETWORK_NAME}" attached "${RECONNECT_WAIT_SECONDS}"
    ;;
  partition)
    printf '==> partizione temporanea: service=%s durata=%ss\n' "${SERVICE}" "${PARTITION_SECONDS}"
    docker network disconnect "${NETWORK_NAME}" "${CONTAINER_ID}" >/dev/null
    wait_network_attachment "${CONTAINER_ID}" "${NETWORK_NAME}" detached "${RECONNECT_WAIT_SECONDS}"
    sleep "${PARTITION_SECONDS}"
    docker network connect "${NETWORK_NAME}" "${CONTAINER_ID}" >/dev/null
    wait_network_attachment "${CONTAINER_ID}" "${NETWORK_NAME}" attached "${RECONNECT_WAIT_SECONDS}"
    ;;
  *)
    fail "azione non supportata: ${ACTION}. Usare disconnect|connect|partition"
    ;;
esac

printf '==> stato Compose aggiornato per %s\n' "${SERVICE}"
run_compose ps "${SERVICE}"
