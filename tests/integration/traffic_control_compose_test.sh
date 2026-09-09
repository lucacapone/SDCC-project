#!/usr/bin/env bash
# Suite lenta Linux-only: esegue il confronto completo e verifica gli artefatti runtime.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
command -v docker >/dev/null || { echo 'SKIP: Docker richiesto (Linux-only)' >&2; exit 0; }
TC_DELAY="${TC_DELAY:-500ms}" RUNS="${RUNS:-3}" OBSERVE_SECONDS="${OBSERVE_SECONDS:-30}" \
  "${ROOT}/scripts/experiments/compare_convergence_tc.sh"
LATEST="$(find "${ROOT}/artifacts/traffic_control" -mindepth 1 -maxdepth 1 -type d | sort | tail -1)"
grep -q '^result=PASS$' "${LATEST}/comparison.txt"
while IFS= read -r qdisc_file; do ! grep -q 'qdisc netem' "${qdisc_file}"; done < <(find "${LATEST}/baseline" -name qdisc.txt)
while IFS= read -r qdisc_file; do grep -q 'qdisc netem' "${qdisc_file}"; done < <(find "${LATEST}" -path '*/delayed-*/*/qdisc.txt')
while IFS= read -r log_file; do
  grep -q 'event=gossip_round' "${log_file}"
  grep -q 'event=remote_merge' "${log_file}"
done < <(find "${LATEST}" -name compose.log)
