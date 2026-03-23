#!/usr/bin/env bash
# Esegue una verifica ripetibile della milestone M07 usando il flusso Compose canonico
# dalla root della repository. Lo script avvia il cluster con build locale, salva lo
# stato `docker compose ps` e raccoglie una quota minima di log per `node1`, `node2`,
# `node3` dentro `artifacts/m07/<timestamp>/`.
#
# Il contenuto prodotto è pensato come artefatto di evidenza: permette di rivedere a
# posteriori bootstrap, discovery tramite service name Compose e primi segnali di
# convergenza della membership senza dover rilanciare manualmente i comandi.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

# Directory dedicata agli artefatti M07 per non mescolare l'evidenza di verifica con
# gli altri snapshot cluster già usati dagli script storici.
M07_ARTIFACTS_DIR="${REPO_ROOT}/artifacts/m07"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="${M07_ARTIFACTS_DIR}/${TIMESTAMP}"
LATEST_LINK="${M07_ARTIFACTS_DIR}/latest"
WAIT_SECONDS="${WAIT_SECONDS:-8}"
LOG_TAIL_LINES="${LOG_TAIL_LINES:-120}"
SERVICES=(node1 node2 node3)

# Esegue il comando Compose canonico dalla root, preservando esattamente il flusso
# richiesto per M07 (`docker compose ...` senza file alternativi o project name custom).
run_root_compose() {
  (
    cd "${REPO_ROOT}"
    docker compose "$@"
  )
}

require_docker
mkdir -p "${RUN_DIR}"
ln -sfn "$(basename "${RUN_DIR}")" "${LATEST_LINK}"

printf '==> [M07] avvio cluster dal root con build locale\n'
run_root_compose up -d --build | tee "${RUN_DIR}/compose-up.txt"

printf '==> [M07] attesa di %ss per consentire bootstrap e primi round gossip\n' "${WAIT_SECONDS}"
sleep "${WAIT_SECONDS}"

printf '==> [M07] acquisizione stato servizi\n'
run_root_compose ps | tee "${RUN_DIR}/compose-ps.txt"

# Raccoglie un estratto minimo di log per nodo, sufficiente a osservare i marker più
# rilevanti del bootstrap e della membership senza generare output eccessivo.
for service in "${SERVICES[@]}"; do
  printf '==> [M07] raccolta log minimi per %s\n' "${service}"
  run_root_compose logs --no-color --tail "${LOG_TAIL_LINES}" "${service}" > "${RUN_DIR}/${service}.log"
done

# Produce un riepilogo leggibile con semplici marker osservabili, utile per capire se
# i log contengono già l'evidenza attesa senza aprire ogni file manualmente.
{
  printf 'timestamp_utc=%s\n' "${TIMESTAMP}"
  printf 'wait_seconds=%s\n' "${WAIT_SECONDS}"
  printf 'log_tail_lines=%s\n' "${LOG_TAIL_LINES}"
  for service in "${SERVICES[@]}"; do
    log_file="${RUN_DIR}/${service}.log"
    bootstrap_hits="$(grep -c 'gossip bootstrap completato' "${log_file}" || true)"
    transport_hits="$(grep -c 'transport gossip avviato' "${log_file}" || true)"
    compose_addr_hits="$(grep -Eoc 'node[123]:700[123]' "${log_file}" || true)"
    membership_hits="$(grep -Ec 'remote_merge|membership_transition|membership_entries=' "${log_file}" || true)"
    printf '%s bootstrap_hits=%s transport_hits=%s compose_addr_hits=%s membership_hits=%s\n' \
      "${service}" "${bootstrap_hits}" "${transport_hits}" "${compose_addr_hits}" "${membership_hits}"
  done
} > "${RUN_DIR}/summary.txt"

printf 'Artefatti M07 salvati in %s\n' "${RUN_DIR}"
printf 'Link rapido aggiornato: %s\n' "${LATEST_LINK}"
printf 'File principali:\n'
printf ' - %s\n' "${RUN_DIR}/compose-up.txt"
printf ' - %s\n' "${RUN_DIR}/compose-ps.txt"
for service in "${SERVICES[@]}"; do
  printf ' - %s\n' "${RUN_DIR}/${service}.log"
done
printf ' - %s\n' "${RUN_DIR}/summary.txt"
