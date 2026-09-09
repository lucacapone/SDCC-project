#!/usr/bin/env bash
# Raccoglie una run Compose e genera log sorgente, CSV, SVG e riepilogo offline.
set -euo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"
require_docker
ensure_artifacts_dir
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)-$$"
# SDCC_RUN_DIR consente all'harness di assegnare una directory univoca alla singola run.
RUN_DIR="${SDCC_RUN_DIR:-${SDCC_ARTIFACTS_DIR:-${ARTIFACTS_DIR}}/${TIMESTAMP}}"
OBSERVE_SECONDS="${OBSERVE_SECONDS:-20}"
TOLERANCE="${TOLERANCE:-0.05}"
[[ ! -e "${RUN_DIR}" ]] || fail "directory run già esistente: ${RUN_DIR}"
mkdir -p "${RUN_DIR}"
LOG_FILE="${RUN_DIR}/compose.log"
CSV_FILE="${RUN_DIR}/convergence.csv"
SVG_FILE="${RUN_DIR}/convergence.svg"
SUMMARY_FILE="${RUN_DIR}/summary.txt"
CONFIG_LIST=""
for service in "${SERVICES[@]}"; do
  suffix="${service#node}"
  config_path="${REPO_ROOT}/configs/node${suffix}.yaml"
  [[ -f "${config_path}" ]] || fail "configurazione associata a ${service} non trovata: ${config_path}"
  CONFIG_LIST="${CONFIG_LIST}${CONFIG_LIST:+,}${config_path}"
done
printf '==> osservazione di %ss del progetto %s\n' "${OBSERVE_SECONDS}" "${PROJECT_NAME}"
# La ricreazione delimita la run ed evita che log di container precedenti entrino nel dataset.
up_args=(up -d --force-recreate)
if [[ "${SDCC_SKIP_BUILD:-false}" != true ]]; then
  up_args+=(--build)
fi
run_compose "${up_args[@]}"
sleep "${OBSERVE_SECONDS}"
# Il log sorgente resta immutato accanto ai prodotti derivati.
run_compose logs --no-color --timestamps >"${LOG_FILE}"
go run ./cmd/convergence-chart -logs "${LOG_FILE}" -csv "${CSV_FILE}" -svg "${SVG_FILE}" -summary "${SUMMARY_FILE}" -configs "${CONFIG_LIST}" -tolerance "${TOLERANCE}"
printf 'Artefatti convergenza: %s\n' "${RUN_DIR}"
