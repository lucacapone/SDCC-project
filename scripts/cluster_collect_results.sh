#!/usr/bin/env bash
# Raccoglie artefatti leggibili del cluster: stato Compose, log aggregati e ultimo riepilogo
# di shutdown per nodo quando disponibile. Lo script è safe da rilanciare più volte.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

require_docker
ensure_artifacts_dir

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
PS_FILE="${ARTIFACTS_DIR}/compose-ps-${TIMESTAMP}.txt"
LOG_FILE="${ARTIFACTS_DIR}/cluster-logs-${TIMESTAMP}.log"
VALUES_FILE="${ARTIFACTS_DIR}/final-values-${TIMESTAMP}.txt"
LATEST_PS_LINK="${ARTIFACTS_DIR}/latest-compose-ps.txt"
LATEST_LOG_LINK="${ARTIFACTS_DIR}/latest-cluster-logs.log"
LATEST_VALUES_LINK="${ARTIFACTS_DIR}/latest-final-values.txt"

printf '==> raccolta stato servizi in %s\n' "${PS_FILE}"
run_compose ps >"${PS_FILE}" || fail "impossibile acquisire lo stato Compose"
ln -sfn "$(basename "${PS_FILE}")" "${LATEST_PS_LINK}"

printf '==> raccolta log cluster in %s\n' "${LOG_FILE}"
run_compose logs --no-color >"${LOG_FILE}" || fail "impossibile acquisire i log del cluster"
ln -sfn "$(basename "${LOG_FILE}")" "${LATEST_LOG_LINK}"

# Estrae i riepiloghi finali prodotti in shutdown dal binario applicativo, se presenti.
# Se il cluster non è ancora stato fermato, il file contiene comunque un messaggio esplicito.
{
  printf '# final values extracted at %s\n' "${TIMESTAMP}"
  extracted_any=false
  while IFS= read -r line; do
    extracted_any=true
    printf '%s\n' "${line}"
  done < <(grep -E 'shutdown nodo completato' "${LOG_FILE}" || true)

  if [[ "${extracted_any}" == false ]]; then
    printf 'Nessun riepilogo finale disponibile nei log: eseguire prima scripts/cluster_down.sh per ottenere i valori finali di shutdown.\n'
  fi
} >"${VALUES_FILE}"
ln -sfn "$(basename "${VALUES_FILE}")" "${LATEST_VALUES_LINK}"

printf 'Artefatti aggiornati:\n'
printf ' - %s\n' "${LATEST_PS_LINK}"
printf ' - %s\n' "${LATEST_LOG_LINK}"
printf ' - %s\n' "${LATEST_VALUES_LINK}"
