package embedder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

// Provider configuration
const (
	ProviderJina   = "jina"
	ProviderOpenAI = "openai"
	ProviderLocal  = "local"

	// Default models
	DefaultJinaModel   = "jina-embeddings-v3"
	DefaultOpenAIModel = "text-embedding-3-small"

	// Dimensions
	JinaDimension   = 1024
	OpenAIDimension = 1536
	LocalDimension  = 384

	// Batch limits
	DefaultBatchSize = 50
	MaxBatchSize     = 100

	// Retry configuration
	MaxRetries        = 3
	InitialBackoffMs  = 100
	MaxBackoffMs      = 5000
	BackoffMultiplier = 2.0
)

// JinaProvider implements Embedder using Jina AI API
type JinaProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
	cache      *Cache
}

// NewJinaProvider creates a new Jina AI embedder
func NewJinaProvider(apiKey string, cache *Cache) (*JinaProvider, error) {
	if apiKey == "" {
		apiKey = os.Getenv(EnvJinaAPIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s not set", ErrNoProviderEnabled, EnvJinaAPIKey)
	}

	return &JinaProvider{
		apiKey: apiKey,
		model:  DefaultJinaModel,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: cache,
	}, nil
}

func (j *JinaProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (*Embedding, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	// Check cache
	hash := ComputeHash(req.Text)
	if j.cache != nil {
		if emb, ok := j.cache.Get(hash); ok {
			return emb, nil
		}
	}

	// Use batch API for consistency
	resp, err := j.GenerateBatch(ctx, BatchEmbeddingRequest{
		Texts: []string{req.Text},
		Model: req.Model,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("%w: no embeddings returned", ErrProviderFailed)
	}

	return resp.Embeddings[0], nil
}

func (j *JinaProvider) GenerateBatch(ctx context.Context, req BatchEmbeddingRequest) (*BatchEmbeddingResponse, error) {
	if err := ValidateBatchRequest(req); err != nil {
		return nil, err
	}

	if len(req.Texts) > MaxBatchSize {
		return nil, fmt.Errorf("%w: max %d texts allowed", ErrBatchTooLarge, MaxBatchSize)
	}

	model := req.Model
	if model == "" {
		model = j.model
	}

	// Use retry logic with exponential backoff
	config := DefaultRetryConfig()
	embeddings, err := retryWithBackoff(ctx, config, func() ([]*Embedding, error) {
		return j.callAPI(ctx, req.Texts, model)
	})

	if err != nil {
		return nil, fmt.Errorf("%w after %d retries: %v", ErrProviderFailed, MaxRetries, err)
	}

	// Cache successful embeddings
	if j.cache != nil {
		for i, emb := range embeddings {
			hash := ComputeHash(req.Texts[i])
			emb.Hash = hash
			j.cache.Set(hash, emb)
		}
	}

	return &BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   ProviderJina,
		Model:      model,
	}, nil
}

func (j *JinaProvider) callAPI(ctx context.Context, texts []string, model string) ([]*Embedding, error) {
	// Jina AI API format
	reqBody := map[string]interface{}{
		"input": texts,
		"model": model,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.jina.ai/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Model string `json:"model"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	embeddings := make([]*Embedding, len(apiResp.Data))
	for i, data := range apiResp.Data {
		embeddings[i] = &Embedding{
			Vector:    data.Embedding,
			Dimension: len(data.Embedding),
			Provider:  ProviderJina,
			Model:     apiResp.Model,
		}
	}

	return embeddings, nil
}

func (j *JinaProvider) Dimension() int {
	return JinaDimension
}

func (j *JinaProvider) Provider() string {
	return ProviderJina
}

func (j *JinaProvider) Model() string {
	return j.model
}

func (j *JinaProvider) Close() error {
	j.httpClient.CloseIdleConnections()
	return nil
}

// OpenAIProvider implements Embedder using OpenAI API
type OpenAIProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
	cache      *Cache
}

// NewOpenAIProvider creates a new OpenAI embedder
func NewOpenAIProvider(apiKey string, cache *Cache) (*OpenAIProvider, error) {
	if apiKey == "" {
		apiKey = os.Getenv(EnvOpenAIAPIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%w: %s not set", ErrNoProviderEnabled, EnvOpenAIAPIKey)
	}

	return &OpenAIProvider{
		apiKey: apiKey,
		model:  DefaultOpenAIModel,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: cache,
	}, nil
}

func (o *OpenAIProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (*Embedding, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	// Check cache
	hash := ComputeHash(req.Text)
	if o.cache != nil {
		if emb, ok := o.cache.Get(hash); ok {
			return emb, nil
		}
	}

	// Use batch API for consistency
	resp, err := o.GenerateBatch(ctx, BatchEmbeddingRequest{
		Texts: []string{req.Text},
		Model: req.Model,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("%w: no embeddings returned", ErrProviderFailed)
	}

	return resp.Embeddings[0], nil
}

func (o *OpenAIProvider) GenerateBatch(ctx context.Context, req BatchEmbeddingRequest) (*BatchEmbeddingResponse, error) {
	if err := ValidateBatchRequest(req); err != nil {
		return nil, err
	}

	if len(req.Texts) > MaxBatchSize {
		return nil, fmt.Errorf("%w: max %d texts allowed", ErrBatchTooLarge, MaxBatchSize)
	}

	model := req.Model
	if model == "" {
		model = o.model
	}

	// Use retry logic with exponential backoff
	config := DefaultRetryConfig()
	embeddings, err := retryWithBackoff(ctx, config, func() ([]*Embedding, error) {
		return o.callAPI(ctx, req.Texts, model)
	})

	if err != nil {
		return nil, fmt.Errorf("%w after %d retries: %v", ErrProviderFailed, MaxRetries, err)
	}

	// Cache successful embeddings
	if o.cache != nil {
		for i, emb := range embeddings {
			hash := ComputeHash(req.Texts[i])
			emb.Hash = hash
			o.cache.Set(hash, emb)
		}
	}

	return &BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   ProviderOpenAI,
		Model:      model,
	}, nil
}

func (o *OpenAIProvider) callAPI(ctx context.Context, texts []string, model string) ([]*Embedding, error) {
	// OpenAI API format
	reqBody := map[string]interface{}{
		"input": texts,
		"model": model,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Model string `json:"model"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	embeddings := make([]*Embedding, len(apiResp.Data))
	for i, data := range apiResp.Data {
		embeddings[i] = &Embedding{
			Vector:    data.Embedding,
			Dimension: len(data.Embedding),
			Provider:  ProviderOpenAI,
			Model:     apiResp.Model,
		}
	}

	return embeddings, nil
}

func (o *OpenAIProvider) Dimension() int {
	return OpenAIDimension
}

func (o *OpenAIProvider) Provider() string {
	return ProviderOpenAI
}

func (o *OpenAIProvider) Model() string {
	return o.model
}

func (o *OpenAIProvider) Close() error {
	o.httpClient.CloseIdleConnections()
	return nil
}

// LocalProvider is a stub for local embedding models
type LocalProvider struct {
	model string
	cache *Cache
}

// NewLocalProvider creates a new local embedder (placeholder implementation)
func NewLocalProvider(cache *Cache) (*LocalProvider, error) {
	return &LocalProvider{
		model: "local-embeddings",
		cache: cache,
	}, nil
}

func (l *LocalProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (*Embedding, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	// Check cache
	hash := ComputeHash(req.Text)
	if l.cache != nil {
		if emb, ok := l.cache.Get(hash); ok {
			return emb, nil
		}
	}

	// Stub implementation: return zero vector
	// TODO: Integrate with actual local model (e.g., sentence-transformers via spago)
	vector := make([]float32, LocalDimension)

	// Simple deterministic "embedding" based on text hash for testing
	// In production, this would call an actual model
	textHash := sha256.Sum256([]byte(req.Text))
	for i := 0; i < LocalDimension && i < len(textHash); i++ {
		vector[i] = float32(textHash[i]) / 255.0
	}

	emb := &Embedding{
		Vector:    vector,
		Dimension: LocalDimension,
		Provider:  ProviderLocal,
		Model:     l.model,
		Hash:      hash,
	}

	// Cache the result
	if l.cache != nil {
		l.cache.Set(hash, emb)
	}

	return emb, nil
}

func (l *LocalProvider) GenerateBatch(ctx context.Context, req BatchEmbeddingRequest) (*BatchEmbeddingResponse, error) {
	if err := ValidateBatchRequest(req); err != nil {
		return nil, err
	}

	embeddings := make([]*Embedding, len(req.Texts))
	for i, text := range req.Texts {
		emb, err := l.GenerateEmbedding(ctx, EmbeddingRequest{Text: text, Model: req.Model})
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	return &BatchEmbeddingResponse{
		Embeddings: embeddings,
		Provider:   ProviderLocal,
		Model:      l.model,
	}, nil
}

func (l *LocalProvider) Dimension() int {
	return LocalDimension
}

func (l *LocalProvider) Provider() string {
	return ProviderLocal
}

func (l *LocalProvider) Model() string {
	return l.model
}

func (l *LocalProvider) Close() error {
	return nil
}

// NormalizeVector normalizes a vector to unit length (for cosine similarity)
func NormalizeVector(v []float32) []float32 {
	var sum float64
	for _, val := range v {
		sum += float64(val * val)
	}

	if sum == 0 {
		return v
	}

	norm := float32(math.Sqrt(sum))
	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = val / norm
	}

	return result
}
