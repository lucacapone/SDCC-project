#!/usr/bin/env bash
# Suite lenta Linux-only: esegue il confronto completo e verifica gli artefatti runtime.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
command -v docker >/dev/null || { echo 'SKIP: Docker richiesto (Linux-only)' >&2; exit 0; }
RUN_COUNT="${RUNS:-3}"
TC_DELAY="${TC_DELAY:-500ms}" RUNS="${RUN_COUNT}" OBSERVE_SECONDS="${OBSERVE_SECONDS:-30}" \
  "${ROOT}/scripts/experiments/compare_convergence_tc.sh"
LATEST="$(find "${ROOT}/artifacts/traffic_control" -mindepth 1 -maxdepth 1 -type d | sort | tail -1)"
grep -q '^result=PASS$' "${LATEST}/comparison.txt"

# Carica il contratto condiviso senza rieseguire main; il confronto lo ha già applicato a caldo,
# inclusa la verifica qdisc tramite traffic_control.sh per ciascuna modalità.
# shellcheck source=../../scripts/experiments/compare_convergence_tc.sh
source "${ROOT}/scripts/experiments/compare_convergence_tc.sh"
mapfile -t RUN_DIRS < <(find "${LATEST}" -mindepth 2 -maxdepth 2 -type d -name 'run-*' | sort)
[[ "${#RUN_DIRS[@]}" -eq "$((RUN_COUNT * 2))" ]]
for run_dir in "${RUN_DIRS[@]}"; do
  for artifact in summary.txt compose.log convergence.csv convergence.svg qdisc.txt average-eligibility.txt; do test -f "${run_dir}/${artifact}"; done
  summary_is_complete "${run_dir}/summary.txt"
  csv_has_run_evidence "${run_dir}/convergence.csv"
  grep -Eq '^first_complete_sample=[0-9]+ line=[0-9]+$' "${run_dir}/average-eligibility.txt"
  grep -qx 'regressions=0' "${run_dir}/average-eligibility.txt"
  ! grep -Eq 'event=membership_transition.*(status=suspect|status=dead)' "${run_dir}/compose.log"
done

# Dopo il confronto il cluster viene intenzionalmente ricreato senza NetEm: valida end-to-end
# una run baseline attraverso la funzione unica e lo script traffic_control reale.
validate_run_evidence "${LATEST}/baseline/run-1" baseline
