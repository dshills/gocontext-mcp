package embedder

import (
	"context"
	"time"
)

// RetryConfig configures exponential backoff retry behavior
type RetryConfig struct {
	MaxRetries int           // Maximum number of retry attempts
	BaseDelay  time.Duration // Initial delay between retries
	MaxDelay   time.Duration // Maximum delay between retries
	Multiplier float64       // Exponential backoff multiplier
}

// DefaultRetryConfig returns sensible defaults for API retry
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: MaxRetries,
		BaseDelay:  time.Duration(InitialBackoffMs) * time.Millisecond,
		MaxDelay:   time.Duration(MaxBackoffMs) * time.Millisecond,
		Multiplier: BackoffMultiplier,
	}
}

// retryWithBackoff executes a function with exponential backoff retry logic
// The function fn should return (result, error). Retry is skipped on context cancellation.
func retryWithBackoff[T any](ctx context.Context, config RetryConfig, fn func() (T, error)) (T, error) {
	var lastErr error
	var zero T
	backoff := config.BaseDelay

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		// Apply exponential backoff before next retry
		if attempt < config.MaxRetries-1 {
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(backoff):
				backoff = time.Duration(float64(backoff) * config.Multiplier)
				if backoff > config.MaxDelay {
					backoff = config.MaxDelay
				}
			}
		}
	}

	return zero, lastErr
}
