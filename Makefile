# PrintMarshal build targets.
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X main.version=$(VERSION)

.PHONY: all build web test lint run sim clean docker

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

docker:
	docker build -t printmarshal:$(VERSION) .

clean:
	rm -rf bin internal/web/dist/assets internal/web/dist/index.html
