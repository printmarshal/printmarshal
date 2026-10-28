# syntax=docker/dockerfile:1.7
# PrintMarshal container image (docs/operations.md).
#
#   Stage 1  node    builds the React SPA into internal/web/dist
#   Stage 2  go      embeds the SPA and builds static binaries (CGO off; SQLite is pure Go)
#   Stage 3  alpine  runs the server as an unprivileged user with a /healthz HEALTHCHECK
#
# Build:   docker build --build-arg VERSION=$(git describe --tags --always) -t printmarshal:dev .
#          (or `make docker`, which also stamps VCS_REF and BUILD_DATE)
# Run:     docker run -d --name printmarshal --network host \
#              -e PM_ADMIN_PASSWORD=change-me -v pm-data:/data printmarshal:dev
#
# Host networking is needed for zero-config discovery (mDNS / WS-Discovery
# multicast) and for SNMP traps; see deploy/docker-compose.yml and the
# "Networking" section of docs/operations.md.

ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.22

# ---- Stage 1: web UI --------------------------------------------------------
FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
# vite.config.ts writes to ../internal/web/dist, i.e. /src/internal/web/dist.
RUN npm run build

# ---- Stage 2: Go binaries ---------------------------------------------------
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/printmarshal ./cmd/printmarshal \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags "-s -w" -o /out/pm-sim ./cmd/pm-sim

# ---- Stage 3: runtime -------------------------------------------------------
FROM ${RUNTIME_IMAGE}
ARG RUNTIME_IMAGE
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="PrintMarshal" \
      org.opencontainers.image.description="Self-hosted, vendor-agnostic printer fleet management" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/printmarshal/printmarshal" \
      org.opencontainers.image.documentation="https://github.com/printmarshal/printmarshal/blob/main/docs/operations.md" \
      org.opencontainers.image.base.name="${RUNTIME_IMAGE}"

# ca-certificates: webhooks / IPPS to real CAs.  tzdata: TZ env.  sqlite: the
# sqlite3 CLI, so `pm-backup` can take a consistent online backup of /data.
RUN apk add --no-cache ca-certificates tzdata sqlite \
 && addgroup -g 10001 -S pm \
 && adduser -u 10001 -S -G pm -h /data -s /sbin/nologin pm \
 && mkdir -p /data && chown pm:pm /data && chmod 0700 /data

COPY --from=build /out/printmarshal /usr/local/bin/printmarshal
COPY --from=build /out/pm-sim       /usr/local/bin/pm-sim
COPY --chmod=0755 deploy/scripts/backup.sh  /usr/local/bin/pm-backup
COPY --chmod=0755 deploy/scripts/restore.sh /usr/local/bin/pm-restore
COPY testdata/fixtures /opt/printmarshal/fixtures

# Trap listener defaults to the unprivileged 1162: the process is not root and
# has no CAP_NET_BIND_SERVICE, so it cannot bind 162 (see docs/operations.md
# for the three ways to receive traps on 162).
ENV PM_LISTEN=:8080 \
    PM_DATA_DIR=/data \
    PM_TRAP_LISTEN=0.0.0.0:1162

VOLUME ["/data"]
EXPOSE 8080/tcp 1162/udp
USER pm:pm

# /healthz needs no authentication and reports {"ok":true,"version":...}.
# PM_HEALTH_URL overrides the probe URL when PM_LISTEN binds a specific address.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -T 2 -O /dev/null "${PM_HEALTH_URL:-http://127.0.0.1:${PM_LISTEN##*:}/healthz}" || exit 1

ENTRYPOINT ["/usr/local/bin/printmarshal"]
