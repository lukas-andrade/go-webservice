# go-webservice

An HTTP echo service in Go. Whatever you send it (any method, any path), it
sends back as JSON: headers, query params, body and path.

This repo is being built in steps, one PR each:

1. **Echo service**: the Go app, tests, Docker, CI and security scans
2. **Kubernetes**: Pulumi (Go) deploying a `Deployment` and `Service` to a local Kind cluster, with a local state file
3. `scripts/ci.sh` (test, build, push to a local registry) and `SETUP.md`

## Quick start

All you need is Docker with the Compose plugin and `make`. Go doesn't have to
be installed. `compose.yaml` only defines the service itself, and the
Makefile runs tests, linters and scanners in throwaway `docker run`
containers.

```sh
make run                         # docker compose: echo on :8080, admin on :9090
curl -s 'localhost:8080/hello?name=ana' -d '{"hi":true}' | jq
make stop
```

To run it on Kubernetes instead, see [Kubernetes](#kubernetes-kind--pulumi).

```json
{
  "Headers": {
    "Accept": "*/*",
    "Content-Length": "11",
    "Content-Type": "application/x-www-form-urlencoded",
    "User-Agent": "curl/8.7.1"
  },
  "Params": { "name": ["ana"] },
  "Body": { "hi": true },
  "Path": "/hello"
}
```

Run `make` with no arguments to list every target.

## API

### Public port (`PORT`, default `8080`)

Every path and method is echoed with `200 application/json`. The field names
match the brief:

| Field     | Type                  | Notes                                                        |
|-----------|-----------------------|--------------------------------------------------------------|
| `Headers` | `map[string]string`   | Repeated headers are joined with `", "`, as RFC 9110 allows  |
| `Params`  | `map[string][]string` | Go's `url.Values`: `?a=1&a=2` → `["1","2"]`. Joining query values would lose data, since a value can contain a comma |
| `Body`    | JSON or string        | Valid JSON is embedded as-is; anything else is a string; empty is `""` |
| `Path`    | `string`              |                                                              |

The one exception to "every request gets an echo" is a body over
`MAX_BODY_BYTES` (1 MiB by default), which gets `413 {"error": "..."}`.
Without that cap, a single client could exhaust the service's memory. A
body that can't be read (for example, a client that disconnects mid-upload)
gets `400`.

### Admin port (`ADMIN_PORT`, default `9090`)

| Path       | Purpose                                                          |
|------------|------------------------------------------------------------------|
| `/healthz` | Liveness, always `200` while the process is up                   |
| `/readyz`  | Readiness, `503` during startup and once shutdown begins          |
| `/metrics` | Prometheus metrics                                               |

## Configuration

Everything is read from environment variables:

| Variable                      | Default        | Description                                   |
|-------------------------------|----------------|-----------------------------------------------|
| `PORT`                        | `8080`         | Public echo port                              |
| `ADMIN_PORT`                  | `9090`         | Health and metrics port                       |
| `MAX_BODY_BYTES`              | `1048576`      | Largest body accepted (1 MiB)                 |
| `SHUTDOWN_TIMEOUT`            | `15s`          | How long in-flight requests get to finish     |
| `LOG_LEVEL`                   | `info`         | `debug`, `info`, `warn`, `error`              |
| `OTEL_SERVICE_NAME`           | `echo-service` | Service name on traces                        |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(unset)*      | Setting this turns tracing on (OTLP over HTTP) |

Bad values stop the service at boot with a clear message, instead of
surfacing later as odd behaviour.

## Design

```
cmd/server/main.go        wiring: config → dependencies → servers → graceful shutdown
internal/
  config/                 env parsing and validation
  echo/                   domain: Request, Response and the Service that builds the echo
  handler/                HTTP layer: echo handler and health probes
  server/                 http.Server with timeouts, routing, logging middleware
  telemetry/              Prometheus metrics and OpenTelemetry tracing
test/integration/         black-box tests against the running container
postman/                  collection and environments
infra/                    Pulumi program (its own Go module) and the Kind cluster config
```

Each package has a single job, and they're joined by composition rather than
inheritance:

- **`echo.Service`** is the only place that knows what an echo looks like. It
  takes a plain `echo.Request` and has no HTTP dependency, so it's tested
  with table-driven tests and no server.
- **`handler.Echo`** deals with HTTP (body limits, status codes, JSON) and
  depends on a one-method `Echoer` interface declared on the consumer side,
  as Go convention suggests. Tests swap in a spy to check the request
  mapping.
- **Cross-cutting concerns** (tracing, logging, metrics) are plain
  `func(http.Handler) http.Handler` wrappers stacked around the handler in
  `server.PublicHandler`, so the echo logic never sees them.
- **`main`** only wires things together. There's no global state and no `init()`.

### Decisions worth calling out

- **Separate admin port.** The brief says *any* path must be echoed. Serving
  `/metrics` or `/healthz` on the same port would quietly break that for
  those paths, so probes and metrics live on `:9090`. It's also the usual
  Kubernetes setup, so metrics don't have to be exposed through the public
  Service.
- **Metric labels are `method` and `code` only.** Since the service accepts
  arbitrary paths, a `path` label would let any client create unlimited time
  series.
- **Tracing is opt-in.** Without an OTLP endpoint, no exporter is created and
  nothing is sent over the network. The W3C propagator is still installed, so
  incoming `traceparent` headers aren't lost. When tracing is on, log lines
  carry a `trace_id`.
- **Graceful shutdown.** On `SIGTERM`, readiness goes to `503` first, then
  both servers drain within `SHUTDOWN_TIMEOUT`, then pending spans are flushed.
- **No framework.** Go 1.22+ `ServeMux` covers the routing this needs, which
  keeps the dependency list to Prometheus and OpenTelemetry.

### Thread safety

Handlers are built once and shared across goroutines, so each one either
holds no mutable state (`echo.Service`, `handler.Echo`), uses types that are
safe for concurrent use (Prometheus collectors, `atomic.Bool` for
readiness), or is only written before the servers start (`config.Config`).
Unit tests run with `-race`, and one test sends 50 concurrent requests
through a single handler instance.

### Production hardening

- `http.Server` read, write, header and idle timeouts (guards against slowloris)
- Request bodies capped with `http.MaxBytesReader`
- Static binary on `distroless/static`, running as UID `65532`: no shell, no
  package manager, not root. The UID is numeric so Kubernetes can enforce
  `runAsNonRoot`; with a user name, the pod is refused.
- Compose and Kubernetes both run the container with a read-only filesystem
  and no privilege escalation
- Structured JSON logs through `log/slog`

## Kubernetes (Kind + Pulumi)

Needs Docker, [Kind](https://kind.sigs.k8s.io/) and `kubectl`. Pulumi runs
in a container, so it doesn't need to be installed.

```sh
make pulumi-preview # create Kind if needed; show the Pulumi plan; apply nothing
make up        # cluster + registry, build and push the image, pulumi up
make forward   # port-forward the Service to localhost:8081
curl -s localhost:8081/hello
make down      # pulumi destroy, then delete the cluster and registry
```

`make pulumi-preview` is the local dry-run workflow. It creates (or reuses)
the local Kind cluster because Pulumi's Kubernetes provider contacts the API
server while planning, then runs `pulumi preview --diff`. It uses the image
already configured in the selected stack; for a new stack only, it sets a
synthetic image reference. It does not build or push an image, pull an image,
or create/update/delete Kubernetes resources. It can create local Pulumi stack
metadata in the gitignored `infra/.pulumi-state` directory.

To write the structured Pulumi plan as YAML:

```sh
make pulumi-preview PREVIEW_FORMAT=yaml
cat infra/pulumi-preview.yaml
```

The Make target checks for `yq`. On macOS it installs it through Homebrew; on
Ubuntu it installs it through `apt-get` (and may request your `sudo` password).
This YAML is a readable representation of Pulumi's preview events, not a
Kubernetes manifest. To print resources that are already deployed, use:

```sh
kubectl --context kind-go-webservice get deployment,service -o yaml
```

After reviewing a successful preview, deploy and test the real image:

```sh
make up
make forward
curl -s 'http://localhost:8081/hello?name=ana' -d '{"hi":true}' | jq
```

Keep `make forward` running in one terminal; it is the local connection to
the `ClusterIP` Service in Kind. Stop it with `Ctrl-C` when done, then remove
the test environment with `make down`.

`make up` is safe to run again: it only creates what's missing, and a rerun
with no code changes reports `4 unchanged`.

How it fits together:

- **Cluster and registry.** `infra/kind/cluster.yaml` creates the cluster,
  and a `registry:3` container on `localhost:5002` sits on the `kind`
  network. A local registry is the option the brief prefers over
  `kind load`. Port 5002 avoids the 5001 used in the kind docs, which is
  often already taken by another project.
- **Image tag.** The image is pushed as
  `localhost:5002/echo-service:<content hash>`. Kubernetes rolls out only
  when the image actually changed, and never deploys a stale `latest`.
- **Pulumi program.** `infra/` is its own Go module, so the service doesn't
  pick up Pulumi's dependencies. `echoservice.EchoService` is a component
  resource that owns the `Deployment` and `Service`, and `main.go` only
  reads config and creates it. State is a local file in
  `infra/.pulumi-state` (gitignored), with no Pulumi Cloud account needed.
- **Configuration.** `Pulumi.yaml` declares `image` and `replicas`
  (default 2). `make up` sets `image` on every run, so no stack file is
  committed.
- **Deployment.** Two replicas; liveness and readiness probes on the admin
  port; CPU and memory requests plus a memory limit; non-root, read-only
  root filesystem, all capabilities dropped, `RuntimeDefault` seccomp.
  `pulumi up` waits until the pods are ready, and gives up after 3 minutes.
- **Service.** `ClusterIP` on port 80, pointing at the container's `http`
  port. Only the echo is exposed, not the admin port.

## Testing

```sh
make test-unit          # unit tests, race detector, coverage → coverage.out
make test-infra         # Pulumi program tests with Pulumi mocks, no cluster needed
make test-integration   # builds the image, starts it, runs test/integration
make test-postman       # runs the Postman collection with newman
make test               # all four
```

- **Unit tests** use only the standard library, plus Prometheus `testutil`.
  They cover config parsing, echo building, the handlers, the server
  lifecycle, routing, metrics and tracing (including a real OTLP export to a
  fake collector).
- **Integration tests** (`//go:build integration`) treat the Docker image as a
  black box over the network: query params, JSON and text bodies, every
  method, the 413 limit, probes and metrics.
- **Infra tests** run the Pulumi program against mocks and check what it
  would create: image, replicas, probes on the admin port, security
  context, and that the Service selector matches the pod labels.
- **Postman**: import `postman/echo-service.postman_collection.json` together
  with `postman/local.postman_environment.json`. Every request has
  assertions, so the collection doubles as a smoke test.

## Security scanning

```sh
make lint    # golangci-lint, including gosec
make vuln    # govulncheck: known CVEs in code paths that are actually called
make scan    # govulncheck + Trivy on the repo (deps, secrets, Dockerfile) and on the built image
```

Lint and govulncheck cover both Go modules, the service and `infra/`.

Trivy fails the build on any `HIGH` or `CRITICAL` finding. The image scan
skips CVEs that don't have a fix yet, since there's nothing to upgrade to.
It has caught two real ones so far:

- A gRPC DoS (CVE-2026-84445) pulled in by the OTLP exporter, fixed by
  moving to `google.golang.org/grpc v1.83.2`.
- A go-git symlink file read/write (CVE-2026-71556) pulled in by the Pulumi
  SDK, fixed by moving to `go-git v6.0.0-alpha.5`. govulncheck didn't flag
  it because the program never calls that code, which is why both scanners
  run.

## CI

`.github/workflows/ci.yml` runs on every push and on PRs to `main`, with
three parallel jobs: **lint**, **test** (unit, infra, integration, Postman) and
**security** (govulncheck, Trivy fs, Trivy image). Each job calls the same
`make` targets you run locally. The workflow token is read-only and the
checkout action is pinned to a commit SHA. Dependabot keeps Go modules, base
images and actions up to date.

`make ci` runs the whole pipeline locally.
