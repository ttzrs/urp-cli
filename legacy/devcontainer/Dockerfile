# ═══════════════════════════════════════════════════════════════════════════════
# URP Dev Container - Full Stack AI Development Environment (Minimal)
# ═══════════════════════════════════════════════════════════════════════════════
#
# Includes: Go, Python, Node, Docker, Claude CLI, Git, GH, HF, NVIDIA, Memgraph
#           Chrome + MCP DevTools
# SELinux compatible (:z volumes)
#
# Base: nvidia/cuda base (~1.5GB vs runtime ~3GB vs devel ~8GB)
# PyTorch bundles its own CUDA libs, so base is sufficient

FROM nvidia/cuda:12.4.0-base-ubuntu22.04

ARG GO_VERSION=1.22.5
ARG NODE_VERSION=20
ARG PYTHON_VERSION=3.11

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

# ─────────────────────────────────────────────────────────────────────────────
# System Dependencies (minimal)
# ─────────────────────────────────────────────────────────────────────────────

RUN apt-get update && apt-get install -y --no-install-recommends \
    # Minimal build tools
    gcc \
    g++ \
    make \
    # Python
    python${PYTHON_VERSION} \
    python${PYTHON_VERSION}-dev \
    python${PYTHON_VERSION}-venv \
    python3-pip \
    # Utils
    curl \
    wget \
    git \
    vim-tiny \
    less \
    jq \
    ca-certificates \
    gnupg \
    lsb-release \
    openssh-client \
    libffi-dev \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/vim.tiny /usr/bin/vim

# Set Python aliases
RUN update-alternatives --install /usr/bin/python python /usr/bin/python${PYTHON_VERSION} 1 && \
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1

# ─────────────────────────────────────────────────────────────────────────────
# Go
# ─────────────────────────────────────────────────────────────────────────────

RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"
ENV GOPATH="/root/go"

# ─────────────────────────────────────────────────────────────────────────────
# Node.js (via NodeSource)
# ─────────────────────────────────────────────────────────────────────────────

RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g npm@latest && \
    rm -rf /var/lib/apt/lists/*

# ─────────────────────────────────────────────────────────────────────────────
# Docker CLI (for host Docker access)
# ─────────────────────────────────────────────────────────────────────────────

RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list && \
    apt-get update && \
    apt-get install -y docker-ce-cli docker-compose-plugin && \
    rm -rf /var/lib/apt/lists/*

# ─────────────────────────────────────────────────────────────────────────────
# GitHub CLI (gh)
# ─────────────────────────────────────────────────────────────────────────────

RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list && \
    apt-get update && \
    apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# ─────────────────────────────────────────────────────────────────────────────
# Google Chrome (for MCP DevTools)
# ─────────────────────────────────────────────────────────────────────────────

RUN curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /usr/share/keyrings/google-chrome.gpg && \
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] http://dl.google.com/linux/chrome/deb/ stable main" > /etc/apt/sources.list.d/google-chrome.list && \
    apt-get update && \
    apt-get install -y google-chrome-stable && \
    rm -rf /var/lib/apt/lists/*

# ─────────────────────────────────────────────────────────────────────────────
# Claude CLI + MCP DevTools
# ─────────────────────────────────────────────────────────────────────────────

RUN npm install -g @anthropic-ai/claude-code @modelcontextprotocol/sdk

# ─────────────────────────────────────────────────────────────────────────────
# HuggingFace CLI
# ─────────────────────────────────────────────────────────────────────────────

RUN pip3 install --no-cache-dir huggingface_hub[cli]

# ─────────────────────────────────────────────────────────────────────────────
# Python Dependencies (URP + AI) - Optimized for size
# ─────────────────────────────────────────────────────────────────────────────

COPY requirements.txt /tmp/requirements.txt
RUN pip3 install --upgrade pip && \
    pip3 install --no-cache-dir -r /tmp/requirements.txt && \
    pip3 install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cu124 && \
    pip3 install --no-cache-dir sentence-transformers psutil && \
    rm -rf /root/.cache/pip

# ─────────────────────────────────────────────────────────────────────────────
# URP Application
# ─────────────────────────────────────────────────────────────────────────────

WORKDIR /app
COPY *.py ./
COPY shell_hooks.sh ./
COPY devcontainer/entrypoint.sh ./entrypoint.sh
COPY devcontainer/knowledge.py ./knowledge.py
RUN chmod +x /app/entrypoint.sh /app/shell_hooks.sh /app/knowledge.py

# Preload embedding model
RUN python3 /app/brain_cortex.py || echo "Model preload skipped"

# ─────────────────────────────────────────────────────────────────────────────
# Shared Memory Directories
# ─────────────────────────────────────────────────────────────────────────────

# Claude config and shared memory
RUN mkdir -p /root/.claude /shared/knowledge /shared/sessions

# Shell configuration - load hooks for ALL bash sessions (interactive and non-interactive)
RUN echo 'source /app/shell_hooks.sh' >> /root/.bashrc && \
    echo 'export PATH="/root/.local/bin:$PATH"' >> /root/.bashrc && \
    echo '[ -f /app/shell_hooks.sh ] && source /app/shell_hooks.sh' >> /etc/bash.bashrc

# Non-root user for Claude Code (--dangerously-skip-permissions needs non-root)
RUN useradd -m -s /bin/bash dev && \
    echo 'source /app/shell_hooks.sh' >> /home/dev/.bashrc && \
    echo 'export PATH="/root/.local/bin:$PATH"' >> /home/dev/.bashrc

# Claude Code wrapper (uses Bash for file writes - workaround for proxies)
COPY devcontainer/claude-write /usr/local/bin/claude-write
RUN chmod +x /usr/local/bin/claude-write

# Claude Code permissions config (auto-accept Bash, Read, Write)
# Create all Claude subdirectories and set proper ownership
RUN mkdir -p /home/dev/.claude/{projects,debug,statsig,todos,plans,session-env,shell-snapshots} && \
    echo '{"permissions":{"allow":["Bash(*)","Read(*)","Write(*)","Edit(*)","Glob(*)","Grep(*)"]}}' > /home/dev/.claude/settings.json && \
    chown -R dev:dev /home/dev && \
    chmod -R 755 /home/dev/.claude

# ─────────────────────────────────────────────────────────────────────────────
# Environment
# ─────────────────────────────────────────────────────────────────────────────

ENV NEO4J_URI=bolt://memgraph:7687
ENV NEO4J_USER=
ENV NEO4J_PASSWORD=
ENV URP_RUNNER=/app/runner.py
ENV URP_ENABLED=1
ENV PYTHONUNBUFFERED=1
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility

# ─────────────────────────────────────────────────────────────────────────────
# Entry
# ─────────────────────────────────────────────────────────────────────────────

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["bash"]
