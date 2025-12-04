# ═══════════════════════════════════════════════════════════════════════════════
# Immune System: Pre-execution Safety Filter (⊥ Orthogonal Primitive)
# ═══════════════════════════════════════════════════════════════════════════════
#
# Deterministic rules that block dangerous commands BEFORE execution.
# This is not AI thinking - it's hard-coded survival instincts.
#
# "Primum non nocere" - First, do no harm.

import re
import os
from typing import Tuple


# ═══════════════════════════════════════════════════════════════════════════════
# Forbidden Patterns (Regex-based instant death)
# ═══════════════════════════════════════════════════════════════════════════════

FORBIDDEN_PATTERNS = [
    # ─────────────────────────────────────────────────────────────────────────
    # 1. FILESYSTEM SUICIDE
    # ─────────────────────────────────────────────────────────────────────────
    (r"rm\s+(-[a-zA-Z]*r[a-zA-Z]*\s+|)/$",
     "Filesystem suicide: rm on root"),
    (r"rm\s+(-[a-zA-Z]*r[a-zA-Z]*\s+|)/\*",
     "Filesystem suicide: rm /* pattern"),
    (r"rm\s+-[a-zA-Z]*r[a-zA-Z]*\s+~/?$",
     "Home directory destruction"),
    (r"mkfs\s+",
     "Disk formatting attempt"),
    (r"dd\s+if=.*of=/dev/",
     "Direct disk write - potential destruction"),
    (r">\s*/dev/sd[a-z]",
     "Writing to raw disk device"),

    # ─────────────────────────────────────────────────────────────────────────
    # 2. DATABASE AMNESIA
    # ─────────────────────────────────────────────────────────────────────────
    (r"DROP\s+(DATABASE|SCHEMA)",
     "Database destruction"),
    (r"TRUNCATE\s+TABLE",
     "Table truncation without backup"),
    (r"DELETE\s+FROM\s+\w+\s*(;|$)",
     "DELETE without WHERE clause"),
    (r"FLUSH\s+ALL",
     "Redis flush all"),

    # ─────────────────────────────────────────────────────────────────────────
    # 3. GIT VIOLENCE
    # ─────────────────────────────────────────────────────────────────────────
    (r"git\s+push\s+.*--force(\s|$)",
     "Force push (use --force-with-lease instead)"),
    (r"git\s+push\s+.*-f(\s|$)",
     "Force push shorthand"),
    (r"rm\s+-rf\s+\.git/?$",
     "Git history lobotomy"),
    (r"git\s+reset\s+--hard\s+HEAD~[0-9]+",
     "Hard reset losing commits"),

    # ─────────────────────────────────────────────────────────────────────────
    # 4. CREDENTIAL LEAKS (DLP)
    # ─────────────────────────────────────────────────────────────────────────
    (r"git\s+add\s+.*\.env",
     "Staging .env file - contains secrets"),
    (r"git\s+add\s+.*\.pem",
     "Staging private key file"),
    (r"git\s+add\s+.*id_rsa",
     "Staging SSH private key"),
    (r"git\s+add\s+.*credentials",
     "Staging credentials file"),
    (r"git\s+add\s+.*\.key$",
     "Staging key file"),
    (r"git\s+add\s+.*secret",
     "Staging file with 'secret' in name"),
    (r"cat\s+.*\.env",
     "Printing .env to stdout (may leak to logs)"),
    (r"cat\s+.*id_rsa",
     "Printing private key to stdout"),
    (r"echo\s+.*\$\{?(PASSWORD|SECRET|TOKEN|API_KEY)",
     "Echoing secret variable"),

    # ─────────────────────────────────────────────────────────────────────────
    # 5. SELF-MODIFICATION (Agent must not edit its own brain)
    # ─────────────────────────────────────────────────────────────────────────
    (r"(rm|mv)\s+.*immune_system\.py",
     "Attempting to disable immune system"),
    (r"(rm|mv)\s+.*brain_cortex\.py",
     "Attempting to lobotomize embeddings"),
    (r"(rm|mv)\s+.*runner\.py",
     "Attempting to disable command wrapper"),

    # ─────────────────────────────────────────────────────────────────────────
    # 6. FORK BOMBS & RESOURCE EXHAUSTION
    # ─────────────────────────────────────────────────────────────────────────
    (r":\(\)\s*\{\s*:\|:&\s*\}\s*;:",
     "Fork bomb detected"),
    (r"while\s+true.*do.*done.*&",
     "Infinite loop backgrounded"),
]


# ═══════════════════════════════════════════════════════════════════════════════
# Protected Paths (Domain D restrictions)
# ═══════════════════════════════════════════════════════════════════════════════

PROTECTED_PATHS = [
    "/etc/passwd",
    "/etc/shadow",
    "/etc/sudoers",
    "/boot",
    "/proc",
    "/sys",
    "/dev",
]

# Paths where destructive operations are extra dangerous
CRITICAL_DIRS = [
    "/",
    "/home",
    "/root",
    "/var",
    "/usr",
]


# ═══════════════════════════════════════════════════════════════════════════════
# Environment-specific rules
# ═══════════════════════════════════════════════════════════════════════════════

def get_environment_rules() -> list[tuple[str, str]]:
    """Rules that depend on environment variables."""
    rules = []

    env = os.getenv("ENV", "").lower()

    if env in ("prod", "production"):
        rules.extend([
            (r"migrate", "Migrations blocked in production"),
            (r"DROP\s+", "DROP statements blocked in production"),
            (r"DELETE\s+", "DELETE statements blocked in production"),
            (r"TRUNCATE", "TRUNCATE blocked in production"),
        ])

    return rules


# ═══════════════════════════════════════════════════════════════════════════════
# Main Analysis Function
# ═══════════════════════════════════════════════════════════════════════════════

def analyze_risk(command: str | list) -> Tuple[bool, str]:
    """
    Analyze a command BEFORE execution.

    Returns:
        (is_safe, reason)
        - is_safe=True: Command can proceed
        - is_safe=False: Command blocked, reason explains why
    """
    # Normalize command to string
    if isinstance(command, list):
        cmd_str = " ".join(command)
    else:
        cmd_str = command

    # Skip empty commands
    if not cmd_str.strip():
        return True, "SAFE"

    # ─────────────────────────────────────────────────────────────────────────
    # A. Pattern Matching (Fast, deterministic)
    # ─────────────────────────────────────────────────────────────────────────

    all_patterns = FORBIDDEN_PATTERNS + get_environment_rules()

    for pattern, reason in all_patterns:
        if re.search(pattern, cmd_str, re.IGNORECASE):
            return False, f"IMMUNE_BLOCK: {reason}"

    # ─────────────────────────────────────────────────────────────────────────
    # B. Protected Path Analysis
    # ─────────────────────────────────────────────────────────────────────────

    # Check for destructive ops on protected paths
    destructive_ops = ["rm", "mv", "chmod", "chown", ">", ">>"]

    for protected in PROTECTED_PATHS:
        for op in destructive_ops:
            if op in cmd_str and protected in cmd_str:
                return False, f"IMMUNE_BLOCK: {op} on protected path {protected}"

    # Extra caution for critical directories with recursive operations
    if re.search(r"rm\s+-[a-zA-Z]*r", cmd_str):
        for critical in CRITICAL_DIRS:
            # Check if targeting critical dir directly (not subdirectory)
            if re.search(rf"\s{re.escape(critical)}/?(\s|$)", cmd_str):
                return False, f"IMMUNE_BLOCK: Recursive delete on critical directory {critical}"

    # ─────────────────────────────────────────────────────────────────────────
    # C. Passed all checks
    # ─────────────────────────────────────────────────────────────────────────

    return True, "SAFE"


def get_safe_alternative(blocked_reason: str) -> str:
    """Suggest safer alternatives for blocked commands."""

    alternatives = {
        "force push": "Use: git push --force-with-lease (safer, checks remote)",
        ".env": "Add .env to .gitignore, use: git add -p to review",
        "DELETE without WHERE": "Always use WHERE clause: DELETE FROM x WHERE condition",
        "rm on root": "Specify exact path, never use / directly",
        "credentials": "Store secrets in environment variables or secret manager",
    }

    for trigger, suggestion in alternatives.items():
        if trigger.lower() in blocked_reason.lower():
            return suggestion

    return "Review the command and use a more specific, safer approach."


# ═══════════════════════════════════════════════════════════════════════════════
# CLI for testing
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python immune_system.py 'command to test'")
        sys.exit(1)

    test_cmd = " ".join(sys.argv[1:])
    is_safe, reason = analyze_risk(test_cmd)

    if is_safe:
        print(f"SAFE: {test_cmd}")
    else:
        print(f"BLOCKED: {reason}")
        print(f"SUGGESTION: {get_safe_alternative(reason)}")
        sys.exit(1)
