# URP Container - Optimized for Speed
# ====================================
# Targets: minimal | dev | full (with AI + fast CLI tools)

# ═══════════════════════════════════════════════════════════════
# Build stage
# ═══════════════════════════════════════════════════════════════
FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
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
# Target: dev (~30MB) - bash, git
# ═══════════════════════════════════════════════════════════════
FROM alpine:3.20 AS dev

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
# Target: full (~180MB) - AI tools + fast CLI
# ═══════════════════════════════════════════════════════════════
# Includes:
#   - Claude Code (AI coding assistant)
#   - ripgrep (rg) - 10x faster grep
#   - fd - 5x faster find
#   - fzf - fuzzy finder
#   - bat - cat with syntax highlighting
#   - delta - beautiful git diffs
#   - jq/yq - JSON/YAML processing
#   - httpie - better curl
#   - zoxide - smarter cd
# ═══════════════════════════════════════════════════════════════
FROM alpine:3.20 AS full

# Fast CLI tools (split layers for caching)
RUN apk add --no-cache \
    bash zsh git curl \
    ripgrep fd bat fzf \
    jq yq make \
    nodejs npm

# Delta (git diff viewer) - from Alpine repos (simpler, no network issues)
RUN apk add --no-cache delta || true

# Zoxide (smarter cd) - from Alpine repos
RUN apk add --no-cache zoxide || true

# Claude Code
RUN npm install -g @anthropic-ai/claude-code --unsafe-perm && \
    npm cache clean --force && \
    rm -rf /root/.npm

# Create user
RUN adduser -D -s /bin/bash urp && \
    mkdir -p /var/lib/urp/vector /home/urp/.config && \
    chown -R urp:urp /var/lib/urp /home/urp

# Copy URP binary
COPY --from=builder /build/urp /usr/local/bin/urp

# Shell configuration
COPY shell_hooks.sh /etc/profile.d/urp.sh
COPY entrypoint.sh /entrypoint.sh
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

# Claude Code shortcut
alias cc='claude'
BASHRC

RUN chmod 755 /entrypoint.sh /etc/profile.d/urp.sh && \
    chown urp:urp /home/urp/.bashrc

ENV URP_PROJECT=unknown \
    NEO4J_URI=bolt://urp-memgraph:7687 \
    TERM=xterm-256color \
    PATH="/usr/local/bin:/home/urp/.local/bin:$PATH" \
    GIT_PAGER="delta"

WORKDIR /workspace
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/bash", "-l"]

# ═══════════════════════════════════════════════════════════════
# Target: master - Full + orchestration + gh CLI + docker CLI
# ═══════════════════════════════════════════════════════════════
FROM full AS master

# Add GitHub CLI for PR creation and Docker CLI for spawning workers
# Create docker group - GID will be matched at runtime via entrypoint
RUN apk add --no-cache github-cli docker-cli shadow

# Master-specific entrypoint that runs ingest + claude
COPY master-entrypoint.sh /master-entrypoint.sh
RUN chmod 755 /master-entrypoint.sh

ENV URP_MASTER=1 \
    URP_READ_ONLY=true

ENTRYPOINT ["/master-entrypoint.sh"]
CMD ["claude"]

# ═══════════════════════════════════════════════════════════════
# Target: worker - Full + dev tools (has Claude CLI for master instructions)
# ═══════════════════════════════════════════════════════════════
FROM full AS worker

# Add language runtimes and dev tools for task execution
RUN apk add --no-cache \
    go python3 py3-pip \
    make gcc musl-dev \
    github-cli

# Setup Python virtual environment for pip installs
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Pre-install common dev packages in venv
RUN pip install --no-cache-dir pytest httpx

# Worker entrypoint that communicates via protocol
COPY worker-entrypoint.sh /worker-entrypoint.sh
RUN chmod 755 /worker-entrypoint.sh

ENV URP_WORKER=1 \
    URP_READ_ONLY=false \
    VIRTUAL_ENV=/opt/venv

ENTRYPOINT ["/worker-entrypoint.sh"]
CMD ["urp", "worker", "run"]

# ═══════════════════════════════════════════════════════════════
# Target: gpu - NVIDIA NeMo for ML/AutoML training
# ═══════════════════════════════════════════════════════════════
FROM nvcr.io/nvidia/nemo:24.07 AS gpu

# Copy URP binary from builder
COPY --from=builder /build/urp /usr/local/bin/urp

# Install additional tools
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
# Default target
# ═══════════════════════════════════════════════════════════════
FROM full
