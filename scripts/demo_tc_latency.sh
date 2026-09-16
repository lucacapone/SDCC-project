#!/usr/bin/env bash
# Avvia e osserva la modalita TC senza interferire con le risorse normali.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/deploy/docker-compose.tc.yml"
TC_DOCKERFILE="${REPO_ROOT}/deploy/traffic-control/Dockerfile"
TC_IMAGE="sdcc-node-tc:local"
PROJECT_NAME="sdcc-tc"
TIMEOUT_SECONDS=30
OBSERVATION_SECONDS=8
EPSILON=0.000001
SERVICES=(node1 node2 node3 node4 node5 node6)
DELAYS=(0 400 800 1200 1600 2000)
JITTERS=(0 80 160 240 320 400)

run_compose() {
  docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" "$@"
}

# build_tc_image costruisce una sola volta l'immagine condivisa prima che
# Compose avvii i sei servizi, evitando export concorrenti sul medesimo tag.
build_tc_image() {
  docker build --file "${TC_DOCKERFILE}" --tag "${TC_IMAGE}" "${REPO_ROOT}"
}

fail() {
  printf 'ERRORE: %s\n' "$*" >&2
  exit 1
}

# oracle_value mantiene target approvati statici: non calcola l'aggregato.
oracle_value() {
  case "${1:-}" in
    average) printf '60\n' ;;
    sum) printf '360\n' ;;
    min) printf '10\n' ;;
    max) printf '110\n' ;;
    *) return 1 ;;
  esac
}

# numeric_equal confronta due numeri con epsilon senza dipendenze non presenti
# di default su macOS (come bc o GNU awk).
numeric_equal() {
  awk -v actual="$1" -v expected="$2" -v epsilon="${3:-${EPSILON}}" 'BEGIN {
    delta = actual - expected
    if (delta < 0) delta = -delta
    exit !(delta <= epsilon)
  }'
}

# extract_latest_sample restituisce node_id, aggregazione e stima dall'ultimo
# convergence_sample completo, anche quando Compose premette il nome servizio.
extract_latest_sample() {
  awk '
    function value(name,    i, token) {
      for (i = 1; i <= NF; i++) {
        token = $i
        if (token ~ ("^" name "=")) {
          sub("^" name "=", "", token); gsub(/^"|"$/, "", token); return token
        }
      }
      return ""
    }
    /event=convergence_sample/ {
      node = value("node_id"); aggregation = value("aggregation"); estimate = value("estimate")
      if (node != "" && aggregation ~ /^(average|sum|min|max)$/ &&
          estimate ~ /^[-+]?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][-+]?[0-9]+)?$/) {
        latest = node "\t" aggregation "\t" estimate
      }
    }
    END { if (latest != "") print latest }
  '
}

# extract_known_peers legge solo la serie gauge, ignorando HELP/TYPE e label.
extract_known_peers() {
  awk '$1 == "sdcc_node_known_peers" && $2 ~ /^[0-9]+([.][0]+)?$/ { latest = $2 }
       END { if (latest != "") { sub(/[.]0+$/, "", latest); print latest } }'
}

# extract_false_suspicion individua una transizione alive -> suspect indipendentemente
# dall'ordine dei campi slog e conserva l'intera riga come evidenza.
extract_false_suspicion() {
  awk '/event=membership_transition/ && /previous_status=alive/ && /status=suspect/ { print; exit }'
}

qdisc_is_netem() {
  grep -Eq 'qdisc netem .* root '
}

all_nodes_ok() {
  [ "$1" -eq 6 ]
}

deadline_reached() {
  [ "$1" -ge "$2" ]
}

# observation_decision centralizza l'esito del monitor: il successo richiede
# convergenza simultanea e almeno otto secondi completi di osservazione.
observation_decision() {
  local ok_count="$1" elapsed="$2"
  if all_nodes_ok "${ok_count}" && deadline_reached "${elapsed}" "${OBSERVATION_SECONDS}"; then
    return 0
  fi
  if deadline_reached "${elapsed}" "${TIMEOUT_SECONDS}"; then
    return 3
  fi
  return 1
}

# report_false_suspicion conserva l'evidenza e restituisce un codice distinto,
# senza adattare automaticamente profilo NetEm o timeout applicativi.
report_false_suspicion() {
  local evidence="$1"
  printf '\nFALSE SUSPICION: cluster ancora attivo ma rilevata alive -> suspect.\n%s\n' "${evidence}" >&2
  printf 'RUN NON VALIDA: il profilo TC deve essere rivalutato.\n' >&2
  return 2
}

# self_test congela parser, oracle, epsilon e decisioni successo/timeout senza
# richiedere Docker; viene richiamato anche dalla validazione statica.
self_test() {
  local sample known suspicion status earliest_success=-1 elapsed ok_count
  [ "$(oracle_value average)" = 60 ] && [ "$(oracle_value sum)" = 360 ] \
    && [ "$(oracle_value min)" = 10 ] && [ "$(oracle_value max)" = 110 ] \
    || fail 'self-test oracle fallito'
  if oracle_value median >/dev/null 2>&1; then fail 'self-test aggregazione non supportata fallito'; fi
  numeric_equal 60.0000004 60 "${EPSILON}" || fail 'self-test epsilon positivo fallito'
  if numeric_equal 60.000002 60 "${EPSILON}"; then fail 'self-test epsilon negativo fallito'; fi
  sample="$(printf '%s\n' 'node1 | time=now level=INFO event=convergence_sample node_id=node-1 aggregation=average estimate=60' | extract_latest_sample)"
  [ "${sample}" = $'node-1\taverage\t60' ] || fail 'self-test convergence_sample fallito'
  known="$(printf '%s\n' '# TYPE sdcc_node_known_peers gauge' 'sdcc_node_known_peers 6' | extract_known_peers)"
  [ "${known}" = 6 ] || fail 'self-test metrics known fallito'
  suspicion="$(printf '%s\n' 'time=now event=membership_transition peer_id=node-6 status=suspect previous_status=alive' | extract_false_suspicion)"
  [ -n "${suspicion}" ] || fail 'self-test false suspicion fallito'
  printf '%s\n' 'qdisc netem 8001: root refcnt 2 limit 1000 delay 500ms 100ms' | qdisc_is_netem \
    || fail 'self-test qdisc fallito'
  all_nodes_ok 6 && ! all_nodes_ok 5 || fail 'self-test successo fallito'
  deadline_reached 30 30 && ! deadline_reached 29 30 || fail 'self-test timeout fallito'
  # Harness deterministico: una perdita di convergenza riporta il monitor in
  # attesa e il primo successo possibile resta esattamente a otto secondi.
  for elapsed in 2 3 4 5 6 7 8; do
    ok_count=6
    [ "${elapsed}" -eq 5 ] && ok_count=5
    observation_decision "${ok_count}" "${elapsed}"
    status=$?
    if [ "${status}" -eq 0 ]; then earliest_success="${elapsed}"; break; fi
    [ "${status}" -eq 1 ] || fail 'self-test harness osservazione fallito'
  done
  [ "${earliest_success}" -eq 8 ] || fail 'self-test successo anticipato rispetto a 8s'
  report_false_suspicion "${suspicion}" >/dev/null 2>&1
  status=$?
  [ "${status}" -eq 2 ] || fail 'self-test exit false suspicion fallito'
  printf 'Self-test Traffic Control: OK\n'
}

running_services_count() {
  run_compose ps --services --status running 2>/dev/null | awk 'NF { count++ } END { print count + 0 }'
}

all_containers_running() {
  [ "$(running_services_count)" -eq 6 ]
}

# peer_interface ricava l'interfaccia con la stessa strategia route-aware usata
# dall'entrypoint, così la diagnostica non assume eth0.
peer_interface() {
  local service="$1" peer="$2"
  run_compose exec -T "${service}" sh -eu -c '
    peer_ip="$(getent ahostsv4 "$1" | awk '\''NR == 1 { print $1 }'\'')"
    [ -n "${peer_ip}" ]
    route="$(ip -o route get "${peer_ip}")"
    interface="$(printf '\''%s\n'\'' "${route}" | awk '\''{ for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }'\'')"
    [ -n "${interface}" ]
    printf '\''%s\n'\'' "${interface}"
  ' sh "${peer}"
}

verify_qdiscs() {
  local index service peer interface output
  for index in "${!SERVICES[@]}"; do
    service="${SERVICES[index]}"
    peer=node1; [ "${service}" = node1 ] && peer=node2
    interface="$(peer_interface "${service}" "${peer}")" || fail "interfaccia non verificabile per ${service}"
    [ -n "${interface}" ] || fail "interfaccia vuota per ${service}"
    output="$(run_compose exec -T "${service}" tc qdisc show dev "${interface}")" \
      || fail "qdisc non verificabile per ${service}"
    if [ "${index}" -eq 0 ]; then
      if printf '%s\n' "${output}" | qdisc_is_netem; then fail 'node1 non e in bypass TC'; fi
      printf 'Diagnostica: node-1 BYPASS TC (interfaccia %s).\n' "${interface}"
    else
      printf '%s\n' "${output}" | qdisc_is_netem \
        || fail "qdisc netem assente per ${service}: ${output}"
    fi
  done
}

check_false_suspicion() {
  local evidence
  evidence="$(run_compose logs --no-color --since "${RUN_STARTED_AT}" 2>/dev/null | extract_false_suspicion)" \
    || fail 'impossibile leggere i log membership'
  if [ -n "${evidence}" ]; then
    all_containers_running || fail "transizione suspect osservata insieme a container non attivi: ${evidence}"
    report_false_suspicion "${evidence}"
    return $?
  fi
  return 0
}

render_and_count_ok() {
  local aggregation="$1" expected="$2" elapsed="$3" index service metrics known logs sample node_id sample_aggregation estimate status ok_count=0
  for index in "${!SERVICES[@]}"; do
    service="${SERVICES[index]}"
    metrics="$(run_compose exec -T "${service}" curl --fail --silent --show-error --max-time 2 http://127.0.0.1:8080/metrics 2>/dev/null)" \
      || fail "metrics non disponibili per ${service}"
    known="$(printf '%s\n' "${metrics}" | extract_known_peers)"
    [ -n "${known}" ] || fail "sdcc_node_known_peers non parsabile per ${service}"
    logs="$(run_compose logs --no-color --since "${RUN_STARTED_AT}" "${service}" 2>/dev/null)" \
      || fail "log non recuperabili per ${service}"
    sample="$(printf '%s\n' "${logs}" | extract_latest_sample)"
    if [ -z "${sample}" ]; then
      if [ "${elapsed}" -gt 5 ]; then
        fail "convergence_sample non disponibile per ${service} dopo lo startup"
      fi
      printf '[START] %-6s | delay: %-6s | jitter: %-5s | known: %s/6 | estimate: ...\n' \
        "node-$((index + 1))" "${DELAYS[index]}ms" "${JITTERS[index]}ms" "${known}"
      continue
    fi
    IFS=$'\t' read -r node_id sample_aggregation estimate <<<"${sample}"
    [ "${sample_aggregation}" = "${aggregation}" ] \
      || fail "aggregazione inattesa per ${service}: ${sample_aggregation}"
    status='[WAIT]'
    if numeric_equal "${estimate}" "${expected}" "${EPSILON}"; then status='[OK]  '; ok_count=$((ok_count + 1)); fi
    printf '%s %-6s | delay: %-6s | jitter: %-5s | known: %s/6 | estimate: %s\n' \
      "${status}" "${node_id}" "${DELAYS[index]}ms" "${JITTERS[index]}ms" "${known}" "${estimate}"
  done
  RENDERED_OK_COUNT="${ok_count}"
}

main() {
  local aggregation="${1:-average}" expected started_epoch elapsed monitor_status suspicion_status first_convergence_seen=0
  expected="$(oracle_value "${aggregation}")" || fail "aggregazione non supportata: ${aggregation} (usare average, sum, min o max)"
  command -v docker >/dev/null 2>&1 || fail 'Docker non disponibile'
  docker info >/dev/null 2>&1 || fail 'daemon Docker non disponibile'
  docker compose version >/dev/null 2>&1 || fail 'Docker Compose non disponibile'
  export TC_AGGREGATION="${aggregation}"
  printf 'Build unica immagine TC %s...\n' "${TC_IMAGE}"
  build_tc_image || fail 'build immagine TC fallito'
  # Il limite temporale nasce immediatamente prima della ricreazione forzata:
  # Compose leggerà poi soltanto i log dei nuovi container della run corrente.
  RUN_STARTED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  export RUN_STARTED_AT
  printf 'Avvio progetto TC isolato %s...\n' "${PROJECT_NAME}"
  run_compose up -d --no-build --force-recreate || fail 'avvio TC fallito'
  all_containers_running || fail 'non tutti i 6 container TC risultano running'
  verify_qdiscs
  started_epoch="$(date +%s)"
  while :; do
    elapsed=$(( $(date +%s) - started_epoch ))
    all_containers_running || fail 'uno o piu container TC non sono piu attivi'
    check_false_suspicion
    suspicion_status=$?
    [ "${suspicion_status}" -eq 0 ] || return "${suspicion_status}"
    printf '\033[2J\033[H'
    printf '%s\n   DEMO - TRAFFIC CONTROL\n%s\n' '========================================================' '========================================================'
    printf 'Aggregazione: %s\nValore atteso: %s\nTempo trascorso: %ss / %ss\n\n' "${aggregation}" "${expected}" "${elapsed}" "${TIMEOUT_SECONDS}"
    render_and_count_ok "${aggregation}" "${expected}" "${elapsed}"
    if all_nodes_ok "${RENDERED_OK_COUNT}"; then first_convergence_seen=1; fi
    if [ "${first_convergence_seen}" -eq 1 ] && ! deadline_reached "${elapsed}" "${OBSERVATION_SECONDS}"; then
      printf '\nStabilita: osservazione in corso fino ad almeno %ss.\n' "${OBSERVATION_SECONDS}"
    fi
    observation_decision "${RENDERED_OK_COUNT}" "${elapsed}"
    monitor_status=$?
    if [ "${monitor_status}" -eq 0 ]; then
      printf '\n%s\nCONVERGENZA RAGGIUNTA\nTempo totale: %ss\n%s\n' \
        '========================================================' "${elapsed}" '========================================================'
      return 0
    fi
    if [ "${monitor_status}" -eq 3 ]; then
      printf '\n%s\nCONVERGENZA NON RAGGIUNTA ENTRO 30s\n%s\n' \
        '========================================================' '========================================================' >&2
      return 3
    fi
    sleep 1
  done
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  if [[ "${1:-}" == "--self-test" ]]; then self_test; else main "${1:-average}"; fi
fi
