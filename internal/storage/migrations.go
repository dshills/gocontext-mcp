package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Masterminds/semver/v3"
)

const (
	// CurrentSchemaVersion tracks the database schema version
	CurrentSchemaVersion = "1.0.0"
)

// Migration represents a database schema migration
type Migration struct {
	Version string
	Up      string
	Down    string
}

// AllMigrations contains all database migrations in order
var AllMigrations = []Migration{
	{
		Version: "1.0.0",
		Up:      migrationV1Up,
		Down:    migrationV1Down,
	},
}

const migrationV1Up = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    root_path TEXT NOT NULL UNIQUE,
    module_name TEXT,
    go_version TEXT,
    total_files INTEGER DEFAULT 0,
    total_chunks INTEGER DEFAULT 0,
    index_version TEXT NOT NULL,
    last_indexed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_projects_root_path ON projects(root_path);

-- Files table
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    package_name TEXT,
    content_hash BLOB NOT NULL,
    mod_time TIMESTAMP,
    size_bytes INTEGER,
    parse_error TEXT,
    last_indexed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    UNIQUE(project_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_files_project ON files(project_id);
CREATE INDEX IF NOT EXISTS idx_files_hash ON files(content_hash);
CREATE INDEX IF NOT EXISTS idx_files_package ON files(package_name);
CREATE INDEX IF NOT EXISTS idx_files_mod_time ON files(mod_time);

-- Symbols table
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    package_name TEXT NOT NULL,
    signature TEXT,
    doc_comment TEXT,
    scope TEXT,
    receiver TEXT,
    start_line INTEGER,
    start_col INTEGER,
    end_line INTEGER,
    end_col INTEGER,
    is_aggregate_root BOOLEAN DEFAULT 0,
    is_entity BOOLEAN DEFAULT 0,
    is_value_object BOOLEAN DEFAULT 0,
    is_repository BOOLEAN DEFAULT 0,
    is_service BOOLEAN DEFAULT 0,
    is_command BOOLEAN DEFAULT 0,
    is_query BOOLEAN DEFAULT 0,
    is_handler BOOLEAN DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_id);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
CREATE INDEX IF NOT EXISTS idx_symbols_package ON symbols(package_name);
CREATE INDEX IF NOT EXISTS idx_symbols_ddd ON symbols(is_aggregate_root, is_entity, is_value_object);
CREATE INDEX IF NOT EXISTS idx_symbols_cqrs ON symbols(is_command, is_query, is_handler);
CREATE UNIQUE INDEX IF NOT EXISTS idx_symbols_unique ON symbols(file_id, name, start_line, start_col);

-- Full-text search on symbols
CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
    name, signature, doc_comment,
    content='symbols',
    content_rowid='id'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS symbols_ai AFTER INSERT ON symbols BEGIN
    INSERT INTO symbols_fts(rowid, name, signature, doc_comment)
    VALUES (new.id, new.name, new.signature, new.doc_comment);
END;

CREATE TRIGGER IF NOT EXISTS symbols_ad AFTER DELETE ON symbols BEGIN
    DELETE FROM symbols_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS symbols_au AFTER UPDATE ON symbols BEGIN
    UPDATE symbols_fts SET
        name = new.name,
        signature = new.signature,
        doc_comment = new.doc_comment
    WHERE rowid = new.id;
END;

-- Chunks table
CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    symbol_id INTEGER,
    content TEXT NOT NULL,
    content_hash BLOB NOT NULL,
    token_count INTEGER,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    context_before TEXT,
    context_after TEXT,
    chunk_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_id);
CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks(symbol_id);
CREATE INDEX IF NOT EXISTS idx_chunks_hash ON chunks(content_hash);
CREATE INDEX IF NOT EXISTS idx_chunks_type ON chunks(chunk_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_unique ON chunks(file_id, start_line, end_line);

-- Full-text search on chunks
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content, context_before, context_after,
    content='chunks',
    content_rowid='id'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, content, context_before, context_after)
    VALUES (new.id, new.content, new.context_before, new.context_after);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
    DELETE FROM chunks_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
    UPDATE chunks_fts SET
        content = new.content,
        context_before = new.context_before,
        context_after = new.context_after
    WHERE rowid = new.id;
END;

-- Embeddings table
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL UNIQUE,
    vector BLOB NOT NULL,
    dimension INTEGER NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_embeddings_chunk ON embeddings(chunk_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_provider ON embeddings(provider, model);

-- Imports table
CREATE TABLE IF NOT EXISTS imports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    import_path TEXT NOT NULL,
    alias TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_imports_file ON imports(file_id);
CREATE INDEX IF NOT EXISTS idx_imports_path ON imports(import_path);

-- Search query cache
CREATE TABLE IF NOT EXISTS search_queries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    query_text TEXT NOT NULL,
    query_hash BLOB NOT NULL UNIQUE,
    result_chunk_ids TEXT NOT NULL,
    result_count INTEGER NOT NULL,
    search_duration_ms INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    hit_count INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_search_hash ON search_queries(query_hash);
CREATE INDEX IF NOT EXISTS idx_search_expires ON search_queries(expires_at);
`

const migrationV1Down = `
-- Drop all tables in reverse order of dependencies
DROP TRIGGER IF EXISTS chunks_au;
DROP TRIGGER IF EXISTS chunks_ad;
DROP TRIGGER IF EXISTS chunks_ai;
DROP TRIGGER IF EXISTS symbols_au;
DROP TRIGGER IF EXISTS symbols_ad;
DROP TRIGGER IF EXISTS symbols_ai;

DROP TABLE IF EXISTS search_queries;
DROP TABLE IF EXISTS imports;
DROP TABLE IF EXISTS embeddings;
DROP TABLE IF EXISTS chunks_fts;
DROP TABLE IF EXISTS chunks;
DROP TABLE IF EXISTS symbols_fts;
DROP TABLE IF EXISTS symbols;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS schema_version;
`

// ApplyMigrations runs all pending migrations
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	// Check if schema_version table exists
	var tableName string
	err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&tableName)

	// Parse current version (default to 0.0.0 if no migrations applied or table doesn't exist)
	var currentVersion *semver.Version
	if err == sql.ErrNoRows {
		// schema_version table doesn't exist, start from 0.0.0
		currentVersion = semver.MustParse("0.0.0")
	} else if err != nil {
		return fmt.Errorf("failed to check schema_version table: %w", err)
	} else {
		// Table exists, check current version
		var currentVersionStr string
		err = db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&currentVersionStr)
		if err == sql.ErrNoRows || currentVersionStr == "" {
			currentVersion = semver.MustParse("0.0.0")
		} else if err != nil {
			return fmt.Errorf("failed to read schema_version: %w", err)
		} else {
			currentVersion, err = semver.NewVersion(currentVersionStr)
			if err != nil {
				return fmt.Errorf("invalid current schema version %s: %w", currentVersionStr, err)
			}
		}
	}

	// Run migrations in order
	for _, migration := range AllMigrations {
		migrationVersion, err := semver.NewVersion(migration.Version)
		if err != nil {
			return fmt.Errorf("invalid migration version %s: %w", migration.Version, err)
		}

		// Skip if already applied (LessThanOrEqual means current >= migration)
		if !currentVersion.LessThan(migrationVersion) {
			continue // Already applied
		}

		// Execute migration
		_, err = db.ExecContext(ctx, migration.Up)
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}

		// Record migration
		_, err = db.ExecContext(ctx, "INSERT INTO schema_version (version) VALUES (?)", migration.Version)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", migration.Version, err)
		}

		// Update current version for next iteration
		currentVersion = migrationVersion
	}

	return nil
}

// RollbackMigration rolls back the most recent migration
func RollbackMigration(ctx context.Context, db *sql.DB) error {
	// Get current version
	var currentVersion string
	err := db.QueryRowContext(ctx, "SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("no migrations to rollback: %w", err)
	}

	// Find migration
	var migration *Migration
	for i := range AllMigrations {
		if AllMigrations[i].Version == currentVersion {
			migration = &AllMigrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %s not found", currentVersion)
	}

	// Execute rollback
	_, err = db.ExecContext(ctx, migration.Down)
	if err != nil {
		return fmt.Errorf("failed to rollback migration %s: %w", currentVersion, err)
	}

	// Remove version record
	_, err = db.ExecContext(ctx, "DELETE FROM schema_version WHERE version = ?", currentVersion)
	if err != nil {
		return fmt.Errorf("failed to remove migration record %s: %w", currentVersion, err)
	}

	return nil
}
