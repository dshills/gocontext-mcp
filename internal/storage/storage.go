package storage

import (
	"context"
	"time"

	"github.com/dshills/gocontext-mcp/pkg/types"
)

// Storage defines the interface for persisting and querying indexed code data
type Storage interface {
	// Project operations
	CreateProject(ctx context.Context, project *Project) error
	GetProject(ctx context.Context, rootPath string) (*Project, error)
	UpdateProject(ctx context.Context, project *Project) error

	// File operations
	UpsertFile(ctx context.Context, file *File) error
	GetFile(ctx context.Context, projectID int64, filePath string) (*File, error)
	GetFileByID(ctx context.Context, fileID int64) (*File, error)
	GetFileByHash(ctx context.Context, contentHash [32]byte) (*File, error)
	DeleteFile(ctx context.Context, fileID int64) error
	ListFiles(ctx context.Context, projectID int64) ([]*File, error)

	// Symbol operations
	UpsertSymbol(ctx context.Context, symbol *Symbol) error
	GetSymbol(ctx context.Context, symbolID int64) (*Symbol, error)
	ListSymbolsByFile(ctx context.Context, fileID int64) ([]*Symbol, error)
	DeleteSymbolsByFile(ctx context.Context, fileID int64) error
	SearchSymbols(ctx context.Context, query string, limit int) ([]*Symbol, error)

	// Chunk operations
	UpsertChunk(ctx context.Context, chunk *Chunk) error
	GetChunk(ctx context.Context, chunkID int64) (*Chunk, error)
	ListChunksByFile(ctx context.Context, fileID int64) ([]*Chunk, error)
	DeleteChunk(ctx context.Context, chunkID int64) error
	DeleteChunksBatch(ctx context.Context, chunkIDs []int64) (deletedCount int, err error)
	DeleteChunksByFile(ctx context.Context, fileID int64) error

	// Embedding operations
	UpsertEmbedding(ctx context.Context, embedding *Embedding) error
	GetEmbedding(ctx context.Context, chunkID int64) (*Embedding, error)
	DeleteEmbedding(ctx context.Context, chunkID int64) error

	// Search operations
	SearchVector(ctx context.Context, projectID int64, vector []float32, limit int, filters *SearchFilters) ([]VectorResult, error)
	SearchText(ctx context.Context, projectID int64, query string, limit int, filters *SearchFilters) ([]TextResult, error)

	// Import operations
	UpsertImport(ctx context.Context, imp *Import) error
	ListImportsByFile(ctx context.Context, fileID int64) ([]*Import, error)
	DeleteImportsByFile(ctx context.Context, fileID int64) error

	// Status operations
	GetStatus(ctx context.Context, projectID int64) (*ProjectStatus, error)

	// Database operations
	Close() error
	BeginTx(ctx context.Context) (Tx, error)
}

// Tx represents a database transaction
type Tx interface {
	Commit() error
	Rollback() error
	Storage // Embed Storage interface for transaction operations
}

// Project represents an indexed Go codebase
type Project struct {
	ID            int64
	RootPath      string
	ModuleName    string
	GoVersion     string
	TotalFiles    int
	TotalChunks   int
	IndexVersion  string
	LastIndexedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// File represents a tracked Go source file
type File struct {
	ID            int64
	ProjectID     int64
	FilePath      string // Relative to project root
	PackageName   string
	ContentHash   [32]byte
	ModTime       time.Time
	SizeBytes     int64
	ParseError    *string // Nullable
	LastIndexedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Symbol represents a code symbol from AST parsing
type Symbol struct {
	ID              int64
	FileID          int64
	Name            string
	Kind            string
	PackageName     string
	Signature       string
	DocComment      string
	Scope           string
	Receiver        string
	StartLine       int
	StartCol        int
	EndLine         int
	EndCol          int
	IsAggregateRoot bool
	IsEntity        bool
	IsValueObject   bool
	IsRepository    bool
	IsService       bool
	IsCommand       bool
	IsQuery         bool
	IsHandler       bool
	CreatedAt       time.Time
}

// Chunk represents a code section for embedding
type Chunk struct {
	ID            int64
	FileID        int64
	SymbolID      *int64 // Nullable
	Content       string
	ContentHash   [32]byte
	TokenCount    int
	StartLine     int
	EndLine       int
	ContextBefore string
	ContextAfter  string
	ChunkType     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Embedding represents a vector embedding for a chunk
type Embedding struct {
	ID        int64
	ChunkID   int64
	Vector    []byte // Serialized float32 array
	Dimension int
	Provider  string
	Model     string
	CreatedAt time.Time
}

// Import represents an import statement in a Go file
type Import struct {
	ID         int64
	FileID     int64
	ImportPath string
	Alias      string
	CreatedAt  time.Time
}

// SearchFilters contains filters for narrowing search results
type SearchFilters struct {
	SymbolTypes  []string // Filter by symbol kind
	FilePattern  string   // Glob pattern for file paths
	DDDPatterns  []string // Filter by DDD pattern flags
	Packages     []string // Filter by package names
	MinRelevance float64  // Minimum relevance score
}

// VectorResult represents a result from vector similarity search
type VectorResult struct {
	ChunkID         int64
	SimilarityScore float64
}

// TextResult represents a result from full-text search
type TextResult struct {
	ChunkID   int64
	BM25Score float64
}

// ProjectStatus contains statistics about an indexed project
type ProjectStatus struct {
	Project         *Project
	FilesCount      int
	SymbolsCount    int
	ChunksCount     int
	EmbeddingsCount int
	IndexSizeMB     float64
	LastIndexedAt   time.Time
	IndexDuration   time.Duration
	Health          HealthStatus
}

// HealthStatus represents the health of the index
type HealthStatus struct {
	DatabaseAccessible  bool
	EmbeddingsAvailable bool
	FTSIndexesBuilt     bool
}

// ToTypesSymbol converts storage Symbol to types.Symbol
func (s *Symbol) ToTypesSymbol() types.Symbol {
	return types.Symbol{
		Name:       s.Name,
		Kind:       types.SymbolKind(s.Kind),
		Package:    s.PackageName,
		Signature:  s.Signature,
		DocComment: s.DocComment,
		Scope:      types.SymbolScope(s.Scope),
		Receiver:   s.Receiver,
		Start: types.Position{
			Line:   s.StartLine,
			Column: s.StartCol,
		},
		End: types.Position{
			Line:   s.EndLine,
			Column: s.EndCol,
		},
		IsAggregateRoot: s.IsAggregateRoot,
		IsEntity:        s.IsEntity,
		IsValueObject:   s.IsValueObject,
		IsRepository:    s.IsRepository,
		IsService:       s.IsService,
		IsCommand:       s.IsCommand,
		IsQuery:         s.IsQuery,
		IsHandler:       s.IsHandler,
	}
}

// FromTypesSymbol converts types.Symbol to storage Symbol
func FromTypesSymbol(s types.Symbol, fileID int64) *Symbol {
	return &Symbol{
		FileID:          fileID,
		Name:            s.Name,
		Kind:            string(s.Kind),
		PackageName:     s.Package,
		Signature:       s.Signature,
		DocComment:      s.DocComment,
		Scope:           string(s.Scope),
		Receiver:        s.Receiver,
		StartLine:       s.Start.Line,
		StartCol:        s.Start.Column,
		EndLine:         s.End.Line,
		EndCol:          s.End.Column,
		IsAggregateRoot: s.IsAggregateRoot,
		IsEntity:        s.IsEntity,
		IsValueObject:   s.IsValueObject,
		IsRepository:    s.IsRepository,
		IsService:       s.IsService,
		IsCommand:       s.IsCommand,
		IsQuery:         s.IsQuery,
		IsHandler:       s.IsHandler,
	}
}
