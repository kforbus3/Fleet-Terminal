# syntax=docker/dockerfile:1
# Multi-stage production build for the Fleet Terminal backend.

FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates openssh-client
# Resolve dependencies first for better layer caching. go.sum is generated here
# when absent (no local Go toolchain is required on the developer machine).
COPY go.mod ./
COPY go.sum* ./
ENV GOFLAGS=-mod=mod
RUN go mod download || true
COPY . .
RUN go mod tidy
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/fleetd ./cmd/fleetd
# Offline admin/recovery CLI shipped alongside the server.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/fleetctl ./cmd/fleetctl

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates openssh-client tzdata && \
    addgroup -S fleet && adduser -S -G fleet fleet && \
    mkdir -p /var/lib/fleet/recordings && chown -R fleet:fleet /var/lib/fleet
COPY --from=build /out/fleetd /usr/local/bin/fleetd
COPY --from=build /out/fleetctl /usr/local/bin/fleetctl
USER fleet
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/fleetd"]
