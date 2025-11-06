package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dshills/gocontext-mcp/internal/mcp"
	"github.com/dshills/gocontext-mcp/internal/storage"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Handle version flag
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("GoContext MCP Server\n")
		fmt.Printf("Version: %s\n", version)
		fmt.Printf("Build Time: %s\n", buildTime)
		fmt.Printf("Build Mode: %s\n", storage.BuildMode)
		fmt.Printf("SQLite Driver: %s\n", storage.DriverName)
		fmt.Printf("Vector Extension: %v\n", storage.VectorExtensionAvailable)
		os.Exit(0)
	}

	// Log startup info to stderr (stdout reserved for MCP protocol)
	log.SetOutput(os.Stderr)
	log.Printf("GoContext MCP Server v%s starting...", version)
	log.Printf("Build Mode: %s, Driver: %s, Vector Extension: %v",
		storage.BuildMode, storage.DriverName, storage.VectorExtensionAvailable)

	// Get database path from environment or use default
	dbPath := os.Getenv("GOCONTEXT_DB_PATH")
	if dbPath == "" {
		dbPath = mcp.DefaultDBPath
	}

	// Create MCP server
	server, err := mcp.NewServer(dbPath)
	if err != nil {
		log.Fatalf("Failed to create MCP server: %v", err)
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Println("MCP server ready, listening on stdio...")
		errChan <- server.Serve(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down gracefully...", sig)
		cancel()
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}

	log.Println("Server stopped")
}
