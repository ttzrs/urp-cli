package modelservice

import (
	"context"
	"fmt"
	"sync"

	"github.com/joss/urp/internal/opencode/domain"
)

// Service manages models from multiple sources
type Service struct {
	mu       sync.RWMutex
	fetchers map[Source]Fetcher
	cache    map[Source][]domain.Model
	// shortCode to model mapping
	shortCodeMap map[string]ModelWithSource
}

// NewService creates a new model service
func NewService() *Service {
	return &Service{
		fetchers:     make(map[Source]Fetcher),
		cache:        make(map[Source][]domain.Model),
		shortCodeMap: make(map[string]ModelWithSource),
	}
}

// RegisterFetcher adds a fetcher for a source
func (s *Service) RegisterFetcher(fetcher Fetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetchers[fetcher.Source()] = fetcher
}

// Refresh fetches models from all registered fetchers
func (s *Service) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear cache
	s.cache = make(map[Source][]domain.Model)
	s.shortCodeMap = make(map[string]ModelWithSource)

	var errors []error
	for source, fetcher := range s.fetchers {
		models, err := fetcher.Fetch()
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", source, err))
			continue
		}
		s.cache[source] = models

		// Build shortcode mapping
		for _, model := range models {
			// Ensure shortcode is set
			if model.ShortCode == "" {
				model.ShortCode = generateShortCode(model.ID)
			}
			// Make shortcode unique by appending number if duplicate
			shortCode := model.ShortCode
			counter := 1
			for {
				if _, exists := s.shortCodeMap[shortCode]; !exists {
					break
				}
				shortCode = fmt.Sprintf("%s%d", model.ShortCode, counter)
				counter++
			}
			if shortCode != model.ShortCode {
				model.ShortCode = shortCode
			}
			s.shortCodeMap[shortCode] = ModelWithSource{
				Model:  model,
				Source: source,
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to fetch some models: %v", errors)
	}
	return nil
}

// ListBySource returns models grouped by source
func (s *Service) ListBySource() map[Source][]domain.Model {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[Source][]domain.Model)
	for source, models := range s.cache {
		result[source] = models
	}
	return result
}

// AllModels returns all models across all sources
func (s *Service) AllModels() []ModelWithSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ModelWithSource, 0, len(s.shortCodeMap))
	for _, m := range s.shortCodeMap {
		result = append(result, m)
	}
	return result
}

// ResolveShortCode finds a model by its 3-letter code
func (s *Service) ResolveShortCode(shortCode string) (ModelWithSource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	model, ok := s.shortCodeMap[shortCode]
	return model, ok
}

// FindModel finds a model by ID across all sources
func (s *Service) FindModel(modelID string) (ModelWithSource, bool) {
	s.mu.RLock()
	// Check if cache is empty and refresh if needed
	isEmpty := len(s.shortCodeMap) == 0 && len(s.fetchers) > 0
	s.mu.RUnlock()

	if isEmpty {
		// Cache is empty, try to refresh
		_ = s.Refresh(context.Background())
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.shortCodeMap {
		if m.ID == modelID {
			return m, true
		}
	}
	return ModelWithSource{}, false
}

// GetProviderForModel returns the provider ID for a given model ID
func (s *Service) GetProviderForModel(modelID string) (Source, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.shortCodeMap {
		if m.ID == modelID {
			return m.Source, true
		}
	}
	return "", false
}
