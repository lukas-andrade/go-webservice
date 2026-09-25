# Local setup

## Prerequisites

- Docker, with the Compose plugin, running
- [Kind](https://kind.sigs.k8s.io/)
- `kubectl`, `make`, `curl`, and Bash

Go and Pulumi do not need to be installed on the host. The Make targets run
them in disposable containers; the first run downloads the required images.

## Deploy to Kind

```sh
./scripts/ci.sh
```

This creates or reuses the Kind cluster and local registry, previews the
Pulumi update, applies it, port-forwards the Echo Service, and makes a sample
request. It uses `http://localhost:8080` when that port is free; otherwise it
prints a highlighted warning and uses `http://localhost:8081`. In an
interactive terminal, the service remains available until you press `Ctrl-C`.

## Full checks before deployment

```sh
./scripts/ci.sh --ci
```

This runs linting, unit and infrastructure tests, integration and Postman
tests, and security scans before deploying.

## Optional observability

```sh
./scripts/ci.sh --observability
```

This additionally deploys Grafana, Loki, Mimir, Tempo, and the OpenTelemetry
Collector through Pulumi. Grafana is available at `http://localhost:3001` and
includes the **Echo Service Overview** dashboard for request rate, HTTP 5xx
rate, p95 latency, and logs.

Use both options when required:

```sh
./scripts/ci.sh --ci --observability
```

## Useful commands

```sh
make pulumi-preview  # show a plan without applying resources
make deploy          # preview and apply one consistent image build
make forward         # Echo Service at http://localhost:8080
make forward-grafana # Grafana at http://localhost:3001
make down            # remove Pulumi resources, Kind, and the local registry
./scripts/ci.sh --help
```
