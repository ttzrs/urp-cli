#!/bin/bash
# URP Alert Hook for Claude
# Reads from /var/lib/urp/alerts/claude-alerts.md

ALERT_FILE="/var/lib/urp/alerts/claude-alerts.md"

if [[ -f "$ALERT_FILE" ]]; then
    if grep -q "ACTIVE SYSTEM ALERTS" "$ALERT_FILE" 2>/dev/null; then
        echo ""
        echo "---"
        cat "$ALERT_FILE"
        echo "---"
        echo ""
    fi
fi
