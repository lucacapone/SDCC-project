#!/usr/bin/env bash
# Attende che il cluster risulti operativo verificando sia lo stato running dei container
# sia la presenza nei log dei messaggi di bootstrap e inizializzazione del transport.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

require_docker
ensure_artifacts_dir

# Consente override semplice del timeout senza modificare lo script.
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-60}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-2}"
DEADLINE_EPOCH="$(( $(date +%s) + TIMEOUT_SECONDS ))"

printf '==> attesa cluster operativo (timeout=%ss, poll=%ss)\n' "${TIMEOUT_SECONDS}" "${POLL_INTERVAL_SECONDS}"

while (( $(date +%s) <= DEADLINE_EPOCH )); do
  all_running=true
  all_bootstrapped=true

  for service in "${SERVICES[@]}"; do
    if ! service_is_running "${service}"; then
      all_running=false
      printf '   - %s non ancora running\n' "${service}"
      continue
    fi

    service_logs="$(run_compose logs --no-color --tail=200 "${service}" 2>/dev/null || true)"
    if [[ "${service_logs}" != *"bootstrap membership completato"* ]] || [[ "${service_logs}" != *"transport inizializzato"* ]]; then
      all_bootstrapped=false
      printf '   - %s running ma non ancora operativo nei log\n' "${service}"
    fi
  done

  if [[ "${all_running}" == true && "${all_bootstrapped}" == true ]]; then
    printf 'Cluster operativo: tutti i servizi sono running e hanno completato bootstrap + transport init.\n'
    exit 0
  fi

  sleep "${POLL_INTERVAL_SECONDS}"
done

run_compose ps || true
fail "timeout raggiunto durante l'attesa dello stato operativo del cluster"
