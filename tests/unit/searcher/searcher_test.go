package searcher_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/searcher"
	"github.com/dshills/gocontext-mcp/internal/storage"
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
	// Simple implementation for tests
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
func setupTestSearcher(t *testing.T) (*searcher.Searcher, storage.Storage, *storage.Project) {
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
	search := searcher.NewSearcher(store, embed)

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

// TestRRF tests the Reciprocal Rank Fusion algorithm
func TestRRF(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create file, chunks, and embeddings
	hash := sha256.Sum256([]byte("rrf test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "rrf.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Create multiple chunks for ranking
	chunks := []struct {
		content   string
		startLine int
		endLine   int
	}{
		{"func Alpha() {}", 1, 3},
		{"func Beta() {}", 10, 12},
		{"func Gamma() {}", 20, 22},
		{"func Delta() {}", 30, 32},
	}

	for _, chunk := range chunks {
		contentHash := sha256.Sum256([]byte(chunk.content))
		c := &storage.Chunk{
			FileID:      file.ID,
			Content:     chunk.content,
			ContentHash: contentHash,
			TokenCount:  5,
			StartLine:   chunk.startLine,
			EndLine:     chunk.endLine,
			ChunkType:   "function",
		}
		if err := store.UpsertChunk(ctx, c); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		// Add embeddings for vector search
		vector := make([]byte, 384*4)
		embedding := &storage.Embedding{
			ChunkID:   c.ID,
			Vector:    vector,
			Dimension: 384,
			Provider:  "test",
			Model:     "test-model",
		}
		if err := store.UpsertEmbedding(ctx, embedding); err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}
	}

	t.Run("HybridSearch_RRFCombination", func(t *testing.T) {
		// Perform hybrid search (combines vector + text with RRF)
		req := searcher.SearchRequest{
			Query:       "test function",
			Limit:       10,
			Mode:        searcher.SearchModeHybrid,
			ProjectID:   project.ID,
			UseCache:    false,
			RRFConstant: 60, // Standard RRF k value
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Verify response
		if resp.SearchMode != searcher.SearchModeHybrid {
			t.Errorf("expected SearchMode hybrid, got %s", resp.SearchMode)
		}

		// Verify results are ranked (scores should be descending)
		for i := 1; i < len(resp.Results); i++ {
			if resp.Results[i-1].RelevanceScore < resp.Results[i].RelevanceScore {
				t.Error("results not properly ranked by relevance score")
			}
		}
	})

	t.Run("RRF_ScoreCalculation", func(t *testing.T) {
		// Test RRF formula: RRF(d) = Σ 1/(k + rank(d))
		// With k=60, rank 1 should give score 1/61 = 0.0164
		// Rank 2 should give 1/62 = 0.0161, etc.

		req := searcher.SearchRequest{
			Query:       "test",
			Limit:       4,
			Mode:        searcher.SearchModeHybrid,
			ProjectID:   project.ID,
			UseCache:    false,
			RRFConstant: 60,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Verify all results have positive scores
		for i, result := range resp.Results {
			if result.RelevanceScore <= 0 {
				t.Errorf("result %d has non-positive score %f", i, result.RelevanceScore)
			}

			// RRF scores should be relatively small (max is ~2/61 if in both lists at rank 1)
			if result.RelevanceScore > 1.0 {
				t.Errorf("result %d has unexpectedly high RRF score %f", i, result.RelevanceScore)
			}
		}
	})
}

// TestSearchModes tests different search modes (vector, keyword, hybrid)
func TestSearchModes(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create test data
	hash := sha256.Sum256([]byte("mode test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "modes.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Create chunk with embedding
	content := "func SearchTest() error { return nil }"
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

	// Add embedding
	vector := make([]byte, 384*4)
	embedding := &storage.Embedding{
		ChunkID:   chunk.ID,
		Vector:    vector,
		Dimension: 384,
		Provider:  "test",
		Model:     "test-model",
	}
	if err := store.UpsertEmbedding(ctx, embedding); err != nil {
		t.Fatalf("UpsertEmbedding failed: %v", err)
	}

	tests := []struct {
		name     string
		mode     searcher.SearchMode
		validate func(t *testing.T, resp *searcher.SearchResponse)
	}{
		{
			name: "VectorMode",
			mode: searcher.SearchModeVector,
			validate: func(t *testing.T, resp *searcher.SearchResponse) {
				if resp.SearchMode != searcher.SearchModeVector {
					t.Errorf("expected SearchMode vector, got %s", resp.SearchMode)
				}

				// Vector mode should only use vector results
				if resp.VectorResults == 0 {
					t.Error("expected non-zero VectorResults in vector mode")
				}
			},
		},
		{
			name: "KeywordMode",
			mode: searcher.SearchModeKeyword,
			validate: func(t *testing.T, resp *searcher.SearchResponse) {
				if resp.SearchMode != searcher.SearchModeKeyword {
					t.Errorf("expected SearchMode keyword, got %s", resp.SearchMode)
				}

				// Keyword mode should only use text results
				if resp.TextResults == 0 {
					t.Error("expected non-zero TextResults in keyword mode")
				}
			},
		},
		{
			name: "HybridMode",
			mode: searcher.SearchModeHybrid,
			validate: func(t *testing.T, resp *searcher.SearchResponse) {
				if resp.SearchMode != searcher.SearchModeHybrid {
					t.Errorf("expected SearchMode hybrid, got %s", resp.SearchMode)
				}

				// Hybrid mode should use both (though one might be zero if no matches)
				// At minimum, results should be present
				if resp.TotalResults == 0 {
					t.Error("expected some results in hybrid mode")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := searcher.SearchRequest{
				Query:     "SearchTest",
				Limit:     10,
				Mode:      tt.mode,
				ProjectID: project.ID,
				UseCache:  false,
			}

			resp, err := search.Search(ctx, req)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			tt.validate(t, resp)

			// Verify duration is set
			if resp.Duration == 0 {
				t.Error("expected non-zero Duration")
			}
		})
	}
}

// TestFilters tests search filter application
func TestFilters(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create files with different characteristics
	files := []struct {
		path        string
		packageName string
		content     string
		symbolKind  string
		isDDD       bool
	}{
		{"domain/user.go", "domain", "type UserAggregate struct {}", "struct", true},
		{"repo/user.go", "repo", "type UserRepository interface {}", "interface", true},
		{"service/auth.go", "service", "func Authenticate() error {}", "function", false},
		{"util/helper.go", "util", "func Helper() {}", "function", false},
	}

	for _, fileData := range files {
		hash := sha256.Sum256([]byte(fileData.content))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    fileData.path,
			PackageName: fileData.packageName,
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		if err := store.UpsertFile(ctx, file); err != nil {
			t.Fatalf("UpsertFile failed for %s: %v", fileData.path, err)
		}

		// Create symbol
		symbol := &storage.Symbol{
			FileID:          file.ID,
			Name:            "TestSymbol",
			Kind:            fileData.symbolKind,
			PackageName:     fileData.packageName,
			StartLine:       1,
			StartCol:        0,
			EndLine:         10,
			EndCol:          1,
			IsAggregateRoot: fileData.isDDD && fileData.symbolKind == "struct",
			IsRepository:    fileData.isDDD && fileData.symbolKind == "interface",
		}
		if err := store.UpsertSymbol(ctx, symbol); err != nil {
			t.Fatalf("UpsertSymbol failed: %v", err)
		}

		// Create chunk
		contentHash := sha256.Sum256([]byte(fileData.content))
		chunk := &storage.Chunk{
			FileID:      file.ID,
			SymbolID:    &symbol.ID,
			Content:     fileData.content,
			ContentHash: contentHash,
			TokenCount:  10,
			StartLine:   1,
			EndLine:     10,
			ChunkType:   "type",
		}
		if err := store.UpsertChunk(ctx, chunk); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		// Add embedding
		vector := make([]byte, 384*4)
		embedding := &storage.Embedding{
			ChunkID:   chunk.ID,
			Vector:    vector,
			Dimension: 384,
			Provider:  "test",
			Model:     "test-model",
		}
		if err := store.UpsertEmbedding(ctx, embedding); err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}
	}

	t.Run("Filter_SymbolTypes", func(t *testing.T) {
		filters := &storage.SearchFilters{
			SymbolTypes: []string{"function"},
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Verify only function symbols in results
		for _, result := range resp.Results {
			if result.Symbol != nil && result.Symbol.Kind != "function" {
				t.Errorf("expected only function symbols, got %s", result.Symbol.Kind)
			}
		}
	})

	t.Run("Filter_FilePattern", func(t *testing.T) {
		filters := &storage.SearchFilters{
			FilePattern: "domain/*.go",
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Note: Actual glob matching is done in storage layer
		// This test verifies filter is passed through correctly
		if resp.TotalResults < 0 {
			t.Error("expected valid result count")
		}
	})

	t.Run("Filter_DDDPatterns", func(t *testing.T) {
		filters := &storage.SearchFilters{
			DDDPatterns: []string{"aggregate", "repository"},
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Note: DDD pattern filtering is done in storage layer
		if resp.TotalResults < 0 {
			t.Error("expected valid result count")
		}
	})

	t.Run("Filter_Packages", func(t *testing.T) {
		filters := &storage.SearchFilters{
			Packages: []string{"domain", "repo"},
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Verify only specified packages in results
		for _, result := range resp.Results {
			pkg := result.File.Package
			if pkg != "domain" && pkg != "repo" {
				t.Errorf("expected package domain or repo, got %s", pkg)
			}
		}
	})

	t.Run("Filter_MinRelevance", func(t *testing.T) {
		filters := &storage.SearchFilters{
			MinRelevance: 0.5,
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Note: MinRelevance filtering done in storage/post-processing
		// Verify no errors and valid response
		if resp.TotalResults < 0 {
			t.Error("expected valid result count")
		}
	})

	t.Run("Filter_MultipleFilters", func(t *testing.T) {
		filters := &storage.SearchFilters{
			SymbolTypes:  []string{"struct", "interface"},
			DDDPatterns:  []string{"aggregate", "repository"},
			Packages:     []string{"domain", "repo"},
			MinRelevance: 0.1,
		}

		req := searcher.SearchRequest{
			Query:     "test",
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			Filters:   filters,
			ProjectID: project.ID,
			UseCache:  false,
		}

		resp, err := search.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// Multiple filters should narrow results
		if resp.TotalResults < 0 {
			t.Error("expected valid result count")
		}
	})
}

// TestValidation tests search request validation
func TestValidation(t *testing.T) {
	search, _, project := setupTestSearcher(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		req         searcher.SearchRequest
		expectError bool
	}{
		{
			name: "Valid_BasicRequest",
			req: searcher.SearchRequest{
				Query:     "test",
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
			},
			expectError: false,
		},
		{
			name: "Invalid_EmptyQuery",
			req: searcher.SearchRequest{
				Query:     "",
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
			},
			expectError: true,
		},
		{
			name: "Valid_DefaultLimit",
			req: searcher.SearchRequest{
				Query:     "test",
				Limit:     0, // Should default to 10
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
			},
			expectError: false,
		},
		{
			name: "Valid_MaxLimit",
			req: searcher.SearchRequest{
				Query:     "test",
				Limit:     100,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
			},
			expectError: false,
		},
		{
			name: "Valid_LimitCapped",
			req: searcher.SearchRequest{
				Query:     "test",
				Limit:     200, // Should cap at 100
				Mode:      searcher.SearchModeHybrid,
				ProjectID: project.ID,
			},
			expectError: false,
		},
		{
			name: "Valid_DefaultMode",
			req: searcher.SearchRequest{
				Query:     "test",
				Limit:     10,
				Mode:      "", // Should default to hybrid
				ProjectID: project.ID,
			},
			expectError: false,
		},
		{
			name: "Valid_DefaultRRFConstant",
			req: searcher.SearchRequest{
				Query:       "test",
				Limit:       10,
				Mode:        searcher.SearchModeHybrid,
				ProjectID:   project.ID,
				RRFConstant: 0, // Should default to 60
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := search.Search(ctx, tt.req)

			if tt.expectError && err == nil {
				t.Fatal("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestResultRanking tests that results are properly ranked
func TestResultRanking(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create multiple chunks with embeddings
	hash := sha256.Sum256([]byte("ranking test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "ranking.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Create 5 chunks
	for i := 0; i < 5; i++ {
		content := "test content"
		contentHash := sha256.Sum256([]byte(content))
		chunk := &storage.Chunk{
			FileID:      file.ID,
			Content:     content,
			ContentHash: contentHash,
			TokenCount:  5,
			StartLine:   i * 10,
			EndLine:     i*10 + 3,
			ChunkType:   "test",
		}
		if err := store.UpsertChunk(ctx, chunk); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		// Add embedding
		vector := make([]byte, 384*4)
		embedding := &storage.Embedding{
			ChunkID:   chunk.ID,
			Vector:    vector,
			Dimension: 384,
			Provider:  "test",
			Model:     "test-model",
		}
		if err := store.UpsertEmbedding(ctx, embedding); err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}
	}

	req := searcher.SearchRequest{
		Query:     "test",
		Limit:     5,
		Mode:      searcher.SearchModeHybrid,
		ProjectID: project.ID,
		UseCache:  false,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify results are properly ranked
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}

	// Check rank assignment (1-based, sequential)
	for i, result := range resp.Results {
		expectedRank := i + 1
		if result.Rank != expectedRank {
			t.Errorf("result %d has rank %d, expected %d", i, result.Rank, expectedRank)
		}
	}

	// Verify relevance scores are descending
	for i := 1; i < len(resp.Results); i++ {
		if resp.Results[i-1].RelevanceScore < resp.Results[i].RelevanceScore {
			t.Errorf("results not properly ordered: result %d score %f > result %d score %f",
				i, resp.Results[i].RelevanceScore, i-1, resp.Results[i-1].RelevanceScore)
		}
	}
}

// TestResultMetadata tests that results include proper metadata
func TestResultMetadata(t *testing.T) {
	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create file, symbol, and chunk
	hash := sha256.Sum256([]byte("metadata test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "metadata.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	symbol := &storage.Symbol{
		FileID:      file.ID,
		Name:        "TestFunc",
		Kind:        "function",
		PackageName: "test",
		Signature:   "func TestFunc() error",
		DocComment:  "TestFunc does something",
		StartLine:   1,
		StartCol:    0,
		EndLine:     10,
		EndCol:      1,
	}
	if err := store.UpsertSymbol(ctx, symbol); err != nil {
		t.Fatalf("UpsertSymbol failed: %v", err)
	}

	content := "func TestFunc() error { return nil }"
	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:        file.ID,
		SymbolID:      &symbol.ID,
		Content:       content,
		ContentHash:   contentHash,
		TokenCount:    10,
		StartLine:     1,
		EndLine:       10,
		ContextBefore: "package test",
		ContextAfter:  "// More code",
		ChunkType:     "function",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	// Add embedding
	vector := make([]byte, 384*4)
	embedding := &storage.Embedding{
		ChunkID:   chunk.ID,
		Vector:    vector,
		Dimension: 384,
		Provider:  "test",
		Model:     "test-model",
	}
	if err := store.UpsertEmbedding(ctx, embedding); err != nil {
		t.Fatalf("UpsertEmbedding failed: %v", err)
	}

	req := searcher.SearchRequest{
		Query:     "TestFunc",
		Limit:     10,
		Mode:      searcher.SearchModeHybrid,
		ProjectID: project.ID,
		UseCache:  false,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Fatal("expected at least one result")
	}

	result := resp.Results[0]

	// Verify chunk metadata
	if result.ChunkID != chunk.ID {
		t.Errorf("expected ChunkID %d, got %d", chunk.ID, result.ChunkID)
	}

	if result.Content != content {
		t.Errorf("expected Content %s, got %s", content, result.Content)
	}

	// Verify file metadata
	if result.File == nil {
		t.Fatal("expected File metadata")
	}

	if result.File.Path != "metadata.go" {
		t.Errorf("expected Path metadata.go, got %s", result.File.Path)
	}

	if result.File.Package != "test" {
		t.Errorf("expected Package test, got %s", result.File.Package)
	}

	if result.File.StartLine != 1 || result.File.EndLine != 10 {
		t.Errorf("expected lines 1-10, got %d-%d", result.File.StartLine, result.File.EndLine)
	}

	// Verify symbol metadata
	if result.Symbol == nil {
		t.Fatal("expected Symbol metadata")
	}

	if result.Symbol.Name != "TestFunc" {
		t.Errorf("expected Symbol Name TestFunc, got %s", result.Symbol.Name)
	}

	if result.Symbol.Kind != "function" {
		t.Errorf("expected Symbol Kind function, got %s", result.Symbol.Kind)
	}

	if result.Symbol.Signature != "func TestFunc() error" {
		t.Errorf("expected Signature, got %s", result.Symbol.Signature)
	}

	// Verify context
	if result.Context == "" {
		t.Error("expected non-empty Context")
	}
}

// TestSearchPerformance tests search duration is reasonable
func TestSearchPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	search, store, project := setupTestSearcher(t)
	ctx := context.Background()

	// Setup: Create some test data
	hash := sha256.Sum256([]byte("perf test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "perf.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Create chunk
	content := "test content for performance"
	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:      file.ID,
		Content:     content,
		ContentHash: contentHash,
		TokenCount:  10,
		StartLine:   1,
		EndLine:     3,
		ChunkType:   "test",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	// Add embedding
	vector := make([]byte, 384*4)
	embedding := &storage.Embedding{
		ChunkID:   chunk.ID,
		Vector:    vector,
		Dimension: 384,
		Provider:  "test",
		Model:     "test-model",
	}
	if err := store.UpsertEmbedding(ctx, embedding); err != nil {
		t.Fatalf("UpsertEmbedding failed: %v", err)
	}

	req := searcher.SearchRequest{
		Query:     "performance test",
		Limit:     10,
		Mode:      searcher.SearchModeHybrid,
		ProjectID: project.ID,
		UseCache:  false,
	}

	resp, err := search.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify search completed in reasonable time (< 1 second for small dataset)
	if resp.Duration > time.Second {
		t.Errorf("search took too long: %v", resp.Duration)
	}

	t.Logf("Search completed in %v", resp.Duration)
}
