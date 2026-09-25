COMPOSE ?= docker compose
IMAGE   ?= echo-service:local

# Detect host architecture so docker build and kind use the native platform
# on both Apple Silicon (arm64) and Linux/CI (amd64).
ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# Matches infra/kind/cluster.yaml. Port 5002 because the kind docs' 5001 is
# often taken by another project's registry.
KIND_CLUSTER    := go-webservice
KIND_NODE_IMAGE := kindest/node:v1.33.1
REGISTRY_NAME   := go-webservice-registry
REGISTRY        := localhost:5002

GO_IMAGE     := golang:1.27.1
LINT_IMAGE   := golangci/golangci-lint:v2.14.0
TRIVY_IMAGE  := aquasec/trivy:0.74.0
NEWMAN_IMAGE := postman/newman:6.1.3-alpine
PULUMI_IMAGE := pulumi/pulumi-go:3.264.0

# Tools run in throwaway containers, so the only local requirements are
# Docker and, for the cluster, Kind. Tests that need the service join the
# compose network and reach it as "echo".
NETWORK := go-webservice_default
GO      := docker run --rm -v $(CURDIR):/src -w /src \
           -v gomodcache:/go/pkg/mod -v gobuildcache:/root/.cache/go-build
TRIVY   := docker run --rm -v $(CURDIR):/src:ro -w /src -v trivycache:/root/.cache/trivy \
           $(TRIVY_IMAGE) --severity HIGH,CRITICAL --exit-code 1

# Pulumi joins the kind network and uses the cluster's internal kubeconfig.
# State lives in infra/.pulumi-state; the passphrase only guards secrets in
# that local file, and this stack has none.
PULUMI  := docker run --rm --network kind -v $(CURDIR)/infra:/infra -w /infra \
           -v gomodcache:/go/pkg/mod -v gobuildcache:/root/.cache/go-build \
           -e PULUMI_BACKEND_URL=file:///infra/.pulumi-state -e PULUMI_CONFIG_PASSPHRASE= \
           -e KUBECONFIG=/infra/.kubeconfig --entrypoint sh $(PULUMI_IMAGE) -c

.DEFAULT_GOAL := help
.PHONY: help build run stop logs wait cluster up down forward \
        test test-unit test-infra test-integration test-postman \
        lint vuln scan scan-fs scan-image ci

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the echo-service image
	docker build --platform linux/$(ARCH) -t $(IMAGE) .

run: build ## Run the echo-service with docker compose on :8080 (admin on :9090)
	$(COMPOSE) up -d echo

stop: ## Stop the docker compose service
	$(COMPOSE) down --remove-orphans

logs: ## Follow the docker compose logs
	$(COMPOSE) logs -f echo

wait: run
	@for i in $$(seq 30); do curl -sf localhost:9090/readyz >/dev/null && exit 0; sleep 1; done; \
		echo "echo-service did not become ready"; exit 1

# Inside the node, localhost:5002 is the node itself, so containerd is told
# to pull those images from the registry container on the kind network.
cluster:
	@docker pull --platform linux/$(ARCH) $(KIND_NODE_IMAGE)
	@kind get clusters | grep -qx $(KIND_CLUSTER) || \
		DOCKER_DEFAULT_PLATFORM= kind create cluster --config infra/kind/cluster.yaml
	@docker exec $(KIND_CLUSTER)-control-plane sh -c 'mkdir -p /etc/containerd/certs.d/$(REGISTRY) && \
		echo "[host.\"http://$(REGISTRY_NAME):5000\"]" > /etc/containerd/certs.d/$(REGISTRY)/hosts.toml'
	@docker inspect $(REGISTRY_NAME) >/dev/null 2>&1 || \
		docker run -d --restart=always --network kind -p 127.0.0.1:5002:5000 --name $(REGISTRY_NAME) registry:3.1.2

# The image is tagged with its own content hash, so Kubernetes only rolls
# out a new version when the image actually changed.
up: build cluster ## Kind cluster + local registry, push the image, pulumi up
	@mkdir -p infra/.pulumi-state
	kind get kubeconfig --internal --name $(KIND_CLUSTER) > infra/.kubeconfig
	@image=$(REGISTRY)/echo-service:$$(docker image inspect -f '{{.Id}}' $(IMAGE) | cut -c8-19); \
	docker tag $(IMAGE) $$image && docker push -q $$image && \
	$(PULUMI) "pulumi stack select --create dev && pulumi config set image $$image && pulumi up --yes --skip-preview"
	@echo "Deployed. Try: make forward, then curl localhost:8081/hello"

down: ## pulumi destroy, then delete the Kind cluster and registry
	-kind get kubeconfig --internal --name $(KIND_CLUSTER) > infra/.kubeconfig && \
		$(PULUMI) "pulumi stack select dev && pulumi destroy --yes --skip-preview"
	kind delete cluster --name $(KIND_CLUSTER)
	docker rm -f $(REGISTRY_NAME)

forward: ## Port-forward the Kind service to localhost:8081
	kubectl --context kind-$(KIND_CLUSTER) port-forward svc/echo-service 8081:80

test: test-unit test-infra test-integration test-postman ## Run every test suite

test-unit: ## Unit tests with the race detector
	$(GO) $(GO_IMAGE) go test -race -count=1 -coverprofile=coverage.out ./...

test-infra: ## Pulumi program tests, using Pulumi mocks (no cluster needed)
	$(GO) -w /src/infra $(GO_IMAGE) go test -race -count=1 ./...

test-integration: wait ## Integration tests against the running container
	$(GO) --network $(NETWORK) -e ECHO_BASE_URL=http://echo:8080 -e ECHO_ADMIN_URL=http://echo:9090 \
		$(GO_IMAGE) go test -tags integration -count=1 -v ./test/integration/...

test-postman: wait ## Run the Postman collection with newman
	docker run --rm --network $(NETWORK) -v $(CURDIR)/postman:/etc/newman:ro $(NEWMAN_IMAGE) \
		run echo-service.postman_collection.json -e docker.postman_environment.json

lint: ## golangci-lint on the service and the Pulumi program
	$(GO) $(LINT_IMAGE) golangci-lint run ./...
	$(GO) -w /src/infra $(LINT_IMAGE) golangci-lint run ./...

vuln: ## govulncheck on the service and the Pulumi program
	$(GO) $(GO_IMAGE) go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
	$(GO) -w /src/infra $(GO_IMAGE) go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

scan-fs: ## Trivy: vulnerable deps, secrets and Dockerfile misconfig
	$(TRIVY) fs --scanners vuln,secret,misconfig --skip-dirs .scan --skip-dirs infra/.pulumi-state .

scan-image: build ## Trivy: vulnerabilities in the built image
	@mkdir -p .scan
	docker save $(IMAGE) -o .scan/image.tar
	$(TRIVY) image --input .scan/image.tar --ignore-unfixed

scan: vuln scan-fs scan-image ## Every security scan

ci: lint test scan ## What the pipeline runs
