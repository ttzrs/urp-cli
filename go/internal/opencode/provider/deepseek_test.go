package provider

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepSeekProviderRegistration(t *testing.T) {
	// Test that DeepSeek provider is registered in the factory
	f := NewFactory()
	
	// Temporarily set the DEEPSEEK_API_KEY for testing
	origKey := os.Getenv("DEEPSEEK_API_KEY")
	os.Setenv("DEEPSEEK_API_KEY", "test-key")
	defer os.Setenv("DEEPSEEK_API_KEY", origKey)
	
	// Test Create method with ProviderDeepSeek
	p, err := f.Create(ProviderDeepSeek)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "deepseek-direct", p.ID())
}

func TestDeepSeekProviderCreationByID(t *testing.T) {
	// Test that DeepSeek provider can be created by ID
	f := NewFactory()
	
	// Temporarily set the DEEPSEEK_API_KEY for testing
	origKey := os.Getenv("DEEPSEEK_API_KEY")
	os.Setenv("DEEPSEEK_API_KEY", "test-key")
	defer os.Setenv("DEEPSEEK_API_KEY", origKey)
	
	// Test CreateByID with "deepseek"
	p, err := f.CreateByID("deepseek")
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "deepseek-direct", p.ID())
	
	// Test CreateByID with "deepseek-chat"
	p, err = f.CreateByID("deepseek-chat")
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "deepseek-direct", p.ID())
	
	// Test CreateByID with "deepseek-coder"
	p, err = f.CreateByID("deepseek-coder")
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "deepseek-direct", p.ID())
}

func TestEnvKeyDeepSeek(t *testing.T) {
	// Temporarily set the DEEPSEEK_API_KEY for testing
	origKey := os.Getenv("DEEPSEEK_API_KEY")
	os.Setenv("DEEPSEEK_API_KEY", "test-key-from-env")
	defer os.Setenv("DEEPSEEK_API_KEY", origKey)
	
	key := envKey(ProviderDeepSeek)
	assert.Equal(t, "test-key-from-env", key)
}

func TestEnvBaseURLDeepSeek(t *testing.T) {
	// Temporarily set the DEEPSEEK_BASE_URL for testing
	origURL := os.Getenv("DEEPSEEK_BASE_URL")
	os.Setenv("DEEPSEEK_BASE_URL", "https://test-deepseek-api.com/v1")
	defer os.Setenv("DEEPSEEK_BASE_URL", origURL)
	
	url := envBaseURL(ProviderDeepSeek)
	assert.Equal(t, "https://test-deepseek-api.com/v1", url)
}