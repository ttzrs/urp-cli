package domain

// Provider represents an LLM provider
type Provider struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Models []Model `json:"models"`
}

type Model struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ContextSize int     `json:"contextSize"`
	InputCost   float64 `json:"inputCost"`  // per 1M tokens
	OutputCost  float64 `json:"outputCost"` // per 1M tokens
}
