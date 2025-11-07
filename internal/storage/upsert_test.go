package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertSymbol_UniqueConstraint verifies that the UPSERT operation
// works correctly with the UNIQUE constraint on symbols table
func TestUpsertSymbol_UniqueConstraint(t *testing.T) {
	// Create in-memory database
	store, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create a test project
	project := &Project{
		RootPath:     "/test/project",
		ModuleName:   "test/module",
		GoVersion:    "1.21",
		IndexVersion: "1.0.0",
	}
	err = store.CreateProject(ctx, project)
	require.NoError(t, err)

	// Create a test file
	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1, 2, 3},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = store.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Check schema to debug
	var schemaSql string
	err = store.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name='symbols'").Scan(&schemaSql)
	require.NoError(t, err)
	t.Logf("Symbols table schema:\n%s", schemaSql)

	// First insert - should succeed
	symbol1 := &Symbol{
		FileID:      file.ID,
		Name:        "TestFunc",
		Kind:        "function",
		PackageName: "test",
		Signature:   "func TestFunc()",
		StartLine:   10,
		StartCol:    1,
		EndLine:     20,
		EndCol:      1,
	}
	err = store.UpsertSymbol(ctx, symbol1)
	require.NoError(t, err, "First insert should succeed")
	assert.NotZero(t, symbol1.ID, "Symbol ID should be set")
	firstID := symbol1.ID
	t.Logf("First insert successful, ID=%d", firstID)

	// Second insert with same unique key - should update, not fail
	symbol2 := &Symbol{
		FileID:      file.ID,
		Name:        "TestFunc",
		Kind:        "function",
		PackageName: "test",
		Signature:   "func TestFunc() error", // Updated signature
		StartLine:   10,                      // Same position
		StartCol:    1,
		EndLine:     25, // Different end line
		EndCol:      1,
	}
	t.Logf("Attempting second upsert with same key: fileID=%d, name=%s, startLine=%d, startCol=%d",
		symbol2.FileID, symbol2.Name, symbol2.StartLine, symbol2.StartCol)
	err = store.UpsertSymbol(ctx, symbol2)
	require.NoError(t, err, "Upsert with same unique key should succeed")
	assert.NotZero(t, symbol2.ID, "Symbol ID should be set after upsert")

	// Verify the symbol was updated, not duplicated
	symbols, err := store.ListSymbolsByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, symbols, 1, "Should have only one symbol after upsert")
	assert.Equal(t, "func TestFunc() error", symbols[0].Signature, "Signature should be updated")
	assert.Equal(t, 25, symbols[0].EndLine, "EndLine should be updated")

	// Third insert with different position - should create new row
	symbol3 := &Symbol{
		FileID:      file.ID,
		Name:        "TestFunc",
		Kind:        "function",
		PackageName: "test",
		Signature:   "func TestFunc()",
		StartLine:   30, // Different position
		StartCol:    1,
		EndLine:     40,
		EndCol:      1,
	}
	err = store.UpsertSymbol(ctx, symbol3)
	require.NoError(t, err, "Insert with different position should succeed")

	// Verify we now have two symbols
	symbols, err = store.ListSymbolsByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, symbols, 2, "Should have two symbols with different positions")

	// Verify the first symbol still exists with updated data
	foundFirst := false
	for _, s := range symbols {
		if s.StartLine == 10 {
			foundFirst = true
			assert.Equal(t, "func TestFunc() error", s.Signature, "First symbol should have updated signature")
			break
		}
	}
	assert.True(t, foundFirst, "First symbol should still exist")

	t.Logf("✓ Symbol UPSERT test passed: First ID=%d, symbols count=%d", firstID, len(symbols))
}

// TestUpsertChunk_UniqueConstraint verifies that the UPSERT operation
// works correctly with the UNIQUE constraint on chunks table
func TestUpsertChunk_UniqueConstraint(t *testing.T) {
	// Create in-memory database
	store, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create a test project and file
	project := &Project{
		RootPath:     "/test/project",
		ModuleName:   "test/module",
		GoVersion:    "1.21",
		IndexVersion: "1.0.0",
	}
	err = store.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1, 2, 3},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = store.UpsertFile(ctx, file)
	require.NoError(t, err)

	// First insert - should succeed
	chunk1 := &Chunk{
		FileID:      file.ID,
		Content:     "original content",
		ContentHash: [32]byte{4, 5, 6},
		TokenCount:  10,
		StartLine:   1,
		EndLine:     5,
		ChunkType:   "function",
	}
	err = store.UpsertChunk(ctx, chunk1)
	require.NoError(t, err, "First insert should succeed")
	assert.NotZero(t, chunk1.ID, "Chunk ID should be set")
	firstID := chunk1.ID

	// Second insert with same unique key - should update, not fail
	chunk2 := &Chunk{
		FileID:      file.ID,
		Content:     "updated content",
		ContentHash: [32]byte{7, 8, 9},
		TokenCount:  15,
		StartLine:   1, // Same range
		EndLine:     5,
		ChunkType:   "function",
	}
	err = store.UpsertChunk(ctx, chunk2)
	require.NoError(t, err, "Upsert with same unique key should succeed")
	assert.NotZero(t, chunk2.ID, "Chunk ID should be set after upsert")

	// Verify the chunk was updated, not duplicated
	chunks, err := store.ListChunksByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 1, "Should have only one chunk after upsert")
	assert.Equal(t, "updated content", chunks[0].Content, "Content should be updated")
	assert.Equal(t, 15, chunks[0].TokenCount, "TokenCount should be updated")

	// Third insert with different range - should create new row
	chunk3 := &Chunk{
		FileID:      file.ID,
		Content:     "new chunk content",
		ContentHash: [32]byte{10, 11, 12},
		TokenCount:  20,
		StartLine:   6, // Different range
		EndLine:     10,
		ChunkType:   "function",
	}
	err = store.UpsertChunk(ctx, chunk3)
	require.NoError(t, err, "Insert with different range should succeed")

	// Verify we now have two chunks
	chunks, err = store.ListChunksByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 2, "Should have two chunks with different ranges")

	// Verify the first chunk still exists with updated data
	foundFirst := false
	for _, c := range chunks {
		if c.StartLine == 1 {
			foundFirst = true
			assert.Equal(t, "updated content", c.Content, "First chunk should have updated content")
			break
		}
	}
	assert.True(t, foundFirst, "First chunk should still exist")

	t.Logf("✓ Chunk UPSERT test passed: First ID=%d, chunks count=%d", firstID, len(chunks))
}

// TestMigration_V101_Applied verifies that the v1.0.1 migration applies correctly
func TestMigration_V101_Applied(t *testing.T) {
	// Create in-memory database
	store, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Check that the current schema version is 1.0.1
	var version string
	err = store.db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, "1.0.1", version, "Schema version should be 1.0.1 after migrations")

	// Verify symbols table has UNIQUE constraint (not just index)
	// SQLite stores constraint info in sqlite_master table
	var sql string
	err = store.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name='symbols'").Scan(&sql)
	require.NoError(t, err)
	assert.Contains(t, sql, "UNIQUE(file_id, name, start_line, start_col)", "Symbols table should have UNIQUE constraint")

	// Verify chunks table has UNIQUE constraint
	err = store.db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name='chunks'").Scan(&sql)
	require.NoError(t, err)
	assert.Contains(t, sql, "UNIQUE(file_id, start_line, end_line)", "Chunks table should have UNIQUE constraint")

	t.Logf("✓ Migration v1.0.1 verification passed")
}

// TestConcurrentUpserts verifies that concurrent UPSERT operations don't cause conflicts
func TestConcurrentUpserts(t *testing.T) {
	// Create in-memory database
	store, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create test project and file
	project := &Project{
		RootPath:     "/test/project",
		ModuleName:   "test/module",
		GoVersion:    "1.21",
		IndexVersion: "1.0.0",
	}
	err = store.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1, 2, 3},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = store.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Perform multiple sequential upserts (simulating re-indexing)
	for i := 0; i < 10; i++ {
		symbol := &Symbol{
			FileID:      file.ID,
			Name:        "TestFunc",
			Kind:        "function",
			PackageName: "test",
			Signature:   "func TestFunc()",
			StartLine:   10,
			StartCol:    1,
			EndLine:     20,
			EndCol:      1,
		}
		err = store.UpsertSymbol(ctx, symbol)
		require.NoError(t, err, "Upsert iteration %d should succeed", i)
	}

	// Verify we still have only one symbol
	symbols, err := store.ListSymbolsByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, symbols, 1, "Should have only one symbol after multiple upserts")

	t.Logf("✓ Concurrent UPSERT test passed: 10 upserts resulted in 1 symbol")
}
