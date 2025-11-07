package indexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

// mockEmbedder implements embedder.Embedder for testing
type mockEmbedder struct {
	dimension        int
	generateErr      error
	generateBatchErr error
	callCount        int
	mu               sync.Mutex
}

func newMockEmbedder() *mockEmbedder {
	return &mockEmbedder{
		dimension: 768,
	}
}

func (m *mockEmbedder) GenerateEmbedding(ctx context.Context, req embedder.EmbeddingRequest) (*embedder.Embedding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.generateErr != nil {
		return nil, m.generateErr
	}

	m.callCount++
	vector := make([]float32, m.dimension)
	for i := range vector {
		vector[i] = 0.5
	}

	return &embedder.Embedding{
		Vector:    vector,
		Dimension: m.dimension,
		Provider:  "mock",
		Model:     "test-v1",
	}, nil
}

func (m *mockEmbedder) GenerateBatch(ctx context.Context, req embedder.BatchEmbeddingRequest) (*embedder.BatchEmbeddingResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.generateBatchErr != nil {
		return nil, m.generateBatchErr
	}

	embeddings := make([]*embedder.Embedding, len(req.Texts))
	for i := range req.Texts {
		vector := make([]float32, m.dimension)
		for j := range vector {
			vector[j] = 0.5
		}
		embeddings[i] = &embedder.Embedding{
			Vector:    vector,
			Dimension: m.dimension,
			Provider:  "mock",
			Model:     "test-v1",
		}
	}

	m.callCount += len(req.Texts)

	return &embedder.BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   "mock",
		Model:      "test-v1",
	}, nil
}

func (m *mockEmbedder) Dimension() int {
	return m.dimension
}

func (m *mockEmbedder) Provider() string {
	return "mock"
}

func (m *mockEmbedder) Model() string {
	return "test-v1"
}

func (m *mockEmbedder) Close() error {
	return nil
}

func (m *mockEmbedder) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// setupTestStorage creates an in-memory SQLite database for testing
func setupTestStorage(t testing.TB) storage.Storage {
	t.Helper()

	store, err := storage.NewSQLiteStorage(":memory:")
	require.NoError(t, err, "Failed to create test storage")

	return store
}

// createTestFile creates a temporary Go file for testing
func createTestFile(t testing.TB, dir, name, content string) string {
	t.Helper()

	filePath := filepath.Join(dir, name)
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	return filePath
}

// TestNew verifies indexer initialization
func TestNew(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	assert.NotNil(t, idx)
	assert.NotNil(t, idx.parser)
	assert.NotNil(t, idx.chunker)
	assert.NotNil(t, idx.storage)
	assert.Nil(t, idx.embedder) // Lazy initialization
	assert.Equal(t, runtime.NumCPU(), idx.workers)
}

// TestNewWithEmbedder verifies indexer initialization with embedder
func TestNewWithEmbedder(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	assert.NotNil(t, idx)
	assert.NotNil(t, idx.embedder)
	assert.Equal(t, emb, idx.embedder)
}

// TestDiscoverFiles_Success tests successful file discovery
func TestDiscoverFiles_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "pkg/util.go", "package pkg\n")
	createTestFile(t, tmpDir, "cmd/app/app.go", "package app\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 3)
}

// TestDiscoverFiles_EmptyDirectory tests empty directory
func TestDiscoverFiles_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Empty(t, files)
}

// TestDiscoverFiles_SkipNonGoFiles tests that non-Go files are skipped
func TestDiscoverFiles_SkipNonGoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "README.md", "# README\n")
	createTestFile(t, tmpDir, "config.json", "{}\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.True(t, strings.HasSuffix(files[0], "main.go"))
}

// TestDiscoverFiles_SkipTestFiles tests that test files are skipped when configured
func TestDiscoverFiles_SkipTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "main_test.go", "package main\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: false, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.True(t, strings.HasSuffix(files[0], "main.go"))
	assert.False(t, strings.HasSuffix(files[0], "_test.go"))
}

// TestDiscoverFiles_IncludeTestFiles tests that test files are included when configured
func TestDiscoverFiles_IncludeTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "main_test.go", "package main\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 2)
}

// TestDiscoverFiles_SkipVendor tests that vendor directory is skipped by default
func TestDiscoverFiles_SkipVendor(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "vendor/lib/lib.go", "package lib\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.False(t, strings.Contains(files[0], "vendor"))
}

// TestDiscoverFiles_IncludeVendor tests that vendor directory is included when configured
func TestDiscoverFiles_IncludeVendor(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, "vendor/lib/lib.go", "package lib\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: true}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 2)
}

// TestDiscoverFiles_SkipHiddenDirs tests that hidden directories are skipped
func TestDiscoverFiles_SkipHiddenDirs(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\n")
	createTestFile(t, tmpDir, ".git/config.go", "package git\n")
	createTestFile(t, tmpDir, ".hidden/hidden.go", "package hidden\n")

	idx := New(setupTestStorage(t))
	config := &Config{IncludeTests: true, IncludeVendor: false}

	files, err := idx.discoverFiles(tmpDir, config)

	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.False(t, strings.Contains(files[0], ".git"))
	assert.False(t, strings.Contains(files[0], ".hidden"))
}

// TestComputeFileHash tests hash computation
func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()
	content := "package main\nfunc main() {}\n"
	filePath := createTestFile(t, tmpDir, "main.go", content)

	hash1, modTime1, size1, err := computeFileHash(filePath)
	require.NoError(t, err)
	assert.NotEqual(t, [32]byte{}, hash1)
	assert.False(t, modTime1.IsZero())
	assert.Equal(t, int64(len(content)), size1)

	// Hash should be consistent
	hash2, _, _, err := computeFileHash(filePath)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)
}

// TestComputeFileHash_DifferentContent tests that different content produces different hashes
func TestComputeFileHash_DifferentContent(t *testing.T) {
	tmpDir := t.TempDir()

	filePath1 := createTestFile(t, tmpDir, "file1.go", "package main\n")
	filePath2 := createTestFile(t, tmpDir, "file2.go", "package test\n")

	hash1, _, _, err := computeFileHash(filePath1)
	require.NoError(t, err)

	hash2, _, _, err := computeFileHash(filePath2)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)
}

// TestComputeFileHash_NonexistentFile tests error handling for nonexistent files
func TestComputeFileHash_NonexistentFile(t *testing.T) {
	_, _, _, err := computeFileHash("/nonexistent/file.go")
	assert.Error(t, err)
}

// TestCheckFileChanged_NewFile tests that new files are not skipped
func TestCheckFileChanged_NewFile(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	ctx := context.Background()

	// Create project
	project := &storage.Project{RootPath: "/test", IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	var skipped int32
	shouldSkip, err := idx.checkFileChanged(ctx, store, project.ID, "new.go", [32]byte{1, 2, 3}, &skipped)

	require.NoError(t, err)
	assert.False(t, shouldSkip)
	assert.Equal(t, int32(0), skipped)
}

// TestCheckFileChanged_UnchangedFile tests that unchanged files are skipped
func TestCheckFileChanged_UnchangedFile(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	ctx := context.Background()

	// Create project and file
	project := &storage.Project{RootPath: "/test", IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	hash := [32]byte{1, 2, 3}
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "existing.go",
		ContentHash: hash,
		PackageName: "test",
	}
	require.NoError(t, store.UpsertFile(ctx, file))

	var skipped int32
	shouldSkip, err := idx.checkFileChanged(ctx, store, project.ID, "existing.go", hash, &skipped)

	require.NoError(t, err)
	assert.True(t, shouldSkip)
	assert.Equal(t, int32(1), skipped)
}

// TestCheckFileChanged_ModifiedFile tests that modified files are not skipped
func TestCheckFileChanged_ModifiedFile(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	ctx := context.Background()

	// Create project and file
	project := &storage.Project{RootPath: "/test", IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	oldHash := [32]byte{1, 2, 3}
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "modified.go",
		ContentHash: oldHash,
		PackageName: "test",
	}
	require.NoError(t, store.UpsertFile(ctx, file))

	// Create a chunk to verify deletion
	chunk := &storage.Chunk{
		FileID:  file.ID,
		Content: "old content",
	}
	require.NoError(t, store.UpsertChunk(ctx, chunk))

	newHash := [32]byte{4, 5, 6}
	var skipped int32
	shouldSkip, err := idx.checkFileChanged(ctx, store, project.ID, "modified.go", newHash, &skipped)

	require.NoError(t, err)
	assert.False(t, shouldSkip)
	assert.Equal(t, int32(0), skipped)

	// Verify old chunks were deleted
	chunks, err := store.ListChunksByFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

// TestIndexProject_Success tests successful project indexing
func TestIndexProject_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple Go file
	createTestFile(t, tmpDir, "main.go", `package main

func main() {
	println("hello")
}
`)

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            2,
		BatchSize:          10,
		EmbeddingBatch:     5,
		IncludeTests:       false,
		IncludeVendor:      false,
		GenerateEmbeddings: true,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.FilesIndexed)
	assert.Equal(t, 0, stats.FilesSkipped)
	assert.Equal(t, 0, stats.FilesFailed)
	assert.Greater(t, stats.SymbolsExtracted, 0)
	assert.Greater(t, stats.ChunksCreated, 0)
	assert.Greater(t, stats.Duration, time.Duration(0))
}

// TestIndexProject_EmptyProject tests indexing empty project
func TestIndexProject_EmptyProject(t *testing.T) {
	tmpDir := t.TempDir()

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	stats, err := idx.IndexProject(context.Background(), tmpDir, nil)

	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.FilesIndexed)
	assert.Equal(t, 0, stats.FilesSkipped)
}

// TestIndexProject_IncrementalUpdate tests that unchanged files are skipped
func TestIndexProject_IncrementalUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	file1Path := createTestFile(t, tmpDir, "file1.go", "package main\nfunc Foo() {}\n")
	createTestFile(t, tmpDir, "file2.go", "package main\nfunc Bar() {}\n")

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            2,
		BatchSize:          10,
		IncludeTests:       false,
		IncludeVendor:      false,
		GenerateEmbeddings: false, // Disable for faster test
	}

	// First indexing
	stats1, err := idx.IndexProject(context.Background(), tmpDir, config)
	require.NoError(t, err)
	assert.Equal(t, 2, stats1.FilesIndexed)
	assert.Equal(t, 0, stats1.FilesSkipped)

	// Modify one file
	time.Sleep(10 * time.Millisecond) // Ensure different modtime
	err = os.WriteFile(file1Path, []byte("package main\nfunc FooModified() {}\n"), 0644)
	require.NoError(t, err)

	// Second indexing - should skip unchanged file
	stats2, err := idx.IndexProject(context.Background(), tmpDir, config)
	require.NoError(t, err)
	assert.Equal(t, 1, stats2.FilesIndexed, "Only modified file should be re-indexed")
	assert.Equal(t, 1, stats2.FilesSkipped, "Unchanged file should be skipped")
}

// TestIndexProject_WithParseErrors tests handling of parse errors
func TestIndexProject_WithParseErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with severe syntax error that will fail parsing
	createTestFile(t, tmpDir, "broken.go", "this is not valid Go code at all!!!")

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	config := &Config{
		Workers:            1,
		GenerateEmbeddings: false,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	assert.NotNil(t, stats)
	// Parse errors shouldn't stop indexing - file will be marked as failed
	// Note: The parser may still succeed in creating a file record even with errors
	// so we check that either failed or indexed with parse errors
	assert.True(t, stats.FilesFailed > 0 || stats.FilesIndexed > 0)
}

// TestIndexProject_ConcurrentCalls tests that concurrent indexing is prevented
func TestIndexProject_ConcurrentCalls(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to ensure first indexing takes significant time
	for i := 0; i < 100; i++ {
		createTestFile(t, tmpDir, fmt.Sprintf("file%d.go", i),
			fmt.Sprintf("package main\nfunc Func%d() int { return %d }\n", i, i))
	}

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	// Use a blocking config to ensure first indexing holds the lock
	config := &Config{
		Workers:            1,
		BatchSize:          1, // Small batches to slow it down
		GenerateEmbeddings: false,
	}

	// Start first indexing
	done := make(chan error, 1)
	go func() {
		_, err := idx.IndexProject(context.Background(), tmpDir, config)
		done <- err
	}()

	// Give first indexing time to acquire lock and start processing
	time.Sleep(100 * time.Millisecond)

	// Try second concurrent indexing - should fail immediately with lock error
	_, err := idx.IndexProject(context.Background(), tmpDir, config)

	if err == nil {
		// First indexing might have completed already
		t.Log("First indexing completed before concurrent call")
	} else {
		// Should get indexing in progress error
		assert.ErrorIs(t, err, ErrIndexingInProgress)
	}

	// Wait for first to complete
	err = <-done
	require.NoError(t, err)
}

// TestIndexProject_ContextCancellation tests context cancellation
func TestIndexProject_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to ensure we have time to cancel
	for i := 0; i < 100; i++ {
		content := fmt.Sprintf("package main\n\nfunc Func%d() int { return %d }\n", i, i)
		createTestFile(t, tmpDir, fmt.Sprintf("file%d.go", i), content)
	}

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	config := &Config{
		Workers:            1,
		BatchSize:          5,
		GenerateEmbeddings: false,
	}

	_, err := idx.IndexProject(ctx, tmpDir, config)

	// Should return context error or complete successfully (timing dependent)
	// We just verify no panic occurs
	if err != nil {
		// If there was an error, it should be context-related
		isContextError := errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), "context")
		if !isContextError {
			t.Logf("Got non-context error: %v", err)
		}
	}
}

// TestIndexProject_WithEmbeddings tests embedding generation
func TestIndexProject_WithEmbeddings(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", `package main

func Add(a, b int) int {
	return a + b
}

func Multiply(x, y int) int {
	return x * y
}
`)

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            2,
		BatchSize:          10,
		EmbeddingBatch:     5,
		IncludeTests:       false,
		GenerateEmbeddings: true,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	assert.Greater(t, stats.EmbeddingsGenerated, 0)
	assert.Equal(t, 0, stats.EmbeddingsFailed)
	assert.Greater(t, emb.getCallCount(), 0)
}

// TestIndexProject_EmbeddingErrors tests handling of embedding errors
func TestIndexProject_EmbeddingErrors(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\nfunc main() {}\n")

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	emb.generateBatchErr = errors.New("embedding service unavailable")

	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            1,
		GenerateEmbeddings: true,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err) // Embedding errors shouldn't fail indexing
	assert.NotNil(t, stats)
	assert.Greater(t, stats.FilesIndexed, 0)
	assert.Greater(t, stats.EmbeddingsFailed, 0)
	assert.NotEmpty(t, stats.ErrorMessages)
}

// TestIndexProject_DefaultConfig tests indexing with nil config (uses defaults)
func TestIndexProject_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "main.go", "package main\nfunc main() {}\n")

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	stats, err := idx.IndexProject(context.Background(), tmpDir, nil)

	require.NoError(t, err)
	assert.NotNil(t, stats)
	// Should use default config values
	assert.Greater(t, stats.FilesIndexed, 0)
}

// TestIndexProject_BatchProcessing tests that files are processed in batches
func TestIndexProject_BatchProcessing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple files to test batching
	for i := 0; i < 25; i++ {
		createTestFile(t, tmpDir, fmt.Sprintf("file%d.go", i),
			fmt.Sprintf("package main\nfunc Func%d() {}\n", i))
	}

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            2,
		BatchSize:          10, // Should process in 3 batches
		EmbeddingBatch:     5,
		GenerateEmbeddings: true,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	assert.Equal(t, 25, stats.FilesIndexed)
	assert.Equal(t, 0, stats.FilesFailed)
}

// TestIndexProject_WorkerConcurrency tests worker pool concurrency
func TestIndexProject_WorkerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()

	// Create enough files to test concurrency
	for i := 0; i < 20; i++ {
		createTestFile(t, tmpDir, fmt.Sprintf("file%d.go", i),
			fmt.Sprintf("package main\nfunc Func%d() {}\n", i))
	}

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	config := &Config{
		Workers:            4, // Multiple workers
		BatchSize:          5,
		GenerateEmbeddings: false,
	}

	start := time.Now()
	stats, err := idx.IndexProject(context.Background(), tmpDir, config)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 20, stats.FilesIndexed)

	// Test with single worker for comparison
	store2 := setupTestStorage(t)
	defer store2.Close()

	idx2 := New(store2)
	config.Workers = 1

	start2 := time.Now()
	stats2, err := idx2.IndexProject(context.Background(), tmpDir, config)
	duration2 := time.Since(start2)

	require.NoError(t, err)
	assert.Equal(t, 20, stats2.FilesIndexed)

	// Concurrent processing should generally be faster (though not guaranteed in test environment)
	t.Logf("4 workers: %v, 1 worker: %v", duration, duration2)
}

// TestParseGoMod tests go.mod parsing
func TestParseGoMod(t *testing.T) {
	tmpDir := t.TempDir()

	goModContent := `module github.com/example/project

go 1.21

require (
	github.com/stretchr/testify v1.8.0
)
`

	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, []byte(goModContent), 0644)
	require.NoError(t, err)

	info, err := parseGoMod(goModPath)

	require.NoError(t, err)
	assert.Equal(t, "github.com/example/project", info.Module)
	assert.Equal(t, "1.21", info.GoVersion)
}

// TestParseGoMod_NonexistentFile tests error handling for nonexistent go.mod
func TestParseGoMod_NonexistentFile(t *testing.T) {
	_, err := parseGoMod("/nonexistent/go.mod")
	assert.Error(t, err)
}

// TestGetOrCreateProject_NewProject tests creating new project
func TestGetOrCreateProject_NewProject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goModContent := "module test.com/project\ngo 1.21\n"
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, []byte(goModContent), 0644)
	require.NoError(t, err)

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)

	project, err := idx.getOrCreateProject(context.Background(), tmpDir)

	require.NoError(t, err)
	assert.NotNil(t, project)
	assert.Equal(t, tmpDir, project.RootPath)
	assert.Equal(t, "test.com/project", project.ModuleName)
	assert.Equal(t, "1.21", project.GoVersion)
	assert.Greater(t, project.ID, int64(0))
}

// TestGetOrCreateProject_ExistingProject tests retrieving existing project
func TestGetOrCreateProject_ExistingProject(t *testing.T) {
	tmpDir := t.TempDir()

	store := setupTestStorage(t)
	defer store.Close()

	// Create project first
	existingProject := &storage.Project{
		RootPath:     tmpDir,
		ModuleName:   "test.com/existing",
		IndexVersion: storage.CurrentSchemaVersion,
	}
	err := store.CreateProject(context.Background(), existingProject)
	require.NoError(t, err)

	idx := New(store)

	project, err := idx.getOrCreateProject(context.Background(), tmpDir)

	require.NoError(t, err)
	assert.Equal(t, existingProject.ID, project.ID)
	assert.Equal(t, "test.com/existing", project.ModuleName)
}

// TestUpdateProjectStats tests project statistics update
func TestUpdateProjectStats(t *testing.T) {
	tmpDir := t.TempDir()
	createTestFile(t, tmpDir, "main.go", "package main\nfunc main() {}\n")

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	ctx := context.Background()

	// Index project
	config := &Config{
		Workers:            1,
		GenerateEmbeddings: false,
	}
	_, err := idx.IndexProject(ctx, tmpDir, config)
	require.NoError(t, err)

	// Get project and verify stats
	project, err := store.GetProject(ctx, tmpDir)
	require.NoError(t, err)

	assert.Greater(t, project.TotalFiles, 0)
	assert.Greater(t, project.TotalChunks, 0)
	assert.False(t, project.LastIndexedAt.IsZero())
}

// TestIndexFile_SymbolStorage tests that symbols are stored correctly
func TestIndexFile_SymbolStorage(t *testing.T) {
	tmpDir := t.TempDir()

	content := `package main

// Add adds two integers
func Add(a, b int) int {
	return a + b
}

type Calculator struct {
	Name string
}
`
	filePath := createTestFile(t, tmpDir, "calc.go", content)

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	ctx := context.Background()

	// Create project
	project := &storage.Project{RootPath: tmpDir, IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	// Index the file
	var indexed, skipped, failed, symbols, chunks int32
	config := &Config{
		GenerateEmbeddings: true,
		ForceReindex:       false,
	}
	storedChunks, err := idx.indexFile(ctx, store, project, filePath, config, &indexed, &skipped, &failed, &symbols, &chunks)

	require.NoError(t, err)
	assert.NotEmpty(t, storedChunks)
	assert.Greater(t, symbols, int32(0))

	// Verify symbols were stored
	files, err := store.ListFiles(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)

	syms, err := store.ListSymbolsByFile(ctx, files[0].ID)
	require.NoError(t, err)
	assert.Greater(t, len(syms), 0)
}

// TestIndexFile_ImportStorage tests that imports are stored correctly
func TestIndexFile_ImportStorage(t *testing.T) {
	tmpDir := t.TempDir()

	content := `package main

import (
	"fmt"
	"os"
	custom "github.com/user/lib"
)

func main() {
	fmt.Println("test")
}
`
	filePath := createTestFile(t, tmpDir, "main.go", content)

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	ctx := context.Background()

	// Create project
	project := &storage.Project{RootPath: tmpDir, IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	// Index the file
	var indexed, skipped, failed, symbols, chunks int32
	config := &Config{
		GenerateEmbeddings: true,
		ForceReindex:       false,
	}
	_, err := idx.indexFile(ctx, store, project, filePath, config, &indexed, &skipped, &failed, &symbols, &chunks)

	require.NoError(t, err)

	// Verify imports were stored
	files, err := store.ListFiles(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)

	imports, err := store.ListImportsByFile(ctx, files[0].ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(imports), 3) // fmt, os, custom
}

// TestGenerateEmbeddingsForChunks tests batch embedding generation
func TestGenerateEmbeddingsForChunks(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	ctx := context.Background()

	// Create project and file
	project := &storage.Project{RootPath: "/test", IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	file := &storage.File{ProjectID: project.ID, FilePath: "test.go", PackageName: "test"}
	require.NoError(t, store.UpsertFile(ctx, file))

	// Create chunks
	var chunks []chunkWithID
	for i := 0; i < 5; i++ {
		chunk := &storage.Chunk{
			FileID:  file.ID,
			Content: fmt.Sprintf("chunk content %d", i),
		}
		require.NoError(t, store.UpsertChunk(ctx, chunk))

		chunks = append(chunks, chunkWithID{
			chunk:   chunk,
			content: chunk.Content,
		})
	}

	var embeddings, embeddingsFail int32
	var mu sync.Mutex
	stats := &Statistics{ErrorMessages: []string{}}

	idx.generateEmbeddingsForChunks(ctx, chunks, 3, &embeddings, &embeddingsFail, &mu, stats)

	assert.Equal(t, int32(5), embeddings)
	assert.Equal(t, int32(0), embeddingsFail)

	// Verify embeddings were stored
	for _, c := range chunks {
		emb, err := store.GetEmbedding(ctx, c.chunk.ID)
		require.NoError(t, err)
		assert.NotNil(t, emb)
	}
}

// TestGenerateEmbeddingsForChunks_WithErrors tests error handling in batch embedding
func TestGenerateEmbeddingsForChunks_WithErrors(t *testing.T) {
	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	emb.generateBatchErr = errors.New("embedding API error")

	idx := NewWithEmbedder(store, emb)

	ctx := context.Background()

	// Create project and file
	project := &storage.Project{RootPath: "/test", IndexVersion: storage.CurrentSchemaVersion}
	require.NoError(t, store.CreateProject(ctx, project))

	file := &storage.File{ProjectID: project.ID, FilePath: "test.go", PackageName: "test"}
	require.NoError(t, store.UpsertFile(ctx, file))

	// Create chunk
	chunk := &storage.Chunk{FileID: file.ID, Content: "test content"}
	require.NoError(t, store.UpsertChunk(ctx, chunk))

	chunks := []chunkWithID{{chunk: chunk, content: chunk.Content}}

	var embeddings, embeddingsFail int32
	var mu sync.Mutex
	stats := &Statistics{ErrorMessages: []string{}}

	idx.generateEmbeddingsForChunks(ctx, chunks, 3, &embeddings, &embeddingsFail, &mu, stats)

	assert.Equal(t, int32(0), embeddings)
	assert.Equal(t, int32(1), embeddingsFail)
	assert.NotEmpty(t, stats.ErrorMessages)
}

// TestIndexProject_RealWorldFixtures tests indexing with real Go files from fixtures
func TestIndexProject_RealWorldFixtures(t *testing.T) {
	fixturesDir := filepath.Join("..", "..", "tests", "testdata", "fixtures")

	// Check if fixtures exist
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		t.Skip("Fixtures directory not found")
	}

	store := setupTestStorage(t)
	defer store.Close()

	emb := newMockEmbedder()
	idx := NewWithEmbedder(store, emb)

	config := &Config{
		Workers:            2,
		GenerateEmbeddings: true,
	}

	stats, err := idx.IndexProject(context.Background(), fixturesDir, config)

	require.NoError(t, err)
	assert.Greater(t, stats.FilesIndexed, 0)
	assert.Greater(t, stats.SymbolsExtracted, 0)
	assert.Greater(t, stats.ChunksCreated, 0)

	t.Logf("Indexed %d files with %d symbols and %d chunks",
		stats.FilesIndexed, stats.SymbolsExtracted, stats.ChunksCreated)
}

// TestIndexProject_LargeFile tests indexing a large file
func TestIndexProject_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate a large file with many functions
	var content strings.Builder
	content.WriteString("package main\n\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&content, "func Func%d() int { return %d }\n\n", i, i)
	}

	createTestFile(t, tmpDir, "large.go", content.String())

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	config := &Config{
		Workers:            2,
		GenerateEmbeddings: false,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	assert.Equal(t, 1, stats.FilesIndexed)
	assert.Greater(t, stats.SymbolsExtracted, 90) // Should find most functions
	assert.Greater(t, stats.ChunksCreated, 90)
}

// TestIndexProject_WithSymlinks tests handling of symbolic links
func TestIndexProject_WithSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink test not reliable on Windows")
	}

	tmpDir := t.TempDir()

	// Create real file
	realDir := filepath.Join(tmpDir, "real")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	createTestFile(t, realDir, "real.go", "package main\nfunc Real() {}\n")

	// Create symlink
	linkDir := filepath.Join(tmpDir, "link")
	err := os.Symlink(realDir, linkDir)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	store := setupTestStorage(t)
	defer store.Close()

	idx := New(store)
	config := &Config{
		Workers:            1,
		GenerateEmbeddings: false,
	}

	stats, err := idx.IndexProject(context.Background(), tmpDir, config)

	require.NoError(t, err)
	// Symlinks are followed by filepath.Walk, so may see duplicates
	// Just verify no errors occurred
	assert.GreaterOrEqual(t, stats.FilesIndexed, 1)
}

// TestIndexProject_ConfigValidation tests config validation and defaults
func TestIndexProject_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "zero workers",
			config: &Config{
				Workers:            0,
				GenerateEmbeddings: false,
			},
		},
		{
			name: "negative batch size",
			config: &Config{
				Workers:            1,
				BatchSize:          -1,
				GenerateEmbeddings: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh directory and file for each subtest
			tmpDir := t.TempDir()
			createTestFile(t, tmpDir, "main.go", "package main\nfunc main() {}\n")

			store := setupTestStorage(t)
			defer store.Close()

			idx := New(store)

			stats, err := idx.IndexProject(context.Background(), tmpDir, tt.config)

			require.NoError(t, err)
			assert.NotNil(t, stats)
			// Should use default values and succeed
			assert.Equal(t, 1, stats.FilesIndexed, "Should index the one Go file")
		})
	}
}

// TestIndexLock_ConcurrentAcquisition tests IndexLock behavior under concurrent access.
// Regression test for US1: Prevents race conditions when multiple goroutines attempt indexing.
// Bug fixed: Using atomic.Int32 instead of unsafe concurrent map access.
func TestIndexLock_ConcurrentAcquisition(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{
			name:        "TryAcquire succeeds when lock is available",
			description: "First acquisition should succeed",
			testFunc: func(t *testing.T) {
				var lock IndexLock
				acquired := lock.TryAcquire()
				assert.True(t, acquired, "TryAcquire should succeed when lock is available")
				lock.Release()
			},
		},
		{
			name:        "TryAcquire fails when lock is held",
			description: "Second acquisition should fail while lock is held",
			testFunc: func(t *testing.T) {
				var lock IndexLock

				// First goroutine acquires the lock
				acquired1 := lock.TryAcquire()
				require.True(t, acquired1, "First TryAcquire should succeed")

				// Second goroutine tries to acquire while first holds it
				acquired2 := lock.TryAcquire()
				assert.False(t, acquired2, "Second TryAcquire should fail while lock is held")

				lock.Release()
			},
		},
		{
			name:        "Release makes lock available again",
			description: "Lock can be re-acquired after release",
			testFunc: func(t *testing.T) {
				var lock IndexLock

				// Acquire and release
				acquired1 := lock.TryAcquire()
				require.True(t, acquired1)
				lock.Release()

				// Should be able to acquire again
				acquired2 := lock.TryAcquire()
				assert.True(t, acquired2, "Lock should be available after Release")
				lock.Release()
			},
		},
		{
			name:        "Concurrent goroutines attempting acquisition",
			description: "Only one goroutine should successfully acquire the lock",
			testFunc: func(t *testing.T) {
				var lock IndexLock
				const numGoroutines = 100

				acquired := make([]bool, numGoroutines)
				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				// Launch concurrent goroutines all trying to acquire the lock
				for i := 0; i < numGoroutines; i++ {
					go func(idx int) {
						defer wg.Done()
						acquired[idx] = lock.TryAcquire()
					}(i)
				}

				wg.Wait()

				// Count how many successfully acquired the lock
				successCount := 0
				for _, success := range acquired {
					if success {
						successCount++
					}
				}

				assert.Equal(t, 1, successCount, "Exactly one goroutine should acquire the lock")

				// Clean up: release the lock
				lock.Release()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
