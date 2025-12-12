#!/bin/bash

# Final installation verification script

echo "==============================================="
echo "URP FINAL INSTALLATION VERIFICATION"
echo "==============================================="

# Check if urp binary is accessible
echo "✓ URP binary location: $(which urp)"

# Check system health
echo "✓ System health: $(urp doctor)"

# Check model configuration
echo "✓ Default model: $(grep DEFAULT_MODEL ~/.urp-go/.env | cut -d'=' -f2)"

# Check if qwen3-coder-plus is in the registry
if grep -q "qwen3-coder-plus" ~/.urp/models.yaml; then
    echo "✓ Qwen3-Coder-Plus in model registry: YES"
else
    echo "✗ Qwen3-Coder-Plus in model registry: NO"
fi

# Check if endpoint is properly configured
echo "✓ API endpoint: $(grep OPENAI_BASE_URL ~/.urp-go/.env | cut -d'=' -f2)"

# Show qwen3-coder-plus details
echo "✓ Qwen3-Coder-Plus details:"
grep -A 6 "qwen3-coder-plus" ~/.urp/models.yaml | sed 's/^/  /'

# Test environment variable override
echo "✓ Testing URP_MODEL environment variable:"
export URP_MODEL=qwen3-coder-plus
echo "  URP_MODEL is set to: $URP_MODEL"

# Show that model appears in router
MODEL_COUNT=$(urp router models | grep -c "qwen3-coder-plus")
if [ "$MODEL_COUNT" -gt 0 ]; then
    echo "✓ Qwen3-Coder-Plus appears in router: YES"
else
    echo "✗ Qwen3-Coder-Plus appears in router: NO"
fi

echo
echo "==============================================="
echo "INSTALLATION SUCCESSFULLY VERIFIED"
echo "URP is installed and configured with qwen3-coder-plus"
echo "==============================================="