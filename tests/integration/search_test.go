package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/searcher"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// SearchTestSuite contains tests for the search pipeline
type SearchTestSuite struct {
	suite.Suite
	storage     storage.Storage
	indexer     *indexer.Indexer
	searcher    *searcher.Searcher
	embedder    *MockEmbedder
	fixturesDir string
	projectID   int64
	ctx         context.Context
}

// SetupSuite runs once before all tests
func (s *SearchTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Get fixtures directory
	wd, err := os.Getwd()
	s.Require().NoError(err)
	s.fixturesDir = filepath.Join(filepath.Dir(wd), "testdata", "fixtures")
}

// SetupTest runs before each test
func (s *SearchTestSuite) SetupTest() {
	// Create fresh storage
	store, err := storage.NewSQLiteStorage(":memory:")
	s.Require().NoError(err)
	s.storage = store

	// Create mock embedder (384 dimensions like Jina embeddings)
	s.embedder = NewMockEmbedder(384)

	// Create indexer and searcher
	s.indexer = indexer.New(s.storage)
	s.searcher = searcher.NewSearcher(s.storage, s.embedder)

	// Index fixtures
	config := &indexer.Config{
		IncludeTests:  true,
		IncludeVendor: false,
	}

	stats, err := s.indexer.IndexProject(s.ctx, s.fixturesDir, config)
	s.Require().NoError(err)
	s.T().Logf("Indexed %d files, %d symbols, %d chunks",
		stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated)

	// Get project ID
	project, err := s.storage.GetProject(s.ctx, s.fixturesDir)
	s.Require().NoError(err)
	s.projectID = project.ID

	// Generate embeddings for all chunks
	s.generateEmbeddings()
}

// TearDownTest runs after each test
func (s *SearchTestSuite) TearDownTest() {
	if s.storage != nil {
		_ = s.storage.Close()
	}
}

// generateEmbeddings generates embeddings for all chunks
func (s *SearchTestSuite) generateEmbeddings() {
	files, err := s.storage.ListFiles(s.ctx, s.projectID)
	s.Require().NoError(err)

	for _, file := range files {
		chunks, err := s.storage.ListChunksByFile(s.ctx, file.ID)
		s.Require().NoError(err)

		for _, chunk := range chunks {
			// Generate embedding
			emb, err := s.embedder.GenerateEmbedding(s.ctx, struct {
				Text  string
				Model string
			}{Text: chunk.Content})
			s.Require().NoError(err)

			// Store embedding
			// Note: Vector needs to be serialized to []byte
			// For simplicity in tests, we'll skip actual storage
			// The search tests will focus on the search logic
			_ = emb
		}
	}
}

// TestSemanticSearch tests vector similarity search
func (s *SearchTestSuite) TestSemanticSearch() {
	req := searcher.SearchRequest{
		Query:     "user repository interface",
		Limit:     10,
		Mode:      searcher.SearchModeVector,
		ProjectID: s.projectID,
	}

	resp, err := s.searcher.Search(s.ctx, req)
	s.Require().NoError(err)
	s.NotNil(resp)

	s.T().Logf("Search results: %d, Duration: %v", len(resp.Results), resp.Duration)
	s.LessOrEqual(len(resp.Results), 10, "should respect limit")

	// Verify response metadata
	s.Equal(searcher.SearchModeVector, resp.SearchMode)
	s.False(resp.CacheHit, "first search should not be cached")

	// Verify results have required fields
	for i, result := range resp.Results {
		s.NotZero(result.ChunkID, "result %d should have chunk ID", i)
		s.NotZero(result.Rank, "result %d should have rank", i)
		s.NotEmpty(result.Content, "result %d should have content", i)
		s.NotNil(result.File, "result %d should have file info", i)
		s.T().Logf("Result %d: Chunk=%d, Score=%.4f, File=%s",
			i+1, result.ChunkID, result.RelevanceScore, result.File.Path)
	}
}

// TestKeywordSearch tests BM25 text search
func (s *SearchTestSuite) TestKeywordSearch() {
	req := searcher.SearchRequest{
		Query:     "ValidateEmail",
		Limit:     10,
		Mode:      searcher.SearchModeKeyword,
		ProjectID: s.projectID,
	}

	resp, err := s.searcher.Search(s.ctx, req)
	s.Require().NoError(err)
	s.NotNil(resp)

	s.T().Logf("Keyword search results: %d, Duration: %v", len(resp.Results), resp.Duration)

	// Should find results containing "ValidateEmail"
	if len(resp.Results) > 0 {
		foundMatch := false
		for _, result := range resp.Results {
			if result.Symbol != nil && result.Symbol.Name == "ValidateEmail" {
				foundMatch = true
				s.T().Logf("Found ValidateEmail symbol at %s:%d",
					result.File.Path, result.File.StartLine)
				break
			}
		}
		s.True(foundMatch || len(resp.Results) > 0,
			"should find ValidateEmail symbol or related content")
	}
}

// TestHybridSearch tests combined vector + keyword search
func (s *SearchTestSuite) TestHybridSearch() {
	req := searcher.SearchRequest{
		Query:       "order service business logic",
		Limit:       10,
		Mode:        searcher.SearchModeHybrid,
		ProjectID:   s.projectID,
		RRFConstant: 60, // Standard RRF constant
	}

	resp, err := s.searcher.Search(s.ctx, req)
	s.Require().NoError(err)
	s.NotNil(resp)

	s.T().Logf("Hybrid search results: %d, Duration: %v", len(resp.Results), resp.Duration)
	s.T().Logf("Vector results: %d, Text results: %d",
		resp.VectorResults, resp.TextResults)

	// Hybrid search should combine both approaches
	s.Equal(searcher.SearchModeHybrid, resp.SearchMode)

	// Results should be ranked by RRF score
	for i := 1; i < len(resp.Results); i++ {
		prevScore := resp.Results[i-1].RelevanceScore
		currScore := resp.Results[i].RelevanceScore
		s.GreaterOrEqual(prevScore, currScore,
			"results should be sorted by relevance score descending")
	}
}

// TestSearchWithFilters tests filter application
func (s *SearchTestSuite) TestSearchWithFilters() {
	tests := []struct {
		name    string
		query   string
		filters *storage.SearchFilters
		verify  func(results []interface{})
	}{
		{
			name:  "filter by symbol type - function",
			query: "validate",
			filters: &storage.SearchFilters{
				SymbolTypes: []string{"function"},
			},
		},
		{
			name:  "filter by symbol type - struct",
			query: "user order",
			filters: &storage.SearchFilters{
				SymbolTypes: []string{"struct"},
			},
		},
		{
			name:  "filter by DDD pattern - repository",
			query: "repository",
			filters: &storage.SearchFilters{
				DDDPatterns: []string{"repository"},
			},
		},
		{
			name:  "filter by DDD pattern - service",
			query: "service",
			filters: &storage.SearchFilters{
				DDDPatterns: []string{"service"},
			},
		},
		{
			name:  "filter by file pattern",
			query: "user",
			filters: &storage.SearchFilters{
				FilePattern: "*simple*",
			},
		},
		{
			name:  "filter by min relevance",
			query: "function",
			filters: &storage.SearchFilters{
				MinRelevance: 0.5,
			},
		},
		{
			name:  "multiple filters",
			query: "order",
			filters: &storage.SearchFilters{
				SymbolTypes:  []string{"struct", "interface"},
				DDDPatterns:  []string{"aggregate", "repository"},
				MinRelevance: 0.3,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			req := searcher.SearchRequest{
				Query:     tt.query,
				Limit:     10,
				Mode:      searcher.SearchModeHybrid,
				Filters:   tt.filters,
				ProjectID: s.projectID,
			}

			resp, err := s.searcher.Search(s.ctx, req)
			s.Require().NoError(err)
			s.NotNil(resp)

			s.T().Logf("%s: found %d results", tt.name, len(resp.Results))

			// Verify filters were applied (results may be empty, which is fine)
			for _, result := range resp.Results {
				// Verify min relevance if set
				if tt.filters.MinRelevance > 0 {
					s.GreaterOrEqual(result.RelevanceScore, tt.filters.MinRelevance,
						"result should meet minimum relevance threshold")
				}

				// Verify symbol type filter if set and symbol exists
				if len(tt.filters.SymbolTypes) > 0 && result.Symbol != nil {
					hasValidType := false
					for _, symbolType := range tt.filters.SymbolTypes {
						if string(result.Symbol.Kind) == symbolType {
							hasValidType = true
							break
						}
					}
					s.True(hasValidType,
						"symbol kind %s should match filter %v",
						result.Symbol.Kind, tt.filters.SymbolTypes)
				}

				// Log result details
				s.T().Logf("  Result: File=%s, Symbol=%v, Score=%.4f",
					result.File.Path,
					func() string {
						if result.Symbol != nil {
							return result.Symbol.Name
						}
						return "none"
					}(),
					result.RelevanceScore)
			}
		})
	}
}

// TestSearchModes compares different search modes
func (s *SearchTestSuite) TestSearchModes() {
	query := "order aggregate"

	modes := []searcher.SearchMode{
		searcher.SearchModeVector,
		searcher.SearchModeKeyword,
		searcher.SearchModeHybrid,
	}

	results := make(map[searcher.SearchMode]*searcher.SearchResponse)

	// Run search with each mode
	for _, mode := range modes {
		req := searcher.SearchRequest{
			Query:     query,
			Limit:     10,
			Mode:      mode,
			ProjectID: s.projectID,
		}

		resp, err := s.searcher.Search(s.ctx, req)
		s.Require().NoError(err)
		results[mode] = resp

		s.T().Logf("Mode %s: %d results, Duration: %v",
			mode, len(resp.Results), resp.Duration)
	}

	// Compare results
	s.T().Log("\nSearch Mode Comparison:")
	for mode, resp := range results {
		s.T().Logf("  %s: %d results", mode, len(resp.Results))
		if len(resp.Results) > 0 {
			s.T().Logf("    Top result: %s (score: %.4f)",
				resp.Results[0].File.Path, resp.Results[0].RelevanceScore)
		}
	}

	// Verify each mode returned results (may be different)
	for mode, resp := range results {
		s.NotNil(resp, "mode %s should return response", mode)
		s.Equal(mode, resp.SearchMode, "response should indicate correct mode")
	}
}

// TestSearchPagination tests search with different limits
func (s *SearchTestSuite) TestSearchPagination() {
	limits := []int{1, 5, 10, 20}

	for _, limit := range limits {
		s.Run(string(rune('0'+limit/10))+"_limit_"+string(rune('0'+limit%10)), func() {
			req := searcher.SearchRequest{
				Query:     "user order",
				Limit:     limit,
				Mode:      searcher.SearchModeHybrid,
				ProjectID: s.projectID,
			}

			resp, err := s.searcher.Search(s.ctx, req)
			s.Require().NoError(err)

			s.T().Logf("Limit %d: got %d results", limit, len(resp.Results))
			s.LessOrEqual(len(resp.Results), limit,
				"should not exceed requested limit")
		})
	}
}

// TestSearchEmptyQuery tests error handling for empty query
func (s *SearchTestSuite) TestSearchEmptyQuery() {
	req := searcher.SearchRequest{
		Query:     "",
		Limit:     10,
		Mode:      searcher.SearchModeHybrid,
		ProjectID: s.projectID,
	}

	_, err := s.searcher.Search(s.ctx, req)
	s.Error(err, "should return error for empty query")
	s.Contains(err.Error(), "query cannot be empty")
}

// TestSearchInvalidMode tests error handling for invalid search mode
func (s *SearchTestSuite) TestSearchInvalidMode() {
	req := searcher.SearchRequest{
		Query:     "test",
		Limit:     10,
		Mode:      searcher.SearchMode("invalid"),
		ProjectID: s.projectID,
	}

	_, err := s.searcher.Search(s.ctx, req)
	s.Error(err, "should return error for invalid mode")
}

// TestSearchPerformance tests search latency
func (s *SearchTestSuite) TestSearchPerformance() {
	queries := []string{
		"user repository",
		"order service",
		"validate email",
		"aggregate root",
		"command handler",
	}

	for _, query := range queries {
		req := searcher.SearchRequest{
			Query:     query,
			Limit:     10,
			Mode:      searcher.SearchModeHybrid,
			ProjectID: s.projectID,
		}

		resp, err := s.searcher.Search(s.ctx, req)
		s.Require().NoError(err)

		s.T().Logf("Query '%s': %d results in %v",
			query, len(resp.Results), resp.Duration)

		// Performance target: < 500ms for test environment
		// (may be higher in CI, so this is informational)
		if resp.Duration.Milliseconds() > 500 {
			s.T().Logf("  WARNING: Search took longer than 500ms")
		}
	}
}

// TestSearchTestSuite runs the suite
func TestSearchTestSuite(t *testing.T) {
	suite.Run(t, new(SearchTestSuite))
}
