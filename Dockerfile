# Stage 1: build the web UI
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Stage 2: build the Go binary (the UI is embedded from internal/web/dist)
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/printmarshal ./cmd/printmarshal \
 && CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/pm-sim ./cmd/pm-sim

# Stage 3: minimal runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 pm
COPY --from=build /out/printmarshal /usr/local/bin/printmarshal
COPY --from=build /out/pm-sim /usr/local/bin/pm-sim
COPY testdata/fixtures /opt/printmarshal/fixtures
ENV PM_LISTEN=:8080 PM_DATA_DIR=/data PM_TRAP_LISTEN=0.0.0.0:1162
VOLUME /data
EXPOSE 8080 1162/udp
USER pm
ENTRYPOINT ["/usr/local/bin/printmarshal"]
