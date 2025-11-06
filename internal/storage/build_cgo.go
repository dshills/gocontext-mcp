//go:build sqlite_vec
// +build sqlite_vec

package storage

// This file is compiled when building with CGO and the sqlite_vec tag.
// It enables the sqlite-vec extension for fast vector similarity search.
//
// Build command:
//   CGO_ENABLED=1 go build -tags "sqlite_vec,fts5" ./...
//
// The sqlite-vec extension provides:
//   - Native vector similarity search (cosine, euclidean)
//   - Fast C implementation for vector operations
//   - FTS5 full-text search support
//   - Recommended for production deployments
//
// Driver used: github.com/mattn/go-sqlite3

import (
	_ "github.com/mattn/go-sqlite3"
)

const (
	// DriverName is the SQLite driver to use
	DriverName = "sqlite3"

	// VectorExtensionAvailable indicates if vector extension is available
	VectorExtensionAvailable = true

	// BuildMode describes the current build configuration
	BuildMode = "cgo"
)
