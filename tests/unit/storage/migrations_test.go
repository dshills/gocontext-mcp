package storage_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/storage"
)

func TestApplyMigrations(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Apply migrations
	ctx := context.Background()
	if err := storage.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Verify schema_version table exists
	var version string
	err = db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to query schema version: %v", err)
	}

	if version != storage.CurrentSchemaVersion {
		t.Errorf("Expected schema version %s, got %s", storage.CurrentSchemaVersion, version)
	}

	// Verify all tables exist
	tables := []string{
		"projects", "files", "symbols", "chunks", "embeddings",
		"imports", "search_queries", "symbols_fts", "chunks_fts",
	}

	for _, table := range tables {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		err := db.QueryRowContext(ctx, query, table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Table %s does not exist", table)
		} else if err != nil {
			t.Errorf("Failed to check table %s: %v", table, err)
		}
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Apply migrations twice
	if err := storage.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("First migration failed: %v", err)
	}

	if err := storage.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("Second migration failed: %v", err)
	}

	// Should only have one version record
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count versions: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 schema version record, got %d", count)
	}
}

func TestProjectCRUD(t *testing.T) {
	// Create storage
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create project
	project := &storage.Project{
		RootPath:     "/test/project",
		ModuleName:   "github.com/test/project",
		GoVersion:    "1.21",
		IndexVersion: "1.0.0",
	}

	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	if project.ID == 0 {
		t.Error("Project ID should be set after creation")
	}

	// Get project
	retrieved, err := store.GetProject(ctx, "/test/project")
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}

	if retrieved.RootPath != project.RootPath {
		t.Errorf("Expected root path %s, got %s", project.RootPath, retrieved.RootPath)
	}

	if retrieved.ModuleName != project.ModuleName {
		t.Errorf("Expected module name %s, got %s", project.ModuleName, retrieved.ModuleName)
	}

	// Update project
	retrieved.TotalFiles = 100
	retrieved.TotalChunks = 500
	if err := store.UpdateProject(ctx, retrieved); err != nil {
		t.Fatalf("Failed to update project: %v", err)
	}

	// Verify update
	updated, err := store.GetProject(ctx, "/test/project")
	if err != nil {
		t.Fatalf("Failed to get updated project: %v", err)
	}

	if updated.TotalFiles != 100 {
		t.Errorf("Expected total files 100, got %d", updated.TotalFiles)
	}

	if updated.TotalChunks != 500 {
		t.Errorf("Expected total chunks 500, got %d", updated.TotalChunks)
	}
}

func TestFileUpsert(t *testing.T) {
	store, err := storage.NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create project first
	project := &storage.Project{
		RootPath:     "/test/project",
		IndexVersion: "1.0.0",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create file
	file := &storage.File{
		ProjectID:   project.ID,
		FilePath:    "main.go",
		PackageName: "main",
		ContentHash: [32]byte{1, 2, 3}, // Dummy hash
		SizeBytes:   1024,
	}

	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("Failed to upsert file: %v", err)
	}

	// Verify file was created
	retrieved, err := store.GetFile(ctx, project.ID, "main.go")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	if retrieved.FilePath != file.FilePath {
		t.Errorf("Expected file path %s, got %s", file.FilePath, retrieved.FilePath)
	}

	// Update file (upsert with same path)
	file.SizeBytes = 2048
	file.ContentHash = [32]byte{4, 5, 6}

	if err := store.UpsertFile(ctx, file); err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Verify update
	updated, err := store.GetFile(ctx, project.ID, "main.go")
	if err != nil {
		t.Fatalf("Failed to get updated file: %v", err)
	}

	if updated.SizeBytes != 2048 {
		t.Errorf("Expected size 2048, got %d", updated.SizeBytes)
	}
}
