package modelservice

import (
	"os"

	"github.com/joss/urp/internal/opencode/domain"
)

// DefaultService is the global model service
var DefaultService = NewService()

func init() {
	// Register static fetchers for built-in providers
	registerStaticFetchers()
	// Register dynamic fetchers from environment
	registerDynamicFetchers()
}

func registerStaticFetchers() {
	// OpenAI static models (from provider/openai.go)
	openaiModels := []domain.Model{
		{ID: "gpt-5.1", Name: "GPT-5.1", ShortCode: "51", ContextSize: 200000, InputCost: 5, OutputCost: 20},
		{ID: "gpt-5", Name: "GPT-5", ShortCode: "5", ContextSize: 200000, InputCost: 5, OutputCost: 20},
		{ID: "gpt-4o", Name: "GPT-4o", ShortCode: "4o", ContextSize: 128000, InputCost: 2.5, OutputCost: 10},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ShortCode: "4om", ContextSize: 128000, InputCost: 0.15, OutputCost: 0.6},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ShortCode: "4tb", ContextSize: 128000, InputCost: 10, OutputCost: 30},
		{ID: "o1", Name: "o1", ShortCode: "o1", ContextSize: 200000, InputCost: 15, OutputCost: 60},
		{ID: "o1-mini", Name: "o1 Mini", ShortCode: "o1m", ContextSize: 128000, InputCost: 3, OutputCost: 12},
	}
	DefaultService.RegisterFetcher(NewStaticFetcher(SourceOpenAI, openaiModels))

	// Anthropic static models (from provider/anthropic.go)
	anthropicModels := []domain.Model{
		{ID: "claude-opus-4-5-20251101", Name: "Claude Opus 4.5", ShortCode: "op45", ContextSize: 200000, InputCost: 15, OutputCost: 75},
		{ID: "claude-opus-4-5-thinking", Name: "Claude Opus 4.5 Thinking", ShortCode: "op5t", ContextSize: 200000, InputCost: 15, OutputCost: 75},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ShortCode: "h45", ContextSize: 200000, InputCost: 0.8, OutputCost: 4},
		{ID: "claude-sonnet-4-5-20250929", Name: "Claude Sonnet 4.5", ShortCode: "sn45", ContextSize: 200000, InputCost: 3, OutputCost: 15},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ShortCode: "sn4", ContextSize: 200000, InputCost: 3, OutputCost: 15},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ShortCode: "op4", ContextSize: 200000, InputCost: 15, OutputCost: 75},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", ShortCode: "s35", ContextSize: 200000, InputCost: 3, OutputCost: 15},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ShortCode: "h35", ContextSize: 200000, InputCost: 0.8, OutputCost: 4},
	}
	DefaultService.RegisterFetcher(NewStaticFetcher(SourceAnthropic, anthropicModels))

	// Google static models (from provider/google.go)
	googleModels := []domain.Model{
		{ID: "gemini-2.0-flash-exp", Name: "Gemini 2.0 Flash", ShortCode: "g2f", ContextSize: 1000000, InputCost: 0, OutputCost: 0},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", ShortCode: "g1p", ContextSize: 2000000, InputCost: 1.25, OutputCost: 5},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", ShortCode: "g1f", ContextSize: 1000000, InputCost: 0.075, OutputCost: 0.3},
		{ID: "gemini-1.5-flash-8b", Name: "Gemini 1.5 Flash 8B", ShortCode: "g8b", ContextSize: 1000000, InputCost: 0.0375, OutputCost: 0.15},
	}
	DefaultService.RegisterFetcher(NewStaticFetcher(SourceGoogle, googleModels))

	// DeepSeek static models (from provider/deepseek.go)
	deepseekModels := []domain.Model{
		{ID: "deepseek-chat", Name: "DeepSeek Chat", ShortCode: "dsc", ContextSize: 64000, InputCost: 0.14, OutputCost: 0.28},
		{ID: "deepseek-coder", Name: "DeepSeek Coder", ShortCode: "dco", ContextSize: 64000, InputCost: 0.14, OutputCost: 0.28},
		{ID: "deepseek-reasoner", Name: "DeepSeek Reasoner", ShortCode: "dsr", ContextSize: 64000, InputCost: 0.55, OutputCost: 2.19},
	}
	DefaultService.RegisterFetcher(NewStaticFetcher(SourceDeepSeek, deepseekModels))
}

func registerDynamicFetchers() {
	// Proxy (unified) from environment
	if baseURL := os.Getenv("UNIFIED_BASE_URL"); baseURL != "" {
		apiKey := os.Getenv("UNIFIED_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("PROXY_API_KEY")
		}
		if apiKey != "" {
			fetcher := NewOpenAICompatibleFetcher(SourceProxy, baseURL, apiKey)
			DefaultService.RegisterFetcher(fetcher)
		}
	}

	// OpenAI direct endpoint
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey != "" {
			fetcher := NewOpenAICompatibleFetcher(SourceOpenAI, baseURL, apiKey)
			DefaultService.RegisterFetcher(fetcher)
		}
	}

	// Anthropic direct endpoint (may not have /models endpoint, but try)
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey != "" {
			fetcher := NewOpenAICompatibleFetcher(SourceAnthropic, baseURL, apiKey)
			DefaultService.RegisterFetcher(fetcher)
		}
	}

	// DeepSeek direct endpoint
	if baseURL := os.Getenv("DEEPSEEK_BASE_URL"); baseURL != "" {
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey != "" {
			fetcher := NewOpenAICompatibleFetcher(SourceDeepSeek, baseURL, apiKey)
			DefaultService.RegisterFetcher(fetcher)
		}
	}
}
