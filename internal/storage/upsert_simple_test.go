package storage

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertSymbol_DirectSQL verifies UPSERT works at the SQL level
// This test bypasses the storage layer to isolate the SQL issue
func TestUpsertSymbol_DirectSQL(t *testing.T) {
	// Open database directly
	db, err := sql.Open(DriverName, ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Apply migrations
	err = ApplyMigrations(ctx, db)
	require.NoError(t, err)

	// Create test data
	_, err = db.ExecContext(ctx, `
		INSERT INTO projects (root_path, module_name, go_version, index_version, created_at, updated_at)
		VALUES ('/test', 'test', '1.21', '1.0.0', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO files (project_id, file_path, package_name, content_hash, mod_time, size_bytes, created_at, updated_at)
		VALUES (1, 'test.go', 'test', X'010203', CURRENT_TIMESTAMP, 100, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	require.NoError(t, err)

	// First insert
	query1 := `
		INSERT INTO symbols (
			file_id, name, kind, package_name, signature, doc_comment, scope, receiver,
			start_line, start_col, end_line, end_col,
			is_aggregate_root, is_entity, is_value_object, is_repository,
			is_service, is_command, is_query, is_handler, created_at
		) VALUES (1, 'TestFunc', 'function', 'test', 'func TestFunc()', '', 'exported', '',
			10, 1, 20, 1, 0, 0, 0, 0, 0, 0, 0, 0, CURRENT_TIMESTAMP)
		RETURNING id
	`
	var id1 int64
	err = db.QueryRowContext(ctx, query1).Scan(&id1)
	require.NoError(t, err, "First insert should succeed")
	t.Logf("First insert: ID=%d", id1)

	// Second insert with UPSERT (same unique key)
	query2 := `
		INSERT INTO symbols (
			file_id, name, kind, package_name, signature, doc_comment, scope, receiver,
			start_line, start_col, end_line, end_col,
			is_aggregate_root, is_entity, is_value_object, is_repository,
			is_service, is_command, is_query, is_handler, created_at
		) VALUES (1, 'TestFunc', 'function', 'test', 'func TestFunc() error', '', 'exported', '',
			10, 1, 25, 1, 0, 0, 0, 0, 0, 0, 0, 0, CURRENT_TIMESTAMP)
		ON CONFLICT(file_id, name, start_line, start_col)
		DO UPDATE SET
			signature = excluded.signature,
			end_line = excluded.end_line
		RETURNING id
	`
	var id2 int64
	err = db.QueryRowContext(ctx, query2).Scan(&id2)
	if err != nil {
		t.Logf("Error on UPSERT: %v", err)

		// Check if FTS table exists
		var ftsExists int
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='symbols_fts'").Scan(&ftsExists)
		t.Logf("FTS table exists: %v", ftsExists > 0)

		// Check triggers
		rows, _ := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='symbols'")
		t.Log("Triggers on symbols table:")
		for rows.Next() {
			var name string
			rows.Scan(&name)
			t.Logf("  - %s", name)
		}
		rows.Close()
	}
	require.NoError(t, err, "UPSERT should succeed")
	t.Logf("Second UPSERT: ID=%d", id2)

	// Verify only one row exists
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols WHERE file_id=1").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only one symbol")

	// Verify the update took effect
	var signature string
	var endLine int
	err = db.QueryRowContext(ctx, "SELECT signature, end_line FROM symbols WHERE id=?", id1).Scan(&signature, &endLine)
	require.NoError(t, err)
	assert.Equal(t, "func TestFunc() error", signature)
	assert.Equal(t, 25, endLine)

	t.Log("✓ Direct SQL UPSERT test passed")
}

// TestNoFTS verifies UPSERT works without FTS
func TestUpsertSymbol_WithoutFTS(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open(DriverName, ":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Create minimal schema WITHOUT FTS and triggers
	_, err = db.ExecContext(ctx, `
		CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			root_path TEXT NOT NULL UNIQUE,
			module_name TEXT,
			go_version TEXT,
			index_version TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			file_path TEXT NOT NULL,
			package_name TEXT,
			content_hash BLOB NOT NULL,
			mod_time TIMESTAMP,
			size_bytes INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			UNIQUE(project_id, file_path)
		);

		CREATE TABLE symbols (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			package_name TEXT NOT NULL,
			signature TEXT,
			start_line INTEGER,
			start_col INTEGER,
			end_line INTEGER,
			end_col INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
			UNIQUE(file_id, name, start_line, start_col)
		);
	`)
	require.NoError(t, err)

	// Create test data
	_, err = db.ExecContext(ctx, "INSERT INTO projects (root_path, module_name, go_version, index_version) VALUES ('/test', 'test', '1.21', '1.0.0')")
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO files (project_id, file_path, package_name, content_hash, mod_time, size_bytes) VALUES (1, 'test.go', 'test', X'010203', CURRENT_TIMESTAMP, 100)")
	require.NoError(t, err)

	// Test UPSERT multiple times
	for i := 0; i < 5; i++ {
		query := `
			INSERT INTO symbols (file_id, name, kind, package_name, signature, start_line, start_col, end_line, end_col, created_at)
			VALUES (1, 'TestFunc', 'function', 'test', ?, 10, 1, ?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT(file_id, name, start_line, start_col)
			DO UPDATE SET signature = excluded.signature, end_line = excluded.end_line
			RETURNING id
		`
		var id int64
		err = db.QueryRowContext(ctx, query, "func TestFunc() iteration "+string(rune('0'+i)), 20+i).Scan(&id)
		require.NoError(t, err, "UPSERT iteration %d should succeed", i)
		t.Logf("Iteration %d: ID=%d", i, id)
	}

	// Verify only one row exists
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols WHERE file_id=1").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only one symbol after 5 upserts")

	t.Log("✓ UPSERT without FTS test passed")
}
