#!/bin/sh
# Configura NetEm in modo fail-fast, poi esegue il nodo come utente non privilegiato.
set -eu

# Questi override rendono i comandi di sistema iniettabili nei test senza cambiare il runtime.
TC_BIN="${TC_BIN:-tc}"
IP_BIN="${IP_BIN:-ip}"
PRIV_DROP_BIN="${PRIV_DROP_BIN:-su-exec}"
NODE_BIN="${NODE_BIN:-/usr/local/bin/sdcc-node}"
TC_ENABLED="${TC_ENABLED:-false}"
TC_DELAY="${TC_DELAY:-}"
TC_ROUTE_TARGET="${TC_ROUTE_TARGET:-}"

# log_result emette l'unica riga strutturata prodotta dall'entrypoint.
log_result() {
  printf 'event=traffic_control state=%s delay=%s container=%s interface=%s result=%s\n' \
    "$1" "${TC_DELAY:-none}" "${HOSTNAME:-unknown}" "${2:-unknown}" "$3"
}

# fail_tc registra l'esito finale prima di interrompere il container.
fail_tc() {
  log_result "$1" "${2:-unknown}" "failed"
  exit 1
}

# Accetta esclusivamente booleani espliciti per evitare attivazioni accidentali.
case "$TC_ENABLED" in
  true|false) ;;
  *) fail_tc invalid unknown ;;
esac

# Quando NetEm è attivo il delay deve essere un intervallo tc positivo e monounità.
if [ "$TC_ENABLED" = true ]; then
  printf '%s\n' "$TC_DELAY" | grep -Eq '^([1-9][0-9]*(\.[0-9]+)?|0\.[0-9]*[1-9][0-9]*)(ms|us|ns|s)$' || fail_tc enabled unknown
fi

# interface_candidates risolve prima una route verso il target opzionale e poi il default.
interface_candidates() {
  if [ -n "$TC_ROUTE_TARGET" ]; then
    "$IP_BIN" route get "$TC_ROUTE_TARGET" 2>/dev/null | awk '{ for (i=1; i<=NF; i++) if ($i=="dev") print $(i+1) }'
  else
    "$IP_BIN" route show default 2>/dev/null | awk '{ for (i=1; i<=NF; i++) if ($i=="dev") print $(i+1) }'
  fi
}

# Se il target non produce una route, tenta esplicitamente la route predefinita.
interfaces="$(interface_candidates | sort -u)"
if [ -z "$interfaces" ] && [ -n "$TC_ROUTE_TARGET" ]; then
  interfaces="$("$IP_BIN" route show default 2>/dev/null | awk '{ for (i=1; i<=NF; i++) if ($i=="dev") print $(i+1) }' | sort -u)"
fi
count="$(printf '%s\n' "$interfaces" | awk 'NF { n++ } END { print n+0 }')"
[ "$count" -eq 1 ] || fail_tc "$TC_ENABLED" unknown
interface="$interfaces"

# Applica idempotentemente NetEm oppure elimina solo una root qdisc NetEm già presente.
if [ "$TC_ENABLED" = true ]; then
  "$TC_BIN" qdisc replace dev "$interface" root netem delay "$TC_DELAY" >/dev/null 2>&1 || fail_tc enabled "$interface"
  qdisc="$($TC_BIN qdisc show dev "$interface" 2>/dev/null)" || fail_tc enabled "$interface"
  printf '%s\n' "$qdisc" | grep -Eq "qdisc netem .* root .*delay ${TC_DELAY}([[:space:]]|$)" || fail_tc enabled "$interface"
  log_result enabled "$interface" applied
else
  current="$($TC_BIN qdisc show dev "$interface" 2>/dev/null)" || fail_tc disabled "$interface"
  if printf '%s\n' "$current" | grep -Eq 'qdisc netem .* root'; then
    "$TC_BIN" qdisc del dev "$interface" root >/dev/null 2>&1 || fail_tc disabled "$interface"
  fi
  qdisc="$($TC_BIN qdisc show dev "$interface" 2>/dev/null)" || fail_tc disabled "$interface"
  printf '%s\n' "$qdisc" | grep -Eq 'qdisc netem .* root' && fail_tc disabled "$interface"
  log_result disabled "$interface" cleared
fi

# su-exec elimina uid/gid root prima che il processo Go sostituisca la shell.
exec "$PRIV_DROP_BIN" 65532:65532 "$NODE_BIN" "$@"
