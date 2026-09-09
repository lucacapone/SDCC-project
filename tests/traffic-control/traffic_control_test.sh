#!/usr/bin/env bash
# Suite statica/unitaria Linux per entrypoint e funzioni pure dell'harness NetEm.
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# assert_failure richiede che il comando di validazione rifiuti l'input.
assert_failure() { if "$@" >/dev/null 2>&1; then echo "atteso fallimento: $*" >&2; exit 1; fi; }
# assert_equal confronta output scalari con diagnostica leggibile.
assert_equal() { [[ "$1" == "$2" ]] || { echo "atteso '$2', ottenuto '$1'" >&2; exit 1; }; }

# Carica le funzioni pure senza eseguire main.
# shellcheck source=../../scripts/experiments/compare_convergence_tc.sh
source "${ROOT}/scripts/experiments/compare_convergence_tc.sh"

TC_DELAY=500ms RUNS=3 OBSERVE_SECONDS=30 TOLERANCE=0.05 SERVICES_RAW='node1 node2 node3 node4 node5 node6' validate_inputs
TC_DELAY=invalid; assert_failure validate_inputs
TC_DELAY=500ms RUNS=0; assert_failure validate_inputs
RUNS=3 OBSERVE_SECONDS=0; assert_failure validate_inputs
OBSERVE_SECONDS=30 TOLERANCE=0; assert_failure validate_inputs
assert_equal "$(median 1 9 5)" '5.000000'
assert_equal "$(median 1 3 5 9)" '4.000000'
comparison_pass 1 2
assert_failure comparison_pass 2 2

cat >"${TMP}/complete.txt" <<'EOF'
expected=60
nodes_expected=6
nodes_observed=6
missing_nodes=nessuno
unexpected_nodes=nessuno
convergence=osservata da 4.250000 s
EOF
assert_equal "$(summary_time "${TMP}/complete.txt")" '4.250000'
summary_is_complete "${TMP}/complete.txt"
sed 's/nodes_observed=6/nodes_observed=5/; s/missing_nodes=nessuno/missing_nodes=node-6/' "${TMP}/complete.txt" >"${TMP}/incomplete.txt"
assert_failure summary_is_complete "${TMP}/incomplete.txt"

# create_complete_run costruisce una fixture minima conforme al contratto unico delle evidenze.
create_complete_run() {
  local run_dir="$1" node
  mkdir -p "${run_dir}"
  cp "${TMP}/complete.txt" "${run_dir}/summary.txt"
  : >"${run_dir}/compose.log"
  : >"${run_dir}/convergence.svg"
  : >"${run_dir}/qdisc.txt"
  printf '%s\n' 'timestamp,elapsed_seconds,node_id,round,aggregation,estimate,event_type' >"${run_dir}/convergence.csv"
  for node in node-1 node-2 node-3 node-4 node-5 node-6; do
    printf '2026-01-01T00:00:01Z,1,%s,1,average,60,local_round\n' "${node}" >>"${run_dir}/convergence.csv"
    printf '2026-01-01T00:00:02Z,2,%s,1,average,60,remote_merge\n' "${node}" >>"${run_dir}/convergence.csv"
  done
}

# Il fake conserva la verifica off/on demandata allo script traffic_control senza richiedere Docker.
cat >"${TMP}/traffic-control" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"${TC_CALLS}"
test "${TC_FORCE_FAILURE:-false}" != true
EOF
chmod +x "${TMP}/traffic-control"
export TRAFFIC_CONTROL_SCRIPT="${TMP}/traffic-control" TC_CALLS="${TMP}/tc-calls"

create_complete_run "${TMP}/valid"
validate_run_evidence "${TMP}/valid" baseline
tail -1 "${TC_CALLS}" | grep -qx 'assert-off'
# L'assenza del marker diagnostico gossip_round non rende incompleta la fixture valida.
validate_run_evidence "${TMP}/valid" delayed
tail -1 "${TC_CALLS}" | grep -qx 'assert-on 500ms'

# Ogni artefatto obbligatorio mancante deve invalidare la run.
for missing in summary.txt compose.log convergence.csv convergence.svg qdisc.txt; do
  cp -R "${TMP}/valid" "${TMP}/missing-${missing}"
  rm "${TMP}/missing-${missing}/${missing}"
  assert_failure validate_run_evidence "${TMP}/missing-${missing}" baseline
done

cp -R "${TMP}/valid" "${TMP}/bad-summary"
sed -i 's/nodes_observed=6/nodes_observed=5/' "${TMP}/bad-summary/summary.txt"
assert_failure validate_run_evidence "${TMP}/bad-summary" baseline

cp -R "${TMP}/valid" "${TMP}/missing-local-round"
sed -i '/node-6,1,average,60,local_round/d' "${TMP}/missing-local-round/convergence.csv"
assert_failure validate_run_evidence "${TMP}/missing-local-round" baseline

cp -R "${TMP}/valid" "${TMP}/zero-local-round"
sed -i 's/node-6,1,average,60,local_round/node-6,0,average,60,local_round/' "${TMP}/zero-local-round/convergence.csv"
assert_failure validate_run_evidence "${TMP}/zero-local-round" baseline

cp -R "${TMP}/valid" "${TMP}/missing-remote-merge"
sed -i '/node-6,1,average,60,remote_merge/d' "${TMP}/missing-remote-merge/convergence.csv"
assert_failure validate_run_evidence "${TMP}/missing-remote-merge" delayed

cp -R "${TMP}/valid" "${TMP}/membership-transition"
printf '%s\n' 'event=membership_transition node_id=node-2 status=suspect' >"${TMP}/membership-transition/compose.log"
assert_failure validate_run_evidence "${TMP}/membership-transition" baseline
assert_failure validate_run_evidence "${TMP}/valid" invalid
(export TC_FORCE_FAILURE=true; assert_failure validate_run_evidence "${TMP}/valid" baseline)

# I fake simulano route, qdisc apply/clear e privilege drop senza NET_ADMIN reale.
cat >"${TMP}/ip" <<'EOF'
#!/bin/sh
echo 'default via 172.18.0.1 dev ens-test'
EOF
cat >"${TMP}/tc" <<EOF
#!/bin/sh
state='${TMP}/state'
case "\$1 \$2" in
  'qdisc replace') echo 'qdisc netem 8001: root refcnt 2 limit 1000 delay 500ms' >"\$state" ;;
  'qdisc del') rm -f "\$state" ;;
  'qdisc show') test ! -f "\$state" || cat "\$state" ;;
esac
EOF
cat >"${TMP}/drop" <<'EOF'
#!/bin/sh
shift
exec "$@"
EOF
cat >"${TMP}/node" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "${TMP}/ip" "${TMP}/tc" "${TMP}/drop" "${TMP}/node"
IP_BIN="${TMP}/ip" TC_BIN="${TMP}/tc" PRIV_DROP_BIN="${TMP}/drop" NODE_BIN="${TMP}/node" TC_ENABLED=true TC_DELAY=500ms HOSTNAME=test \
  "${ROOT}/deploy/traffic-control/entrypoint.sh" | grep -qx 'event=traffic_control state=enabled delay=500ms container=test interface=ens-test result=applied'
test -f "${TMP}/state"
IP_BIN="${TMP}/ip" TC_BIN="${TMP}/tc" PRIV_DROP_BIN="${TMP}/drop" NODE_BIN="${TMP}/node" TC_ENABLED=false HOSTNAME=test \
  "${ROOT}/deploy/traffic-control/entrypoint.sh" | grep -qx 'event=traffic_control state=disabled delay=none container=test interface=ens-test result=cleared'
test ! -f "${TMP}/state"
# Un secondo clear prova l'idempotenza logica quando la qdisc è già assente.
IP_BIN="${TMP}/ip" TC_BIN="${TMP}/tc" PRIV_DROP_BIN="${TMP}/drop" NODE_BIN="${TMP}/node" TC_ENABLED=false HOSTNAME=test \
  "${ROOT}/deploy/traffic-control/entrypoint.sh" >/dev/null
assert_failure env IP_BIN="${TMP}/ip" TC_BIN="${TMP}/tc" PRIV_DROP_BIN="${TMP}/drop" NODE_BIN="${TMP}/node" TC_ENABLED=maybe "${ROOT}/deploy/traffic-control/entrypoint.sh"

echo 'traffic-control unit tests: PASS'
