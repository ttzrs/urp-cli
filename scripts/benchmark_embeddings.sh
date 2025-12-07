#!/bin/bash
set -e

# Benchmark Sovereign Embeddings (TEI)
# Switch between Jina v3 and Nomic v1.5 and measure latency.

function benchmark_model() {
    MODEL=$1
    NAME=$2
    
    echo "==================================================="
    echo "🧪 Benchmarking: $NAME"
    echo "   Model ID: $MODEL"
    echo "==================================================="

    # Restart TEI with new model
    echo "🔄 Restarting TEI container..."
    export TEI_MODEL="$MODEL"
    docker compose up -d tei
    
    # Wait for healthcheck
    echo "⏳ Waiting for model to load (may take download time)..."
    RETRIES=0
    while ! curl -f -s http://localhost:8080/health > /dev/null; do
        sleep 5
        echo -n "."
        RETRIES=$((RETRIES+1))
        if [ $RETRIES -gt 60 ]; then
            echo "❌ Timeout waiting for TEI"
            return 1
        fi
    done
    echo " ✓ Ready"

    # Run URP vector operation
    # We use 'urp vec add' (if exists) or 'urp think context' to trigger embedding
    echo "🚀 Running Benchmark..."
    
    start_time=$(date +%s%N)
    
    # Simulate embedding a query
    # We use a direct curl to isolate TEI latency from URP logic first
    curl -s -X POST http://localhost:8080/embed \
        -H "Content-Type: application/json" \
        -d '{"inputs": "This is a benchmark string to test embedding latency and quality.", "normalize": true, "truncate": true}' > /dev/null
        
    end_time=$(date +%s%N)
    duration=$(( (end_time - start_time) / 1000000 ))
    echo "   ⚡ Latency (Cold): ${duration}ms"

    # Warm run (batch)
    start_time=$(date +%s%N)
    curl -s -X POST http://localhost:8080/embed \
        -H "Content-Type: application/json" \
        -d '{"inputs": ["Query 1 about code", "Query 2 about architecture", "Query 3 about bugs"], "normalize": true, "truncate": true}' > /dev/null
    end_time=$(date +%s%N)
    duration=$(( (end_time - start_time) / 1000000 ))
    echo "   ⚡ Latency (Batch 3): ${duration}ms"
    
    echo "✅ Benchmark Complete for $NAME"
    echo ""
}

# 1. Jina Embeddings v3 (8k context, SOTA)
benchmark_model "jinaai/jina-embeddings-v3" "Jina v3"

# 2. Nomic Embed Text v1.5 (Matryoshka, fast)
benchmark_model "nomic-ai/nomic-embed-text-v1.5" "Nomic v1.5"

echo "🏁 All benchmarks completed."
echo "   Select your preferred model by setting TEI_MODEL in .env"
