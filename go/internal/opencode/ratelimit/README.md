# Rate Limit Detection and Provider Switching System

## Overview

The URP system now includes an intelligent rate limit detection and provider switching mechanism that automatically detects when API rate limits are reached and switches to an alternative provider until the limits reset.

## Components

### 1. RateLimit Detector (`detector.go`)
- Detects rate limit errors by analyzing error messages for common patterns
- Extracts reset time information from error messages and HTTP headers
- Supports common rate limit indicators like "429", "rate limit", "quota exceeded", etc.

### 2. Provider Manager (`manager.go`)
- Manages switching between primary and alternative providers
- Monitors reset times and automatically restores the primary provider when limits reset
- Provides status information about the current provider state

## How It Works

### Detection Phase
1. When an error occurs during an API call, the system checks if it's a rate limit error
2. It looks for patterns like:
   - HTTP 429 status code
   - "rate limit exceeded" 
   - "quota exceeded"
   - "Too many requests"
   - "requests per minute/hour/day exceeded"
3. If a rate limit error is detected, it extracts the reset time

### Switching Phase  
1. Automatically switches to the alternative provider
2. Continues operations with the alternative provider
3. Schedules a check to restore the primary provider at reset time

### Restoration Phase
1. Monitors for when the rate limit reset time is reached
2. Automatically restores the primary provider when limits reset
3. Continues operations with the primary provider

## Integration with Agent

The system is integrated with the Agent through:

1. **Functional Options**: `WithRateLimitManager()` option
2. **Automatic Setup**: If DeepSeek provider is available, rate limit manager is created automatically
3. **Error Handling**: Errors from API calls are checked for rate limit conditions
4. **Provider Selection**: Uses active provider from rate limit manager

## Usage

The system works automatically when you configure multiple providers:

```go
// Create providers
primaryProvider := provider.NewAnthropic(primaryAPIKey, "")
alternativeProvider := provider.NewOpenAI(alternativeAPIKey, baseURL)

// Create rate limit manager
rateLimitManager := ratelimit.NewProviderManager(primaryProvider, alternativeProvider)

// Create agent with rate limit manager
agent := agent.New(
    config,
    primaryProvider,
    tools,
    agent.WithRateLimitManager(rateLimitManager),
    agent.WithDeepSeekProvider(alternativeProvider),
)
```

## Configuration

The system supports callbacks for monitoring provider switches:

```go
rateLimitManager.WithSwitchCallback(func(newProvider llm.Provider) {
    fmt.Printf("Switched to alternative provider: %s\n", newProvider.ID())
})

rateLimitManager.WithRestoreCallback(func(originalProvider llm.Provider) {
    fmt.Printf("Restored primary provider: %s\n", originalProvider.ID())
})
```

## Error Pattern Detection

The system detects various rate limit error patterns:
- HTTP status code 429
- "Rate limit exceeded"
- "Too many requests" 
- "Quota exceeded"
- "requests per [time unit] exceeded"
- "API limit reached"
- "Usage limit exceeded"
- "Over quota"
- "Try again in [time]"

## Reset Time Detection

The system attempts to extract reset times from:
1. HTTP headers (X-RateLimit-Reset, Retry-After)
2. Error message text (e.g., "Try again in 1 hour")
3. Common time patterns in responses
4. Defaults to 1 hour if no reset time found

## Thread Safety

All components are thread-safe and can be used in concurrent environments.

## Cleanup

Don't forget to call `agent.Close()` to clean up the rate limit manager resources.