# PrintMarshal build targets.
GO         ?= go
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VCS_REF    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE      ?= printmarshal
LDFLAGS     = -s -w -X main.version=$(VERSION)

# Override to run the tools from containers when they are not installed, e.g.
#   make helm-lint HELM="docker run --rm -v $$PWD:/w -w /w alpine/helm:3.19.0"
HELM       ?= helm
KUBECTL    ?= kubectl

.PHONY: all build web test lint run sim clean docker docker-run helm-lint k8s-dry-run

all: web build

## Build the server binary (embeds internal/web/dist; run `make web` first for the real UI).
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/printmarshal ./cmd/printmarshal
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/pm-sim ./cmd/pm-sim

## Build the SPA into internal/web/dist.
web:
	cd web && npm ci --no-audit --no-fund && npm run build

test:
	$(GO) vet ./...
	$(GO) test ./...

run: build
	PM_DATA_DIR=./data ./bin/printmarshal

## Start the printer simulator (SNMP on udp/1161, IPP on tcp/6631).
sim: build
	./bin/pm-sim -fixture testdata/fixtures/generic-laser.snmp

## Build the container image, tagged $(IMAGE):$(VERSION) and $(IMAGE):latest.
docker:
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg VCS_REF=$(VCS_REF) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

## Run the image with host networking (discovery + traps work); data in the pm-data volume.
docker-run: docker
	docker run --rm -it --name printmarshal --network host \
	  -e PM_ADMIN_PASSWORD=$${PM_ADMIN_PASSWORD:-change-me} \
	  -v pm-data:/data $(IMAGE):$(VERSION)

## Lint and render the Helm chart (deploy/helm/printmarshal).
helm-lint:
	@command -v $(firstword $(HELM)) >/dev/null 2>&1 || { echo "helm not found: install it or set HELM=... (see docs/operations.md)"; exit 1; }
	$(HELM) lint deploy/helm/printmarshal
	$(HELM) template printmarshal deploy/helm/printmarshal >/dev/null
	$(HELM) template printmarshal deploy/helm/printmarshal --set hostNetwork=false --set ingress.enabled=true --set service.trap.enabled=true >/dev/null
	@echo "helm chart OK"

## Client-side validation of the plain manifests (deploy/kubernetes). Needs a kube context.
k8s-dry-run:
	@command -v $(firstword $(KUBECTL)) >/dev/null 2>&1 || { echo "kubectl not found"; exit 1; }
	$(KUBECTL) apply --dry-run=client -k deploy/kubernetes

clean:
	rm -rf bin internal/web/dist/assets internal/web/dist/index.html
