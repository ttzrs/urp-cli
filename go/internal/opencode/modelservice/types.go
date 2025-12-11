package modelservice

import (
	"github.com/joss/urp/internal/opencode/domain"
)

// Source identifies where a model comes from
type Source string

const (
	SourceProxy     Source = "proxy"
	SourceDeepSeek  Source = "deepseek"
	SourceOpenAI    Source = "openai"
	SourceAnthropic Source = "anthropic"
	SourceGoogle    Source = "google"
)

// ModelWithSource extends domain.Model with source information
type ModelWithSource struct {
	domain.Model
	Source Source
}

// Fetcher retrieves models from a specific source
type Fetcher interface {
	Source() Source
	Fetch() ([]domain.Model, error)
}

// StaticFetcher returns a fixed list of models
type StaticFetcher struct {
	source Source
	models []domain.Model
}

func NewStaticFetcher(source Source, models []domain.Model) *StaticFetcher {
	return &StaticFetcher{source: source, models: models}
}

func (f *StaticFetcher) Source() Source { return f.source }
func (f *StaticFetcher) Fetch() ([]domain.Model, error) {
	return f.models, nil
}
