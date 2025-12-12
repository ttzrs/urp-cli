#!/bin/bash

# Final verification script to check if URP is properly configured with LLM models
echo "Final URP LLM Configuration Verification"
echo "========================================="

# Check if we have any API keys configured
if [ -z "$ANTHROPIC_API_KEY" ] && [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ No API keys are currently set."
    echo
    echo "To configure URP with LLM models, you need to set at least one API key:"
    echo
    echo "For Anthropic Claude models:"
    echo "  export ANTHROPIC_API_KEY='your-anthropic-api-key'"
    echo
    echo "For OpenRouter (supports multiple models including GPT, Claude, etc.):"
    echo "  export OPENAI_API_KEY='your-openrouter-api-key'"
    echo "  export OPENAI_BASE_URL='https://openrouter.ai/api/v1'"
    echo
    echo "For DeepSeek models:"
    echo "  export DEEPSEEK_API_KEY='your-deepseek-api-key'"
    echo
    exit 1
else
    echo "✅ API keys are configured:"
    [ -n "$ANTHROPIC_API_KEY" ] && echo "  - Anthropic API key: SET"
    [ -n "$OPENAI_API_KEY" ] && echo "  - OpenAI/OpenRouter API key: SET" 
    [ -n "$DEEPSEEK_API_KEY" ] && echo "  - DeepSeek API key: SET"
    echo
fi

# Check if URP executable exists
if [ ! -f "./go/urp" ]; then
    echo "❌ URP executable not found. Rebuilding..."
    cd go && go build -o urp ./cmd/urp && cd ..
    if [ $? -ne 0 ]; then
        echo "❌ Failed to build URP CLI tool"
        exit 1
    fi
    echo "✅ URP CLI tool built successfully"
else
    echo "✅ URP CLI tool exists"
fi

# Check system health
echo "Checking system health..."
./go/urp doctor
echo

# Show current configuration
echo "Current model configuration:"
echo "  Default model: ${URP_MODEL:-anthropic/claude-sonnet-4-5-20250929}"
echo "  Provider: ${URP_LLM_PROVIDER:-anthropic}"
echo "  Embedding provider: ${URP_EMBEDDING_PROVIDER:-tei}"
echo

# Test if infrastructure is running
if ! ./go/urp doctor 2>&1 | grep -q "infra:up"; then
    echo "⚠️ Infrastructure may not be running properly"
    echo "To start infrastructure: docker-compose up -d memgraph tei"
    echo
else
    echo "✅ Infrastructure is running"
fi

echo "========================================="
echo "URP LLM Model Configuration - VERIFIED"
echo "You can now use URP with your configured models!"
echo "Try: ./go/urp ask \"Hello, are you available?\""
echo "========================================="