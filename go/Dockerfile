# URP Container - Optimized for Speed
# ====================================
# Two targets: minimal (CLI only) and dev (interactive shell)

# ═══════════════════════════════════════════════════════════════
# Build stage (shared)
# ═══════════════════════════════════════════════════════════════
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build with all optimizations
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -X main.version=0.1.0" \
    -trimpath \
    -o urp ./cmd/urp

# ═══════════════════════════════════════════════════════════════
# Target: minimal (~12MB) - for CI/CD, batch jobs
# ═══════════════════════════════════════════════════════════════
FROM gcr.io/distroless/static-debian12:nonroot AS minimal

COPY --from=builder /build/urp /urp

ENV NEO4J_URI=bolt://urp-memgraph:7687
WORKDIR /workspace
ENTRYPOINT ["/urp"]

# ═══════════════════════════════════════════════════════════════
# Target: dev (~25MB) - interactive shell, git, full features
# ═══════════════════════════════════════════════════════════════
FROM alpine:3.20 AS dev

# Single layer for all packages
RUN apk add --no-cache bash git && \
    adduser -D -s /bin/bash urp && \
    mkdir -p /var/lib/urp/vector && \
    chown -R urp:urp /var/lib/urp

COPY --from=builder /build/urp /usr/local/bin/urp
COPY shell_hooks.sh /etc/profile.d/urp.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /etc/profile.d/urp.sh

ENV URP_PROJECT=unknown \
    NEO4J_URI=bolt://urp-memgraph:7687 \
    TERM=xterm-256color

WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash", "-l"]

# ═══════════════════════════════════════════════════════════════
# Default target
# ═══════════════════════════════════════════════════════════════
FROM dev
