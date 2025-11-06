package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotFound is returned when a requested entity doesn't exist
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists is returned when trying to create a duplicate entity
	ErrAlreadyExists = errors.New("already exists")
)

// SQLiteStorage implements the Storage interface using SQLite
type SQLiteStorage struct {
	db *sql.DB
}

// openDatabase opens a SQLite database with appropriate settings
func openDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open(DriverName, dbPath)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite benefits from single writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return db, nil
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := openDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Apply migrations
	if err := ApplyMigrations(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	return &SQLiteStorage{db: db}, nil
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// BeginTx starts a new transaction
func (s *SQLiteStorage) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx, storage: s}, nil
}

// querier is an interface that both *sql.DB and *sql.Tx implement
type querier interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// sqliteTx wraps a SQL transaction
type sqliteTx struct {
	tx      *sql.Tx
	storage *SQLiteStorage
}

func (t *sqliteTx) Commit() error {
	return t.tx.Commit()
}

func (t *sqliteTx) Rollback() error {
	return t.tx.Rollback()
}

// querier returns the transaction querier
func (t *sqliteTx) querier() querier {
	return t.tx
}

// querier returns the DB querier
func (s *SQLiteStorage) querier() querier {
	return s.db
}

// Project operations

// createProjectWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) createProjectWithQuerier(ctx context.Context, q querier, project *Project) error {
	query := `
		INSERT INTO projects (root_path, module_name, go_version, index_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, query,
		project.RootPath, project.ModuleName, project.GoVersion,
		project.IndexVersion, now, now)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	project.ID = id
	project.CreatedAt = now
	project.UpdatedAt = now
	return nil
}

func (s *SQLiteStorage) CreateProject(ctx context.Context, project *Project) error {
	return s.createProjectWithQuerier(ctx, s.querier(), project)
}

func (s *SQLiteStorage) GetProject(ctx context.Context, rootPath string) (*Project, error) {
	query := `
		SELECT id, root_path, module_name, go_version, total_files, total_chunks,
		       index_version, last_indexed_at, created_at, updated_at
		FROM projects
		WHERE root_path = ?
	`
	var project Project
	var lastIndexedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, rootPath).Scan(
		&project.ID, &project.RootPath, &project.ModuleName, &project.GoVersion,
		&project.TotalFiles, &project.TotalChunks, &project.IndexVersion,
		&lastIndexedAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastIndexedAt.Valid {
		project.LastIndexedAt = lastIndexedAt.Time
	}
	return &project, nil
}

// updateProjectWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) updateProjectWithQuerier(ctx context.Context, q querier, project *Project) error {
	query := `
		UPDATE projects
		SET module_name = ?, go_version = ?, total_files = ?, total_chunks = ?,
		    last_indexed_at = ?, updated_at = ?
		WHERE id = ?
	`
	now := time.Now()
	_, err := q.ExecContext(ctx, query,
		project.ModuleName, project.GoVersion, project.TotalFiles, project.TotalChunks,
		project.LastIndexedAt, now, project.ID)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	project.UpdatedAt = now
	return nil
}

func (s *SQLiteStorage) UpdateProject(ctx context.Context, project *Project) error {
	return s.updateProjectWithQuerier(ctx, s.querier(), project)
}

// File operations

// upsertFileWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) upsertFileWithQuerier(ctx context.Context, q querier, file *File) error {
	query := `
		INSERT INTO files (project_id, file_path, package_name, content_hash, mod_time, size_bytes, parse_error, last_indexed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, file_path) DO UPDATE SET
			package_name = excluded.package_name,
			content_hash = excluded.content_hash,
			mod_time = excluded.mod_time,
			size_bytes = excluded.size_bytes,
			parse_error = excluded.parse_error,
			last_indexed_at = excluded.last_indexed_at,
			updated_at = excluded.updated_at
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, query,
		file.ProjectID, file.FilePath, file.PackageName, file.ContentHash[:],
		file.ModTime, file.SizeBytes, file.ParseError, now, now, now)
	if err != nil {
		return fmt.Errorf("failed to upsert file: %w", err)
	}

	// Get the ID if this was an insert
	if file.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			file.ID = id
		}
	}

	file.LastIndexedAt = now
	file.UpdatedAt = now
	return nil
}

func (s *SQLiteStorage) UpsertFile(ctx context.Context, file *File) error {
	return s.upsertFileWithQuerier(ctx, s.querier(), file)
}

func (s *SQLiteStorage) GetFile(ctx context.Context, projectID int64, filePath string) (*File, error) {
	query := `
		SELECT id, project_id, file_path, package_name, content_hash, mod_time,
		       size_bytes, parse_error, last_indexed_at, created_at, updated_at
		FROM files
		WHERE project_id = ? AND file_path = ?
	`
	var file File
	var hash []byte
	var parseError sql.NullString
	err := s.db.QueryRowContext(ctx, query, projectID, filePath).Scan(
		&file.ID, &file.ProjectID, &file.FilePath, &file.PackageName,
		&hash, &file.ModTime, &file.SizeBytes, &parseError,
		&file.LastIndexedAt, &file.CreatedAt, &file.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	copy(file.ContentHash[:], hash)
	if parseError.Valid {
		file.ParseError = &parseError.String
	}
	return &file, nil
}

func (s *SQLiteStorage) GetFileByID(ctx context.Context, fileID int64) (*File, error) {
	query := `
		SELECT id, project_id, file_path, package_name, content_hash, mod_time,
		       size_bytes, parse_error, last_indexed_at, created_at, updated_at
		FROM files
		WHERE id = ?
	`
	var file File
	var hash []byte
	var parseError sql.NullString
	err := s.db.QueryRowContext(ctx, query, fileID).Scan(
		&file.ID, &file.ProjectID, &file.FilePath, &file.PackageName,
		&hash, &file.ModTime, &file.SizeBytes, &parseError,
		&file.LastIndexedAt, &file.CreatedAt, &file.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	copy(file.ContentHash[:], hash)
	if parseError.Valid {
		file.ParseError = &parseError.String
	}
	return &file, nil
}

func (s *SQLiteStorage) GetFileByHash(ctx context.Context, contentHash [32]byte) (*File, error) {
	// Stub implementation
	return nil, fmt.Errorf("not implemented")
}

// deleteFileWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) deleteFileWithQuerier(ctx context.Context, q querier, fileID int64) error {
	query := `DELETE FROM files WHERE id = ?`
	_, err := q.ExecContext(ctx, query, fileID)
	return err
}

func (s *SQLiteStorage) DeleteFile(ctx context.Context, fileID int64) error {
	return s.deleteFileWithQuerier(ctx, s.querier(), fileID)
}

func (s *SQLiteStorage) ListFiles(ctx context.Context, projectID int64) ([]*File, error) {
	query := `
		SELECT id, project_id, file_path, package_name, content_hash, mod_time,
		       size_bytes, parse_error, last_indexed_at, created_at, updated_at
		FROM files
		WHERE project_id = ?
		ORDER BY file_path
	`
	rows, err := s.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	files := make([]*File, 0)
	for rows.Next() {
		var file File
		var hash []byte
		var parseError sql.NullString

		err := rows.Scan(
			&file.ID, &file.ProjectID, &file.FilePath, &file.PackageName,
			&hash, &file.ModTime, &file.SizeBytes, &parseError,
			&file.LastIndexedAt, &file.CreatedAt, &file.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		copy(file.ContentHash[:], hash)
		if parseError.Valid {
			file.ParseError = &parseError.String
		}

		files = append(files, &file)
	}
	return files, rows.Err()
}

// Symbol operations

// upsertSymbolWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) upsertSymbolWithQuerier(ctx context.Context, q querier, symbol *Symbol) error {
	// Check if symbol already exists (natural key: file_id + name + start_line + start_col)
	checkQuery := `
		SELECT id, created_at FROM symbols
		WHERE file_id = ? AND name = ? AND start_line = ? AND start_col = ?
		LIMIT 1
	`
	var existingID int64
	var createdAt time.Time
	err := q.QueryRowContext(ctx, checkQuery,
		symbol.FileID, symbol.Name, symbol.StartLine, symbol.StartCol,
	).Scan(&existingID, &createdAt)

	if err == nil {
		// Symbol exists - UPDATE it
		updateQuery := `
			UPDATE symbols SET
				kind = ?, package_name = ?, signature = ?, doc_comment = ?,
				scope = ?, receiver = ?, end_line = ?, end_col = ?,
				is_aggregate_root = ?, is_entity = ?, is_value_object = ?, is_repository = ?,
				is_service = ?, is_command = ?, is_query = ?, is_handler = ?
			WHERE id = ?
		`
		_, err = q.ExecContext(ctx, updateQuery,
			symbol.Kind, symbol.PackageName, symbol.Signature, symbol.DocComment,
			symbol.Scope, symbol.Receiver, symbol.EndLine, symbol.EndCol,
			symbol.IsAggregateRoot, symbol.IsEntity, symbol.IsValueObject, symbol.IsRepository,
			symbol.IsService, symbol.IsCommand, symbol.IsQuery, symbol.IsHandler,
			existingID,
		)
		if err != nil {
			return fmt.Errorf("failed to update symbol: %w", err)
		}
		symbol.ID = existingID
		symbol.CreatedAt = createdAt
		return nil
	}

	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing symbol: %w", err)
	}

	// Symbol doesn't exist - INSERT it
	insertQuery := `
		INSERT INTO symbols (
			file_id, name, kind, package_name, signature, doc_comment, scope, receiver,
			start_line, start_col, end_line, end_col,
			is_aggregate_root, is_entity, is_value_object, is_repository,
			is_service, is_command, is_query, is_handler, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, insertQuery,
		symbol.FileID, symbol.Name, symbol.Kind, symbol.PackageName,
		symbol.Signature, symbol.DocComment, symbol.Scope, symbol.Receiver,
		symbol.StartLine, symbol.StartCol, symbol.EndLine, symbol.EndCol,
		symbol.IsAggregateRoot, symbol.IsEntity, symbol.IsValueObject, symbol.IsRepository,
		symbol.IsService, symbol.IsCommand, symbol.IsQuery, symbol.IsHandler, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert symbol: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	symbol.ID = id
	symbol.CreatedAt = now
	return nil
}

func (s *SQLiteStorage) UpsertSymbol(ctx context.Context, symbol *Symbol) error {
	return s.upsertSymbolWithQuerier(ctx, s.querier(), symbol)
}

func (s *SQLiteStorage) GetSymbol(ctx context.Context, symbolID int64) (*Symbol, error) {
	query := `
		SELECT id, file_id, name, kind, package_name, signature, doc_comment, scope, receiver,
		       start_line, start_col, end_line, end_col,
		       is_aggregate_root, is_entity, is_value_object, is_repository,
		       is_service, is_command, is_query, is_handler, created_at
		FROM symbols
		WHERE id = ?
	`
	var symbol Symbol
	err := s.db.QueryRowContext(ctx, query, symbolID).Scan(
		&symbol.ID, &symbol.FileID, &symbol.Name, &symbol.Kind, &symbol.PackageName,
		&symbol.Signature, &symbol.DocComment, &symbol.Scope, &symbol.Receiver,
		&symbol.StartLine, &symbol.StartCol, &symbol.EndLine, &symbol.EndCol,
		&symbol.IsAggregateRoot, &symbol.IsEntity, &symbol.IsValueObject, &symbol.IsRepository,
		&symbol.IsService, &symbol.IsCommand, &symbol.IsQuery, &symbol.IsHandler, &symbol.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &symbol, nil
}

func (s *SQLiteStorage) ListSymbolsByFile(ctx context.Context, fileID int64) ([]*Symbol, error) {
	query := `
		SELECT id, file_id, name, kind, package_name, signature, doc_comment, scope, receiver,
		       start_line, start_col, end_line, end_col,
		       is_aggregate_root, is_entity, is_value_object, is_repository,
		       is_service, is_command, is_query, is_handler, created_at
		FROM symbols
		WHERE file_id = ?
		ORDER BY start_line
	`
	rows, err := s.db.QueryContext(ctx, query, fileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	symbols := make([]*Symbol, 0)
	for rows.Next() {
		var symbol Symbol
		err := rows.Scan(
			&symbol.ID, &symbol.FileID, &symbol.Name, &symbol.Kind, &symbol.PackageName,
			&symbol.Signature, &symbol.DocComment, &symbol.Scope, &symbol.Receiver,
			&symbol.StartLine, &symbol.StartCol, &symbol.EndLine, &symbol.EndCol,
			&symbol.IsAggregateRoot, &symbol.IsEntity, &symbol.IsValueObject, &symbol.IsRepository,
			&symbol.IsService, &symbol.IsCommand, &symbol.IsQuery, &symbol.IsHandler, &symbol.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, &symbol)
	}
	return symbols, rows.Err()
}

func (s *SQLiteStorage) SearchSymbols(ctx context.Context, query string, limit int) ([]*Symbol, error) {
	sqlQuery := `
		SELECT s.id, s.file_id, s.name, s.kind, s.package_name, s.signature, s.doc_comment, s.scope, s.receiver,
		       s.start_line, s.start_col, s.end_line, s.end_col,
		       s.is_aggregate_root, s.is_entity, s.is_value_object, s.is_repository,
		       s.is_service, s.is_command, s.is_query, s.is_handler, s.created_at
		FROM symbols s
		JOIN symbols_fts fts ON s.id = fts.rowid
		WHERE symbols_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, sqlQuery, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	symbols := make([]*Symbol, 0)
	for rows.Next() {
		var symbol Symbol
		err := rows.Scan(
			&symbol.ID, &symbol.FileID, &symbol.Name, &symbol.Kind, &symbol.PackageName,
			&symbol.Signature, &symbol.DocComment, &symbol.Scope, &symbol.Receiver,
			&symbol.StartLine, &symbol.StartCol, &symbol.EndLine, &symbol.EndCol,
			&symbol.IsAggregateRoot, &symbol.IsEntity, &symbol.IsValueObject, &symbol.IsRepository,
			&symbol.IsService, &symbol.IsCommand, &symbol.IsQuery, &symbol.IsHandler, &symbol.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, &symbol)
	}
	return symbols, rows.Err()
}

// Chunk operations

// upsertChunkWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) upsertChunkWithQuerier(ctx context.Context, q querier, chunk *Chunk) error {
	// Check if chunk already exists (natural key: file_id + start_line + end_line)
	checkQuery := `
		SELECT id, created_at FROM chunks
		WHERE file_id = ? AND start_line = ? AND end_line = ?
		LIMIT 1
	`
	var existingID int64
	var createdAt time.Time
	err := q.QueryRowContext(ctx, checkQuery,
		chunk.FileID, chunk.StartLine, chunk.EndLine,
	).Scan(&existingID, &createdAt)

	var symbolID interface{}
	if chunk.SymbolID != nil {
		symbolID = *chunk.SymbolID
	}

	if err == nil {
		// Chunk exists - UPDATE it
		updateQuery := `
			UPDATE chunks SET
				symbol_id = ?, content = ?, content_hash = ?, token_count = ?,
				context_before = ?, context_after = ?, chunk_type = ?, updated_at = ?
			WHERE id = ?
		`
		now := time.Now()
		_, err = q.ExecContext(ctx, updateQuery,
			symbolID, chunk.Content, chunk.ContentHash[:], chunk.TokenCount,
			chunk.ContextBefore, chunk.ContextAfter, chunk.ChunkType, now,
			existingID,
		)
		if err != nil {
			return fmt.Errorf("failed to update chunk: %w", err)
		}
		chunk.ID = existingID
		chunk.CreatedAt = createdAt
		chunk.UpdatedAt = now
		return nil
	}

	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing chunk: %w", err)
	}

	// Chunk doesn't exist - INSERT it
	insertQuery := `
		INSERT INTO chunks (
			file_id, symbol_id, content, content_hash, token_count,
			start_line, end_line, context_before, context_after, chunk_type,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, insertQuery,
		chunk.FileID, symbolID, chunk.Content, chunk.ContentHash[:],
		chunk.TokenCount, chunk.StartLine, chunk.EndLine,
		chunk.ContextBefore, chunk.ContextAfter, chunk.ChunkType,
		now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	chunk.ID = id
	chunk.CreatedAt = now
	chunk.UpdatedAt = now
	return nil
}

func (s *SQLiteStorage) UpsertChunk(ctx context.Context, chunk *Chunk) error {
	return s.upsertChunkWithQuerier(ctx, s.querier(), chunk)
}

func (s *SQLiteStorage) GetChunk(ctx context.Context, chunkID int64) (*Chunk, error) {
	query := `
		SELECT id, file_id, symbol_id, content, content_hash, token_count,
		       start_line, end_line, context_before, context_after, chunk_type,
		       created_at, updated_at
		FROM chunks
		WHERE id = ?
	`
	var chunk Chunk
	var hash []byte
	var symbolID sql.NullInt64

	err := s.db.QueryRowContext(ctx, query, chunkID).Scan(
		&chunk.ID, &chunk.FileID, &symbolID, &chunk.Content, &hash, &chunk.TokenCount,
		&chunk.StartLine, &chunk.EndLine, &chunk.ContextBefore, &chunk.ContextAfter,
		&chunk.ChunkType, &chunk.CreatedAt, &chunk.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	copy(chunk.ContentHash[:], hash)
	if symbolID.Valid {
		id := symbolID.Int64
		chunk.SymbolID = &id
	}

	return &chunk, nil
}

func (s *SQLiteStorage) ListChunksByFile(ctx context.Context, fileID int64) ([]*Chunk, error) {
	query := `
		SELECT id, file_id, symbol_id, content, content_hash, token_count,
		       start_line, end_line, context_before, context_after, chunk_type,
		       created_at, updated_at
		FROM chunks
		WHERE file_id = ?
		ORDER BY start_line
	`
	rows, err := s.db.QueryContext(ctx, query, fileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	chunks := make([]*Chunk, 0)
	for rows.Next() {
		var chunk Chunk
		var hash []byte
		var symbolID sql.NullInt64

		err := rows.Scan(
			&chunk.ID, &chunk.FileID, &symbolID, &chunk.Content, &hash, &chunk.TokenCount,
			&chunk.StartLine, &chunk.EndLine, &chunk.ContextBefore, &chunk.ContextAfter,
			&chunk.ChunkType, &chunk.CreatedAt, &chunk.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		copy(chunk.ContentHash[:], hash)
		if symbolID.Valid {
			id := symbolID.Int64
			chunk.SymbolID = &id
		}

		chunks = append(chunks, &chunk)
	}
	return chunks, rows.Err()
}

// deleteChunksByFileWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) deleteChunksByFileWithQuerier(ctx context.Context, q querier, fileID int64) error {
	query := `DELETE FROM chunks WHERE file_id = ?`
	_, err := q.ExecContext(ctx, query, fileID)
	return err
}

func (s *SQLiteStorage) DeleteChunksByFile(ctx context.Context, fileID int64) error {
	return s.deleteChunksByFileWithQuerier(ctx, s.querier(), fileID)
}

// Embedding operations

// upsertEmbeddingWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) upsertEmbeddingWithQuerier(ctx context.Context, q querier, embedding *Embedding) error {
	query := `
		INSERT INTO embeddings (chunk_id, vector, dimension, provider, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id) DO UPDATE SET
			vector = excluded.vector,
			dimension = excluded.dimension,
			provider = excluded.provider,
			model = excluded.model
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, query,
		embedding.ChunkID, embedding.Vector, embedding.Dimension,
		embedding.Provider, embedding.Model, now)
	if err != nil {
		return fmt.Errorf("failed to upsert embedding: %w", err)
	}

	if embedding.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			embedding.ID = id
		}
	}

	embedding.CreatedAt = now
	return nil
}

func (s *SQLiteStorage) UpsertEmbedding(ctx context.Context, embedding *Embedding) error {
	return s.upsertEmbeddingWithQuerier(ctx, s.querier(), embedding)
}

func (s *SQLiteStorage) GetEmbedding(ctx context.Context, chunkID int64) (*Embedding, error) {
	query := `
		SELECT id, chunk_id, vector, dimension, provider, model, created_at
		FROM embeddings
		WHERE chunk_id = ?
	`
	var embedding Embedding
	err := s.db.QueryRowContext(ctx, query, chunkID).Scan(
		&embedding.ID, &embedding.ChunkID, &embedding.Vector,
		&embedding.Dimension, &embedding.Provider, &embedding.Model,
		&embedding.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &embedding, nil
}

// deleteEmbeddingWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) deleteEmbeddingWithQuerier(ctx context.Context, q querier, chunkID int64) error {
	query := `DELETE FROM embeddings WHERE chunk_id = ?`
	_, err := q.ExecContext(ctx, query, chunkID)
	return err
}

func (s *SQLiteStorage) DeleteEmbedding(ctx context.Context, chunkID int64) error {
	return s.deleteEmbeddingWithQuerier(ctx, s.querier(), chunkID)
}

// Search operations

func (s *SQLiteStorage) SearchVector(ctx context.Context, projectID int64, queryVector []float32, limit int, filters *SearchFilters) ([]VectorResult, error) {
	// Implementation moved to separate file for clarity
	return searchVector(ctx, s.db, projectID, queryVector, limit, filters)
}

func (s *SQLiteStorage) SearchText(ctx context.Context, projectID int64, query string, limit int, filters *SearchFilters) ([]TextResult, error) {
	// Implementation moved to separate file for clarity
	return searchText(ctx, s.db, projectID, query, limit, filters)
}

// Import operations

// upsertImportWithQuerier is the internal implementation that uses a querier
func (s *SQLiteStorage) upsertImportWithQuerier(ctx context.Context, q querier, imp *Import) error {
	query := `
		INSERT INTO imports (file_id, import_path, alias, created_at)
		VALUES (?, ?, ?, ?)
	`
	now := time.Now()
	result, err := q.ExecContext(ctx, query, imp.FileID, imp.ImportPath, imp.Alias, now)
	if err != nil {
		return fmt.Errorf("failed to upsert import: %w", err)
	}

	if imp.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			imp.ID = id
		}
	}
	imp.CreatedAt = now
	return nil
}

func (s *SQLiteStorage) UpsertImport(ctx context.Context, imp *Import) error {
	return s.upsertImportWithQuerier(ctx, s.querier(), imp)
}

func (s *SQLiteStorage) ListImportsByFile(ctx context.Context, fileID int64) ([]*Import, error) {
	query := `
		SELECT id, file_id, import_path, alias, created_at
		FROM imports
		WHERE file_id = ?
		ORDER BY import_path
	`
	rows, err := s.db.QueryContext(ctx, query, fileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	imports := make([]*Import, 0)
	for rows.Next() {
		var imp Import
		err := rows.Scan(&imp.ID, &imp.FileID, &imp.ImportPath, &imp.Alias, &imp.CreatedAt)
		if err != nil {
			return nil, err
		}
		imports = append(imports, &imp)
	}
	return imports, rows.Err()
}

// Status operations

func (s *SQLiteStorage) GetStatus(ctx context.Context, projectID int64) (*ProjectStatus, error) {
	// Get project info
	project, err := s.getProjectByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	status := &ProjectStatus{
		Project:       project,
		LastIndexedAt: project.LastIndexedAt,
	}

	// Count files
	var fileCount int
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files WHERE project_id = ?", projectID).Scan(&fileCount)
	if err != nil {
		return nil, err
	}
	status.FilesCount = fileCount

	// Count symbols
	var symbolCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM symbols s
		JOIN files f ON s.file_id = f.id
		WHERE f.project_id = ?
	`, projectID).Scan(&symbolCount)
	if err != nil {
		return nil, err
	}
	status.SymbolsCount = symbolCount

	// Count chunks
	var chunkCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM chunks c
		JOIN files f ON c.file_id = f.id
		WHERE f.project_id = ?
	`, projectID).Scan(&chunkCount)
	if err != nil {
		return nil, err
	}
	status.ChunksCount = chunkCount

	// Count embeddings
	var embeddingCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM embeddings e
		JOIN chunks c ON e.chunk_id = c.id
		JOIN files f ON c.file_id = f.id
		WHERE f.project_id = ?
	`, projectID).Scan(&embeddingCount)
	if err != nil {
		return nil, err
	}
	status.EmbeddingsCount = embeddingCount

	// Calculate database size
	var pageCount, pageSize int
	err = s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == nil {
		_ = s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
		status.IndexSizeMB = float64(pageCount*pageSize) / (1024 * 1024)
	}

	// Check health status
	status.Health = HealthStatus{
		DatabaseAccessible:  true,
		EmbeddingsAvailable: embeddingCount > 0,
		FTSIndexesBuilt:     true, // FTS indexes are created with migrations
	}

	return status, nil
}

// getProjectByID retrieves a project by ID
func (s *SQLiteStorage) getProjectByID(ctx context.Context, projectID int64) (*Project, error) {
	query := `
		SELECT id, root_path, module_name, go_version, total_files, total_chunks,
		       index_version, last_indexed_at, created_at, updated_at
		FROM projects
		WHERE id = ?
	`
	var project Project
	var lastIndexedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, projectID).Scan(
		&project.ID, &project.RootPath, &project.ModuleName, &project.GoVersion,
		&project.TotalFiles, &project.TotalChunks, &project.IndexVersion,
		&lastIndexedAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastIndexedAt.Valid {
		project.LastIndexedAt = lastIndexedAt.Time
	}
	return &project, nil
}

// Transaction implementations - delegate to main storage for now

// Delegate read-only operations to storage (they can use DB or Tx)
// Write operations should use the internal helper that uses querier()

func (t *sqliteTx) CreateProject(ctx context.Context, project *Project) error {
	return t.storage.createProjectWithQuerier(ctx, t.querier(), project)
}

func (t *sqliteTx) GetProject(ctx context.Context, rootPath string) (*Project, error) {
	return t.storage.GetProject(ctx, rootPath)
}

func (t *sqliteTx) UpdateProject(ctx context.Context, project *Project) error {
	return t.storage.updateProjectWithQuerier(ctx, t.querier(), project)
}

func (t *sqliteTx) UpsertFile(ctx context.Context, file *File) error {
	return t.storage.upsertFileWithQuerier(ctx, t.querier(), file)
}

func (t *sqliteTx) GetFile(ctx context.Context, projectID int64, filePath string) (*File, error) {
	return t.storage.GetFile(ctx, projectID, filePath)
}

func (t *sqliteTx) GetFileByID(ctx context.Context, fileID int64) (*File, error) {
	return t.storage.GetFileByID(ctx, fileID)
}

func (t *sqliteTx) GetFileByHash(ctx context.Context, contentHash [32]byte) (*File, error) {
	return t.storage.GetFileByHash(ctx, contentHash)
}

func (t *sqliteTx) DeleteFile(ctx context.Context, fileID int64) error {
	return t.storage.deleteFileWithQuerier(ctx, t.querier(), fileID)
}

func (t *sqliteTx) ListFiles(ctx context.Context, projectID int64) ([]*File, error) {
	return t.storage.ListFiles(ctx, projectID)
}

func (t *sqliteTx) UpsertSymbol(ctx context.Context, symbol *Symbol) error {
	return t.storage.upsertSymbolWithQuerier(ctx, t.querier(), symbol)
}

func (t *sqliteTx) GetSymbol(ctx context.Context, symbolID int64) (*Symbol, error) {
	return t.storage.GetSymbol(ctx, symbolID)
}

func (t *sqliteTx) ListSymbolsByFile(ctx context.Context, fileID int64) ([]*Symbol, error) {
	return t.storage.ListSymbolsByFile(ctx, fileID)
}

func (t *sqliteTx) SearchSymbols(ctx context.Context, query string, limit int) ([]*Symbol, error) {
	return t.storage.SearchSymbols(ctx, query, limit)
}

func (t *sqliteTx) UpsertChunk(ctx context.Context, chunk *Chunk) error {
	return t.storage.upsertChunkWithQuerier(ctx, t.querier(), chunk)
}

func (t *sqliteTx) GetChunk(ctx context.Context, chunkID int64) (*Chunk, error) {
	return t.storage.GetChunk(ctx, chunkID)
}

func (t *sqliteTx) ListChunksByFile(ctx context.Context, fileID int64) ([]*Chunk, error) {
	return t.storage.ListChunksByFile(ctx, fileID)
}

func (t *sqliteTx) DeleteChunksByFile(ctx context.Context, fileID int64) error {
	return t.storage.deleteChunksByFileWithQuerier(ctx, t.querier(), fileID)
}

func (t *sqliteTx) UpsertEmbedding(ctx context.Context, embedding *Embedding) error {
	return t.storage.upsertEmbeddingWithQuerier(ctx, t.querier(), embedding)
}

func (t *sqliteTx) GetEmbedding(ctx context.Context, chunkID int64) (*Embedding, error) {
	return t.storage.GetEmbedding(ctx, chunkID)
}

func (t *sqliteTx) DeleteEmbedding(ctx context.Context, chunkID int64) error {
	return t.storage.deleteEmbeddingWithQuerier(ctx, t.querier(), chunkID)
}

func (t *sqliteTx) SearchVector(ctx context.Context, projectID int64, vector []float32, limit int, filters *SearchFilters) ([]VectorResult, error) {
	return t.storage.SearchVector(ctx, projectID, vector, limit, filters)
}

func (t *sqliteTx) SearchText(ctx context.Context, projectID int64, query string, limit int, filters *SearchFilters) ([]TextResult, error) {
	return t.storage.SearchText(ctx, projectID, query, limit, filters)
}

func (t *sqliteTx) UpsertImport(ctx context.Context, imp *Import) error {
	return t.storage.upsertImportWithQuerier(ctx, t.querier(), imp)
}

func (t *sqliteTx) ListImportsByFile(ctx context.Context, fileID int64) ([]*Import, error) {
	return t.storage.ListImportsByFile(ctx, fileID)
}

func (t *sqliteTx) GetStatus(ctx context.Context, projectID int64) (*ProjectStatus, error) {
	return t.storage.GetStatus(ctx, projectID)
}

func (t *sqliteTx) Close() error {
	// Transactions don't close the underlying connection
	return nil
}

func (t *sqliteTx) BeginTx(ctx context.Context) (Tx, error) {
	return nil, errors.New("cannot start nested transaction")
}
