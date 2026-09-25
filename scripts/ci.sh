#!/usr/bin/env bash
# The challenge's reproducible build, test and Kind-registry workflow.

# This is an executable command, not shell configuration. Returning here
# keeps an accidental `source scripts/ci.sh` from changing (or exiting) the
# caller's terminal.
if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
    echo "Run this script with: ./scripts/ci.sh [--ci] [--observability]" >&2
    return 2
fi

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/ci.sh [--ci] [--observability]

Default flow:
  1. creates/reuses Kind and its local image registry;
  2. runs Pulumi preview, then applies the deployment;
  3. port-forwards the Echo Service and calls it with curl.

Options:
  --ci              Run lint, all tests and security scans before deployment.
  --observability   Deploys the optional LGTM stack (Grafana,
                    Loki, Mimir, Tempo and OTel Collector), port-forwards
                    Grafana to http://localhost:3001 and provisions its
                    default Echo Service dashboard.
  -h, --help       Show this help text.
EOF
}

run_full_ci=false
enable_observability=false
for argument in "$@"; do
    case "$argument" in
        --ci)
            run_full_ci=true
            ;;
        --observability)
            enable_observability=true
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $argument" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if "$run_full_ci"; then
    make ci
fi

if "$enable_observability"; then
    deployment_options=(OBSERVABILITY=true)
else
    deployment_options=(OBSERVABILITY=false)
fi

make pulumi-preview "${deployment_options[@]}"
make up "${deployment_options[@]}"

forward_processes=()
cleanup() {
    local process_id
    for process_id in "${forward_processes[@]}"; do
        kill "$process_id" 2>/dev/null || true
    done
}
trap cleanup EXIT INT TERM

wait_for_url() {
    local url="$1"
    local name="$2"
    local attempt
    for attempt in $(seq 1 30); do
        if curl --fail --silent --show-error "$url" >/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo "$name did not become reachable at $url" >&2
    return 1
}

make forward >/tmp/go-webservice-forward.log 2>&1 &
forward_processes+=("$!")
wait_for_url "http://localhost:8081/healthz" "Echo Service"
echo "Echo Service:"
curl --fail --silent --show-error 'http://localhost:8081/hello?name=platform' -d '{"source":"scripts/ci.sh"}'
echo

if "$enable_observability"; then
    make forward-grafana >/tmp/go-webservice-grafana.log 2>&1 &
    forward_processes+=("$!")
    wait_for_url "http://localhost:3001/api/health" "Grafana"
    echo "Grafana dashboard: http://localhost:3001/d/echo-service-overview"
fi

if [[ -t 1 ]]; then
    echo "Port-forward active. Press Ctrl-C to stop it."
    wait "${forward_processes[@]}"
fi
