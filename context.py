# ═══════════════════════════════════════════════════════════════════════════════
# URP Context: Identity and Environment for Sessions
# ═══════════════════════════════════════════════════════════════════════════════
#
# Provides the identity model for multi-session memory:
# - instance_id: container/deployment identity
# - session_id: logical URP session
# - context_signature: environment fingerprint for compatibility checks

import os
import time
import socket
import hashlib
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class URPContext:
    """
    Identity context for a URP session.

    Used to tag all memory/knowledge with provenance info.
    """
    instance_id: str
    session_id: str
    user_id: str
    scope: str  # "session" | "instance" | "global"
    context_signature: str
    tags: list[str] = field(default_factory=list)
    started_at: str = field(default_factory=lambda: time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()))


def build_context_signature(
    project: str,
    dataset: Optional[str] = None,
    branch: Optional[str] = None,
    env: Optional[str] = None,
    extra: Optional[list[str]] = None,
) -> str:
    """
    Create a readable context signature.

    Example: 'urp-cli|UNSW-NB15|master|fedora-41'

    This is used to determine if knowledge from another session
    is "compatible" with the current environment.
    """
    parts = [project]
    if dataset:
        parts.append(dataset)
    if branch:
        parts.append(branch)
    if env:
        parts.append(env)
    if extra:
        parts.extend(extra)
    return "|".join(parts)


def get_context_hash(signature: str) -> str:
    """Get a short hash of the context signature for IDs."""
    return hashlib.sha256(signature.encode()).hexdigest()[:8]


def is_context_compatible(sig_a: str, sig_b: str, strict: bool = False) -> bool:
    """
    Check if two context signatures are compatible.

    In strict mode: must match exactly.
    In loose mode: must share at least the project name.
    """
    if strict:
        return sig_a == sig_b

    # Loose: check if first component (project) matches
    parts_a = sig_a.split("|")
    parts_b = sig_b.split("|")

    if not parts_a or not parts_b:
        return False

    return parts_a[0] == parts_b[0]


# ─────────────────────────────────────────────────────────────────────────────
# Global context singleton
# ─────────────────────────────────────────────────────────────────────────────

_current_context: Optional[URPContext] = None


def get_current_context() -> URPContext:
    """
    Get or create the current session context.

    Reads from environment variables:
    - URP_INSTANCE_ID: container/deployment ID (default: hostname)
    - URP_SESSION_ID: session ID (default: auto-generated)
    - URP_USER_ID: user identifier (default: "default")
    - URP_PROJECT: project name (default: "urp-cli")
    - URP_DATASET: dataset name (default: None)
    - URP_BRANCH: git branch (default: "master")
    - URP_ENV: environment name (default: "local")
    - URP_CONTEXT_SIGNATURE: override full signature
    """
    global _current_context

    if _current_context is not None:
        return _current_context

    instance_id = os.getenv("URP_INSTANCE_ID") or socket.gethostname()
    session_id = os.getenv("URP_SESSION_ID") or f"s-{int(time.time())}-{os.getpid()}"
    user_id = os.getenv("URP_USER_ID", "default")

    # Build context signature
    ctx_sig = os.getenv("URP_CONTEXT_SIGNATURE")
    if not ctx_sig:
        ctx_sig = build_context_signature(
            project=os.getenv("URP_PROJECT", "urp-cli"),
            dataset=os.getenv("URP_DATASET"),
            branch=os.getenv("URP_BRANCH", "master"),
            env=os.getenv("URP_ENV", "local"),
        )

    # Build tags from environment
    tags = []
    if os.getenv("URP_PROJECT"):
        tags.append(os.getenv("URP_PROJECT"))
    if os.getenv("URP_DATASET"):
        tags.append(os.getenv("URP_DATASET"))
    if os.getenv("URP_ENV"):
        tags.append(os.getenv("URP_ENV"))

    _current_context = URPContext(
        instance_id=instance_id,
        session_id=session_id,
        user_id=user_id,
        scope="session",
        context_signature=ctx_sig,
        tags=tags if tags else ["urp-cli", "local"],
    )

    return _current_context


def set_current_context(ctx: URPContext) -> None:
    """Override the current context (for testing or special cases)."""
    global _current_context
    _current_context = ctx


def reset_context() -> None:
    """Reset the context singleton (forces re-initialization)."""
    global _current_context
    _current_context = None


def new_session() -> URPContext:
    """
    Start a new session with a fresh session_id.

    Keeps the same instance_id and context_signature.
    """
    old = get_current_context()

    new_ctx = URPContext(
        instance_id=old.instance_id,
        session_id=f"s-{int(time.time())}-{os.getpid()}",
        user_id=old.user_id,
        scope="session",
        context_signature=old.context_signature,
        tags=old.tags.copy(),
    )

    set_current_context(new_ctx)
    return new_ctx


# ─────────────────────────────────────────────────────────────────────────────
# CLI / Debug
# ─────────────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    import json

    ctx = get_current_context()
    print("Current URP Context:")
    print(json.dumps({
        "instance_id": ctx.instance_id,
        "session_id": ctx.session_id,
        "user_id": ctx.user_id,
        "scope": ctx.scope,
        "context_signature": ctx.context_signature,
        "tags": ctx.tags,
        "started_at": ctx.started_at,
    }, indent=2))
