package searcher

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/storage"
	"github.com/dshills/gocontext-mcp/pkg/types"
)

// mockEmbedder implements the Embedder interface for testing
type mockEmbedder struct {
	generateFunc func(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error)
}

func (m *mockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}

	// Default mock: return simple vector
	vector := make([]float32, 384)
	for i := range vector {
		vector[i] = float32(i) * 0.01
	}

	return &embedder.Embedding{
		Vector:    vector,
		Dimension: 384,
		Model:     "mock-model",
		Provider:  "mock",
		Hash:      "mock-hash",
	}, nil
}

func (m *mockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
	embeddings := make([]*embedder.Embedding, len(req.Texts))
	for i, text := range req.Texts {
		vector := make([]float32, 384)
		for j := range vector {
			vector[j] = float32(i+j) * 0.01
		}
		embeddings[i] = &embedder.Embedding{
			Vector:    vector,
			Dimension: 384,
			Model:     "mock-model",
			Provider:  "mock",
			Hash:      embedder.ComputeHash(text),
		}
	}
	return &embedder.BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   "mock",
		Model:      "mock-model",
	}, nil
}

func (m *mockEmbedder) Dimension() int {
	return 384
}

func (m *mockEmbedder) Provider() string {
	return "mock"
}

func (m *mockEmbedder) Model() string {
	return "mock-model"
}

func (m *mockEmbedder) Close() error {
	return nil
}

// setupTestSearcher creates a searcher with in-memory storage and mock embedder
func setupTestSearcher(t *testing.T) (*Searcher, storage.Storage, *storage.Project) {
	t.Helper()

	// Create in-memory storage
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create test storage: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	// Create mock embedder
	embed := &mockEmbedder{}

	// Create searcher
	search := NewSearcher(store, embed)

	// Create test project
	ctx := context.Background()
	project := &storage.Project{
		RootPath:     "/test/search",
		ModuleName:   "github.com/test/search",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	return search, store, project
}

// TestNewSearcher verifies searcher creation
func TestNewSearcher(t *testing.T) {
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	embed := &mockEmbedder{}
	searcher := NewSearcher(store, embed)

	if searcher == nil {
		t.Fatal("expected non-nil searcher")
	}

	if searcher.storage != store {
		t.Error("searcher storage not set correctly")
	}

	if searcher.embedder != embed {
		t.Error("searcher embedder not set correctly")
	}
}

// TestValidateRequest tests request validation
func TestValidateRequest(t *testing.T) {
	s := &Searcher{}

	tests := []struct {
		name        string
		req         SearchRequest
		expectError bool
		validate    func(t *testing.T, req *SearchRequest)
	}{
		{
			name: "EmptyQuery",
			req: SearchRequest{
				Query: "",
			},
			expectError: true,
		},
		{
			name: "ValidBasicRequest",
			req: SearchRequest{
				Query: "test query",
				Limit: 10,
				Mode:  SearchModeHybrid,
			},
			expectError: false,
		},
		{
			name: "ZeroLimit_DefaultsTo10",
			req: SearchRequest{
				Query: "test",
				Limit: 0,
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.Limit != 10 {
					t.Errorf("expected default limit 10, got %d", req.Limit)
				}
			},
		},
		{
			name: "NegativeLimit_DefaultsTo10",
			req: SearchRequest{
				Query: "test",
				Limit: -5,
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.Limit != 10 {
					t.Errorf("expected default limit 10, got %d", req.Limit)
				}
			},
		},
		{
			name: "ExcessiveLimit_CapsAt100",
			req: SearchRequest{
				Query: "test",
				Limit: 500,
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.Limit != 100 {
					t.Errorf("expected capped limit 100, got %d", req.Limit)
				}
			},
		},
		{
			name: "EmptyMode_DefaultsToHybrid",
			req: SearchRequest{
				Query: "test",
				Limit: 10,
				Mode:  "",
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.Mode != SearchModeHybrid {
					t.Errorf("expected default mode hybrid, got %s", req.Mode)
				}
			},
		},
		{
			name: "ZeroRRFConstant_DefaultsTo60",
			req: SearchRequest{
				Query:       "test",
				Limit:       10,
				RRFConstant: 0,
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.RRFConstant != 60 {
					t.Errorf("expected default RRF constant 60, got %f", req.RRFConstant)
				}
			},
		},
		{
			name: "ZeroCacheTTL_DefaultsTo1Hour",
			req: SearchRequest{
				Query:    "test",
				Limit:    10,
				CacheTTL: 0,
			},
			expectError: false,
			validate: func(t *testing.T, req *SearchRequest) {
				if req.CacheTTL != 1*time.Hour {
					t.Errorf("expected default cache TTL 1h, got %v", req.CacheTTL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.validateRequest(&tt.req)

			if tt.expectError && err == nil {
				t.Fatal("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, &tt.req)
			}
		})
	}
}

// TestApplyRRF tests the Reciprocal Rank Fusion algorithm
func TestApplyRRF(t *testing.T) {
	s := &Searcher{}

	tests := []struct {
		name          string
		vectorResults []storage.VectorResult
		textResults   []storage.TextResult
		k             float64
		validate      func(t *testing.T, results []rankedResult)
	}{
		{
			name: "BothListsWithOverlap",
			vectorResults: []storage.VectorResult{
				{ChunkID: 1, SimilarityScore: 0.9},
				{ChunkID: 2, SimilarityScore: 0.8},
				{ChunkID: 3, SimilarityScore: 0.7},
			},
			textResults: []storage.TextResult{
				{ChunkID: 2, BM25Score: 10.5},
				{ChunkID: 3, BM25Score: 9.0},
				{ChunkID: 4, BM25Score: 8.0},
			},
			k: 60,
			validate: func(t *testing.T, results []rankedResult) {
				// Chunk 2 and 3 appear in both, should rank higher
				// Expected scores:
				// Chunk 2: 1/(60+2) + 1/(60+1) = 0.0161 + 0.0164 = 0.0325
				// Chunk 3: 1/(60+3) + 1/(60+2) = 0.0159 + 0.0161 = 0.0320
				// Chunk 1: 1/(60+1) = 0.0164
				// Chunk 4: 1/(60+3) = 0.0159

				if len(results) != 4 {
					t.Fatalf("expected 4 results, got %d", len(results))
				}

				// Verify results are sorted by score (descending)
				for i := 1; i < len(results); i++ {
					if results[i-1].score < results[i].score {
						t.Errorf("results not sorted: result[%d] score %f < result[%d] score %f",
							i-1, results[i-1].score, i, results[i].score)
					}
				}

				// Verify ranks are assigned sequentially
				for i, result := range results {
					expectedRank := i + 1
					if result.rank != expectedRank {
						t.Errorf("result %d has rank %d, expected %d", i, result.rank, expectedRank)
					}
				}

				// Chunk 2 should be top ranked (appears in both lists at high positions)
				if results[0].chunkID != 2 {
					t.Errorf("expected chunk 2 as top result, got chunk %d", results[0].chunkID)
				}
			},
		},
		{
			name: "NoOverlap",
			vectorResults: []storage.VectorResult{
				{ChunkID: 1, SimilarityScore: 0.9},
				{ChunkID: 2, SimilarityScore: 0.8},
			},
			textResults: []storage.TextResult{
				{ChunkID: 3, BM25Score: 10.0},
				{ChunkID: 4, BM25Score: 9.0},
			},
			k: 60,
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 4 {
					t.Fatalf("expected 4 results, got %d", len(results))
				}

				// All chunks should have similar scores (no overlap bonus)
				// Chunks at rank 1 get 1/61 = 0.0164
				// Chunks at rank 2 get 1/62 = 0.0161
				for _, result := range results {
					if result.score < 0.0 || result.score > 0.02 {
						t.Errorf("unexpected RRF score %f for chunk %d", result.score, result.chunkID)
					}
				}
			},
		},
		{
			name:          "EmptyVectorResults",
			vectorResults: []storage.VectorResult{},
			textResults: []storage.TextResult{
				{ChunkID: 1, BM25Score: 10.0},
				{ChunkID: 2, BM25Score: 9.0},
			},
			k: 60,
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 2 {
					t.Fatalf("expected 2 results, got %d", len(results))
				}

				// Should still work with only text results
				if results[0].chunkID != 1 {
					t.Errorf("expected chunk 1 first, got chunk %d", results[0].chunkID)
				}
			},
		},
		{
			name:          "EmptyTextResults",
			vectorResults: []storage.VectorResult{{ChunkID: 1, SimilarityScore: 0.9}},
			textResults:   []storage.TextResult{},
			k:             60,
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}
			},
		},
		{
			name:          "BothEmpty",
			vectorResults: []storage.VectorResult{},
			textResults:   []storage.TextResult{},
			k:             60,
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 0 {
					t.Fatalf("expected 0 results, got %d", len(results))
				}
			},
		},
		{
			name: "CustomKValue",
			vectorResults: []storage.VectorResult{
				{ChunkID: 1, SimilarityScore: 0.9},
			},
			textResults: []storage.TextResult{
				{ChunkID: 1, BM25Score: 10.0},
			},
			k: 30, // Custom k value
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}

				// With k=30, rank 1 in both: 1/(30+1) + 1/(30+1) = 2/31 = 0.0645
				expectedScore := 2.0 / 31.0
				if abs(results[0].score-expectedScore) > 0.0001 {
					t.Errorf("expected score ~%f, got %f", expectedScore, results[0].score)
				}
			},
		},
		{
			name: "ZeroKValue_DefaultsTo60",
			vectorResults: []storage.VectorResult{
				{ChunkID: 1, SimilarityScore: 0.9},
			},
			textResults: []storage.TextResult{},
			k:           0, // Should default to 60
			validate: func(t *testing.T, results []rankedResult) {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}

				// With k=60 (default), rank 1: 1/(60+1) = 1/61 = 0.0164
				expectedScore := 1.0 / 61.0
				if abs(results[0].score-expectedScore) > 0.0001 {
					t.Errorf("expected score ~%f, got %f", expectedScore, results[0].score)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := s.applyRRF(tt.vectorResults, tt.textResults, tt.k)
			tt.validate(t, results)
		})
	}
}

// TestSortRankedResults tests sorting of ranked results
func TestSortRankedResults(t *testing.T) {
	tests := []struct {
		name     string
		input    []rankedResult
		expected []int64 // Expected chunk IDs in order
	}{
		{
			name: "AlreadySorted",
			input: []rankedResult{
				{chunkID: 1, score: 0.9},
				{chunkID: 2, score: 0.8},
				{chunkID: 3, score: 0.7},
			},
			expected: []int64{1, 2, 3},
		},
		{
			name: "ReverseSorted",
			input: []rankedResult{
				{chunkID: 1, score: 0.7},
				{chunkID: 2, score: 0.8},
				{chunkID: 3, score: 0.9},
			},
			expected: []int64{3, 2, 1},
		},
		{
			name: "EqualScores",
			input: []rankedResult{
				{chunkID: 1, score: 0.8},
				{chunkID: 2, score: 0.8},
				{chunkID: 3, score: 0.8},
			},
			expected: []int64{1, 2, 3}, // Should maintain relative order
		},
		{
			name: "MixedScores",
			input: []rankedResult{
				{chunkID: 4, score: 0.5},
				{chunkID: 1, score: 0.9},
				{chunkID: 3, score: 0.7},
				{chunkID: 2, score: 0.8},
			},
			expected: []int64{1, 2, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]rankedResult, len(tt.input))
			copy(results, tt.input)

			sortRankedResults(results)

			for i, expectedID := range tt.expected {
				if results[i].chunkID != expectedID {
					t.Errorf("position %d: expected chunk %d, got %d", i, expectedID, results[i].chunkID)
				}
			}

			// Verify descending order
			for i := 1; i < len(results); i++ {
				if results[i-1].score < results[i].score {
					t.Errorf("results not in descending order at position %d", i)
				}
			}
		})
	}
}

// TestComputeQueryHash tests query hash computation
func TestComputeQueryHash(t *testing.T) {
	tests := []struct {
		name     string
		req1     SearchRequest
		req2     SearchRequest
		shouldEq bool
	}{
		{
			name: "IdenticalRequests",
			req1: SearchRequest{
				Query:     "test query",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			req2: SearchRequest{
				Query:     "test query",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			shouldEq: true,
		},
		{
			name: "DifferentQuery",
			req1: SearchRequest{
				Query:     "query one",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			req2: SearchRequest{
				Query:     "query two",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			shouldEq: false,
		},
		{
			name: "DifferentMode",
			req1: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			req2: SearchRequest{
				Query:     "test",
				Mode:      SearchModeVector,
				ProjectID: 1,
			},
			shouldEq: false,
		},
		{
			name: "DifferentProject",
			req1: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
			},
			req2: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 2,
			},
			shouldEq: false,
		},
		{
			name: "WithFilters",
			req1: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters: &storage.SearchFilters{
					SymbolTypes:  []string{"function", "method"},
					FilePattern:  "*.go",
					DDDPatterns:  []string{"aggregate"},
					Packages:     []string{"domain"},
					MinRelevance: 0.5,
				},
			},
			req2: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters: &storage.SearchFilters{
					SymbolTypes:  []string{"function", "method"},
					FilePattern:  "*.go",
					DDDPatterns:  []string{"aggregate"},
					Packages:     []string{"domain"},
					MinRelevance: 0.5,
				},
			},
			shouldEq: true,
		},
		{
			name: "DifferentFilters",
			req1: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters: &storage.SearchFilters{
					SymbolTypes: []string{"function"},
				},
			},
			req2: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters: &storage.SearchFilters{
					SymbolTypes: []string{"method"},
				},
			},
			shouldEq: false,
		},
		{
			name: "OneWithFiltersOneWithout",
			req1: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters:   &storage.SearchFilters{SymbolTypes: []string{"function"}},
			},
			req2: SearchRequest{
				Query:     "test",
				Mode:      SearchModeHybrid,
				ProjectID: 1,
				Filters:   nil,
			},
			shouldEq: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := computeQueryHash(tt.req1)
			hash2 := computeQueryHash(tt.req2)

			equal := hash1 == hash2

			if tt.shouldEq && !equal {
				t.Error("expected hashes to be equal but they differ")
			}

			if !tt.shouldEq && equal {
				t.Error("expected hashes to differ but they are equal")
			}
		})
	}
}

// TestCheckCache tests cache lookup (currently stubbed)
func TestCheckCache(t *testing.T) {
	s := &Searcher{}
	ctx := context.Background()

	req := SearchRequest{
		Query:     "test",
		Mode:      SearchModeHybrid,
		ProjectID: 1,
		UseCache:  true,
	}

	// Cache is stubbed to always return not found
	resp, err := s.checkCache(ctx, req)

	if err == nil {
		t.Error("expected error from stubbed cache")
	}

	if resp != nil {
		t.Error("expected nil response from stubbed cache")
	}
}

// TestStoreInCache tests cache storage (currently stubbed)
func TestStoreInCache(t *testing.T) {
	s := &Searcher{}
	ctx := context.Background()

	req := SearchRequest{
		Query:     "test",
		Mode:      SearchModeHybrid,
		ProjectID: 1,
	}

	resp := &SearchResponse{
		Results:      []types.SearchResult{},
		TotalResults: 0,
	}

	// Cache storage is stubbed to be no-op
	err := s.storeInCache(ctx, req, resp)

	if err != nil {
		t.Errorf("unexpected error from stubbed cache: %v", err)
	}
}

// TestInvalidateCache tests cache invalidation (stubbed)
func TestInvalidateCache(t *testing.T) {
	s := &Searcher{}
	ctx := context.Background()

	err := s.InvalidateCache(ctx, 1)

	if err != nil {
		t.Errorf("unexpected error from stubbed InvalidateCache: %v", err)
	}
}

// TestEvictLRU tests LRU eviction (stubbed)
func TestEvictLRU(t *testing.T) {
	s := &Searcher{}
	ctx := context.Background()

	err := s.EvictLRU(ctx, 100)

	if err != nil {
		t.Errorf("unexpected error from stubbed EvictLRU: %v", err)
	}
}

// Integration tests with real storage

// TestSearchModeVector tests vector-only search
func TestSearchModeVector(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create test data
	_, chunk := createTestFileAndChunk(t, store, project, "vector.go", "func VectorTest() {}")

	// Add embedding
	addTestEmbedding(t, store, chunk.ID)

	req := SearchRequest{
		Query:     "vector test",
		Limit:     10,
		Mode:      SearchModeVector,
		ProjectID: project.ID,
		UseCache:  false,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if resp.SearchMode != SearchModeVector {
		t.Errorf("expected SearchMode vector, got %s", resp.SearchMode)
	}

	if resp.VectorResults == 0 {
		t.Error("expected non-zero VectorResults in vector mode")
	}

	if resp.TextResults != 0 {
		t.Error("expected zero TextResults in vector mode")
	}

	if resp.Duration == 0 {
		t.Error("expected non-zero Duration")
	}
}

// TestSearchModeKeyword tests keyword-only search
func TestSearchModeKeyword(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create test data
	_, chunk := createTestFileAndChunk(t, store, project, "keyword.go", "func KeywordTest() {}")
	addTestEmbedding(t, store, chunk.ID)

	req := SearchRequest{
		Query:     "KeywordTest",
		Limit:     10,
		Mode:      SearchModeKeyword,
		ProjectID: project.ID,
		UseCache:  false,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if resp.SearchMode != SearchModeKeyword {
		t.Errorf("expected SearchMode keyword, got %s", resp.SearchMode)
	}

	if resp.TextResults == 0 {
		t.Error("expected non-zero TextResults in keyword mode")
	}

	if resp.VectorResults != 0 {
		t.Error("expected zero VectorResults in keyword mode")
	}
}

// TestSearchModeHybrid tests hybrid search with RRF
func TestSearchModeHybrid(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create multiple chunks
	for i := 0; i < 3; i++ {
		content := fmt.Sprintf("func Test%d() {}", i)
		_, chunk := createTestFileAndChunk(t, store, project, fmt.Sprintf("test%d.go", i), content)
		addTestEmbedding(t, store, chunk.ID)
	}

	req := SearchRequest{
		Query:       "Test",
		Limit:       10,
		Mode:        SearchModeHybrid,
		ProjectID:   project.ID,
		UseCache:    false,
		RRFConstant: 60,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if resp.SearchMode != SearchModeHybrid {
		t.Errorf("expected SearchMode hybrid, got %s", resp.SearchMode)
	}

	// Verify results are ranked by RRF score
	for i := 1; i < len(resp.Results); i++ {
		if resp.Results[i-1].RelevanceScore < resp.Results[i].RelevanceScore {
			t.Error("results not properly ranked by RRF score")
		}
	}
}

// TestSearchWithUnsupportedMode tests error handling for invalid mode
func TestSearchWithUnsupportedMode(t *testing.T) {
	search, _, project := setupTestSearcher(t)
	ctx := context.Background()

	req := SearchRequest{
		Query:     "test",
		Limit:     10,
		Mode:      SearchMode("invalid"),
		ProjectID: project.ID,
	}

	_, err := search.Search(ctx, req)
	if err == nil {
		t.Fatal("expected error for unsupported search mode")
	}

	if err.Error() != "unsupported search mode: invalid" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestHybridSearchWithEmbedderError tests error handling in hybrid search
func TestHybridSearchWithEmbedderError(t *testing.T) {
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer store.Close()

	// Create embedder that always fails
	embed := &mockEmbedder{
		generateFunc: func(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
			return nil, errors.New("embedding generation failed")
		},
	}

	search := NewSearcher(store, embed)

	ctx := context.Background()
	project := &storage.Project{
		RootPath:     "/test",
		ModuleName:   "test",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	req := SearchRequest{
		Query:     "test",
		Limit:     10,
		Mode:      SearchModeVector, // Use vector mode to ensure embedder is called
		ProjectID: project.ID,
	}

	_, err = search.Search(ctx, req)
	if err == nil {
		t.Fatal("expected error from vector search with embedder failure")
	}
}

// TestHybridSearchContextCancellation tests context cancellation during hybrid search
func TestHybridSearchContextCancellation(t *testing.T) {
	search, store, project := setupTestSearcher(t)

	// Create test data
	_, chunk := createTestFileAndChunk(t, store, project, "cancel.go", "func CancelTest() {}")
	addTestEmbedding(t, store, chunk.ID)

	// Create context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := SearchRequest{
		Query:     "test",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: project.ID,
	}

	_, err := search.Search(ctx, req)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

// TestFetchResults tests result fetching
func TestFetchResults(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create file with symbol and chunk directly
	hash := sha256.Sum256([]byte("fetch test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "fetch.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Create symbol
	symbol := &storage.Symbol{
		FileID:      file.ID,
		Name:        "FetchTest",
		Kind:        "function",
		PackageName: "test",
		Signature:   "func FetchTest()",
		StartLine:   1,
		StartCol:    0,
		EndLine:     3,
		EndCol:      1,
	}
	if err := store.UpsertSymbol(ctx, symbol); err != nil {
		t.Fatalf("UpsertSymbol failed: %v", err)
	}

	// Create chunk with symbol reference and context
	content := "func FetchTest() {}"
	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:        file.ID,
		SymbolID:      &symbol.ID,
		Content:       content,
		ContentHash:   contentHash,
		TokenCount:    5,
		StartLine:     1,
		EndLine:       3,
		ContextBefore: "package test",
		ContextAfter:  "// end",
		ChunkType:     "function",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	ranked := []rankedResult{
		{chunkID: chunk.ID, score: 0.95, rank: 1},
	}

	results, err := search.fetchResults(ctx, ranked, 10)
	if err != nil {
		t.Fatalf("fetchResults failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// Verify chunk data
	if result.ChunkID != chunk.ID {
		t.Errorf("expected ChunkID %d, got %d", chunk.ID, result.ChunkID)
	}

	if result.Rank != 1 {
		t.Errorf("expected Rank 1, got %d", result.Rank)
	}

	if result.RelevanceScore != 0.95 {
		t.Errorf("expected RelevanceScore 0.95, got %f", result.RelevanceScore)
	}

	if result.Content != chunk.Content {
		t.Errorf("expected Content %s, got %s", chunk.Content, result.Content)
	}

	// Verify file metadata
	if result.File == nil {
		t.Fatal("expected File metadata")
	}

	if result.File.Path != "fetch.go" {
		t.Errorf("expected Path fetch.go, got %s", result.File.Path)
	}

	// Verify symbol metadata
	if result.Symbol == nil {
		t.Fatal("expected Symbol metadata")
	}

	if result.Symbol.Name != "FetchTest" {
		t.Errorf("expected Symbol Name FetchTest, got %s", result.Symbol.Name)
	}

	// Verify context
	if result.Context == "" {
		t.Error("expected non-empty Context")
	}
}

// TestFetchResultsWithMissingChunks tests graceful handling of missing chunks
func TestFetchResultsWithMissingChunks(t *testing.T) {
	search, _, _ := setupTestSearcher(t)
	ctx := context.Background()

	// Request non-existent chunk IDs
	ranked := []rankedResult{
		{chunkID: 99999, score: 0.95, rank: 1},
		{chunkID: 88888, score: 0.90, rank: 2},
	}

	results, err := search.fetchResults(ctx, ranked, 10)
	if err != nil {
		t.Fatalf("fetchResults failed: %v", err)
	}

	// Should return empty results without error
	if len(results) != 0 {
		t.Errorf("expected 0 results for missing chunks, got %d", len(results))
	}
}

// TestFetchResultsLimitRespected tests limit parameter
func TestFetchResultsLimitRespected(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Create 5 chunks
	var ranked []rankedResult
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("func Test%d() {}", i)
		_, chunk := createTestFileAndChunk(t, store, project, fmt.Sprintf("test%d.go", i), content)
		ranked = append(ranked, rankedResult{
			chunkID: chunk.ID,
			score:   float64(5-i) * 0.1, // Descending scores
			rank:    i + 1,
		})
	}

	// Request only 3 results
	results, err := search.fetchResults(ctx, ranked, 3)
	if err != nil {
		t.Fatalf("fetchResults failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

// TestSearchWithCache tests cache behavior
func TestSearchWithCache(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Create test data
	_, chunk := createTestFileAndChunk(t, store, project, "cache.go", "func CacheTest() {}")
	addTestEmbedding(t, store, chunk.ID)

	req := SearchRequest{
		Query:     "cache test",
		Limit:     10,
		Mode:      SearchModeHybrid,
		ProjectID: project.ID,
		UseCache:  true,
		CacheTTL:  1 * time.Hour,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Cache is stubbed, so CacheHit should be false
	if resp.CacheHit {
		t.Error("expected CacheHit false with stubbed cache")
	}
}

// Helper functions

func createTestFileAndChunk(t *testing.T, store storage.Storage, project *storage.Project, filePath, content string) (*storage.File, *storage.Chunk) {
	t.Helper()
	ctx := context.Background()

	hash := sha256.Sum256([]byte(content))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    filePath,
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   int64(len(content)),
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:      file.ID,
		Content:     content,
		ContentHash: contentHash,
		TokenCount:  10,
		StartLine:   1,
		EndLine:     3,
		ChunkType:   "function",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	return file, chunk
}

func addTestEmbedding(t *testing.T, store storage.Storage, chunkID int64) {
	t.Helper()
	ctx := context.Background()

	vector := make([]byte, 384*4)
	embedding := &storage.Embedding{
		ChunkID:   chunkID,
		Vector:    vector,
		Dimension: 384,
		Provider:  "test",
		Model:     "test-model",
	}
	if err := store.UpsertEmbedding(ctx, embedding); err != nil {
		t.Fatalf("UpsertEmbedding failed: %v", err)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
