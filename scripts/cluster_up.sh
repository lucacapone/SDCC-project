#!/usr/bin/env bash
# Avvia il cluster locale in modo idempotente: pulisce eventuali residui del progetto
# Compose canonico e poi ricrea i container usando il file docker-compose.yml di root.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

# Restituisce il comando Compose canonico come stringa leggibile per i messaggi diagnostici.
compose_command_string() {
  printf 'docker compose -p %q -f %q' "${PROJECT_NAME}" "${COMPOSE_FILE}"
}

# Stampa lo stato corrente dei container Compose senza far fallire la diagnostica in caso di errori secondari.
print_compose_ps_diagnostics() {
  local ps_output

  printf '\n==> docker compose ps (diagnostica)\n' >&2
  if ps_output="$(run_compose ps 2>&1)"; then
    printf '%s\n' "${ps_output}" >&2
  else
    printf '%s\n' "${ps_output}" >&2
    printf '   impossibile raccogliere docker compose ps\n' >&2
  fi
}

# Stampa una coda dei log dei servizi principali per facilitare il triage di bootstrap/build/runtime.
print_service_log_tails() {
  local service

  printf '\n==> tail log servizi principali (ultime 40 righe)\n' >&2
  for service in "${SERVICES[@]}"; do
    printf '\n--- %s ---\n' "${service}" >&2
    if ! run_compose logs --tail=40 "${service}" >&2; then
      printf 'log non disponibili per %s\n' "${service}" >&2
    fi
  done
}

# Classifica il motivo più probabile del fallimento per rendere immediato il troubleshooting manuale.
classify_compose_failure() {
  local compose_output="$1"
  local ps_output="$2"

  if grep -Eqi 'docker: .*compose is not a docker command|unknown shorthand flag: .*compose|docker compose version.*not found|compose plugin' <<<"${compose_output}"; then
    printf 'plugin compose assente o non funzionante'
    return
  fi

  if grep -Eqi 'failed to solve|pull access denied|error from sender|executor failed running|build failed|COPY failed|RUN .* returned a non-zero code|failed to read dockerfile|no such file or directory.*dockerfile' <<<"${compose_output}"; then
    printf 'build immagine fallita'
    return
  fi

  if grep -Eqi 'unhealthy|health check|starting' <<<"${compose_output}${ps_output}"; then
    printf 'container avviati ma unhealthy'
    return
  fi

  printf 'errore compose non classificato'
}


# Esegue cleanup best-effort di container legacy con nomi noti, anche se esterni al project Compose corrente.
cleanup_named_container_best_effort() {
  local container_name="$1"
  local container_ids

  if ! container_ids="$(docker ps -a --filter "name=^/${container_name}$" -q 2>/dev/null)"; then
    printf 'WARN: impossibile interrogare i container per %s; proseguo comunque\n' "${container_name}" >&2
    return 0
  fi

  if [[ -z "${container_ids}" ]]; then
    printf '==> nessun container preesistente da rimuovere: %s\n' "${container_name}" >&2
    return 0
  fi

  printf '==> rimozione best-effort container preesistente: %s (id: %s)\n' "${container_name}" "${container_ids//$'\n'/, }" >&2
  while IFS= read -r container_id; do
    [[ -z "${container_id}" ]] && continue
    if ! docker rm -f "${container_id}" >/dev/null 2>&1; then
      printf 'WARN: rimozione fallita per container %s (id=%s); proseguo comunque\n' "${container_name}" "${container_id}" >&2
      continue
    fi
    printf '==> rimosso container %s (id=%s)\n' "${container_name}" "${container_id}" >&2
  done <<<"${container_ids}"
}

# Rimuove eventuali container legacy del cluster locale per evitare conflitti di nome non gestiti da Compose.
remove_preexisting_sdcc_node_containers() {
  local legacy_name

  printf '==> cleanup best-effort container legacy: sdcc-node1 sdcc-node2 sdcc-node3\n' >&2
  for legacy_name in sdcc-node1 sdcc-node2 sdcc-node3; do
    cleanup_named_container_best_effort "${legacy_name}"
  done
}

# Esegue `docker compose up -d --build` catturando output e fornendo diagnostica ricca in caso di errore.
run_compose_up_with_diagnostics() {
  local -a up_args=(up -d --build)
  local compose_cmd
  local compose_output
  local failure_classification
  local ps_output

  compose_cmd="$(compose_command_string)"
  printf '==> avvio cluster tramite %s\n' "${COMPOSE_FILE}"

  remove_preexisting_sdcc_node_containers

  set +e
  compose_output="$(run_compose "${up_args[@]}" 2>&1)"
  local status=$?
  set -e

  if [[ ${status} -eq 0 ]]; then
    printf '%s\n' "${compose_output}"
    return 0
  fi

  ps_output="$(run_compose ps 2>&1 || true)"
  failure_classification="$(classify_compose_failure "${compose_output}" "${ps_output}")"

  printf 'ERRORE: %s durante `%s %s`\n' \
    "${failure_classification}" \
    "${compose_cmd}" \
    "${up_args[*]}" >&2
  printf '\n==> output di `%s %s`\n%s\n' \
    "${compose_cmd}" \
    "${up_args[*]}" \
    "${compose_output}" >&2

  if [[ -n "${ps_output}" ]]; then
    printf '\n==> docker compose ps (diagnostica)\n%s\n' "${ps_output}" >&2
  else
    print_compose_ps_diagnostics
  fi
  print_service_log_tails

  fail "avvio cluster fallito: ${failure_classification}"
}

require_docker
ensure_artifacts_dir

printf '==> cleanup preventivo del cluster %s\n' "${PROJECT_NAME}"
run_compose down --remove-orphans || {
  cleanup_status=$?
  printf "cleanup ignorato: docker compose down --remove-orphans fallito (exit=%s)\n" "${cleanup_status}" >&2
  true
}

run_compose_up_with_diagnostics

printf '==> servizi attesi: %s\n' "${SERVICES[*]}"
run_compose ps
