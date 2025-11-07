package storage

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVectorFormatCompatibility tests if our vector serialization is compatible with sqlite-vec
func TestVectorFormatCompatibility(t *testing.T) {
	if !VectorExtensionAvailable {
		t.Skip("Skipping test: sqlite-vec extension not available")
	}

	db, err := sql.Open(DriverName, ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Create a simple test vector
	testVector := []float32{1.0, 2.0, 3.0, 4.0}
	vectorBlob := serializeVector(testVector)

	// Try to use vec_distance_cosine with our serialized vector
	var distance float64
	err = db.QueryRowContext(ctx, "SELECT vec_distance_cosine(?, ?)", vectorBlob, vectorBlob).Scan(&distance)
	if err != nil {
		t.Logf("Error using vec_distance_cosine: %v", err)
		t.Logf("Vector blob length: %d bytes", len(vectorBlob))
		t.Logf("Expected length: %d bytes (4 floats * 4 bytes)", len(testVector)*4)

		// Try to get more information about what sqlite-vec expects
		var version string
		err2 := db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&version)
		if err2 != nil {
			t.Logf("Cannot get vec_version: %v", err2)
		} else {
			t.Logf("sqlite-vec version: %s", version)
		}

		t.Fatalf("Vector format incompatible with sqlite-vec: %v", err)
	}

	t.Logf("Distance from vector to itself: %f (should be very close to 0)", distance)

	// Distance from a vector to itself should be 0 (or very close)
	if distance > 0.001 {
		t.Errorf("Expected distance ~0, got %f", distance)
	}

	// Test with different vectors
	testVector2 := []float32{4.0, 3.0, 2.0, 1.0}
	vectorBlob2 := serializeVector(testVector2)

	err = db.QueryRowContext(ctx, "SELECT vec_distance_cosine(?, ?)", vectorBlob, vectorBlob2).Scan(&distance)
	require.NoError(t, err)

	t.Logf("Distance between different vectors: %f", distance)

	// Calculate Go-based cosine similarity
	goSimilarity := cosineSimilarity(testVector, testVector2)
	goDistance := 1.0 - goSimilarity

	t.Logf("Go cosine similarity: %f", goSimilarity)
	t.Logf("Go cosine distance (1-similarity): %f", goDistance)
	t.Logf("sqlite-vec distance: %f", distance)
	t.Logf("Difference: %f", distance-goDistance)
}

// TestCompareVectorSearchResults compares results between optimized and fallback with detailed logging
func TestCompareVectorSearchResults(t *testing.T) {
	if !VectorExtensionAvailable {
		t.Skip("Skipping test: sqlite-vec extension not available")
	}

	db, err := sql.Open(DriverName, ":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = ApplyMigrations(context.Background(), db)
	require.NoError(t, err)

	ctx := context.Background()
	projectID, _ := setupVectorTestData(t, ctx, db)

	// Test query vector
	queryVector := make([]float32, 384)
	for i := range queryVector {
		queryVector[i] = float32(i) * 0.01
	}

	// Get results from both implementations
	optimizedResults, err := searchVectorOptimized(ctx, db, projectID, queryVector, 10, nil)
	require.NoError(t, err)

	fallbackResults, err := searchVectorFallback(ctx, db, projectID, queryVector, 10, nil)
	require.NoError(t, err)

	fmt.Println("\n=== Optimized Results (sqlite-vec) ===")
	for i, r := range optimizedResults {
		fmt.Printf("%d. ChunkID: %d, Score: %.10f\n", i+1, r.ChunkID, r.SimilarityScore)
	}

	fmt.Println("\n=== Fallback Results (Go) ===")
	for i, r := range fallbackResults {
		fmt.Printf("%d. ChunkID: %d, Score: %.10f\n", i+1, r.ChunkID, r.SimilarityScore)
	}

	// Compare scores for matching chunk IDs
	fmt.Println("\n=== Score Comparison ===")
	for i := 0; i < len(optimizedResults) && i < len(fallbackResults); i++ {
		diff := optimizedResults[i].SimilarityScore - fallbackResults[i].SimilarityScore
		fmt.Printf("Position %d: Optimized=%.10f, Fallback=%.10f, Diff=%.10f\n",
			i+1, optimizedResults[i].SimilarityScore, fallbackResults[i].SimilarityScore, diff)
	}
}
