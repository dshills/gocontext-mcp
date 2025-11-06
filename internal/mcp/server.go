package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"

	"github.com/dshills/gocontext-mcp/internal/embedder"
	"github.com/dshills/gocontext-mcp/internal/indexer"
	"github.com/dshills/gocontext-mcp/internal/searcher"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

const (
	// ServerName is the MCP server name
	ServerName = "gocontext-mcp"
	// ServerVersion is the current server version
	ServerVersion = "1.0.0"
	// DefaultDBPath is the default location for the database
	DefaultDBPath = "~/.gocontext/indices"
)

// Server wraps the MCP server with application dependencies
type Server struct {
	mcp      *server.MCPServer
	storage  storage.Storage
	indexer  *indexer.Indexer
	searcher *searcher.Searcher
}

// NewServer creates a new MCP server instance
func NewServer(dbPath string) (*Server, error) {
	// Expand home directory if needed
	if dbPath == "" || dbPath == "~/.gocontext/indices" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dbPath = filepath.Join(home, ".gocontext", "indices")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// For now, use a single database file
	// In future, we could have per-project databases
	dbFile := filepath.Join(dbPath, "gocontext.db")

	// Initialize storage
	store, err := storage.NewSQLiteStorage(dbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Create embedder
	emb, err := embedder.NewFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedder: %w", err)
	}

	// Create indexer
	idx := indexer.New(store)

	// Create searcher
	srch := searcher.NewSearcher(store, emb)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
	)

	s := &Server{
		mcp:      mcpServer,
		storage:  store,
		indexer:  idx,
		searcher: srch,
	}

	// Register tools
	if err := s.registerTools(); err != nil {
		return nil, fmt.Errorf("failed to register tools: %w", err)
	}

	return s, nil
}

// Serve starts the MCP server on stdio and blocks until shutdown
func (s *Server) Serve(ctx context.Context) error {
	defer func() { _ = s.storage.Close() }()
	return server.ServeStdio(s.mcp)
}

// registerTools registers all MCP tools
func (s *Server) registerTools() error {
	// Register index_codebase tool
	s.mcp.AddTool(indexCodebaseTool(), s.handleIndexCodebase)

	// Register search_code tool
	s.mcp.AddTool(searchCodeTool(), s.handleSearchCode)

	// Register get_status tool
	s.mcp.AddTool(getStatusTool(), s.handleGetStatus)

	return nil
}
