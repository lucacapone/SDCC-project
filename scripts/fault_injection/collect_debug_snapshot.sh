#!/usr/bin/env bash
# Raccoglie uno snapshot diagnostico minimale per scenari crash/restart manuali.
# Produce bundle leggibili con ps, log, inspect e metadata del servizio target,
# allineati al Compose canonico e riutilizzabili senza orchestrazione centralizzata.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

require_docker
ensure_artifacts_dir
ensure_fault_artifacts_dir

SERVICE="${SERVICE:-${1:-${DEFAULT_SERVICE}}}"
LOG_TAIL_LINES="${LOG_TAIL_LINES:-300}"
SNAPSHOT_LABEL="${SNAPSHOT_LABEL:-manual}"
TIMESTAMP="$(fi_timestamp)"
SNAPSHOT_DIR="${FAULT_ARTIFACTS_DIR}/${TIMESTAMP}-${SERVICE}-${SNAPSHOT_LABEL}"
LATEST_LINK="${FAULT_ARTIFACTS_DIR}/latest-${SERVICE}"
CONTAINER_ID=""

require_known_service "${SERVICE}"
mkdir -p "${SNAPSHOT_DIR}"

printf '==> raccolta snapshot fault injection per %s in %s\n' "${SERVICE}" "${SNAPSHOT_DIR}"
run_compose ps >"${SNAPSHOT_DIR}/compose-ps.txt" || fail "impossibile salvare docker compose ps"
run_compose logs --no-color --tail="${LOG_TAIL_LINES}" >"${SNAPSHOT_DIR}/cluster-logs-tail.txt" || fail "impossibile salvare i log cluster"
run_compose logs --no-color --tail="${LOG_TAIL_LINES}" "${SERVICE}" >"${SNAPSHOT_DIR}/${SERVICE}-logs-tail.txt" || fail "impossibile salvare i log del servizio ${SERVICE}"

CONTAINER_ID="$(container_id_for "${SERVICE}")"
if [[ -n "${CONTAINER_ID}" ]]; then
  docker inspect "${CONTAINER_ID}" >"${SNAPSHOT_DIR}/${SERVICE}-inspect.json" || fail "impossibile salvare docker inspect per ${SERVICE}"
  docker logs --tail "${LOG_TAIL_LINES}" "${CONTAINER_ID}" >"${SNAPSHOT_DIR}/${SERVICE}-docker-logs.txt" 2>&1 || fail "impossibile salvare docker logs per ${SERVICE}"
else
  printf 'Container non presente per %s al momento dello snapshot.\n' "${SERVICE}" >"${SNAPSHOT_DIR}/${SERVICE}-inspect.txt"
fi

{
  printf 'timestamp_utc=%s\n' "${TIMESTAMP}"
  printf 'service=%s\n' "${SERVICE}"
  printf 'snapshot_label=%s\n' "${SNAPSHOT_LABEL}"
  printf 'log_tail_lines=%s\n' "${LOG_TAIL_LINES}"
  printf 'compose_file=%s\n' "${COMPOSE_FILE}"
  printf 'project_name=%s\n' "${PROJECT_NAME}"
  printf 'container_id=%s\n' "${CONTAINER_ID:-absent}"
} >"${SNAPSHOT_DIR}/metadata.env"

ln -sfn "$(basename "${SNAPSHOT_DIR}")" "${LATEST_LINK}"
printf 'Snapshot pronto: %s\n' "${SNAPSHOT_DIR}"
printf 'Link rapido aggiornato: %s\n' "${LATEST_LINK}"
