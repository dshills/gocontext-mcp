package storage_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dshills/gocontext-mcp/internal/storage"
)

func TestApplyMigrations(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open(storage.DriverName, ":memory:")
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
	db, err := sql.Open(storage.DriverName, ":memory:")
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

// TestSemanticVersionComparison tests that semantic version comparison works correctly.
// Regression test for T054 [US3]: Prevents incorrect version ordering (1.10.0 > 1.2.0, not lexicographic).
// Bug fixed: Using semver library for proper semantic version comparison instead of string comparison.
func TestSemanticVersionComparison(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		v1Higher bool // true if v1 > v2
	}{
		{
			name:     "Major version difference",
			v1:       "2.0.0",
			v2:       "1.9.9",
			v1Higher: true,
		},
		{
			name:     "Minor version difference - semantic ordering",
			v1:       "1.10.0",
			v2:       "1.2.0",
			v1Higher: true, // Critical: 1.10.0 > 1.2.0 (not lexicographic "1.10.0" < "1.2.0")
		},
		{
			name:     "Patch version difference",
			v1:       "1.0.10",
			v2:       "1.0.2",
			v1Higher: true,
		},
		{
			name:     "Equal versions",
			v1:       "1.0.0",
			v2:       "1.0.0",
			v1Higher: false,
		},
		{
			name:     "Pre-release version lower than release",
			v1:       "1.0.0-alpha",
			v2:       "1.0.0",
			v1Higher: false,
		},
		{
			name:     "Pre-release version ordering",
			v1:       "1.0.0-beta",
			v2:       "1.0.0-alpha",
			v1Higher: true,
		},
		{
			name:     "Build metadata ignored in comparison",
			v1:       "1.0.0+build.1",
			v2:       "1.0.0+build.2",
			v1Higher: false, // Build metadata should be ignored
		},
		{
			name:     "Complex semantic version",
			v1:       "1.12.3",
			v2:       "1.9.15",
			v1Higher: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory database for testing
			db, err := sql.Open(storage.DriverName, ":memory:")
			if err != nil {
				t.Fatalf("Failed to open database: %v", err)
			}
			defer db.Close()

			ctx := context.Background()

			// Create schema_version table
			_, err = db.ExecContext(ctx, `CREATE TABLE schema_version (
				version TEXT PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)`)
			if err != nil {
				t.Fatalf("Failed to create schema_version table: %v", err)
			}

			// Insert first version
			_, err = db.ExecContext(ctx, "INSERT INTO schema_version (version) VALUES (?)", tt.v2)
			if err != nil {
				t.Fatalf("Failed to insert version: %v", err)
			}

			// Create a test migration with v1
			testMigration := storage.Migration{
				Version: tt.v1,
				Up:      "SELECT 1", // Dummy migration
				Down:    "SELECT 1",
			}

			// Save original migrations and replace temporarily
			originalMigrations := storage.AllMigrations
			storage.AllMigrations = []storage.Migration{testMigration}
			defer func() { storage.AllMigrations = originalMigrations }()

			// Apply migrations - should only run if v1 > v2
			err = storage.ApplyMigrations(ctx, db)
			if err != nil {
				t.Fatalf("ApplyMigrations failed: %v", err)
			}

			// Check how many version records exist
			var count int
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_version").Scan(&count)
			if err != nil {
				t.Fatalf("Failed to count versions: %v", err)
			}

			if tt.v1Higher {
				// If v1 > v2, migration should have run (2 records: v2 and v1)
				if count != 2 {
					t.Errorf("Expected 2 version records (v1 > v2), got %d", count)
				}
			} else {
				// If v1 <= v2, migration should NOT have run (1 record: v2 only)
				if count != 1 {
					t.Errorf("Expected 1 version record (v1 <= v2), got %d", count)
				}
			}
		})
	}
}

// TestMigrationErrorHandling tests that migration error handling distinguishes DB errors from "no migrations".
// Regression test for T055 [US3]: Ensures sql.ErrNoRows returns "0.0.0" as current version, other errors are wrapped.
// Bug fixed: Proper error handling in ApplyMigrations to distinguish between no rows and database errors.
func TestMigrationErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupDB       func(t *testing.T) *sql.DB
		expectError   bool
		expectVersion string
		errorContains string
	}{
		{
			name: "No schema_version table - starts from 0.0.0",
			setupDB: func(t *testing.T) *sql.DB {
				db, err := sql.Open(storage.DriverName, ":memory:")
				if err != nil {
					t.Fatalf("Failed to open database: %v", err)
				}
				// Don't create schema_version table
				return db
			},
			expectError:   false,
			expectVersion: "1.0.1", // Should apply all migrations starting from 0.0.0
		},
		{
			name: "Empty schema_version table - starts from 0.0.0",
			setupDB: func(t *testing.T) *sql.DB {
				db, err := sql.Open(storage.DriverName, ":memory:")
				if err != nil {
					t.Fatalf("Failed to open database: %v", err)
				}
				// Create empty schema_version table
				_, err = db.Exec(`CREATE TABLE schema_version (
					version TEXT PRIMARY KEY,
					applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				)`)
				if err != nil {
					t.Fatalf("Failed to create table: %v", err)
				}
				return db
			},
			expectError:   false,
			expectVersion: "1.0.1", // Should apply all migrations starting from 0.0.0
		},
		{
			name: "Invalid version in database",
			setupDB: func(t *testing.T) *sql.DB {
				db, err := sql.Open(storage.DriverName, ":memory:")
				if err != nil {
					t.Fatalf("Failed to open database: %v", err)
				}
				// Create schema_version table with invalid version
				_, err = db.Exec(`CREATE TABLE schema_version (
					version TEXT PRIMARY KEY,
					applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				)`)
				if err != nil {
					t.Fatalf("Failed to create table: %v", err)
				}
				_, err = db.Exec("INSERT INTO schema_version (version) VALUES (?)", "invalid-version")
				if err != nil {
					t.Fatalf("Failed to insert version: %v", err)
				}
				return db
			},
			expectError:   true,
			errorContains: "invalid current schema version",
		},
		{
			name: "Already at current version - no error",
			setupDB: func(t *testing.T) *sql.DB {
				db, err := sql.Open(storage.DriverName, ":memory:")
				if err != nil {
					t.Fatalf("Failed to open database: %v", err)
				}
				// Apply migrations normally
				ctx := context.Background()
				if err := storage.ApplyMigrations(ctx, db); err != nil {
					t.Fatalf("Initial migration failed: %v", err)
				}
				return db
			},
			expectError:   false,
			expectVersion: storage.CurrentSchemaVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.setupDB(t)
			defer db.Close()

			ctx := context.Background()
			err := storage.ApplyMigrations(ctx, db)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				// Verify final version
				var version string
				err = db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&version)
				if err == sql.ErrNoRows {
					// No migrations applied yet is OK for some tests
					if tt.expectVersion != "" {
						t.Errorf("Expected version %s but no version found", tt.expectVersion)
					}
				} else if err != nil {
					t.Fatalf("Failed to query version: %v", err)
				} else if version != tt.expectVersion {
					t.Errorf("Expected version %s, got %s", tt.expectVersion, version)
				}
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
