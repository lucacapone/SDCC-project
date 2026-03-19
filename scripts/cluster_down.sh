#!/usr/bin/env bash
# Esegue un teardown pulito: richiede ai container di fermarsi, raccoglie i log finali
# con i valori emessi in shutdown e rimuove poi le risorse Compose residue.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/cluster_common.sh"

require_docker
ensure_artifacts_dir

printf '==> stop pulito del cluster %s\n' "${PROJECT_NAME}"
run_compose stop -t 10 || true

printf '==> raccolta artefatti finali dopo stop\n'
"${SCRIPT_DIR}/cluster_collect_results.sh"

printf '==> rimozione risorse Compose residue\n'
run_compose down --remove-orphans || true
