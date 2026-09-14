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
