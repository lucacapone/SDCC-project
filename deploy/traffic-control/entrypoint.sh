#!/bin/sh
# Configura esclusivamente la qdisc sperimentale TC e poi cede PID 1 al nodo.

set -eu

delay_ms="${TC_DELAY_MS:-}"
jitter_ms="${TC_JITTER_MS:-}"
peer_host="${TC_PEER_HOST:-}"

# Docker DNS puo pubblicare i nomi dei servizi con un breve ritardo durante
# l'avvio concorrente. Questi limiti rendono robusto soltanto il bootstrap:
# il primo lookup e immediato, poi sono ammessi 40 retry ogni 250 ms (10 s).
dns_retry_interval_seconds='0.25'
dns_retry_limit=40

fail() {
  printf 'ERRORE TC: %s\n' "$*" >&2
  exit 1
}

# I valori accettati sono millisecondi interi non negativi: nessuna stringa tc
# arbitraria raggiunge quindi il comando privilegiato.
case "${delay_ms}" in ''|*[!0-9]*) fail 'TC_DELAY_MS deve essere un intero non negativo' ;; esac
case "${jitter_ms}" in ''|*[!0-9]*) fail 'TC_JITTER_MS deve essere un intero non negativo' ;; esac
[ -n "${peer_host}" ] || fail 'TC_PEER_HOST non configurato'
command -v ip >/dev/null 2>&1 || fail 'iproute2/ip non disponibile'
command -v tc >/dev/null 2>&1 || fail 'tc non disponibile'

# resolve_peer_ip mantiene bounded il workaround per la race di Docker DNS e
# restituisce appena il primo indirizzo IPv4 del peer diventa disponibile.
resolve_peer_ip() {
  retries_remaining="${dns_retry_limit}"
  while :; do
    resolved_ip="$(getent ahostsv4 "${peer_host}" 2>/dev/null | awk 'NR == 1 { print $1 }')"
    if [ -n "${resolved_ip}" ]; then
      printf '%s\n' "${resolved_ip}"
      return 0
    fi
    if [ "${retries_remaining}" -eq 0 ]; then
      return 1
    fi
    retries_remaining=$((retries_remaining - 1))
    sleep "${dns_retry_interval_seconds}"
  done
}

# La route verso un peer reale identifica l'interfaccia della rete Compose senza
# assumere che il suo nome sia eth0.
peer_ip="$(resolve_peer_ip)" \
  || fail "impossibile risolvere il peer ${peer_host} entro 10 secondi"
interface="$(ip -o route get "${peer_ip}" 2>/dev/null | awk '{ for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
[ -n "${interface}" ] || fail "interfaccia peer non individuabile dalla route verso ${peer_ip}"

if [ "${delay_ms}" -eq 0 ]; then
  [ "${jitter_ms}" -eq 0 ] || fail 'il bypass a delay 0 richiede jitter 0'
  printf 'TC BYPASS: peer=%s interface=%s delay=0ms jitter=0ms\n' "${peer_host}" "${interface}"
else
  # replace rende idempotente la configurazione e fallisce se NET_ADMIN non e
  # efficace nel container.
  tc qdisc replace dev "${interface}" root netem delay "${delay_ms}ms" "${jitter_ms}ms" distribution normal \
    || fail 'applicazione NetEm fallita (verificare NET_ADMIN)'
  qdisc="$(tc qdisc show dev "${interface}" 2>/dev/null)" \
    || fail 'qdisc non verificabile'
  printf '%s\n' "${qdisc}" | grep -Eq 'qdisc netem .* root ' \
    || fail "qdisc netem non attiva su ${interface}: ${qdisc}"
  printf 'TC ATTIVO: peer=%s interface=%s delay=%sms jitter=%sms qdisc=%s\n' \
    "${peer_host}" "${interface}" "${delay_ms}" "${jitter_ms}" "${qdisc}"
fi

# exec conserva la corretta propagazione dei segnali al processo Go.
exec /usr/local/bin/sdcc-node "$@"
