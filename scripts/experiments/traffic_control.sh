#!/usr/bin/env bash
# Ispeziona e verifica NetEm sui servizi definiti dall'harness cluster comune.
set -euo pipefail

EXPERIMENT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../cluster_common.sh
source "${EXPERIMENT_DIR}/../cluster_common.sh"

# tc_for_service esegue tc nel container e conserva l'output per le asserzioni.
tc_for_service() {
  local service="$1"
  local container_id interface
  container_id="$(container_id_for "${service}")"
  [[ -n "${container_id}" ]] || fail "container assente per ${service}"
  interface="$(docker exec "${container_id}" sh -c "ip route show default | awk '{for(i=1;i<=NF;i++) if(\$i==\"dev\") print \$(i+1)}' | sort -u")"
  [[ "$(printf '%s\n' "${interface}" | awk 'NF{n++} END{print n+0}')" -eq 1 ]] || fail "interfaccia non univoca per ${service}"
  TC_SERVICE_CONTAINER="${container_id}"
  TC_SERVICE_INTERFACE="${interface}"
  TC_SERVICE_OUTPUT="$(docker exec "${container_id}" tc qdisc show dev "${interface}")"
}

# show stampa una sezione stabile per ogni servizio esterno configurato.
show_all() {
  local service
  for service in "${SERVICES[@]}"; do
    tc_for_service "${service}"
    printf 'service=%s container=%s interface=%s\n%s\n' "${service}" "${TC_SERVICE_CONTAINER}" "${TC_SERVICE_INTERFACE}" "${TC_SERVICE_OUTPUT}"
  done
}

# assert_on richiede NetEm e il delay normalizzato mostrato da tc su ogni servizio.
assert_on() {
  local expected="$1" service expected_tc
  [[ -n "${expected}" ]] || fail "specificare il delay atteso"
  expected_tc="${expected%ms}"
  for service in "${SERVICES[@]}"; do
    tc_for_service "${service}"
    grep -Eq "qdisc netem .* root .*delay ${expected_tc}(ms)?([[:space:]]|$)" <<<"${TC_SERVICE_OUTPUT}" || fail "NetEm ${expected} non verificato su ${service}"
  done
  printf 'NetEm delay=%s verificato su %d servizi.\n' "${expected}" "${#SERVICES[@]}"
}

# assert_off garantisce che nessun servizio esponga una root qdisc NetEm.
assert_off() {
  local service
  for service in "${SERVICES[@]}"; do
    tc_for_service "${service}"
    ! grep -Eq 'qdisc netem .* root' <<<"${TC_SERVICE_OUTPUT}" || fail "NetEm ancora presente su ${service}"
  done
  printf 'NetEm assente su %d servizi.\n' "${#SERVICES[@]}"
}

# clear elimina NetEm solo se presente e considera idempotentemente valida l'assenza.
clear_all() {
  local service
  for service in "${SERVICES[@]}"; do
    tc_for_service "${service}"
    if grep -Eq 'qdisc netem .* root' <<<"${TC_SERVICE_OUTPUT}"; then
      docker exec "${TC_SERVICE_CONTAINER}" tc qdisc del dev "${TC_SERVICE_INTERFACE}" root
    fi
    tc_for_service "${service}"
    ! grep -Eq 'qdisc netem .* root' <<<"${TC_SERVICE_OUTPUT}" || fail "clear fallito su ${service}"
  done
  printf 'NetEm rimosso da %d servizi.\n' "${#SERVICES[@]}"
}

require_docker
case "${1:-}" in
  show) show_all ;;
  assert-on) assert_on "${2:-${TC_DELAY:-}}" ;;
  assert-off) assert_off ;;
  clear) clear_all ;;
  *) fail "uso: $0 {show|assert-on [delay]|assert-off|clear}" ;;
esac
