//go:build purego || !sqlite_vec
// +build purego !sqlite_vec

package storage

// This file is compiled when building without CGO or with the purego tag.
// It uses a pure Go SQLite implementation without the sqlite-vec extension.
//
// Build command:
//   CGO_ENABLED=0 go build -tags "purego" ./...
//
// The pure Go implementation provides:
//   - No C compiler required
//   - Cross-platform compilation
//   - Slower vector operations (pure Go)
//   - Suitable for development and smaller codebases
//
// Driver used: modernc.org/sqlite

import (
	_ "modernc.org/sqlite"
)

const (
	// DriverName is the SQLite driver to use
	DriverName = "sqlite"

	// VectorExtensionAvailable indicates if vector extension is available
	VectorExtensionAvailable = false

	// BuildMode describes the current build configuration
	BuildMode = "purego"
)
