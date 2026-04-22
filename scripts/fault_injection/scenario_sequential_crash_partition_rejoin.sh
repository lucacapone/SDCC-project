#!/usr/bin/env bash
# Esegue uno scenario combinato: crash sequenziale di due nodi, partizione temporanea
# di rete su un nodo residuo, recovery e rejoin dei nodi fermati.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

require_docker
ensure_artifacts_dir
ensure_fault_artifacts_dir

CRASH_NODE_A="${CRASH_NODE_A:-node1}"
CRASH_NODE_B="${CRASH_NODE_B:-node2}"
PARTITION_NODE="${PARTITION_NODE:-node3}"
SEQUENTIAL_GAP_SECONDS="${SEQUENTIAL_GAP_SECONDS:-2}"
PARTITION_SECONDS="${PARTITION_SECONDS:-6}"
STOP_TIMEOUT_SECONDS="${STOP_TIMEOUT_SECONDS:-20}"
START_TIMEOUT_SECONDS="${START_TIMEOUT_SECONDS:-30}"

require_known_service "${CRASH_NODE_A}"
require_known_service "${CRASH_NODE_B}"
require_known_service "${PARTITION_NODE}"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

printf '==> scenario combinato: crash_seq=(%s,%s) partition=%s gap=%ss partition=%ss\n' \
  "${CRASH_NODE_A}" "${CRASH_NODE_B}" "${PARTITION_NODE}" "${SEQUENTIAL_GAP_SECONDS}" "${PARTITION_SECONDS}"

ACTION=stop SERVICE="${CRASH_NODE_A}" STOP_TIMEOUT_SECONDS="${STOP_TIMEOUT_SECONDS}" "${SCRIPT_DIR}/node_stop_start.sh"
sleep "${SEQUENTIAL_GAP_SECONDS}"
ACTION=stop SERVICE="${CRASH_NODE_B}" STOP_TIMEOUT_SECONDS="${STOP_TIMEOUT_SECONDS}" "${SCRIPT_DIR}/node_stop_start.sh"

ACTION=partition SERVICE="${PARTITION_NODE}" PARTITION_SECONDS="${PARTITION_SECONDS}" "${SCRIPT_DIR}/network_partition.sh"

ACTION=start SERVICE="${CRASH_NODE_A}" START_TIMEOUT_SECONDS="${START_TIMEOUT_SECONDS}" "${SCRIPT_DIR}/node_stop_start.sh"
ACTION=start SERVICE="${CRASH_NODE_B}" START_TIMEOUT_SECONDS="${START_TIMEOUT_SECONDS}" "${SCRIPT_DIR}/node_stop_start.sh"

printf '==> scenario combinato completato\n'
run_compose ps
