COMPOSE ?= docker compose
IMAGE   ?= echo-service:local

GO_IMAGE     := golang:1.27.1
LINT_IMAGE   := golangci/golangci-lint:v2.14.0
TRIVY_IMAGE  := aquasec/trivy:0.74.0
NEWMAN_IMAGE := postman/newman:6.1.3-alpine

# Tools run in throwaway containers, so the only local requirement is Docker.
# Tests that need the service join the compose network and reach it as "echo".
NETWORK := go-webservice_default
GO      := docker run --rm -v $(CURDIR):/src -w /src \
           -v gomodcache:/go/pkg/mod -v gobuildcache:/root/.cache/go-build
TRIVY   := docker run --rm -v $(CURDIR):/src:ro -w /src -v trivycache:/root/.cache/trivy \
           $(TRIVY_IMAGE) --severity HIGH,CRITICAL --exit-code 1

.DEFAULT_GOAL := help
.PHONY: help build up down logs wait \
        test test-unit test-integration test-postman \
        lint vuln scan scan-fs scan-image ci

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the echo-service image
	docker build -t $(IMAGE) .

up: build ## Run the echo-service on :8080 (admin on :9090)
	$(COMPOSE) up -d echo

down: ## Stop and remove the echo-service
	$(COMPOSE) down --remove-orphans

logs: ## Follow the echo-service logs
	$(COMPOSE) logs -f echo

wait: up
	@for i in $$(seq 30); do curl -sf localhost:9090/readyz >/dev/null && exit 0; sleep 1; done; \
		echo "echo-service did not become ready"; exit 1

test: test-unit test-integration test-postman ## Run every test suite

test-unit: ## Unit tests with the race detector
	$(GO) $(GO_IMAGE) go test -race -count=1 -coverprofile=coverage.out ./...

test-integration: wait ## Integration tests against the running container
	$(GO) --network $(NETWORK) -e ECHO_BASE_URL=http://echo:8080 -e ECHO_ADMIN_URL=http://echo:9090 \
		$(GO_IMAGE) go test -tags integration -count=1 -v ./test/integration/...

test-postman: wait ## Run the Postman collection with newman
	docker run --rm --network $(NETWORK) -v $(CURDIR)/postman:/etc/newman:ro $(NEWMAN_IMAGE) \
		run echo-service.postman_collection.json -e docker.postman_environment.json

lint: ## golangci-lint
	$(GO) $(LINT_IMAGE) golangci-lint run ./...

vuln: ## govulncheck on Go code and dependencies
	$(GO) $(GO_IMAGE) go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

scan-fs: ## Trivy: vulnerable deps, secrets and Dockerfile misconfig
	$(TRIVY) fs --scanners vuln,secret,misconfig --skip-dirs .scan .

scan-image: build ## Trivy: vulnerabilities in the built image
	@mkdir -p .scan
	docker save $(IMAGE) -o .scan/image.tar
	$(TRIVY) image --input .scan/image.tar --ignore-unfixed

scan: vuln scan-fs scan-image ## Every security scan

ci: lint test scan ## What the pipeline runs
