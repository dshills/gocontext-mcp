package storage

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// searchVector performs vector similarity search using cosine similarity
func searchVector(ctx context.Context, db *sql.DB, projectID int64, queryVector []float32, limit int, filters *SearchFilters) ([]VectorResult, error) {
	// Use optimized SQL-based search when sqlite-vec is available
	if VectorExtensionAvailable {
		return searchVectorOptimized(ctx, db, projectID, queryVector, limit, filters)
	}
	// Fall back to Go-based computation for purego builds
	return searchVectorFallback(ctx, db, projectID, queryVector, limit, filters)
}

// searchVectorOptimized uses sqlite-vec extension for SQL-based vector similarity search
func searchVectorOptimized(ctx context.Context, db *sql.DB, projectID int64, queryVector []float32, limit int, filters *SearchFilters) ([]VectorResult, error) {
	// Serialize query vector for sqlite-vec
	queryVectorBlob := serializeVector(queryVector)

	// Build SQL query that computes distance at database layer
	// Note: sqlite-vec's vec_distance_cosine returns distance (lower is better)
	// We convert to similarity (1 - distance) to maintain API compatibility
	query := `
		SELECT
			c.id as chunk_id,
			1.0 - vec_distance_cosine(e.vector, ?) as similarity
		FROM chunks c
		INNER JOIN embeddings e ON c.id = e.chunk_id
		INNER JOIN files f ON c.file_id = f.id
		WHERE f.project_id = ?
	`
	args := []interface{}{queryVectorBlob, projectID}

	// Apply filters in SQL WHERE clause
	query, args = applyVectorFilters(query, args, filters)

	// Apply minimum relevance filter in SQL if specified
	if filters != nil && filters.MinRelevance > 0 {
		query += " AND (1.0 - vec_distance_cosine(e.vector, ?)) >= ?"
		args = append(args, queryVectorBlob, filters.MinRelevance)
	}

	// Order by similarity (descending) and apply LIMIT in SQL
	query += " ORDER BY similarity DESC LIMIT ?"
	args = append(args, limit)

	// Execute query - results are already sorted and filtered
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute vector search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Collect results - no sorting needed as SQL handles it
	// Handle edge case: negative or zero limit
	if limit <= 0 {
		return []VectorResult{}, nil
	}
	results := make([]VectorResult, 0, limit)
	for rows.Next() {
		var result VectorResult
		if err := rows.Scan(&result.ChunkID, &result.SimilarityScore); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// searchVectorFallback performs vector search using Go-based cosine similarity computation
// This is used when sqlite-vec extension is not available (purego builds)
func searchVectorFallback(ctx context.Context, db *sql.DB, projectID int64, queryVector []float32, limit int, filters *SearchFilters) ([]VectorResult, error) {
	// Build query with filters
	query := `
		SELECT
			c.id as chunk_id,
			e.vector
		FROM chunks c
		INNER JOIN embeddings e ON c.id = e.chunk_id
		INNER JOIN files f ON c.file_id = f.id
		WHERE f.project_id = ?
	`
	args := []interface{}{projectID}

	// Apply filters
	query, args = applyVectorFilters(query, args, filters)

	// Execute query to get all candidate embeddings
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Compute similarity scores and rank in Go
	candidates, err := computeSimilarityScores(rows, queryVector, filters)
	if err != nil {
		return nil, err
	}

	// Sort by similarity (descending)
	sortCandidates(candidates)

	// Return top K
	return buildVectorResults(candidates, limit), nil
}

// searchText performs BM25 full-text search using FTS5
func searchText(ctx context.Context, db *sql.DB, projectID int64, query string, limit int, filters *SearchFilters) ([]TextResult, error) {
	// Sanitize query for FTS5
	sanitized := sanitizeFTSQuery(query)
	if sanitized == "" {
		return nil, fmt.Errorf("empty search query")
	}

	// Build query with filters
	sqlQuery := `
		SELECT
			c.id as chunk_id,
			bm25(chunks_fts) as score
		FROM chunks_fts
		INNER JOIN chunks c ON chunks_fts.chunk_id = c.id
		INNER JOIN files f ON c.file_id = f.id
		WHERE chunks_fts MATCH ?
		AND f.project_id = ?
	`
	args := []interface{}{sanitized, projectID}

	// Apply filters
	sqlQuery, args = applyTextFilters(sqlQuery, args, filters)

	// Order by BM25 score (lower is better) and limit
	sqlQuery += " ORDER BY score LIMIT ?"
	args = append(args, limit)

	// Execute query
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Collect and normalize results
	return collectTextResults(rows, filters)
}

// Helper functions

// applyVectorFilters adds WHERE clause filters for vector search
func applyVectorFilters(query string, args []interface{}, filters *SearchFilters) (string, []interface{}) {
	if filters == nil {
		return query, args
	}

	if len(filters.Packages) > 0 {
		query += " AND f.package_name IN ("
		for i, pkg := range filters.Packages {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, pkg)
		}
		query += ")"
	}

	if len(filters.SymbolTypes) > 0 && filters.SymbolTypes[0] != "" {
		query += " AND c.chunk_type IN ("
		for i, typ := range filters.SymbolTypes {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, typ)
		}
		query += ")"
	}

	if filters.FilePattern != "" {
		query += " AND f.file_path GLOB ?"
		args = append(args, filters.FilePattern)
	}

	query = applyDDDFilters(query, filters)
	return query, args
}

// applyTextFilters adds WHERE clause filters for text search
func applyTextFilters(query string, args []interface{}, filters *SearchFilters) (string, []interface{}) {
	if filters == nil {
		return query, args
	}

	if len(filters.Packages) > 0 {
		query += " AND f.package_name IN ("
		for i, pkg := range filters.Packages {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, pkg)
		}
		query += ")"
	}

	if len(filters.SymbolTypes) > 0 && filters.SymbolTypes[0] != "" {
		query += " AND c.chunk_type IN ("
		for i, typ := range filters.SymbolTypes {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, typ)
		}
		query += ")"
	}

	if filters.FilePattern != "" {
		query += " AND f.file_path GLOB ?"
		args = append(args, filters.FilePattern)
	}

	query = applyDDDFilters(query, filters)
	return query, args
}

// applyDDDFilters adds DDD pattern filters to query
func applyDDDFilters(query string, filters *SearchFilters) string {
	if filters == nil || len(filters.DDDPatterns) == 0 {
		return query
	}

	query += " AND c.symbol_id IN (SELECT id FROM symbols WHERE "
	dddConditions := buildDDDConditions(filters.DDDPatterns)

	if len(dddConditions) > 0 {
		query += dddConditions[0]
		for i := 1; i < len(dddConditions); i++ {
			query += " OR " + dddConditions[i]
		}
		query += ")"
	}
	return query
}

// buildDDDConditions creates SQL conditions for DDD patterns
func buildDDDConditions(patterns []string) []string {
	conditions := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		switch pattern {
		case "aggregate":
			conditions = append(conditions, "is_aggregate_root = 1")
		case "entity":
			conditions = append(conditions, "is_entity = 1")
		case "value_object":
			conditions = append(conditions, "is_value_object = 1")
		case "repository":
			conditions = append(conditions, "is_repository = 1")
		case "service":
			conditions = append(conditions, "is_service = 1")
		case "command":
			conditions = append(conditions, "is_command = 1")
		case "query":
			conditions = append(conditions, "is_query = 1")
		case "handler":
			conditions = append(conditions, "is_handler = 1")
		}
	}
	return conditions
}

// computeSimilarityScores processes rows and computes cosine similarity
func computeSimilarityScores(rows *sql.Rows, queryVector []float32, filters *SearchFilters) ([]candidate, error) {
	candidates := make([]candidate, 0, 1000)

	for rows.Next() {
		var chunkID int64
		var vectorBlob []byte
		if err := rows.Scan(&chunkID, &vectorBlob); err != nil {
			return nil, err
		}

		// Deserialize vector
		vector := deserializeVector(vectorBlob)
		if len(vector) != len(queryVector) {
			continue // Dimension mismatch, skip
		}

		// Compute cosine similarity
		similarity := cosineSimilarity(queryVector, vector)

		// Apply minimum relevance filter
		if filters != nil && filters.MinRelevance > 0 && similarity < filters.MinRelevance {
			continue
		}

		candidates = append(candidates, candidate{chunkID: chunkID, score: similarity})
	}

	return candidates, rows.Err()
}

// buildVectorResults creates VectorResult slice from candidates
func buildVectorResults(candidates []candidate, limit int) []VectorResult {
	// Handle negative or zero limit - return all candidates
	if limit <= 0 {
		limit = len(candidates)
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}

	results := make([]VectorResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = VectorResult{
			ChunkID:         candidates[i].chunkID,
			SimilarityScore: candidates[i].score,
		}
	}
	return results
}

// collectTextResults processes text search results and normalizes scores
func collectTextResults(rows *sql.Rows, filters *SearchFilters) ([]TextResult, error) {
	results := make([]TextResult, 0)

	for rows.Next() {
		var result TextResult
		if err := rows.Scan(&result.ChunkID, &result.BM25Score); err != nil {
			return nil, err
		}

		// Convert BM25 score (negative, lower is better) to positive normalized score
		// BM25 scores are typically in range [-50, 0]
		normalizedScore := 1.0 / (1.0 + math.Abs(result.BM25Score)/50.0)
		result.BM25Score = normalizedScore

		// Apply minimum relevance filter
		if filters != nil && filters.MinRelevance > 0 && result.BM25Score < filters.MinRelevance {
			continue
		}

		results = append(results, result)
	}

	return results, rows.Err()
}

// serializeVector converts a float32 slice to a byte blob (little-endian)
func serializeVector(vector []float32) []byte {
	blob := make([]byte, len(vector)*4)
	for i, v := range vector {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(v))
	}
	return blob
}

// deserializeVector converts a byte blob back to a float32 slice
func deserializeVector(blob []byte) []float32 {
	vector := make([]float32, len(blob)/4)
	for i := range vector {
		bits := binary.LittleEndian.Uint32(blob[i*4:])
		vector[i] = math.Float32frombits(bits)
	}
	return vector
}

// cosineSimilarity computes the cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// candidate represents a chunk with its similarity score
type candidate struct {
	chunkID int64
	score   float64
}

// sortCandidates sorts candidates by score in descending order using O(n log n) algorithm
func sortCandidates(candidates []candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
}

// FTS5 operator pattern for escaping Boolean operators
var ftsOperatorPattern = regexp.MustCompile(`\b(AND|OR|NOT|NEAR)\b`)

// sanitizeFTSQuery sanitizes a search query for FTS5 to prevent injection attacks.
// Escapes special FTS5 operators and characters that could be used for SQL injection.
func sanitizeFTSQuery(query string) string {
	if query == "" {
		return ""
	}

	// Replace special characters that have meaning in FTS5
	replacer := strings.NewReplacer(
		`"`, `\"`, // Quote
		`*`, `\*`, // Wildcard
		`(`, `\(`, // Grouping
		`)`, `\)`, // Grouping
	)
	escaped := replacer.Replace(query)

	// Escape Boolean operators to prevent injection
	escaped = ftsOperatorPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		return `\` + match
	})

	return escaped
}

// SerializeVector is an exported helper for testing
func SerializeVector(vector []float32) []byte {
	return serializeVector(vector)
}

// DeserializeVector is an exported helper for testing
func DeserializeVector(blob []byte) []float32 {
	return deserializeVector(blob)
}

// CosineSimilarity is an exported helper for testing
func CosineSimilarity(a, b []float32) float64 {
	return cosineSimilarity(a, b)
}
