#!/usr/bin/env bash
# Helper condivisi per fault injection manuale sul cluster Docker Compose canonico.
# Questo file riusa `scripts/cluster_common.sh` per evitare naming duplicato,
# mantenere allineamento al Compose di root e offrire primitive semplici riutilizzabili.

set -euo pipefail

# Carica i helper cluster canonici della repository.
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/cluster_common.sh"

# Directory dedicata agli artefatti delle prove di crash/restart manuali.
FAULT_ARTIFACTS_DIR="${REPO_ROOT}/artifacts/fault_injection"
DEFAULT_SERVICE="node1"

# Crea la directory artefatti specifica del fault injection.
ensure_fault_artifacts_dir() {
  mkdir -p "${FAULT_ARTIFACTS_DIR}"
}

# Valida il nome del servizio richiesto rispetto ai servizi canonici del Compose.
require_known_service() {
  local service="${1:-}"
  local known

  [[ -n "${service}" ]] || fail "specificare un servizio Compose valido (es. node1)"

  for known in "${SERVICES[@]}"; do
    if [[ "${service}" == "${known}" ]]; then
      return 0
    fi
  done

  fail "servizio sconosciuto: ${service}. Valori supportati: ${SERVICES[*]}"
}

# Restituisce un timestamp UTC stabile per naming di snapshot e bundle diagnostici.
fi_timestamp() {
  date -u +%Y%m%dT%H%M%SZ
}

# Attende che un servizio entri nello stato desiderato (`running` oppure `exited`).
wait_for_service_state() {
  local service="$1"
  local expected_state="$2"
  local timeout_seconds="${3:-30}"
  local poll_interval_seconds="${4:-1}"
  local deadline_epoch="$(( $(date +%s) + timeout_seconds ))"
  local container_id current_state

  require_known_service "${service}"

  while (( $(date +%s) <= deadline_epoch )); do
    container_id="$(container_id_for "${service}")"
    if [[ -n "${container_id}" ]]; then
      current_state="$(docker inspect -f '{{.State.Status}}' "${container_id}" 2>/dev/null || true)"
      if [[ "${current_state}" == "${expected_state}" ]]; then
        return 0
      fi
    fi
    sleep "${poll_interval_seconds}"
  done

  fail "timeout in attesa di ${service} nello stato ${expected_state}"
}
