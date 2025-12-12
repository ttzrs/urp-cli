# URP Container - Optimized for Speed
# ===================================
# Targets: minimal | base-agent | master | worker | dev-full | gpu

# ═══════════════════════════════════════════════════════════════
# Build stage
# ═══════════════════════════════════════════════════════════════
FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN apk add --no-cache git && \
    go mod download # git is needed for go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -X main.version=0.1.0" \
    -trimpath \
    -o urp ./cmd/urp

# ═══════════════════════════════════════════════════════════════
# Target: minimal (~10MB) - distroless, CLI only
# ═══════════════════════════════════════════════════════════════
FROM gcr.io/distroless/static-debian12:nonroot AS minimal

COPY --from=builder /build/urp /urp
ENV NEO4J_URI=bolt://urp-memgraph:7687
WORKDIR /workspace
ENTRYPOINT ["/urp"]

# ═══════════════════════════════════════════════════════════════
# Target: base-agent (Minimal base for URP agents)
# This target contains only essential tools and configurations for the URP Go agent
# to function, without developer-centric CLI tools or additional language runtimes.
# ═══════════════════════════════════════════════════════════════
FROM alpine:3.20 AS base-agent

# Create user and essential directories
RUN adduser -D -s /bin/bash urp && \
    mkdir -p /var/lib/urp/vector /home/urp/.config && \
    chown -R urp:urp /var/lib/urp /home/urp && \
    touch /.urp-container

# Install essential agent runtime tools:
# bash: For shell scripting in entrypoints and running commands.
# git: For version control operations (ingesting history, cloning).
# curl: For making HTTP requests (e.g., to LLM APIs, web fetching tool).
RUN apk add --no-cache bash git curl

# Copy URP binary and shell configuration
COPY --from=builder /build/urp /usr/local/bin/urp
COPY shell_hooks.sh /etc/profile.d/urp.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /etc/profile.d/urp.sh

ENV URP_PROJECT=unknown \
    NEO4J_URI=bolt://urp-memgraph:7687 \
    TERM=xterm-256color \
    PATH="/usr/local/bin:/home/urp/.local/bin:$PATH"

WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash", "-l"]

# ═══════════════════════════════════════════════════════════════
# Target: master (URP Master Agent)
# Built upon base-agent, adds orchestration and Claude CLI.
# ═══════════════════════════════════════════════════════════════
FROM base-agent AS master

# Add GitHub CLI for PR creation and Docker CLI for spawning workers
# These are essential for the master agent's orchestration capabilities.
# shadow: For user/group management (e.g., adding 'urp' user to 'docker' group).
RUN apk add --no-cache github-cli docker-cli shadow

# Master-specific entrypoint
COPY master-entrypoint.sh /master-entrypoint.sh
# Remove Claude-specific configuration files
# COPY claude-alert-hook.sh /etc/urp/claude-alert-hook.sh
# COPY claude-container-settings.json /etc/urp/claude-settings.json
RUN chmod +x /master-entrypoint.sh && \
    mkdir -p /var/lib/urp/alerts && \
    chown -R urp:urp /var/lib/urp

ENV URP_MASTER=1 \
    URP_READ_ONLY=true

# Health check - verifies container is functional
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD urp doctor --quick || exit 1

ENTRYPOINT ["/master-entrypoint.sh"]
CMD ["/bin/bash", "-l"] # Default to bash for interactive shell


# ═══════════════════════════════════════════════════════════════
# Target: worker (URP Worker Agent)
# Built upon base-agent, adds language runtimes and dev tools for task execution.
# ═══════════════════════════════════════════════════════════════
FROM base-agent AS worker

# Add language runtimes and dev tools for task execution.
# These are essential for a worker that executes arbitrary code (Go, Python) and orchestrates Docker containers.
RUN apk add --no-cache \
    go python3 py3-pip \
    make gcc musl-dev \
    github-cli docker-cli shadow # github-cli for repo ops, docker-cli/shadow for container control.

# Setup Python virtual environment for pip installs
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Pre-install common dev packages in venv (pytest, httpx - useful for testing/http tasks)
RUN pip install --no-cache-dir pytest httpx

# Worker entrypoint that communicates via protocol
COPY worker-entrypoint.sh /worker-entrypoint.sh
RUN chmod 755 /worker-entrypoint.sh

ENV URP_WORKER=1 \
    URP_READ_ONLY=false \
    VIRTUAL_ENV=/opt/venv

# Health check - verifies worker is functional and docker access maintained
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD urp doctor --quick || exit 1

ENTRYPOINT ["/worker-entrypoint.sh"]
CMD ["urp", "worker", "run"]

# ═══════════════════════════════════════════════════════════════
# Target: gpu (Specialized for NVIDIA NeMo ML/AutoML training)
# This is highly specialized and its minimization depends on specific GPU task needs.
# ═══════════════════════════════════════════════════════════════
FROM nvcr.io/nvidia/nemo:24.07 AS gpu

# Copy URP binary from builder
COPY --from=builder /build/urp /usr/local/bin/urp

# Install additional tools:
# pytest, httpx: for Python testing and HTTP requests.
# optuna, pytorch-lightning: ML frameworks.
# git, jq: git for repo ops, jq for JSON processing.
RUN pip install --no-cache-dir \
    pytest httpx \
    optuna \
    pytorch-lightning \
    && apt-get update && apt-get install -y --no-install-recommends \
    git jq \
    && rm -rf /var/lib/apt/lists/*

# Shell configuration
COPY shell_hooks.sh /etc/profile.d/urp.sh
COPY worker-entrypoint.sh /worker-entrypoint.sh
RUN chmod 755 /worker-entrypoint.sh /etc/profile.d/urp.sh

ENV URP_WORKER=1 \
    URP_READ_ONLY=false \
    URP_GPU=1 \
    NEO4J_URI=bolt://urp-memgraph:7687

WORKDIR /workspace
ENTRYPOINT ["/worker-entrypoint.sh"]
CMD ["urp", "worker", "run"]

# ═══════════════════════════════════════════════════════════════
# Target: dev-full (NEW TARGET: For human development with full CLI tools)
# This target consolidates developer convenience tools that are NOT
# strictly necessary for the URP agent's autonomous operations.
# ═══════════════════════════════════════════════════════════════
FROM base-agent AS dev-full

# Fast CLI tools for enhanced human development experience:
# zsh: Alternative shell.
# ripgrep, fd, bat, fzf, jq, yq, httpie, zoxide, delta: Advanced CLI utilities.
# make: Often used in dev environments.
RUN apk add --no-cache \
    zsh \
    ripgrep fd bat fzf \
    jq yq make httpie \
    zoxide delta

# Shell configuration with aliases and fzf setup for human convenience
# (These are for an interactive user, not the agent itself).
COPY <<'BASHRC' /home/urp/.bashrc
# Fast aliases
alias ll='ls -la'
alias l='ls -CF'
alias g='git'
alias gs='git status'
alias gd='git diff'
alias gc='git commit'
alias gp='git push'

# Use fast tools
alias grep='rg'
alias find='fd'
alias cat='bat --paging=never'

# Zoxide (smarter cd)
eval "$(zoxide init bash)"
alias cd='z'

# FZF configuration
export FZF_DEFAULT_COMMAND='fd --type f --hidden --follow --exclude .git'
export FZF_CTRL_T_COMMAND="$FZF_DEFAULT_COMMAND"

# Source URP hooks
source /etc/profile.d/urp.sh

# Quick commands
alias pain='urp events errors'
alias recent='urp events list'
alias vitals='urp sys vitals'
alias wisdom='urp think wisdom'
alias novelty='urp think novelty'
alias learn='urp think learn'
alias focus='urp focus'

# Claude Code shortcut (if included in base-agent or master)
alias cc='claude' # Assuming 'claude' command is available (from npm install)
BASHRC

RUN chown urp:urp /home/urp/.bashrc

# ═══════════════════════════════════════════════════════════════
# Default target
# (Changed from 'full' to 'master' to ensure a functional agent setup by default.)
# ═══════════════════════════════════════════════════════════════
FROM master