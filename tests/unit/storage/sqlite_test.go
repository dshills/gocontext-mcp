package storage_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/dshills/gocontext-mcp/internal/storage"
)

// setupTestStorage creates an in-memory SQLite database for testing
func setupTestStorage(t *testing.T) storage.Storage {
	t.Helper()

	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create test storage: %v", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close storage: %v", err)
		}
	})

	return store
}

// TestProjectOperations tests comprehensive project operations
func TestProjectOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	t.Run("CreateProject_Success", func(t *testing.T) {
		project := &storage.Project{
			RootPath:     "/test/project",
			ModuleName:   "github.com/test/project",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}

		err := store.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		if project.ID == 0 {
			t.Error("expected non-zero ID after creation")
		}

		if project.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after creation")
		}

		if project.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt after creation")
		}
	})

	t.Run("GetProject_Success", func(t *testing.T) {
		// Create project
		project := &storage.Project{
			RootPath:     "/test/get-project",
			ModuleName:   "github.com/test/get",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err := store.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		// Retrieve project
		retrieved, err := store.GetProject(ctx, "/test/get-project")
		if err != nil {
			t.Fatalf("GetProject failed: %v", err)
		}

		if retrieved.ID != project.ID {
			t.Errorf("expected ID %d, got %d", project.ID, retrieved.ID)
		}

		if retrieved.RootPath != project.RootPath {
			t.Errorf("expected RootPath %s, got %s", project.RootPath, retrieved.RootPath)
		}

		if retrieved.ModuleName != project.ModuleName {
			t.Errorf("expected ModuleName %s, got %s", project.ModuleName, retrieved.ModuleName)
		}
	})

	t.Run("GetProject_NotFound", func(t *testing.T) {
		_, err := store.GetProject(ctx, "/nonexistent/path")
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UpdateProject_Success", func(t *testing.T) {
		// Create project
		project := &storage.Project{
			RootPath:     "/test/update-project",
			ModuleName:   "github.com/test/update",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err := store.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		// Update project
		project.TotalFiles = 10
		project.TotalChunks = 50
		project.LastIndexedAt = time.Now()

		err = store.UpdateProject(ctx, project)
		if err != nil {
			t.Fatalf("UpdateProject failed: %v", err)
		}

		// Verify update
		retrieved, err := store.GetProject(ctx, "/test/update-project")
		if err != nil {
			t.Fatalf("GetProject failed: %v", err)
		}

		if retrieved.TotalFiles != 10 {
			t.Errorf("expected TotalFiles 10, got %d", retrieved.TotalFiles)
		}

		if retrieved.TotalChunks != 50 {
			t.Errorf("expected TotalChunks 50, got %d", retrieved.TotalChunks)
		}
	})
}

// TestFileOperations tests comprehensive file operations
func TestFileOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Create a project for testing
	project := &storage.Project{
		RootPath:     "/test/files",
		ModuleName:   "github.com/test/files",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	t.Run("UpsertFile_Insert", func(t *testing.T) {
		hash := sha256.Sum256([]byte("test content"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "main.go",
			PackageName: "main",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}

		err := store.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		if file.ID == 0 {
			t.Error("expected non-zero ID after upsert")
		}

		if file.LastIndexedAt.IsZero() {
			t.Error("expected non-zero LastIndexedAt after upsert")
		}
	})

	t.Run("UpsertFile_Update", func(t *testing.T) {
		hash := sha256.Sum256([]byte("initial"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "update.go",
			PackageName: "pkg1",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}

		// Initial insert
		err := store.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile (insert) failed: %v", err)
		}
		firstID := file.ID

		// Update with same project_id and file_path
		newHash := sha256.Sum256([]byte("updated"))
		file.ContentHash = newHash
		file.PackageName = "pkg2"
		file.SizeBytes = 200

		err = store.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile (update) failed: %v", err)
		}

		// Verify update happened on same record
		retrieved, err := store.GetFile(ctx, project.ID, "update.go")
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}

		if retrieved.ID != firstID {
			t.Errorf("expected same ID %d, got %d (should update not insert)", firstID, retrieved.ID)
		}

		if retrieved.PackageName != "pkg2" {
			t.Errorf("expected PackageName pkg2, got %s", retrieved.PackageName)
		}

		if retrieved.SizeBytes != 200 {
			t.Errorf("expected SizeBytes 200, got %d", retrieved.SizeBytes)
		}

		if retrieved.ContentHash != newHash {
			t.Error("content hash not updated")
		}
	})

	t.Run("GetFile_Success", func(t *testing.T) {
		hash := sha256.Sum256([]byte("get test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "get.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   50,
		}
		if err := store.UpsertFile(ctx, file); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		retrieved, err := store.GetFile(ctx, project.ID, "get.go")
		if err != nil {
			t.Fatalf("GetFile failed: %v", err)
		}

		if retrieved.FilePath != "get.go" {
			t.Errorf("expected FilePath get.go, got %s", retrieved.FilePath)
		}

		if retrieved.PackageName != "test" {
			t.Errorf("expected PackageName test, got %s", retrieved.PackageName)
		}
	})

	t.Run("GetFile_NotFound", func(t *testing.T) {
		_, err := store.GetFile(ctx, project.ID, "nonexistent.go")
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("GetFileByID_Success", func(t *testing.T) {
		hash := sha256.Sum256([]byte("by id test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "byid.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   30,
		}
		if err := store.UpsertFile(ctx, file); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		retrieved, err := store.GetFileByID(ctx, file.ID)
		if err != nil {
			t.Fatalf("GetFileByID failed: %v", err)
		}

		if retrieved.ID != file.ID {
			t.Errorf("expected ID %d, got %d", file.ID, retrieved.ID)
		}

		if retrieved.FilePath != "byid.go" {
			t.Errorf("expected FilePath byid.go, got %s", retrieved.FilePath)
		}
	})

	t.Run("ListFiles_Success", func(t *testing.T) {
		// Create fresh project for clean list test
		listProject := &storage.Project{
			RootPath:     "/test/list-files",
			ModuleName:   "github.com/test/list",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		if err := store.CreateProject(ctx, listProject); err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		// Insert multiple files
		files := []string{"a.go", "b.go", "c.go"}
		for _, fileName := range files {
			hash := sha256.Sum256([]byte(fileName))
			file := &storage.File{
				ProjectID:   listProject.ID,
				FilePath:    fileName,
				PackageName: "test",
				ContentHash: hash,
				ModTime:     time.Now(),
				SizeBytes:   10,
			}
			if err := store.UpsertFile(ctx, file); err != nil {
				t.Fatalf("UpsertFile failed for %s: %v", fileName, err)
			}
		}

		// List files
		retrieved, err := store.ListFiles(ctx, listProject.ID)
		if err != nil {
			t.Fatalf("ListFiles failed: %v", err)
		}

		if len(retrieved) != 3 {
			t.Errorf("expected 3 files, got %d", len(retrieved))
		}

		// Verify order (should be sorted by file_path)
		if len(retrieved) >= 3 {
			if retrieved[0].FilePath != "a.go" || retrieved[1].FilePath != "b.go" || retrieved[2].FilePath != "c.go" {
				t.Error("files not sorted by file_path")
			}
		}
	})

	t.Run("DeleteFile_Success", func(t *testing.T) {
		hash := sha256.Sum256([]byte("delete test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "delete.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   10,
		}
		if err := store.UpsertFile(ctx, file); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		// Delete file
		err := store.DeleteFile(ctx, file.ID)
		if err != nil {
			t.Fatalf("DeleteFile failed: %v", err)
		}

		// Verify deletion
		_, err = store.GetFileByID(ctx, file.ID)
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound after deletion, got %v", err)
		}
	})
}

// TestSymbolOperations tests comprehensive symbol operations
func TestSymbolOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project and file
	project := &storage.Project{
		RootPath:     "/test/symbols",
		ModuleName:   "github.com/test/symbols",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("symbol test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "symbols.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	t.Run("UpsertSymbol_Insert", func(t *testing.T) {
		symbol := &storage.Symbol{
			FileID:       file.ID,
			Name:         "TestFunction",
			Kind:         "function",
			PackageName:  "test",
			Signature:    "func TestFunction()",
			DocComment:   "TestFunction does something",
			Scope:        "exported",
			StartLine:    10,
			StartCol:     0,
			EndLine:      20,
			EndCol:       1,
			IsRepository: true,
		}

		err := store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("UpsertSymbol failed: %v", err)
		}

		if symbol.ID == 0 {
			t.Error("expected non-zero ID after upsert")
		}

		if symbol.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after upsert")
		}
	})

	t.Run("UpsertSymbol_Update", func(t *testing.T) {
		symbol := &storage.Symbol{
			FileID:      file.ID,
			Name:        "UpdateFunc",
			Kind:        "function",
			PackageName: "test",
			Signature:   "func UpdateFunc() v1",
			StartLine:   30,
			StartCol:    0,
			EndLine:     40,
			EndCol:      1,
		}

		// Initial insert
		err := store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("UpsertSymbol (insert) failed: %v", err)
		}
		firstID := symbol.ID

		// Update with same natural key (file_id, name, start_line, start_col)
		symbol.Signature = "func UpdateFunc() v2"
		symbol.DocComment = "Updated documentation"
		symbol.IsService = true

		err = store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("UpsertSymbol (update) failed: %v", err)
		}

		// Verify update happened on same record
		retrieved, err := store.GetSymbol(ctx, firstID)
		if err != nil {
			t.Fatalf("GetSymbol failed: %v", err)
		}

		if retrieved.ID != firstID {
			t.Errorf("expected same ID %d, got %d (should update not insert)", firstID, retrieved.ID)
		}

		if retrieved.Signature != "func UpdateFunc() v2" {
			t.Errorf("expected updated signature, got %s", retrieved.Signature)
		}

		if retrieved.DocComment != "Updated documentation" {
			t.Errorf("expected updated doc comment, got %s", retrieved.DocComment)
		}

		if !retrieved.IsService {
			t.Error("expected IsService to be true after update")
		}
	})

	t.Run("GetSymbol_Success", func(t *testing.T) {
		symbol := &storage.Symbol{
			FileID:      file.ID,
			Name:        "GetSymbol",
			Kind:        "function",
			PackageName: "test",
			StartLine:   50,
			StartCol:    0,
			EndLine:     60,
			EndCol:      1,
		}
		if err := store.UpsertSymbol(ctx, symbol); err != nil {
			t.Fatalf("UpsertSymbol failed: %v", err)
		}

		retrieved, err := store.GetSymbol(ctx, symbol.ID)
		if err != nil {
			t.Fatalf("GetSymbol failed: %v", err)
		}

		if retrieved.Name != "GetSymbol" {
			t.Errorf("expected Name GetSymbol, got %s", retrieved.Name)
		}
	})

	t.Run("ListSymbolsByFile_Success", func(t *testing.T) {
		// Create new file for clean list test
		hash := sha256.Sum256([]byte("list symbols"))
		listFile := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "list.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		if err := store.UpsertFile(ctx, listFile); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		// Insert multiple symbols
		symbols := []string{"Alpha", "Beta", "Gamma"}
		for i, name := range symbols {
			symbol := &storage.Symbol{
				FileID:      listFile.ID,
				Name:        name,
				Kind:        "function",
				PackageName: "test",
				StartLine:   (i + 1) * 10,
				StartCol:    0,
				EndLine:     (i+1)*10 + 5,
				EndCol:      1,
			}
			if err := store.UpsertSymbol(ctx, symbol); err != nil {
				t.Fatalf("UpsertSymbol failed for %s: %v", name, err)
			}
		}

		// List symbols
		retrieved, err := store.ListSymbolsByFile(ctx, listFile.ID)
		if err != nil {
			t.Fatalf("ListSymbolsByFile failed: %v", err)
		}

		if len(retrieved) != 3 {
			t.Errorf("expected 3 symbols, got %d", len(retrieved))
		}

		// Verify order (should be sorted by start_line)
		if len(retrieved) >= 3 {
			if retrieved[0].StartLine >= retrieved[1].StartLine || retrieved[1].StartLine >= retrieved[2].StartLine {
				t.Error("symbols not sorted by start_line")
			}
		}
	})

	t.Run("Symbol_DDDFlags", func(t *testing.T) {
		symbol := &storage.Symbol{
			FileID:          file.ID,
			Name:            "UserAggregate",
			Kind:            "struct",
			PackageName:     "domain",
			StartLine:       100,
			StartCol:        0,
			EndLine:         120,
			EndCol:          1,
			IsAggregateRoot: true,
			IsEntity:        true,
			IsValueObject:   false,
			IsRepository:    false,
			IsService:       false,
			IsCommand:       false,
			IsQuery:         false,
			IsHandler:       false,
		}

		err := store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("UpsertSymbol failed: %v", err)
		}

		retrieved, err := store.GetSymbol(ctx, symbol.ID)
		if err != nil {
			t.Fatalf("GetSymbol failed: %v", err)
		}

		if !retrieved.IsAggregateRoot {
			t.Error("expected IsAggregateRoot to be true")
		}

		if !retrieved.IsEntity {
			t.Error("expected IsEntity to be true")
		}

		if retrieved.IsValueObject {
			t.Error("expected IsValueObject to be false")
		}
	})
}

// TestChunkOperations tests comprehensive chunk operations
func TestChunkOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project and file
	project := &storage.Project{
		RootPath:     "/test/chunks",
		ModuleName:   "github.com/test/chunks",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("chunk test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "chunks.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	t.Run("UpsertChunk_Insert", func(t *testing.T) {
		content := "package test\n\nfunc Example() {}"
		contentHash := sha256.Sum256([]byte(content))
		chunk := &storage.Chunk{
			FileID:        file.ID,
			Content:       content,
			ContentHash:   contentHash,
			TokenCount:    10,
			StartLine:     1,
			EndLine:       3,
			ContextBefore: "// File header",
			ContextAfter:  "// Next section",
			ChunkType:     "function",
		}

		err := store.UpsertChunk(ctx, chunk)
		if err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		if chunk.ID == 0 {
			t.Error("expected non-zero ID after upsert")
		}

		if chunk.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after upsert")
		}

		if chunk.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt after upsert")
		}
	})

	t.Run("UpsertChunk_Update", func(t *testing.T) {
		content := "initial content"
		contentHash := sha256.Sum256([]byte(content))
		chunk := &storage.Chunk{
			FileID:      file.ID,
			Content:     content,
			ContentHash: contentHash,
			TokenCount:  5,
			StartLine:   10,
			EndLine:     12,
			ChunkType:   "comment",
		}

		// Initial insert
		err := store.UpsertChunk(ctx, chunk)
		if err != nil {
			t.Fatalf("UpsertChunk (insert) failed: %v", err)
		}
		firstID := chunk.ID

		// Update with same natural key (file_id, start_line, end_line)
		newContent := "updated content"
		newHash := sha256.Sum256([]byte(newContent))
		chunk.Content = newContent
		chunk.ContentHash = newHash
		chunk.TokenCount = 7

		err = store.UpsertChunk(ctx, chunk)
		if err != nil {
			t.Fatalf("UpsertChunk (update) failed: %v", err)
		}

		// Verify update happened on same record
		retrieved, err := store.GetChunk(ctx, firstID)
		if err != nil {
			t.Fatalf("GetChunk failed: %v", err)
		}

		if retrieved.ID != firstID {
			t.Errorf("expected same ID %d, got %d (should update not insert)", firstID, retrieved.ID)
		}

		if retrieved.Content != newContent {
			t.Errorf("expected updated content, got %s", retrieved.Content)
		}

		if retrieved.TokenCount != 7 {
			t.Errorf("expected TokenCount 7, got %d", retrieved.TokenCount)
		}
	})

	t.Run("GetChunk_Success", func(t *testing.T) {
		content := "get test content"
		contentHash := sha256.Sum256([]byte(content))
		chunk := &storage.Chunk{
			FileID:      file.ID,
			Content:     content,
			ContentHash: contentHash,
			TokenCount:  4,
			StartLine:   20,
			EndLine:     22,
			ChunkType:   "test",
		}
		if err := store.UpsertChunk(ctx, chunk); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		retrieved, err := store.GetChunk(ctx, chunk.ID)
		if err != nil {
			t.Fatalf("GetChunk failed: %v", err)
		}

		if retrieved.Content != content {
			t.Errorf("expected Content %s, got %s", content, retrieved.Content)
		}
	})

	t.Run("ListChunksByFile_Success", func(t *testing.T) {
		// Create new file for clean list test
		hash := sha256.Sum256([]byte("list chunks"))
		listFile := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "listchunks.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		if err := store.UpsertFile(ctx, listFile); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		// Insert multiple chunks
		for i := 0; i < 3; i++ {
			content := "chunk content"
			contentHash := sha256.Sum256([]byte(content))
			chunk := &storage.Chunk{
				FileID:      listFile.ID,
				Content:     content,
				ContentHash: contentHash,
				TokenCount:  5,
				StartLine:   i*10 + 1,
				EndLine:     i*10 + 3,
				ChunkType:   "test",
			}
			if err := store.UpsertChunk(ctx, chunk); err != nil {
				t.Fatalf("UpsertChunk failed for chunk %d: %v", i, err)
			}
		}

		// List chunks
		retrieved, err := store.ListChunksByFile(ctx, listFile.ID)
		if err != nil {
			t.Fatalf("ListChunksByFile failed: %v", err)
		}

		if len(retrieved) != 3 {
			t.Errorf("expected 3 chunks, got %d", len(retrieved))
		}

		// Verify order (should be sorted by start_line)
		if len(retrieved) >= 3 {
			if retrieved[0].StartLine >= retrieved[1].StartLine || retrieved[1].StartLine >= retrieved[2].StartLine {
				t.Error("chunks not sorted by start_line")
			}
		}
	})

	t.Run("Chunk_WithSymbol", func(t *testing.T) {
		// Create a symbol first
		symbol := &storage.Symbol{
			FileID:      file.ID,
			Name:        "ChunkedFunc",
			Kind:        "function",
			PackageName: "test",
			StartLine:   100,
			StartCol:    0,
			EndLine:     110,
			EndCol:      1,
		}
		if err := store.UpsertSymbol(ctx, symbol); err != nil {
			t.Fatalf("UpsertSymbol failed: %v", err)
		}

		// Create chunk with symbol reference
		content := "chunked function content"
		contentHash := sha256.Sum256([]byte(content))
		chunk := &storage.Chunk{
			FileID:      file.ID,
			SymbolID:    &symbol.ID,
			Content:     content,
			ContentHash: contentHash,
			TokenCount:  6,
			StartLine:   100,
			EndLine:     110,
			ChunkType:   "function",
		}

		err := store.UpsertChunk(ctx, chunk)
		if err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		// Retrieve and verify symbol reference
		retrieved, err := store.GetChunk(ctx, chunk.ID)
		if err != nil {
			t.Fatalf("GetChunk failed: %v", err)
		}

		if retrieved.SymbolID == nil {
			t.Fatal("expected SymbolID to be non-nil")
		}

		if *retrieved.SymbolID != symbol.ID {
			t.Errorf("expected SymbolID %d, got %d", symbol.ID, *retrieved.SymbolID)
		}
	})

	t.Run("DeleteChunksByFile_Success", func(t *testing.T) {
		// Create file with chunks
		hash := sha256.Sum256([]byte("delete chunks"))
		deleteFile := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "deletechunks.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		if err := store.UpsertFile(ctx, deleteFile); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		// Insert chunks
		for i := 0; i < 2; i++ {
			content := "chunk to delete"
			contentHash := sha256.Sum256([]byte(content))
			chunk := &storage.Chunk{
				FileID:      deleteFile.ID,
				Content:     content,
				ContentHash: contentHash,
				TokenCount:  4,
				StartLine:   i * 10,
				EndLine:     i*10 + 2,
				ChunkType:   "test",
			}
			if err := store.UpsertChunk(ctx, chunk); err != nil {
				t.Fatalf("UpsertChunk failed: %v", err)
			}
		}

		// Delete chunks
		err := store.DeleteChunksByFile(ctx, deleteFile.ID)
		if err != nil {
			t.Fatalf("DeleteChunksByFile failed: %v", err)
		}

		// Verify deletion
		chunks, err := store.ListChunksByFile(ctx, deleteFile.ID)
		if err != nil {
			t.Fatalf("ListChunksByFile failed: %v", err)
		}

		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks after deletion, got %d", len(chunks))
		}
	})
}

// TestEmbeddingOperations tests comprehensive embedding operations
func TestEmbeddingOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project, file, and chunk
	project := &storage.Project{
		RootPath:     "/test/embeddings",
		ModuleName:   "github.com/test/embeddings",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("embedding test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "embeddings.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	content := "test chunk content"
	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:      file.ID,
		Content:     content,
		ContentHash: contentHash,
		TokenCount:  5,
		StartLine:   1,
		EndLine:     3,
		ChunkType:   "test",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	t.Run("UpsertEmbedding_Insert", func(t *testing.T) {
		// Create mock vector (384 dimensions)
		vector := make([]float32, 384)
		for i := range vector {
			vector[i] = float32(i) * 0.01
		}

		// Serialize vector to bytes
		vectorBytes := make([]byte, len(vector)*4)
		for i, v := range vector {
			bits := uint32(v * 1000) // Simple serialization for test
			vectorBytes[i*4] = byte(bits >> 24)
			vectorBytes[i*4+1] = byte(bits >> 16)
			vectorBytes[i*4+2] = byte(bits >> 8)
			vectorBytes[i*4+3] = byte(bits)
		}

		embedding := &storage.Embedding{
			ChunkID:   chunk.ID,
			Vector:    vectorBytes,
			Dimension: 384,
			Provider:  "jina",
			Model:     "jina-embeddings-v3",
		}

		err := store.UpsertEmbedding(ctx, embedding)
		if err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}

		if embedding.ID == 0 {
			t.Error("expected non-zero ID after upsert")
		}

		if embedding.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after upsert")
		}
	})

	t.Run("UpsertEmbedding_Update", func(t *testing.T) {
		// Create another chunk for update test
		content2 := "update embedding test"
		contentHash2 := sha256.Sum256([]byte(content2))
		chunk2 := &storage.Chunk{
			FileID:      file.ID,
			Content:     content2,
			ContentHash: contentHash2,
			TokenCount:  5,
			StartLine:   10,
			EndLine:     12,
			ChunkType:   "test",
		}
		if err := store.UpsertChunk(ctx, chunk2); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		// Initial embedding
		vector1 := []byte{1, 2, 3, 4}
		embedding := &storage.Embedding{
			ChunkID:   chunk2.ID,
			Vector:    vector1,
			Dimension: 128,
			Provider:  "openai",
			Model:     "text-embedding-3-small",
		}

		err := store.UpsertEmbedding(ctx, embedding)
		if err != nil {
			t.Fatalf("UpsertEmbedding (insert) failed: %v", err)
		}

		// Update with same chunk_id
		vector2 := []byte{5, 6, 7, 8, 9, 10}
		embedding.Vector = vector2
		embedding.Dimension = 256
		embedding.Model = "text-embedding-3-large"

		err = store.UpsertEmbedding(ctx, embedding)
		if err != nil {
			t.Fatalf("UpsertEmbedding (update) failed: %v", err)
		}

		// Verify update
		retrieved, err := store.GetEmbedding(ctx, chunk2.ID)
		if err != nil {
			t.Fatalf("GetEmbedding failed: %v", err)
		}

		if len(retrieved.Vector) != len(vector2) {
			t.Errorf("expected vector length %d, got %d", len(vector2), len(retrieved.Vector))
		}

		if retrieved.Dimension != 256 {
			t.Errorf("expected Dimension 256, got %d", retrieved.Dimension)
		}

		if retrieved.Model != "text-embedding-3-large" {
			t.Errorf("expected Model text-embedding-3-large, got %s", retrieved.Model)
		}
	})

	t.Run("GetEmbedding_Success", func(t *testing.T) {
		// Create chunk for get test
		content3 := "get embedding test"
		contentHash3 := sha256.Sum256([]byte(content3))
		chunk3 := &storage.Chunk{
			FileID:      file.ID,
			Content:     content3,
			ContentHash: contentHash3,
			TokenCount:  5,
			StartLine:   20,
			EndLine:     22,
			ChunkType:   "test",
		}
		if err := store.UpsertChunk(ctx, chunk3); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		vector := []byte{11, 12, 13, 14}
		embedding := &storage.Embedding{
			ChunkID:   chunk3.ID,
			Vector:    vector,
			Dimension: 4,
			Provider:  "test",
			Model:     "test-model",
		}
		if err := store.UpsertEmbedding(ctx, embedding); err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}

		retrieved, err := store.GetEmbedding(ctx, chunk3.ID)
		if err != nil {
			t.Fatalf("GetEmbedding failed: %v", err)
		}

		if retrieved.ChunkID != chunk3.ID {
			t.Errorf("expected ChunkID %d, got %d", chunk3.ID, retrieved.ChunkID)
		}

		if retrieved.Provider != "test" {
			t.Errorf("expected Provider test, got %s", retrieved.Provider)
		}
	})

	t.Run("GetEmbedding_NotFound", func(t *testing.T) {
		_, err := store.GetEmbedding(ctx, 999999)
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("DeleteEmbedding_Success", func(t *testing.T) {
		// Create chunk for delete test
		content4 := "delete embedding test"
		contentHash4 := sha256.Sum256([]byte(content4))
		chunk4 := &storage.Chunk{
			FileID:      file.ID,
			Content:     content4,
			ContentHash: contentHash4,
			TokenCount:  5,
			StartLine:   30,
			EndLine:     32,
			ChunkType:   "test",
		}
		if err := store.UpsertChunk(ctx, chunk4); err != nil {
			t.Fatalf("UpsertChunk failed: %v", err)
		}

		vector := []byte{20, 21, 22}
		embedding := &storage.Embedding{
			ChunkID:   chunk4.ID,
			Vector:    vector,
			Dimension: 3,
			Provider:  "test",
			Model:     "test-model",
		}
		if err := store.UpsertEmbedding(ctx, embedding); err != nil {
			t.Fatalf("UpsertEmbedding failed: %v", err)
		}

		// Delete embedding
		err := store.DeleteEmbedding(ctx, chunk4.ID)
		if err != nil {
			t.Fatalf("DeleteEmbedding failed: %v", err)
		}

		// Verify deletion
		_, err = store.GetEmbedding(ctx, chunk4.ID)
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound after deletion, got %v", err)
		}
	})
}

// TestImportOperations tests comprehensive import operations
func TestImportOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project and file
	project := &storage.Project{
		RootPath:     "/test/imports",
		ModuleName:   "github.com/test/imports",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("import test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "imports.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	t.Run("UpsertImport_Success", func(t *testing.T) {
		imp := &storage.Import{
			FileID:     file.ID,
			ImportPath: "fmt",
			Alias:      "",
		}

		err := store.UpsertImport(ctx, imp)
		if err != nil {
			t.Fatalf("UpsertImport failed: %v", err)
		}

		if imp.ID == 0 {
			t.Error("expected non-zero ID after upsert")
		}

		if imp.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt after upsert")
		}
	})

	t.Run("UpsertImport_WithAlias", func(t *testing.T) {
		imp := &storage.Import{
			FileID:     file.ID,
			ImportPath: "github.com/example/pkg",
			Alias:      "epkg",
		}

		err := store.UpsertImport(ctx, imp)
		if err != nil {
			t.Fatalf("UpsertImport failed: %v", err)
		}

		if imp.Alias != "epkg" {
			t.Errorf("expected Alias epkg, got %s", imp.Alias)
		}
	})

	t.Run("ListImportsByFile_Success", func(t *testing.T) {
		// Create new file for clean list test
		hash := sha256.Sum256([]byte("list imports"))
		listFile := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "listimports.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		if err := store.UpsertFile(ctx, listFile); err != nil {
			t.Fatalf("UpsertFile failed: %v", err)
		}

		// Insert multiple imports
		imports := []string{"context", "fmt", "strings"}
		for _, impPath := range imports {
			imp := &storage.Import{
				FileID:     listFile.ID,
				ImportPath: impPath,
				Alias:      "",
			}
			if err := store.UpsertImport(ctx, imp); err != nil {
				t.Fatalf("UpsertImport failed for %s: %v", impPath, err)
			}
		}

		// List imports
		retrieved, err := store.ListImportsByFile(ctx, listFile.ID)
		if err != nil {
			t.Fatalf("ListImportsByFile failed: %v", err)
		}

		if len(retrieved) != 3 {
			t.Errorf("expected 3 imports, got %d", len(retrieved))
		}

		// Verify order (should be sorted by import_path)
		if len(retrieved) >= 3 {
			if retrieved[0].ImportPath != "context" || retrieved[1].ImportPath != "fmt" || retrieved[2].ImportPath != "strings" {
				t.Error("imports not sorted by import_path")
			}
		}
	})
}

// TestTransactions tests transaction commit and rollback behavior
func TestTransactions(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	t.Run("Transaction_Commit", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project in transaction
		project := &storage.Project{
			RootPath:     "/test/tx-commit",
			ModuleName:   "github.com/test/tx",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Commit transaction
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify project persisted
		retrieved, err := store.GetProject(ctx, "/test/tx-commit")
		if err != nil {
			t.Fatalf("GetProject after commit failed: %v", err)
		}

		if retrieved.RootPath != "/test/tx-commit" {
			t.Errorf("expected RootPath /test/tx-commit, got %s", retrieved.RootPath)
		}
	})

	t.Run("Transaction_Rollback", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project in transaction
		project := &storage.Project{
			RootPath:     "/test/tx-rollback",
			ModuleName:   "github.com/test/rollback",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Rollback transaction
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		// Verify project does NOT exist
		_, err = store.GetProject(ctx, "/test/tx-rollback")
		if err != storage.ErrNotFound {
			t.Errorf("expected ErrNotFound after rollback, got %v", err)
		}
	})

	t.Run("Transaction_MultipleOperations", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project
		project := &storage.Project{
			RootPath:     "/test/tx-multi",
			ModuleName:   "github.com/test/multi",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Create file
		hash := sha256.Sum256([]byte("tx multi test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "multi.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   50,
		}
		err = tx.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile in tx failed: %v", err)
		}

		// Commit
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Verify both persisted
		retrievedProject, err := store.GetProject(ctx, "/test/tx-multi")
		if err != nil {
			t.Fatalf("GetProject after commit failed: %v", err)
		}

		retrievedFile, err := store.GetFile(ctx, retrievedProject.ID, "multi.go")
		if err != nil {
			t.Fatalf("GetFile after commit failed: %v", err)
		}

		if retrievedFile.FilePath != "multi.go" {
			t.Errorf("expected FilePath multi.go, got %s", retrievedFile.FilePath)
		}
	})
}

// TestCascadeDeletes tests that deleting a file cascades to related entities
func TestCascadeDeletes(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project, file, symbol, and chunk
	project := &storage.Project{
		RootPath:     "/test/cascade",
		ModuleName:   "github.com/test/cascade",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("cascade test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "cascade.go",
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
		Name:        "CascadeFunc",
		Kind:        "function",
		PackageName: "test",
		StartLine:   1,
		StartCol:    0,
		EndLine:     10,
		EndCol:      1,
	}
	if err := store.UpsertSymbol(ctx, symbol); err != nil {
		t.Fatalf("UpsertSymbol failed: %v", err)
	}

	// Create chunk
	content := "cascade chunk"
	contentHash := sha256.Sum256([]byte(content))
	chunk := &storage.Chunk{
		FileID:      file.ID,
		Content:     content,
		ContentHash: contentHash,
		TokenCount:  3,
		StartLine:   1,
		EndLine:     10,
		ChunkType:   "function",
	}
	if err := store.UpsertChunk(ctx, chunk); err != nil {
		t.Fatalf("UpsertChunk failed: %v", err)
	}

	// Delete file
	err := store.DeleteFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// Verify symbols are deleted (cascade)
	symbols, err := store.ListSymbolsByFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("ListSymbolsByFile failed: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols after file deletion, got %d", len(symbols))
	}

	// Verify chunks are deleted (cascade)
	chunks, err := store.ListChunksByFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("ListChunksByFile failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks after file deletion, got %d", len(chunks))
	}
}

// TestConcurrentReads tests concurrent read operations
func TestConcurrentReads(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project and file
	project := &storage.Project{
		RootPath:     "/test/concurrent",
		ModuleName:   "github.com/test/concurrent",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	hash := sha256.Sum256([]byte("concurrent test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "concurrent.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	// Spawn 10 goroutines reading simultaneously
	const numReaders = 10
	done := make(chan error, numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			_, err := store.GetProject(ctx, "/test/concurrent")
			done <- err
		}()
	}

	// Wait for all reads to complete
	for i := 0; i < numReaders; i++ {
		err := <-done
		if err != nil {
			t.Errorf("concurrent read %d failed: %v", i, err)
		}
	}
}

// TestTransactionIsolation tests that reads within a transaction see uncommitted writes.
// Regression test for T056 [US3]: Verifies transaction isolation - reads see uncommitted writes within tx,
// but not outside tx until commit.
// Bug fixed: Proper querier interface usage ensures transaction isolation works correctly.
func TestTransactionIsolation(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	t.Run("Read within transaction sees uncommitted write", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}
		defer tx.Rollback()

		// Create project within transaction
		project := &storage.Project{
			RootPath:     "/test/tx-isolation-1",
			ModuleName:   "github.com/test/isolation",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Read within same transaction - should see the uncommitted write
		retrieved, err := tx.GetProject(ctx, "/test/tx-isolation-1")
		if err != nil {
			t.Fatalf("GetProject within tx failed: %v", err)
		}

		if retrieved.RootPath != project.RootPath {
			t.Errorf("Expected to see uncommitted write within tx: got RootPath %s, want %s",
				retrieved.RootPath, project.RootPath)
		}
	})

	t.Run("Read after rollback does not see write", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project within transaction
		project := &storage.Project{
			RootPath:     "/test/tx-isolation-2",
			ModuleName:   "github.com/test/isolation2",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Rollback the transaction
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		// Read from outside transaction - should NOT see rolled-back write
		_, err = store.GetProject(ctx, "/test/tx-isolation-2")
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound when reading rolled-back data, got: %v", err)
		}
	})

	t.Run("Read after commit sees write", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project within transaction
		project := &storage.Project{
			RootPath:     "/test/tx-isolation-3",
			ModuleName:   "github.com/test/isolation3",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Commit the transaction
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Read from outside transaction - should NOW see the committed write
		retrieved, err := store.GetProject(ctx, "/test/tx-isolation-3")
		if err != nil {
			t.Fatalf("GetProject after commit failed: %v", err)
		}

		if retrieved.RootPath != project.RootPath {
			t.Errorf("Expected to see committed write: got RootPath %s, want %s",
				retrieved.RootPath, project.RootPath)
		}
	})

	t.Run("Verify querier interface works correctly", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}
		defer tx.Rollback()

		// Setup project and file for querier test
		project := &storage.Project{
			RootPath:     "/test/querier",
			ModuleName:   "github.com/test/querier",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		hash := sha256.Sum256([]byte("querier test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "querier.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   42,
		}
		err = tx.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile in tx failed: %v", err)
		}

		// Read file within transaction using querier
		retrieved, err := tx.GetFile(ctx, project.ID, "querier.go")
		if err != nil {
			t.Fatalf("GetFile within tx failed: %v", err)
		}

		if retrieved.SizeBytes != 42 {
			t.Errorf("Expected SizeBytes 42, got %d", retrieved.SizeBytes)
		}
	})
}

// TestConcurrentUpsertOperations tests that concurrent upserts don't cause race conditions.
// Regression test for T057 [US3]: Verifies atomic UPSERT clause prevents race conditions during concurrent writes.
// Bug fixed: Using INSERT ... ON CONFLICT DO UPDATE ensures atomic upsert operations.
func TestConcurrentUpsertOperations(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	// Setup: Create project
	project := &storage.Project{
		RootPath:     "/test/concurrent-upsert",
		ModuleName:   "github.com/test/upsert",
		GoVersion:    "1.23",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Setup: Create file
	hash := sha256.Sum256([]byte("concurrent upsert test"))
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "upsert.go",
		PackageName: "test",
		ContentHash: hash,
		ModTime:     time.Now(),
		SizeBytes:   100,
	}
	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("UpsertFile failed: %v", err)
	}

	t.Run("Concurrent symbol upserts with same key", func(t *testing.T) {
		const numGoroutines = 20
		done := make(chan error, numGoroutines)

		// All goroutines try to upsert the same symbol (same natural key)
		for i := 0; i < numGoroutines; i++ {
			iteration := i
			go func() {
				symbol := &storage.Symbol{
					FileID:      file.ID,
					Name:        "ConcurrentFunc",
					Kind:        "function",
					PackageName: "test",
					Signature:   "func ConcurrentFunc() v" + string(rune('0'+iteration%10)),
					StartLine:   100,
					StartCol:    0,
					EndLine:     110,
					EndCol:      1,
				}
				err := store.UpsertSymbol(ctx, symbol)
				done <- err
			}()
		}

		// Wait for all upserts
		for i := 0; i < numGoroutines; i++ {
			err := <-done
			if err != nil {
				t.Errorf("concurrent upsert %d failed: %v", i, err)
			}
		}

		// Verify only one symbol exists (UNIQUE constraint prevented duplicates)
		symbols, err := store.ListSymbolsByFile(ctx, file.ID)
		if err != nil {
			t.Fatalf("ListSymbolsByFile failed: %v", err)
		}

		// Should have exactly 1 symbol due to UNIQUE constraint on (file_id, name, start_line, start_col)
		if len(symbols) != 1 {
			t.Errorf("Expected 1 symbol after concurrent upserts, got %d", len(symbols))
		}

		if len(symbols) > 0 && symbols[0].Name != "ConcurrentFunc" {
			t.Errorf("Expected Name ConcurrentFunc, got %s", symbols[0].Name)
		}
	})

	t.Run("Concurrent chunk upserts with atomic INSERT ON CONFLICT", func(t *testing.T) {
		const numGoroutines = 15
		done := make(chan error, numGoroutines)

		// All goroutines try to upsert the same chunk (same natural key: file_id, start_line, end_line)
		for i := 0; i < numGoroutines; i++ {
			iteration := i
			go func() {
				content := "concurrent chunk content v" + string(rune('0'+iteration%10))
				contentHash := sha256.Sum256([]byte(content))
				chunk := &storage.Chunk{
					FileID:      file.ID,
					Content:     content,
					ContentHash: contentHash,
					TokenCount:  10 + iteration,
					StartLine:   50,
					EndLine:     60,
					ChunkType:   "function",
				}
				err := store.UpsertChunk(ctx, chunk)
				done <- err
			}()
		}

		// Wait for all upserts
		for i := 0; i < numGoroutines; i++ {
			err := <-done
			if err != nil {
				t.Errorf("concurrent chunk upsert %d failed: %v", i, err)
			}
		}

		// Verify only one chunk exists (UNIQUE constraint on file_id, start_line, end_line)
		chunks, err := store.ListChunksByFile(ctx, file.ID)
		if err != nil {
			t.Fatalf("ListChunksByFile failed: %v", err)
		}

		// Count chunks at lines 50-60
		var targetChunks int
		for _, chunk := range chunks {
			if chunk.StartLine == 50 && chunk.EndLine == 60 {
				targetChunks++
			}
		}

		if targetChunks != 1 {
			t.Errorf("Expected 1 chunk at lines 50-60 after concurrent upserts, got %d", targetChunks)
		}
	})

	t.Run("UNIQUE constraints prevent duplicates", func(t *testing.T) {
		// Create a symbol
		symbol := &storage.Symbol{
			FileID:      file.ID,
			Name:        "UniqueTest",
			Kind:        "function",
			PackageName: "test",
			StartLine:   200,
			StartCol:    0,
			EndLine:     210,
			EndCol:      1,
		}
		err := store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("Initial UpsertSymbol failed: %v", err)
		}

		// Upsert again with same key - should update, not create duplicate
		symbol.Signature = "updated signature"
		err = store.UpsertSymbol(ctx, symbol)
		if err != nil {
			t.Fatalf("Second UpsertSymbol failed: %v", err)
		}

		// Verify only one symbol exists
		symbols, err := store.ListSymbolsByFile(ctx, file.ID)
		if err != nil {
			t.Fatalf("ListSymbolsByFile failed: %v", err)
		}

		// Count symbols named "UniqueTest"
		var uniqueTestCount int
		for _, s := range symbols {
			if s.Name == "UniqueTest" {
				uniqueTestCount++
			}
		}

		if uniqueTestCount != 1 {
			t.Errorf("Expected 1 UniqueTest symbol, got %d (UNIQUE constraint failed)", uniqueTestCount)
		}
	})
}

// TestNestedTransactionBehavior tests nested transaction handling.
// Regression test for T058 [US3]: Verifies nested transaction behavior is properly handled/documented.
// Bug fixed: BeginTx within transaction returns clear error (SQLite doesn't support true nested transactions).
func TestNestedTransactionBehavior(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	t.Run("BeginTx within transaction returns error", func(t *testing.T) {
		// Start outer transaction
		outerTx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx (outer) failed: %v", err)
		}
		defer outerTx.Rollback()

		// Try to start nested transaction - should fail
		nestedTx, err := outerTx.BeginTx(ctx)
		if err == nil {
			defer nestedTx.Rollback()
			t.Error("Expected error when calling BeginTx within transaction, got nil")
		}

		// Error should indicate nested transactions not supported
		if err != nil && !contains(err.Error(), "nested transactions not supported") {
			t.Errorf("Expected 'nested transactions not supported' error, got: %v", err)
		}
	})

	t.Run("Rollback outer transaction discards all changes", func(t *testing.T) {
		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}

		// Create project
		project := &storage.Project{
			RootPath:     "/test/nested-rollback",
			ModuleName:   "github.com/test/nested",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject in tx failed: %v", err)
		}

		// Create file
		hash := sha256.Sum256([]byte("nested test"))
		file := &storage.File{
			ProjectID:   project.ID,
			FilePath:    "nested.go",
			PackageName: "test",
			ContentHash: hash,
			ModTime:     time.Now(),
			SizeBytes:   100,
		}
		err = tx.UpsertFile(ctx, file)
		if err != nil {
			t.Fatalf("UpsertFile in tx failed: %v", err)
		}

		// Rollback entire transaction
		err = tx.Rollback()
		if err != nil {
			t.Fatalf("Rollback failed: %v", err)
		}

		// Verify both project and file are not persisted
		_, err = store.GetProject(ctx, "/test/nested-rollback")
		if err != storage.ErrNotFound {
			t.Errorf("Expected ErrNotFound for project after rollback, got: %v", err)
		}
	})

	t.Run("Document expected behavior for future savepoint support", func(t *testing.T) {
		// This test documents the expected behavior if savepoints are added in the future
		// Currently, nested transactions are not supported, but the architecture supports adding them

		tx, err := store.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx failed: %v", err)
		}
		defer tx.Rollback()

		// Create project in outer transaction
		project := &storage.Project{
			RootPath:     "/test/savepoint-future",
			ModuleName:   "github.com/test/savepoint",
			GoVersion:    "1.23",
			IndexVersion: "1.0.0",
		}
		err = tx.CreateProject(ctx, project)
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		// If savepoints were supported, we would:
		// 1. Call nestedTx, err := tx.BeginTx(ctx) - would create SAVEPOINT
		// 2. Do some work in nested transaction
		// 3. nestedTx.Rollback() would ROLLBACK TO SAVEPOINT
		// 4. Outer transaction would still have the project

		// For now, verify nested transactions are properly rejected
		_, err = tx.BeginTx(ctx)
		if err == nil {
			t.Error("Expected nested transaction to fail (savepoints not yet implemented)")
		}
	})
}
