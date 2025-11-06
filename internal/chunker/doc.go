// Package chunker divides Go source code into semantic chunks for embedding and search.
//
// The chunker creates chunks at natural code boundaries (functions, types, etc.) to
// preserve semantic meaning and enable accurate code search.
//
// # Basic Usage
//
//	c := chunker.New()
//	chunks, err := c.ChunkFile("/path/to/file.go", parseResult, fileID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for _, chunk := range chunks {
//	    fmt.Printf("Chunk: %d tokens, lines %d-%d\n",
//	        chunk.TokenCount, chunk.StartLine, chunk.EndLine)
//	}
//
// # Chunking Strategy
//
// Chunks are created at semantic boundaries:
//   - Functions: Complete function body with signature
//   - Methods: Complete method including receiver
//   - Types: Full type declaration (struct, interface, etc.)
//   - Const/var groups: Related declarations together
//
// Each chunk includes context:
//   - ContextBefore: Package declaration and relevant imports
//   - ContextAfter: Related symbols (next 1-2 functions/types)
//
// # Chunk Sizing
//
// Target token counts:
//   - Minimum: 50 tokens (avoid tiny chunks)
//   - Ideal: 200-500 tokens (embedding sweet spot)
//   - Maximum: 2000 tokens (model limits)
//
// Token estimation uses a simple heuristic (chars/4). For more accuracy,
// use a proper tokenizer library.
//
// # Content Hashing
//
// Each chunk computes a SHA-256 hash of its content:
//
//	chunk.ComputeContentHash()
//	// chunk.ContentHash is now [32]byte SHA-256 hash
//
// This enables incremental indexing by detecting unchanged chunks:
//
//	if storedHash == chunk.ContentHash {
//	    // Skip re-embedding this chunk
//	}
//
// # Example: Processing Parse Results
//
//	// Parse file
//	parser := parser.New()
//	parseResult, err := parser.ParseFile("service.go")
//
//	// Create chunks
//	chunker := chunker.New()
//	chunks, err := chunker.ChunkFile("service.go", parseResult, fileID)
//
//	// Process each chunk
//	for _, chunk := range chunks {
//	    chunk.ComputeContentHash()
//	    chunk.ComputeTokenCount()
//
//	    if err := chunk.Validate(); err != nil {
//	        log.Printf("Invalid chunk: %v", err)
//	        continue
//	    }
//
//	    // Ready for embedding
//	    fullContent := chunk.FullContent() // includes context
//	}
package chunker
