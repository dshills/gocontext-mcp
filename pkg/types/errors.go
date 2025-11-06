package types

import "errors"

// Domain errors for type validation
var (
	// Search result errors
	ErrInvalidChunkID        = errors.New("invalid chunk ID")
	ErrInvalidRank           = errors.New("rank must be >= 1")
	ErrInvalidRelevanceScore = errors.New("relevance score must be between 0 and 1")
	ErrMissingFileInfo       = errors.New("file info is required")
	ErrEmptyContent          = errors.New("content cannot be empty")
)
