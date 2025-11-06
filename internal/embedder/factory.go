package embedder

import (
	"fmt"
	"os"
	"strings"
)

// Config holds embedder configuration
type Config struct {
	Provider  string
	APIKey    string
	CacheSize int
}

// NewFromEnv creates an embedder based on environment variables
// Priority:
// 1. GOCONTEXT_EMBEDDING_PROVIDER (jina, openai, local)
// 2. Check for API keys: JINA_API_KEY, OPENAI_API_KEY
// 3. Default to local if no API keys found
func NewFromEnv() (Embedder, error) {
	provider := os.Getenv("GOCONTEXT_EMBEDDING_PROVIDER")
	jinaKey := os.Getenv("JINA_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	cache := NewCache(10000) // Default cache size

	// Explicit provider selection
	if provider != "" {
		provider = strings.ToLower(provider)
		switch provider {
		case ProviderJina:
			return NewJinaProvider(jinaKey, cache)
		case ProviderOpenAI:
			return NewOpenAIProvider(openaiKey, cache)
		case ProviderLocal:
			return NewLocalProvider(cache)
		default:
			return nil, fmt.Errorf("%w: unknown provider %s", ErrUnsupportedModel, provider)
		}
	}

	// Auto-detect based on available API keys
	if jinaKey != "" {
		return NewJinaProvider(jinaKey, cache)
	}
	if openaiKey != "" {
		return NewOpenAIProvider(openaiKey, cache)
	}

	// Fallback to local provider
	return NewLocalProvider(cache)
}

// New creates an embedder with explicit configuration
func New(cfg Config) (Embedder, error) {
	var cache *Cache
	if cfg.CacheSize > 0 {
		cache = NewCache(cfg.CacheSize)
	}

	provider := strings.ToLower(cfg.Provider)
	switch provider {
	case ProviderJina:
		return NewJinaProvider(cfg.APIKey, cache)
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg.APIKey, cache)
	case ProviderLocal:
		return NewLocalProvider(cache)
	default:
		return nil, fmt.Errorf("%w: unknown provider %s", ErrUnsupportedModel, cfg.Provider)
	}
}

// DetectProvider returns the provider that would be used based on current environment
func DetectProvider() string {
	provider := os.Getenv("GOCONTEXT_EMBEDDING_PROVIDER")
	if provider != "" {
		return strings.ToLower(provider)
	}

	if os.Getenv("JINA_API_KEY") != "" {
		return ProviderJina
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return ProviderOpenAI
	}

	return ProviderLocal
}
