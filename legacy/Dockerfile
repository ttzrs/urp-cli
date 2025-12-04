# ═══════════════════════════════════════════════════════════════════════════════
# URP-CLI: Universal Repository Perception
# Dev container with semantic terminal capture
# ═══════════════════════════════════════════════════════════════════════════════

FROM python:3.11-slim

# ─────────────────────────────────────────────────────────────────────────────
# System dependencies
# ─────────────────────────────────────────────────────────────────────────────

RUN apt-get update && apt-get install -y --no-install-recommends \
    # Build tools for tree-sitter
    git \
    build-essential \
    # For docker observation (docker CLI)
    curl \
    ca-certificates \
    gnupg \
    # Useful tools
    vim \
    less \
    jq \
    bc \
    # Required by Claude Code
    procps \
    && rm -rf /var/lib/apt/lists/*

# Install Docker CLI (for container observation)
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends docker-ce-cli && \
    rm -rf /var/lib/apt/lists/*

# Install Node.js and Claude Code CLI
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g @anthropic-ai/claude-code && \
    rm -rf /var/lib/apt/lists/*

# Install OpenCode CLI (open-source alternative with multi-provider support)
RUN curl -fsSL https://opencode.ai/install | bash && \
    mv /root/.local/bin/opencode /usr/local/bin/ 2>/dev/null || true

# Install Go for local proxy (token tracking)
RUN curl -fsSL https://go.dev/dl/go1.22.0.linux-amd64.tar.gz | tar -C /usr/local -xzf - && \
    ln -s /usr/local/go/bin/go /usr/local/bin/go

# ─────────────────────────────────────────────────────────────────────────────
# Python dependencies
# ─────────────────────────────────────────────────────────────────────────────

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# ─────────────────────────────────────────────────────────────────────────────
# Application code
# ─────────────────────────────────────────────────────────────────────────────

COPY *.py ./
COPY shell_hooks.sh ./
COPY master_commands.sh ./
COPY claude_token_hook.sh ./
COPY entrypoint.sh ./
COPY .claude/ ./.claude/
COPY .opencode/ ./.opencode/
COPY proxy/ ./proxy/

# Build local proxy (Go binary for token tracking)
RUN cd /app/proxy && \
    go mod init urp-proxy && \
    go mod tidy && \
    CGO_ENABLED=1 go build -o /usr/local/bin/urp-proxy . && \
    rm -rf /app/proxy

# Note: pcx/ and tutorial/ are copied if present in build context
# They are optional components

RUN chmod +x /app/entrypoint.sh /app/shell_hooks.sh /app/master_commands.sh /app/claude_token_hook.sh

# ─────────────────────────────────────────────────────────────────────────────
# Preload embedding model (avoids download on first use)
# ─────────────────────────────────────────────────────────────────────────────

RUN python3 /app/brain_cortex.py || echo "Model preload skipped"

# ─────────────────────────────────────────────────────────────────────────────
# Shell configuration
# ─────────────────────────────────────────────────────────────────────────────

# Add shell hooks to bash/zsh startup
RUN echo 'source /app/shell_hooks.sh' >> /root/.bashrc && \
    echo 'source /app/shell_hooks.sh' >> /etc/bash.bashrc

# Create Claude Code config - use env var for API key
RUN mkdir -p /root/.claude && \
    echo '{"env": {"ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}"}}' > /root/.claude/settings.json && \
    echo '#!/bin/bash\necho "$ANTHROPIC_API_KEY"' > /root/.claude/api_key_helper.sh && \
    chmod +x /root/.claude/api_key_helper.sh

# ─────────────────────────────────────────────────────────────────────────────
# Environment
# ─────────────────────────────────────────────────────────────────────────────

ENV NEO4J_URI=bolt://memgraph:7687
ENV NEO4J_USER=
ENV NEO4J_PASSWORD=
ENV URP_RUNNER=/app/runner.py
ENV URP_ENABLED=1
ENV PYTHONUNBUFFERED=1

# Claude Code configuration - proxy sets ANTHROPIC_BASE_URL at runtime
ENV ANTHROPIC_UPSTREAM=http://100.105.212.98:8317
ENV ANTHROPIC_AUTH_TOKEN=sk-dummy

# ─────────────────────────────────────────────────────────────────────────────
# Entry
# ─────────────────────────────────────────────────────────────────────────────

ENTRYPOINT ["/app/entrypoint.sh"]
CMD []
