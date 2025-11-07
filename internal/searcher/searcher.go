package searcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/storage"
	"github.com/dshills/gocontext-mcp/pkg/types"
)

// SearchMode defines how search is performed
type SearchMode string

const (
	SearchModeHybrid  SearchMode = "hybrid"  // Vector + BM25 with RRF
	SearchModeVector  SearchMode = "vector"  // Vector similarity only
	SearchModeKeyword SearchMode = "keyword" // BM25 text search only
)

// SearchRequest contains parameters for a search operation
type SearchRequest struct {
	Query       string
	Limit       int
	Mode        SearchMode
	Filters     *storage.SearchFilters
	ProjectID   int64
	UseCache    bool // Whether to use query cache
	CacheTTL    time.Duration
	RRFConstant float64 // k value for Reciprocal Rank Fusion (default 60)
}

// SearchResponse contains search results and metadata
type SearchResponse struct {
	Results       []types.SearchResult
	TotalResults  int
	SearchMode    SearchMode
	Duration      time.Duration
	CacheHit      bool
	VectorResults int
	TextResults   int
}

// cacheEntry represents a cached search response with expiration time
type cacheEntry struct {
	response  *SearchResponse
	expiresAt time.Time
}

// Searcher coordinates search operations across vector and text search
type Searcher struct {
	storage  storage.Storage
	embedder embedder.Embedder
	cache    *lru.Cache[[32]byte, *cacheEntry]
	cacheMu  sync.RWMutex
}

// NewSearcher creates a new Searcher instance
func NewSearcher(storage storage.Storage, embedder embedder.Embedder) *Searcher {
	// Create LRU cache with 1000 entry limit
	// Cache will automatically evict least recently used entries
	cache, err := lru.New[[32]byte, *cacheEntry](1000)
	if err != nil {
		// This should never happen with valid size parameter
		panic(fmt.Sprintf("failed to create LRU cache: %v", err))
	}

	return &Searcher{
		storage:  storage,
		embedder: embedder,
		cache:    cache,
	}
}

// Search performs a search based on the request parameters
func (s *Searcher) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	startTime := time.Now()

	// Validate searcher state
	if s.embedder == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}

	// Validate request
	if err := s.validateRequest(&req); err != nil {
		return nil, fmt.Errorf("invalid search request: %w", err)
	}

	// Check cache if enabled
	if req.UseCache {
		cached, err := s.checkCache(ctx, req)
		if err == nil && cached != nil {
			cached.CacheHit = true
			cached.Duration = time.Since(startTime)
			return cached, nil
		}
	}

	// Perform search based on mode
	var response *SearchResponse
	var err error

	switch req.Mode {
	case SearchModeHybrid:
		response, err = s.hybridSearch(ctx, req)
	case SearchModeVector:
		response, err = s.vectorSearch(ctx, req)
	case SearchModeKeyword:
		response, err = s.keywordSearch(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported search mode: %s", req.Mode)
	}

	if err != nil {
		return nil, err
	}

	response.Duration = time.Since(startTime)
	response.SearchMode = req.Mode

	// Store in cache if enabled
	if req.UseCache && len(response.Results) > 0 {
		// Cache storage is stubbed out for now
		_ = s.storeInCache(ctx, req, response)
	}

	return response, nil
}

// searchResult holds results from concurrent search operations
type searchResult struct {
	vectorResults []storage.VectorResult
	textResults   []storage.TextResult
	err           error
}

// runVectorSearch executes vector search in a goroutine
func (s *Searcher) runVectorSearch(ctx context.Context, req SearchRequest, resultChan chan<- searchResult) {
	var res searchResult
	embReq := embedder.EmbeddingRequest{Text: req.Query}
	embedding, err := s.embedder.GenerateEmbedding(ctx, embReq)
	if err != nil {
		res.err = fmt.Errorf("failed to generate query embedding: %w", err)
	} else {
		res.vectorResults, res.err = s.storage.SearchVector(ctx, req.ProjectID, embedding.Vector, req.Limit*2, req.Filters)
	}
	select {
	case resultChan <- res:
	case <-ctx.Done():
	}
}

// runTextSearch executes text search in a goroutine
func (s *Searcher) runTextSearch(ctx context.Context, req SearchRequest, resultChan chan<- searchResult) {
	var res searchResult
	res.textResults, res.err = s.storage.SearchText(ctx, req.ProjectID, req.Query, req.Limit*2, req.Filters)
	select {
	case resultChan <- res:
	case <-ctx.Done():
	}
}

// hybridSearch combines vector and BM25 search using Reciprocal Rank Fusion
func (s *Searcher) hybridSearch(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	vectorChan := make(chan searchResult, 1)
	textChan := make(chan searchResult, 1)

	go s.runVectorSearch(ctx, req, vectorChan)
	go s.runTextSearch(ctx, req, textChan)

	// Wait for both searches
	var vectorRes, textRes searchResult
	var vectorDone, textDone bool
	for !vectorDone || !textDone {
		select {
		case vectorRes = <-vectorChan:
			vectorDone = true
		case textRes = <-textChan:
			textDone = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Check for errors (allow one to fail)
	if vectorRes.err != nil && textRes.err != nil {
		return nil, fmt.Errorf("both searches failed: vector=%w, text=%v", vectorRes.err, textRes.err)
	}

	// Apply RRF and fetch results
	rrf := s.applyRRF(vectorRes.vectorResults, textRes.textResults, req.RRFConstant)
	results, err := s.fetchResults(ctx, rrf, req.Limit)
	if err != nil {
		return nil, err
	}

	return &SearchResponse{
		Results:       results,
		TotalResults:  len(results),
		VectorResults: len(vectorRes.vectorResults),
		TextResults:   len(textRes.textResults),
	}, nil
}

// vectorSearch performs only vector similarity search
func (s *Searcher) vectorSearch(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	embReq := embedder.EmbeddingRequest{
		Text: req.Query,
	}
	embedding, err := s.embedder.GenerateEmbedding(ctx, embReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	vectorResults, err := s.storage.SearchVector(ctx, req.ProjectID, embedding.Vector, req.Limit, req.Filters)
	if err != nil {
		return nil, err
	}

	// Convert to unified format
	rankedResults := make([]rankedResult, len(vectorResults))
	for i, vr := range vectorResults {
		rankedResults[i] = rankedResult{
			chunkID: vr.ChunkID,
			score:   vr.SimilarityScore,
			rank:    i + 1,
		}
	}

	results, err := s.fetchResults(ctx, rankedResults, req.Limit)
	if err != nil {
		return nil, err
	}

	return &SearchResponse{
		Results:       results,
		TotalResults:  len(results),
		VectorResults: len(vectorResults),
	}, nil
}

// keywordSearch performs only BM25 text search
func (s *Searcher) keywordSearch(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	textResults, err := s.storage.SearchText(ctx, req.ProjectID, req.Query, req.Limit, req.Filters)
	if err != nil {
		return nil, err
	}

	// Convert to unified format
	rankedResults := make([]rankedResult, len(textResults))
	for i, tr := range textResults {
		rankedResults[i] = rankedResult{
			chunkID: tr.ChunkID,
			score:   tr.BM25Score,
			rank:    i + 1,
		}
	}

	results, err := s.fetchResults(ctx, rankedResults, req.Limit)
	if err != nil {
		return nil, err
	}

	return &SearchResponse{
		Results:      results,
		TotalResults: len(results),
		TextResults:  len(textResults),
	}, nil
}

// rankedResult represents a chunk with its relevance score and rank
type rankedResult struct {
	chunkID int64
	score   float64
	rank    int
}

// applyRRF applies Reciprocal Rank Fusion to combine vector and text results
// RRF formula: RRF(d) = Σ 1/(k + rank(d))
func (s *Searcher) applyRRF(vectorResults []storage.VectorResult, textResults []storage.TextResult, k float64) []rankedResult {
	if k == 0 {
		k = 60 // Default RRF constant
	}

	// Combine scores by chunk ID
	scores := make(map[int64]float64)

	// Add vector results
	for rank, vr := range vectorResults {
		scores[vr.ChunkID] += 1.0 / (k + float64(rank+1))
	}

	// Add text results
	for rank, tr := range textResults {
		scores[tr.ChunkID] += 1.0 / (k + float64(rank+1))
	}

	// Convert to ranked results
	results := make([]rankedResult, 0, len(scores))
	for chunkID, score := range scores {
		results = append(results, rankedResult{
			chunkID: chunkID,
			score:   score,
		})
	}

	// Sort by score (descending)
	sortRankedResults(results)

	// Assign ranks
	for i := range results {
		results[i].rank = i + 1
	}

	return results
}

// fetchResults retrieves full chunk data and metadata for ranked results
func (s *Searcher) fetchResults(ctx context.Context, ranked []rankedResult, limit int) ([]types.SearchResult, error) {
	if limit > len(ranked) {
		limit = len(ranked)
	}

	results := make([]types.SearchResult, 0, limit)

	for i := 0; i < limit; i++ {
		rr := ranked[i]

		// Get chunk with joins
		chunk, err := s.storage.GetChunk(ctx, rr.chunkID)
		if err != nil {
			continue // Skip chunks that can't be loaded
		}

		// Get file info
		file, err := s.storage.GetFileByID(ctx, chunk.FileID)
		if err != nil {
			continue
		}

		// Get symbol info if available
		var symbol *types.Symbol
		if chunk.SymbolID != nil {
			storageSymbol, err := s.storage.GetSymbol(ctx, *chunk.SymbolID)
			if err == nil {
				typesSymbol := storageSymbol.ToTypesSymbol()
				symbol = &typesSymbol
			}
		}

		// Build search result
		result := types.SearchResult{
			ChunkID:        rr.chunkID,
			Rank:           rr.rank,
			RelevanceScore: rr.score,
			Symbol:         symbol,
			File: &types.FileInfo{
				Path:      file.FilePath,
				Package:   file.PackageName,
				StartLine: chunk.StartLine,
				EndLine:   chunk.EndLine,
			},
			Content: chunk.Content,
			Context: fmt.Sprintf("%s\n\n%s", chunk.ContextBefore, chunk.ContextAfter),
		}

		results = append(results, result)
	}

	return results, nil
}

// validateRequest ensures search request is valid
func (s *Searcher) validateRequest(req *SearchRequest) error {
	if req.Query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	if req.Limit <= 0 {
		req.Limit = 10 // Default limit
	}

	if req.Limit > 100 {
		req.Limit = 100 // Max limit
	}

	if req.Mode == "" {
		req.Mode = SearchModeHybrid // Default mode
	}

	if req.RRFConstant == 0 {
		req.RRFConstant = 60 // Default k value
	}

	if req.CacheTTL == 0 {
		req.CacheTTL = 1 * time.Hour // Default TTL
	}

	return nil
}

// checkCache looks up cached search results
func (s *Searcher) checkCache(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	hash := computeQueryHash(req)
	now := time.Now()

	s.cacheMu.RLock()
	entry, found := s.cache.Get(hash)

	if !found {
		s.cacheMu.RUnlock()
		return nil, fmt.Errorf("cache miss")
	}

	// Check if entry has expired while holding read lock to avoid race condition
	if now.After(entry.expiresAt) {
		s.cacheMu.RUnlock()

		// Remove expired entry - need write lock
		s.cacheMu.Lock()
		s.cache.Remove(hash)
		s.cacheMu.Unlock()
		return nil, fmt.Errorf("cache expired")
	}

	// Entry is valid - return a deep copy while still holding read lock
	// to ensure entry isn't modified during copy
	response := copySearchResponse(entry.response)
	s.cacheMu.RUnlock()

	return response, nil
}

// storeInCache saves search results to cache
func (s *Searcher) storeInCache(ctx context.Context, req SearchRequest, response *SearchResponse) error {
	hash := computeQueryHash(req)

	// Calculate expiration time using TTL from request
	expiresAt := time.Now().Add(req.CacheTTL)

	// Create cache entry with deep copy to prevent external modifications
	entry := &cacheEntry{
		response:  copySearchResponse(response),
		expiresAt: expiresAt,
	}

	s.cacheMu.Lock()
	s.cache.Add(hash, entry)
	s.cacheMu.Unlock()

	return nil
}

// copySearchResponse creates a deep copy of a SearchResponse
func copySearchResponse(src *SearchResponse) *SearchResponse {
	if src == nil {
		return nil
	}

	// Create new response with copied metadata
	dst := &SearchResponse{
		TotalResults:  src.TotalResults,
		SearchMode:    src.SearchMode,
		Duration:      src.Duration,
		CacheHit:      src.CacheHit,
		VectorResults: src.VectorResults,
		TextResults:   src.TextResults,
		Results:       make([]types.SearchResult, len(src.Results)),
	}

	// Deep copy each search result
	for i, result := range src.Results {
		dst.Results[i] = types.SearchResult{
			ChunkID:        result.ChunkID,
			Rank:           result.Rank,
			RelevanceScore: result.RelevanceScore,
			Content:        result.Content,
			Context:        result.Context,
		}

		// Copy Symbol pointer if it exists
		// Note: Symbol contains only primitive types and nested Position structs,
		// so shallow copy is sufficient. If Symbol is modified to include slice/map
		// fields in the future, this must be updated to deep copy those fields.
		if result.Symbol != nil {
			symbolCopy := *result.Symbol
			dst.Results[i].Symbol = &symbolCopy
		}

		// Copy FileInfo pointer if it exists
		// Note: FileInfo contains only primitive types, so shallow copy is sufficient.
		// If FileInfo is modified to include slice/map fields in the future, this must
		// be updated to deep copy those fields.
		if result.File != nil {
			fileCopy := *result.File
			dst.Results[i].File = &fileCopy
		}
	}

	return dst
}

// computeQueryHash computes a unique hash for a search request
func computeQueryHash(req SearchRequest) [32]byte {
	// Build deterministic string representation
	var data strings.Builder
	data.WriteString(req.Query)
	data.WriteString("|")
	data.WriteString(string(req.Mode))
	data.WriteString("|")
	data.WriteString(fmt.Sprintf("%d", req.ProjectID))

	// Add filters with stable serialization
	if req.Filters != nil {
		data.WriteString("|filters:")
		data.WriteString(strings.Join(req.Filters.SymbolTypes, ","))
		data.WriteString("|")
		data.WriteString(req.Filters.FilePattern)
		data.WriteString("|")
		data.WriteString(strings.Join(req.Filters.DDDPatterns, ","))
		data.WriteString("|")
		data.WriteString(strings.Join(req.Filters.Packages, ","))
		data.WriteString("|")
		data.WriteString(fmt.Sprintf("%.2f", req.Filters.MinRelevance))
	}

	return sha256.Sum256([]byte(data.String()))
}

// sortRankedResults sorts results by score in descending order
func sortRankedResults(results []rankedResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
}

// InvalidateCache removes cached queries for a specific project
func (s *Searcher) InvalidateCache(ctx context.Context, projectID int64) error {
	// Since we need to check each entry's project ID, we need to iterate through all keys
	// LRU cache doesn't support filtering, so we purge the entire cache
	// This is acceptable as cache invalidation typically happens on reindexing
	s.cacheMu.Lock()
	s.cache.Purge()
	s.cacheMu.Unlock()
	return nil
}

// EvictLRU removes least-used cache entries when cache size exceeds limit
func (s *Searcher) EvictLRU(ctx context.Context, maxEntries int) error {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// LRU cache handles eviction automatically when entries are added
	// This method is primarily for downsizing the cache capacity

	currentLen := s.cache.Len()
	if currentLen <= maxEntries {
		// No action needed - cache is within limits
		return nil
	}

	// NOTE: hashicorp/golang-lru doesn't support resizing existing cache
	// When downsizing is required, we intentionally clear the cache
	// This is acceptable because:
	// 1. Cache downsizing is rare (typically only on configuration changes)
	// 2. The cache will rebuild with most-recently-used entries
	// 3. This prevents memory issues when drastically reducing cache size
	newCache, err := lru.New[[32]byte, *cacheEntry](maxEntries)
	if err != nil {
		return fmt.Errorf("failed to create new cache: %w", err)
	}

	s.cache = newCache

	return nil
}
