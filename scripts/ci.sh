#!/usr/bin/env bash

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

if [[ "$run_full_ci" == true ]]; then
    make ci
fi

if [[ "$enable_observability" == true ]]; then
    deployment_options=(OBSERVABILITY=true)
else
    deployment_options=(OBSERVABILITY=false)
fi

make deploy "${deployment_options[@]}"

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
        if curl --fail --silent "$url" >/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo "$name did not become reachable at $url" >&2
    return 1
}

port_is_in_use() {
    local port="$1"
    if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        return 0
    fi
    if command -v ss >/dev/null 2>&1 && ss -ltn "sport = :$port" | grep -q ":$port"; then
        return 0
    fi
    (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1
}

forward_port=8080
if port_is_in_use "$forward_port"; then
    forward_port=8081
    printf '\033[1;33mWARNING: port 8080 is already in use. Echo Service will be forwarded to http://localhost:8081.\033[0m\n'
fi

make forward FORWARD_PORT="$forward_port" >/tmp/go-webservice-forward.log 2>&1 &
forward_processes+=("$!")
echo_url="http://localhost:$forward_port"
wait_for_url "$echo_url/healthz" "Echo Service"
echo "Echo Service:"
curl --fail --silent --show-error "$echo_url/hello?name=platform" -d '{"source":"scripts/ci.sh"}'
echo

if [[ "$enable_observability" == true ]]; then
    make forward-grafana >/tmp/go-webservice-grafana.log 2>&1 &
    forward_processes+=("$!")
    wait_for_url "http://localhost:3001/api/health" "Grafana"
    echo "Grafana dashboard: http://localhost:3001/d/echo-service-overview"
fi

if [[ -t 1 ]]; then
    echo "Port-forward active. Press Ctrl-C to stop it."
    wait "${forward_processes[@]}"
fi
