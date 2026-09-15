#!/usr/bin/env bash
# Mostra una fotografia istantanea delle ultime stime pubblicate dai soli servizi
# running del cluster Compose di scala, senza polling o calcoli aggregativi locali.

set -uo pipefail

# Risolve il file Compose rispetto alla repository, così lo script funziona anche
# quando viene invocato tramite percorso assoluto da una directory differente.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/deploy/docker-compose.scale.yml"
PROJECT_NAME="sdcc-scale"

# Centralizza il comando Compose richiesto dallo scenario approvato.
run_compose() {
  docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" "$@"
}

# extract_latest_sample legge stdin e restituisce l'ultimo campione completo con
# aggregazione supportata e stima numerica, nel formato node_id<TAB>aggregation<TAB>estimate.
extract_latest_sample() {
  awk '
    function field_value(name,    position, token) {
      for (position = 1; position <= NF; position++) {
        token = $position
        if (token ~ ("^" name "=")) {
          sub("^" name "=", "", token)
          gsub(/^"|"$/, "", token)
          return token
        }
      }
      return ""
    }

    /event=convergence_sample/ {
      node_id = field_value("node_id")
      aggregation = field_value("aggregation")
      estimate = field_value("estimate")
      supported = aggregation == "average" || aggregation == "sum" || aggregation == "min" || aggregation == "max"
      numeric = estimate ~ /^[-+]?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][-+]?[0-9]+)?$/
      if (node_id != "" && supported && numeric) {
        latest = node_id "\t" aggregation "\t" estimate
      }
    }

    END {
      if (latest != "") {
        print latest
      }
    }
  '
}

# Un errore nel censimento impedisce di stabilire quali siano i soli servizi
# running: in questo caso non viene presentata una fotografia potenzialmente falsa.
if ! running_output="$(run_compose ps --services --status running 2>/dev/null)"; then
  printf 'ERRORE: impossibile leggere i servizi running del cluster sdcc-scale.\n' >&2
  exit 1
fi

# Bash 3.2 non fornisce mapfile: il ciclo popola l'array ignorando righe vuote.
running_services=()
while IFS= read -r service; do
  [[ -n "${service}" ]] && running_services+=("${service}")
done <<<"${running_output}"

# Gli array paralleli conservano l'ordine restituito da Compose e permettono di
# stampare tutti i risultati disponibili anche quando un singolo nodo fallisce.
node_ids=()
aggregations=()
estimates=()
read_error=0

for service in "${running_services[@]}"; do
  if ! service_logs="$(run_compose logs --no-color "${service}" 2>/dev/null)"; then
    node_ids+=("${service}")
    aggregations+=("N/D")
    estimates+=("N/D")
    read_error=1
    continue
  fi

  sample="$(printf '%s\n' "${service_logs}" | extract_latest_sample)"
  if [[ -z "${sample}" ]]; then
    node_ids+=("${service}")
    aggregations+=("N/D")
    estimates+=("N/D")
    read_error=1
    continue
  fi

  IFS=$'\t' read -r node_id aggregation estimate <<<"${sample}"
  node_ids+=("${node_id}")
  aggregations+=("${aggregation}")
  estimates+=("${estimate}")
done

# Determina esclusivamente la coerenza delle etichette lette: non ricalcola mai
# l'aggregazione o la stima prodotta dai nodi.
common_aggregation=""
inconsistent=0
for aggregation in "${aggregations[@]}"; do
  [[ "${aggregation}" == "N/D" ]] && continue
  if [[ -z "${common_aggregation}" ]]; then
    common_aggregation="${aggregation}"
  elif [[ "${aggregation}" != "${common_aggregation}" ]]; then
    inconsistent=1
  fi
done

if (( inconsistent )); then
  printf 'STATO AGGREGAZIONE: INCOERENTE\n\n'
  printf '%-10s %-14s %s\n' 'NODO' 'AGGREGAZIONE' 'STIMA'
  for index in "${!node_ids[@]}"; do
    printf '%-10s %-14s %s\n' "${node_ids[index]}" "${aggregations[index]}" "${estimates[index]}"
  done
else
  [[ -n "${common_aggregation}" ]] || common_aggregation="N/D"
  printf 'STATO AGGREGAZIONE: %s\n\n' "${common_aggregation}"
  printf '%-10s %s\n' 'NODO' 'STIMA'
  for index in "${!node_ids[@]}"; do
    printf '%-10s %s\n' "${node_ids[index]}" "${estimates[index]}"
  done
fi

# Un campione assente o illeggibile rende la fotografia incompleta, pur lasciando
# visibili le righe raccolte correttamente secondo il comportamento best-effort.
exit "${read_error}"
