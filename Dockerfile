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
    && rm -rf /var/lib/apt/lists/*

# Install Docker CLI (for container observation)
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends docker-ce-cli && \
    rm -rf /var/lib/apt/lists/*

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
COPY entrypoint.sh ./

RUN chmod +x /app/entrypoint.sh /app/shell_hooks.sh

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

# ─────────────────────────────────────────────────────────────────────────────
# Environment
# ─────────────────────────────────────────────────────────────────────────────

ENV NEO4J_URI=bolt://memgraph:7687
ENV NEO4J_USER=
ENV NEO4J_PASSWORD=
ENV URP_RUNNER=/app/runner.py
ENV URP_ENABLED=1
ENV PYTHONUNBUFFERED=1

# ─────────────────────────────────────────────────────────────────────────────
# Entry
# ─────────────────────────────────────────────────────────────────────────────

ENTRYPOINT ["/app/entrypoint.sh"]
CMD []
