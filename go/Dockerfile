# URP Container Image
# ====================
# Multi-stage build for minimal final image

# Stage 1: Build
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o urp ./cmd/urp

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    bash \
    git \
    curl \
    jq \
    docker-cli \
    && rm -rf /var/cache/apk/*

# Create urp user
RUN adduser -D -s /bin/bash urp

# Copy binary
COPY --from=builder /build/urp /usr/local/bin/urp

# Copy shell hooks
COPY shell_hooks.sh /etc/profile.d/urp-hooks.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /etc/profile.d/urp-hooks.sh

# Create directories
RUN mkdir -p /var/lib/urp/vector && chown -R urp:urp /var/lib/urp

# Environment
ENV URP_PROJECT=unknown \
    NEO4J_URI=bolt://urp-memgraph:7687 \
    URP_SESSION_ID="" \
    URP_MASTER=0 \
    URP_READ_ONLY=false \
    TERM=xterm-256color

WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash", "-l"]
