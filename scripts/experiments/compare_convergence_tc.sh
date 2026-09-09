#!/usr/bin/env bash
# Esegue run baseline e NetEm omogenee e confronta i tempi prodotti da convergence-chart.
set -euo pipefail

EXPERIMENT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${EXPERIMENT_DIR}/../.." && pwd)"
COMPOSE_RELATIVE="${SDCC_COMPOSE_FILE:-deploy/docker-compose.tc.yml}"
PROJECT="${SDCC_PROJECT_NAME:-sdcc-tc}"
SERVICES_RAW="${SDCC_SERVICES:-node1 node2 node3 node4 node5 node6}"
TC_DELAY="${TC_DELAY:-500ms}"
RUNS="${RUNS:-3}"
OBSERVE_SECONDS="${OBSERVE_SECONDS:-30}"
TOLERANCE="${TOLERANCE:-0.05}"

# validate_inputs rifiuta tutti gli input prima della prima operazione Docker.
validate_inputs() {
  [[ "${TC_DELAY}" =~ ^([1-9][0-9]*([.][0-9]+)?|0[.][0-9]*[1-9][0-9]*)(ms|us|ns|s)$ ]] || { echo "TC_DELAY non valido: ${TC_DELAY}" >&2; return 1; }
  [[ "${RUNS}" =~ ^[1-9][0-9]*$ ]] || { echo "RUNS non valido: ${RUNS}" >&2; return 1; }
  [[ "${OBSERVE_SECONDS}" =~ ^[1-9][0-9]*$ ]] || { echo "OBSERVE_SECONDS non valido: ${OBSERVE_SECONDS}" >&2; return 1; }
  [[ "${TOLERANCE}" =~ ^(0|[0-9]+)([.][0-9]+)?$ ]] && awk -v v="${TOLERANCE}" 'BEGIN{exit !(v>0)}' || { echo "TOLERANCE non valida: ${TOLERANCE}" >&2; return 1; }
  [[ -n "${SERVICES_RAW//[ ,]/}" ]] || { echo "SDCC_SERVICES vuota" >&2; return 1; }
}

# summary_time riusa esclusivamente il valore calcolato dal report canonico.
summary_time() {
  sed -nE 's/^convergence=osservata da ([0-9]+([.][0-9]+)?) s$/\1/p' "$1"
}

# summary_is_complete verifica valore, topologia completa e convergenza osservata.
summary_is_complete() {
  local file="$1"
  grep -qx 'expected=60' "${file}" && grep -qx 'nodes_expected=6' "${file}" && grep -qx 'nodes_observed=6' "${file}" && \
    grep -qx 'missing_nodes=nessuno' "${file}" && grep -qx 'unexpected_nodes=nessuno' "${file}" && [[ -n "$(summary_time "${file}")" ]]
}

# median ordina numericamente e calcola il centro per cardinalità pari o dispari.
median() {
  printf '%s\n' "$@" | sort -n | awk '{v[NR]=$1} END{if(NR%2) printf "%.6f",v[(NR+1)/2]; else printf "%.6f",(v[NR/2]+v[NR/2+1])/2}'
}

# comparison_pass codifica il solo criterio temporale, dopo le validazioni di completezza.
comparison_pass() {
  awk -v baseline="$1" -v delayed="$2" 'BEGIN{exit !(delayed>baseline)}'
}

# main è separata per consentire test statici delle funzioni senza invocare Docker.
main() {
  validate_inputs
  command -v docker >/dev/null || { echo 'docker non trovato nel PATH' >&2; return 1; }
  docker info >/dev/null
  local timestamp root baseline_root delayed_root comparison mode index run_dir summary time_value logs invalid_membership=false
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)-$$"
  root="${REPO_ROOT}/artifacts/traffic_control/${timestamp}"
  baseline_root="${root}/baseline"
  delayed_root="${root}/delayed-${TC_DELAY}"
  comparison="${root}/comparison.txt"
  mkdir -p "${baseline_root}" "${delayed_root}"
  cleanup() { docker compose -p "${PROJECT}" -f "${REPO_ROOT}/${COMPOSE_RELATIVE}" down --remove-orphans >/dev/null 2>&1 || true; }
  trap cleanup EXIT INT TERM
  export SDCC_COMPOSE_FILE="${COMPOSE_RELATIVE}" SDCC_PROJECT_NAME="${PROJECT}" SDCC_SERVICES="${SERVICES_RAW}"
  docker compose -p "${PROJECT}" -f "${REPO_ROOT}/${COMPOSE_RELATIVE}" build
  baseline_times=()
  delayed_times=()
  for mode in baseline delayed; do
    for ((index=1; index<=RUNS; index++)); do
      cleanup
      if [[ "${mode}" == baseline ]]; then TC_ENABLED=false; run_dir="${baseline_root}/run-${index}"; else TC_ENABLED=true; run_dir="${delayed_root}/run-${index}"; fi
      export TC_ENABLED TC_DELAY OBSERVE_SECONDS TOLERANCE SDCC_RUN_DIR="${run_dir}" SDCC_SKIP_BUILD=true
      "${REPO_ROOT}/scripts/cluster_convergence_report.sh"
      if [[ "${mode}" == baseline ]]; then
        "${EXPERIMENT_DIR}/traffic_control.sh" assert-off
      else
        "${EXPERIMENT_DIR}/traffic_control.sh" assert-on "${TC_DELAY}"
      fi
      "${EXPERIMENT_DIR}/traffic_control.sh" show >"${run_dir}/qdisc.txt"
      summary="${run_dir}/summary.txt"; logs="${run_dir}/compose.log"
      summary_is_complete "${summary}" || { echo "run incompleta: ${run_dir}" >&2; return 1; }
      grep -q 'event=gossip_round' "${logs}" || { echo "round gossip non osservati: ${run_dir}" >&2; return 1; }
      grep -q 'event=remote_merge' "${logs}" || { echo "merge gossip non osservati: ${run_dir}" >&2; return 1; }
      if grep -Eq 'event=membership_transition.*(status=suspect|status=dead)' "${logs}"; then
        invalid_membership=true
        printf 'Run non valida: transizioni suspect/dead osservate; decisione umana richiesta prima di modificare membership_timeout_ms.\n' >"${run_dir}/membership-invalid.txt"
      fi
      time_value="$(summary_time "${summary}")"
      [[ "${mode}" == baseline ]] && baseline_times+=("${time_value}") || delayed_times+=("${time_value}")
      cleanup
    done
  done
  # Ricrea infine la stessa immagine con TC disattivato e prova il ritorno alla modalità normale.
  cleanup
  TC_ENABLED=false
  export TC_ENABLED
  docker compose -p "${PROJECT}" -f "${REPO_ROOT}/${COMPOSE_RELATIVE}" up -d --force-recreate
  "${EXPERIMENT_DIR}/traffic_control.sh" assert-off
  baseline_median="$(median "${baseline_times[@]}")"; delayed_median="$(median "${delayed_times[@]}")"
  increment="$(awk -v a="${baseline_median}" -v b="${delayed_median}" 'BEGIN{printf "%.6f",b-a}')"
  ratio="$(awk -v a="${baseline_median}" -v b="${delayed_median}" 'BEGIN{if(a==0) print "inf"; else printf "%.6f",b/a}')"
  result=FAIL
  if [[ "${invalid_membership}" == false ]] && comparison_pass "${baseline_median}" "${delayed_median}"; then result=PASS; fi
  {
    printf 'baseline_times=%s\n' "${baseline_times[*]}"
    printf 'tc_times=%s\nmedian_baseline=%s\nmedian_tc=%s\nincrement=%s\nratio=%s\nfinal_value=60\nresult=%s\n' "${delayed_times[*]}" "${baseline_median}" "${delayed_median}" "${increment}" "${ratio}" "${result}"
  } | tee "${comparison}"
  [[ "${result}" == PASS ]]
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
