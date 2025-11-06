package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *SQLiteStorage {
	// Use in-memory database for testing
	storage, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	require.NotNil(t, storage)
	return storage
}

func TestNewSQLiteStorage(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	assert.NotNil(t, storage)
	assert.NotNil(t, storage.db)
}

func TestClose(t *testing.T) {
	storage := setupTestDB(t)
	err := storage.Close()
	assert.NoError(t, err)
}

func TestCreateProject(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{
		RootPath:   "/test/path",
		ModuleName: "github.com/test/project",
		GoVersion:  "1.21",
	}

	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)
	assert.Greater(t, project.ID, int64(0))

	// Try to create duplicate - should fail
	duplicate := &Project{
		RootPath:   "/test/path",
		ModuleName: "another",
	}
	err = storage.CreateProject(ctx, duplicate)
	assert.Error(t, err) // Unique constraint violation
}

func TestGetProject(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{
		RootPath:   "/test/path",
		ModuleName: "github.com/test/project",
	}

	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	// Get the project
	retrieved, err := storage.GetProject(ctx, "/test/path")
	require.NoError(t, err)
	assert.Equal(t, project.ID, retrieved.ID)
	assert.Equal(t, project.ModuleName, retrieved.ModuleName)
	assert.Equal(t, project.RootPath, retrieved.RootPath)
}

func TestGetProject_NotFound(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	_, err := storage.GetProject(ctx, "/nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateProject(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{
		RootPath:   "/test/path",
		ModuleName: "github.com/test/project",
	}

	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	// Update the project
	project.ModuleName = "github.com/test/updated"
	project.TotalFiles = 10
	project.TotalChunks = 100
	project.LastIndexedAt = time.Now()

	err = storage.UpdateProject(ctx, project)
	require.NoError(t, err)

	// Verify update
	updated, err := storage.GetProject(ctx, "/test/path")
	require.NoError(t, err)
	assert.Equal(t, "github.com/test/updated", updated.ModuleName)
	assert.Equal(t, 10, updated.TotalFiles)
	assert.Equal(t, 100, updated.TotalChunks)
}

func TestUpsertFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "main.go",
		PackageName: "main",
		ContentHash: [32]byte{1, 2, 3},
		ModTime:     time.Now(),
		SizeBytes:   1234,
	}

	// Create file
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)
	assert.Greater(t, file.ID, int64(0))

	originalID := file.ID

	// Update same file
	file.SizeBytes = 5678
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)
	assert.Equal(t, originalID, file.ID) // ID should remain the same
}

func TestGetFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1, 2, 3},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}

	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Get by project and path
	retrieved, err := storage.GetFile(ctx, project.ID, "test.go")
	require.NoError(t, err)
	assert.Equal(t, file.ID, retrieved.ID)
	assert.Equal(t, file.FilePath, retrieved.FilePath)
}

func TestGetFile_NotFound(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	_, err := storage.GetFile(ctx, 999, "nonexistent.go")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListFiles(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	// Create multiple files
	for i := 0; i < 3; i++ {
		file := &File{
			ProjectID:   project.ID,
			FilePath:    "file" + string(rune('A'+i)) + ".go",
			PackageName: "test",
			ContentHash: [32]byte{byte(i)},
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		err = storage.UpsertFile(ctx, file)
		require.NoError(t, err)
	}

	// List files
	files, err := storage.ListFiles(ctx, project.ID)
	require.NoError(t, err)
	assert.Len(t, files, 3)
}

func TestDeleteFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "delete.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}

	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Delete the file
	err = storage.DeleteFile(ctx, file.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = storage.GetFile(ctx, project.ID, "delete.go")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpsertSymbol(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	symbol := &Symbol{
		FileID:     file.ID,
		Name:       "TestFunc",
		Kind:       "function",
		Signature:  "func TestFunc()",
		DocComment: "Test function",
		Scope:      "exported",
		StartLine:  10,
		EndLine:    20,
	}

	err = storage.UpsertSymbol(ctx, symbol)
	require.NoError(t, err)
	assert.Greater(t, symbol.ID, int64(0))
}

func TestListSymbolsByFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Create multiple symbols
	for i := 0; i < 3; i++ {
		symbol := &Symbol{
			FileID:    file.ID,
			Name:      "Func" + string(rune('A'+i)),
			Kind:      "function",
			Signature: "func()",
			Scope:     "exported",
			StartLine: i * 10,
			EndLine:   i*10 + 5,
		}
		err = storage.UpsertSymbol(ctx, symbol)
		require.NoError(t, err)
	}

	// List symbols
	symbols, err := storage.ListSymbolsByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, symbols, 3)
}

func TestUpsertChunk(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	chunk := &Chunk{
		FileID:      file.ID,
		Content:     "func Test() { }",
		ContentHash: [32]byte{1, 2, 3},
		TokenCount:  10,
		StartLine:   1,
		EndLine:     3,
		ChunkType:   "function",
	}

	err = storage.UpsertChunk(ctx, chunk)
	require.NoError(t, err)
	assert.Greater(t, chunk.ID, int64(0))
}

func TestListChunksByFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Create multiple chunks
	for i := 0; i < 3; i++ {
		chunk := &Chunk{
			FileID:      file.ID,
			Content:     "content" + string(rune('A'+i)),
			ContentHash: [32]byte{byte(i)},
			TokenCount:  10,
			StartLine:   i * 10,
			EndLine:     i*10 + 5,
			ChunkType:   "function",
		}
		err = storage.UpsertChunk(ctx, chunk)
		require.NoError(t, err)
	}

	// List chunks
	chunks, err := storage.ListChunksByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 3)
}

func TestDeleteChunksByFile(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	// Create chunks
	for i := 0; i < 3; i++ {
		chunk := &Chunk{
			FileID:      file.ID,
			Content:     "content",
			ContentHash: [32]byte{byte(i)},
			TokenCount:  10,
			StartLine:   i,
			EndLine:     i + 1,
			ChunkType:   "function",
		}
		err = storage.UpsertChunk(ctx, chunk)
		require.NoError(t, err)
	}

	// Delete chunks
	err = storage.DeleteChunksByFile(ctx, file.ID)
	require.NoError(t, err)

	// Verify deletion
	chunks, err := storage.ListChunksByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestBeginTx_CommitRollback(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()

	// Test commit
	tx, err := storage.BeginTx(ctx)
	require.NoError(t, err)

	project := &Project{RootPath: "/test", ModuleName: "test"}
	err = tx.CreateProject(ctx, project)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify committed
	retrieved, err := storage.GetProject(ctx, "/test")
	require.NoError(t, err)
	assert.Equal(t, project.ID, retrieved.ID)

	// Test rollback
	tx2, err := storage.BeginTx(ctx)
	require.NoError(t, err)

	project2 := &Project{RootPath: "/test2", ModuleName: "test2"}
	err = tx2.CreateProject(ctx, project2)
	require.NoError(t, err)

	err = tx2.Rollback()
	require.NoError(t, err)

	// Verify not committed
	_, err = storage.GetProject(ctx, "/test2")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpsertImport(t *testing.T) {
	storage := setupTestDB(t)
	defer storage.Close()

	ctx := context.Background()
	project := &Project{RootPath: "/test", ModuleName: "test"}
	err := storage.CreateProject(ctx, project)
	require.NoError(t, err)

	file := &File{
		ProjectID:   project.ID,
		FilePath:    "test.go",
		PackageName: "test",
		ContentHash: [32]byte{1},
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	err = storage.UpsertFile(ctx, file)
	require.NoError(t, err)

	imp := &Import{
		FileID:     file.ID,
		ImportPath: "fmt",
		Alias:      "",
	}

	err = storage.UpsertImport(ctx, imp)
	require.NoError(t, err)
	assert.Greater(t, imp.ID, int64(0))
}
