package types

// SearchResult represents a single search result with relevance information
type SearchResult struct {
	// Identification
	ChunkID int64
	Rank    int // Position in result set (1-based)

	// Scoring
	RelevanceScore float64 // Combined score from vector + BM25 + RRF

	// Metadata
	Symbol  *Symbol // Nullable - may not have an associated symbol
	File    *FileInfo
	Content string // Chunk content
	Context string // Combined context before and after
}

// FileInfo contains file metadata for a search result
type FileInfo struct {
	Path      string // Relative to project root
	Package   string
	StartLine int
	EndLine   int
}

// Validate checks if the search result is valid
func (sr *SearchResult) Validate() error {
	if sr.ChunkID == 0 {
		return ErrInvalidChunkID
	}

	if sr.Rank < 1 {
		return ErrInvalidRank
	}

	if sr.RelevanceScore < 0 || sr.RelevanceScore > 1 {
		return ErrInvalidRelevanceScore
	}

	if sr.File == nil {
		return ErrMissingFileInfo
	}

	if sr.Content == "" {
		return ErrEmptyContent
	}

	return nil
}
