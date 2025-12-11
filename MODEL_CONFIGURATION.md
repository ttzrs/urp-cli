# Model Configuration Guide for URP

This document explains how to configure different models for various functions in the URP system.

## Overview

URP uses a flexible model configuration system that allows you to specify different models for different functions and purposes. All configurations are managed through environment variables in your `.env` file.

## Configuration Structure

The configuration is organized by function and priority:

### 1. Primary Models

- `URP_MASTER_MODEL_ID`: The primary "Brain" or "Architect" model responsible for high-level reasoning, planning, and task decomposition.
- `URP_GATE_MODEL_ID`: The "Gatekeeper" or "Noise Filter" model that filters irrelevant information to present concise context.
- `URP_WORKER_MODEL_ID`: Model for execution tasks in worker containers.

### 2. Specialized Models by Function

- `URP_CODING_MODEL_ID`: Optimized for coding tasks
- `URP_REASONING_MODEL_ID`: Optimized for complex reasoning
- `URP_FAST_MODEL_ID`: For quick responses and filtering
- `URP_VISION_MODEL_ID`: For vision-based tasks
- `URP_LONG_CONTEXT_MODEL_ID`: For tasks requiring large context windows

### 3. Fallback Models

- `URP_FALLBACK_MODEL_ID`: Used when primary models fail to initialize

### 4. Default Models (Fallbacks)

- `URP_DEFAULT_MASTER_MODEL`: Default if `URP_MASTER_MODEL_ID` is not set
- `URP_DEFAULT_GATE_MODEL`: Default if `URP_GATE_MODEL_ID` is not set

## Setting Up Your Configuration

### Step 1: Copy the template
```bash
cp .env.example ~/.urp-go/.env
```

### Step 2: Add your API keys
```bash
# API keys
ANTHROPIC_API_KEY="your-anthropic-key"
OPENAI_API_KEY="your-openai-key"
DEEPSEEK_API_KEY="your-deepseek-key"
```

### Step 3: Configure your models
```bash
# Primary models
URP_MASTER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
URP_GATE_MODEL_ID="gpt-4o-mini"
URP_WORKER_MODEL_ID="deepseek-chat"

# Specialized models
URP_CODING_MODEL_ID="deepseek-coder"
URP_REASONING_MODEL_ID="o1"
URP_FAST_MODEL_ID="gpt-4o-mini"
```

## Model Selection Logic

The system follows this priority when selecting a model:

### For Master Models (using the provider system):
1. Check the specific function's primary variable (e.g., `URP_MASTER_MODEL_ID`)
2. If initialization fails, try models in the fallback sequence:
   - `URP_DEFAULT_MASTER_MODEL`
   - `gpt-4o-mini`
   - `gpt-4o`
3. If all models fail, return an error

### For Gate/Filter Models (using HTTP clients):
1. Check the specific model variable passed to the client
2. If not set, use `URP_GATE_MODEL_ID`
3. If not set, use `URP_DEFAULT_GATE_MODEL`
4. If not set, default to `gpt-4o-mini`

### Fallback Strategy

The system has multi-level fallbacks:
- **Primary model**: Your explicitly configured model
- **Fallback models**: A prioritized list of alternatives
- **Hardcoded defaults**: Reliable models that should always work

## Provider URLs (Optional)

You can specify custom endpoints for specific models:

- `URP_MASTER_MODEL_URL`: Custom URL for the master model (overrides provider base URL)
- `URP_GATE_MODEL_URL`: Custom URL for the gate model
- `URP_WORKER_MODEL_URL`: Custom URL for the worker model

## API Keys (Optional)

You can specify custom API keys for specific models:

- `URP_MASTER_MODEL_API_KEY`: Custom API key for the master model
- `URP_GATE_MODEL_API_KEY`: Custom API key for the gate model
- `URP_WORKER_MODEL_API_KEY`: Custom API key for the worker model

## Model Naming Convention

Models should be specified in the format: `provider/model-name`
Examples:
- `anthropic/claude-sonnet-4-5-20250929`
- `openai/gpt-4o`
- `deepseek/deepseek-chat`
- `google/gemini-pro`

## Troubleshooting

1. **Model not found**: Check that your model ID matches the expected format and that you have the appropriate API key configured.

2. **Permission denied**: Ensure your API key has access to the specified model.

3. **Slow responses**: Consider using the `URP_FAST_MODEL_ID` for quick tasks or switching to a faster model for the gate function.

4. **Configuration not taking effect**: Restart the application after making changes to your `.env` file.

## Example Configuration

```bash
# API Keys
ANTHROPIC_API_KEY="sk-ant-..."
OPENAI_API_KEY="sk-or-v1-..."
HF_TOKEN="hf_..."

# Primary Models
URP_MASTER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
URP_GATE_MODEL_ID="gpt-4o-mini"
URP_WORKER_MODEL_ID="deepseek-chat"

# Fallback Models
URP_FALLBACK_MODEL_ID="gpt-4o-mini"

# Specialized Models
URP_CODING_MODEL_ID="deepseek-coder"
URP_REASONING_MODEL_ID="o1"
URP_FAST_MODEL_ID="gpt-4o-mini"

# Custom Provider Configuration (optional)
# URP_MASTER_MODEL_URL="https://my-custom-provider.com/v1"
```