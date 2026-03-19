#!/usr/bin/env bash
# Avvia il cluster locale in modo idempotente: pulisce eventuali residui del progetto
# Compose canonico e poi ricrea i container usando il file docker-compose.yml di root.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

require_docker
ensure_artifacts_dir

printf '==> cleanup preventivo del cluster %s\n' "${PROJECT_NAME}"
run_compose down --remove-orphans >/dev/null 2>&1 || true

printf '==> avvio cluster tramite %s\n' "${COMPOSE_FILE}"
run_compose up -d --build

printf '==> servizi attesi: %s\n' "${SERVICES[*]}"
run_compose ps
