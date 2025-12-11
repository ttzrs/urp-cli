// Package ratelimit provides rate limit detection and provider switching functionality.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/joss/urp/pkg/llm"
)

// ProviderManager manages rate limits and provider switching
type ProviderManager struct {
	detector         *Detector
	currentProvider  llm.Provider
	primaryProvider  llm.Provider
	alternativeProvider llm.Provider
	switchTime       time.Time
	resetTime        time.Time
	mu               sync.RWMutex
	ctx              context.Context
	cancelFunc       context.CancelFunc
	monitoring       bool
}

// NewProviderManager creates a new provider manager
func NewProviderManager(primary, alternative llm.Provider) *ProviderManager {
	ctx, cancelFunc := context.WithCancel(context.Background())

	manager := &ProviderManager{
		primaryProvider:     primary,
		alternativeProvider: alternative,
		currentProvider:     primary,
		ctx:                 ctx,
		cancelFunc:          cancelFunc,
	}

	// Initialize detector with callbacks
	manager.detector = NewDetector(primary, alternative).
		WithSwitchCallback(manager.onProviderSwitch).
		WithRestoreCallback(manager.onProviderRestore)

	return manager
}

// WithSwitchCallback sets a callback for when provider switching occurs
func (pm *ProviderManager) WithSwitchCallback(cb func(newProvider llm.Provider)) *ProviderManager {
	pm.detector.WithSwitchCallback(cb)
	return pm
}

// WithRestoreCallback sets a callback for when the original provider is restored
func (pm *ProviderManager) WithRestoreCallback(cb func(originalProvider llm.Provider)) *ProviderManager {
	pm.detector.WithRestoreCallback(cb)
	return pm
}

// GetProvider returns the current active provider
func (pm *ProviderManager) GetProvider() llm.Provider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentProvider
}

// CheckAndHandleError checks if an error is a rate limit error and handles it
func (pm *ProviderManager) CheckAndHandleError(err error) (llm.Provider, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rateLimitErr, isRateLimit := pm.detector.IsRateLimitError(err)
	if !isRateLimit {
		return pm.currentProvider, false
	}

	// Store reset time
	pm.resetTime = rateLimitErr.ResetTime
	pm.switchTime = time.Now()

	// Switch to alternative provider
	pm.currentProvider = pm.detector.SwitchToAlternative()
	
	// Start monitoring for reset if not already monitoring
	if !pm.monitoring {
		pm.monitoring = true
		go pm.monitorReset()
	}

	return pm.currentProvider, true
}

// onProviderSwitch is called when switching to alternative provider
func (pm *ProviderManager) onProviderSwitch(newProvider llm.Provider) {
	// Callback for when provider is switched
}

// onProviderRestore is called when restoring primary provider
func (pm *ProviderManager) onProviderRestore(originalProvider llm.Provider) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.currentProvider = originalProvider
	pm.switchTime = time.Time{}
	pm.resetTime = time.Time{}
}

// monitorReset monitors for when the rate limit resets
func (pm *ProviderManager) monitorReset() {
	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
			time.Sleep(30 * time.Second) // Check every 30 seconds
			
			pm.mu.Lock()
			resetTime := pm.resetTime
			pm.mu.Unlock()
			
			if !resetTime.IsZero() && time.Now().After(resetTime) {
				pm.restorePrimary()
				return
			}
		}
	}
}

// restorePrimary restores the primary provider
func (pm *ProviderManager) restorePrimary() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if pm.currentProvider != pm.alternativeProvider {
		// Already using primary or different provider
		return
	}
	
	// Restore primary provider
	pm.currentProvider = pm.detector.RestorePrimary()
	pm.switchTime = time.Time{}
	pm.resetTime = time.Time{}
	pm.monitoring = false
}

// Close stops the provider manager
func (pm *ProviderManager) Close() {
	pm.cancelFunc()
}

// GetStatus returns the current status of the provider manager
func (pm *ProviderManager) GetStatus() (currentProvider llm.Provider, isUsingAlternative bool, resetTime time.Time) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	isUsingAlternative = pm.currentProvider == pm.alternativeProvider
	return pm.currentProvider, isUsingAlternative, pm.resetTime
}