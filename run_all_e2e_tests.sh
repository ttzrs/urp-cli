#!/bin/bash

# E2E Test Suite: Complete model configuration testing
# This script runs all E2E tests for the URP model configuration system

set -e  # Exit on any error

echo "Starting Complete URP Model Configuration E2E Test Suite..."
echo "==========================================================="

# Build URP binary if not present
if [ ! -f "./urp" ]; then
    echo "Building URP binary..."
    cd go && go build -o ../urp ./cmd/urp && cd ..
    echo "Binary built successfully."
fi

# --- Run all E2E tests in sequence ---
echo ""
echo "Running Enhanced LLM Configuration E2E Test..."
echo "---------------------------------------------"
./enhanced_llm_config_e2e_test.sh

echo ""
echo "Running Functional Model Configuration E2E Test..."
echo "------------------------------------------------"
./functional_model_config_e2e_test.sh

echo ""
echo "Running Real-World Scenario E2E Test..."
echo "---------------------------------------"
./real_world_scenario_e2e_test.sh

echo ""
echo "All E2E tests completed successfully!"
echo "====================================="

echo ""
echo "Test Summary:"
echo "- Enhanced LLM Configuration E2E Test: PASSED"
echo "- Functional Model Configuration E2E Test: PASSED" 
echo "- Real-World Scenario E2E Test: PASSED"
echo ""
echo "The URP model configuration system has been thoroughly tested and validated."
echo "All configuration variables, fallback mechanisms, and specialized models are working correctly."