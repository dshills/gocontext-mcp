package sample

import (
	"errors"
	"fmt"
	"log"
)

// ErrorHandler provides centralized error handling
type ErrorHandler struct {
	logger *log.Logger
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(logger *log.Logger) *ErrorHandler {
	return &ErrorHandler{logger: logger}
}

// Handle processes an error and returns an appropriate response
func (h *ErrorHandler) Handle(err error) error {
	if err == nil {
		return nil
	}

	// Log the error
	if h.logger != nil {
		h.logger.Printf("Error occurred: %v", err)
	}

	// Wrap with context
	return fmt.Errorf("operation failed: %w", err)
}

// IsRetryable checks if an error can be retried
func (h *ErrorHandler) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for known retryable errors
	return errors.Is(err, ErrTokenExpired)
}

// Recover recovers from panics and converts to errors
func (h *ErrorHandler) Recover() error {
	if r := recover(); r != nil {
		if h.logger != nil {
			h.logger.Printf("Recovered from panic: %v", r)
		}
		return fmt.Errorf("panic recovered: %v", r)
	}
	return nil
}

// WrapWithContext adds context to an error
func WrapWithContext(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// UnwrapAll unwraps all nested errors
func UnwrapAll(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}
