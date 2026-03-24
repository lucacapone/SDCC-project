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
  done < <(
    grep 'shutdown nodo completato' "${LOG_FILE}" \
      | python3 -c '
import datetime
import re
import sys

node_pattern = re.compile(r"\\bnode_id=([^ ]+)")
time_pattern = re.compile(r"\\btime=([^ ]+)")
best_by_node = {}

for index, raw_line in enumerate(sys.stdin):
    line = raw_line.rstrip("\n")
    node_match = node_pattern.search(line)
    time_match = time_pattern.search(line)
    if node_match is None or time_match is None:
        continue

    node_id = node_match.group(1)
    timestamp_raw = time_match.group(1)
    normalized_timestamp = timestamp_raw.replace("Z", "+00:00")
    try:
        timestamp = datetime.datetime.fromisoformat(normalized_timestamp)
    except ValueError:
        continue

    current = best_by_node.get(node_id)
    if current is None or timestamp > current[0] or (timestamp == current[0] and index > current[1]):
        best_by_node[node_id] = (timestamp, index, line)

for node_id in sorted(best_by_node):
    print(best_by_node[node_id][2])
' || true
  )

  if [[ "${extracted_any}" == false ]]; then
    printf 'Nessun riepilogo finale disponibile nei log: eseguire prima scripts/cluster_down.sh per ottenere i valori finali di shutdown.\n'
  fi
} >"${VALUES_FILE}"
ln -sfn "$(basename "${VALUES_FILE}")" "${LATEST_VALUES_LINK}"

printf 'Artefatti aggiornati:\n'
printf ' - %s\n' "${LATEST_PS_LINK}"
printf ' - %s\n' "${LATEST_LOG_LINK}"
printf ' - %s\n' "${LATEST_VALUES_LINK}"
