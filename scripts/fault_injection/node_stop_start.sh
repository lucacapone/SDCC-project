#!/usr/bin/env bash
# Simula stop/start manuale di un nodo del cluster Docker Compose canonico.
# Lo script è pensato per validazione operativa e debug locale, non per essere
# una dipendenza hard della suite Go automatica.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

require_docker
ensure_artifacts_dir
ensure_fault_artifacts_dir

# Parametri configurabili via env oppure argomenti posizionali.
ACTION="${ACTION:-${1:-bounce}}"
SERVICE="${SERVICE:-${2:-${DEFAULT_SERVICE}}}"
STOP_TIMEOUT_SECONDS="${STOP_TIMEOUT_SECONDS:-10}"
START_TIMEOUT_SECONDS="${START_TIMEOUT_SECONDS:-30}"
AFTER_STOP_SLEEP_SECONDS="${AFTER_STOP_SLEEP_SECONDS:-0}"
WAIT_FOR_RUNNING="${WAIT_FOR_RUNNING:-1}"

require_known_service "${SERVICE}"

case "${ACTION}" in
  stop)
    printf '==> stop del servizio %s (timeout stop=%ss)\n' "${SERVICE}" "${STOP_TIMEOUT_SECONDS}"
    run_compose stop -t "${STOP_TIMEOUT_SECONDS}" "${SERVICE}"
    wait_for_service_state "${SERVICE}" exited "${STOP_TIMEOUT_SECONDS}" 1
    ;;
  start)
    printf '==> start del servizio %s\n' "${SERVICE}"
    run_compose up -d "${SERVICE}"
    if [[ "${WAIT_FOR_RUNNING}" == "1" ]]; then
      wait_for_service_state "${SERVICE}" running "${START_TIMEOUT_SECONDS}" 1
    fi
    ;;
  bounce|restart)
    printf '==> bounce del servizio %s (stop=%ss start=%ss sleep_post_stop=%ss)\n' "${SERVICE}" "${STOP_TIMEOUT_SECONDS}" "${START_TIMEOUT_SECONDS}" "${AFTER_STOP_SLEEP_SECONDS}"
    run_compose stop -t "${STOP_TIMEOUT_SECONDS}" "${SERVICE}"
    wait_for_service_state "${SERVICE}" exited "${STOP_TIMEOUT_SECONDS}" 1
    if (( AFTER_STOP_SLEEP_SECONDS > 0 )); then
      sleep "${AFTER_STOP_SLEEP_SECONDS}"
    fi
    run_compose up -d "${SERVICE}"
    if [[ "${WAIT_FOR_RUNNING}" == "1" ]]; then
      wait_for_service_state "${SERVICE}" running "${START_TIMEOUT_SECONDS}" 1
    fi
    ;;
  *)
    fail "azione non supportata: ${ACTION}. Usare stop|start|bounce"
    ;;
esac

printf '==> stato aggiornato del servizio %s\n' "${SERVICE}"
run_compose ps "${SERVICE}"
