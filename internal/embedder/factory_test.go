package embedder

import (
	"os"
	"testing"
)

func TestDetectProvider(t *testing.T) {
	// Save original env vars
	origProvider := os.Getenv("GOCONTEXT_EMBEDDING_PROVIDER")
	origJina := os.Getenv("JINA_API_KEY")
	origOpenAI := os.Getenv("OPENAI_API_KEY")

	// Restore after test
	defer func() {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", origProvider)
		os.Setenv("JINA_API_KEY", origJina)
		os.Setenv("OPENAI_API_KEY", origOpenAI)
	}()

	tests := []struct {
		name           string
		provider       string
		jinaKey        string
		openaiKey      string
		expectedResult string
	}{
		{
			name:           "explicit jina provider",
			provider:       "jina",
			jinaKey:        "",
			openaiKey:      "",
			expectedResult: ProviderJina,
		},
		{
			name:           "explicit openai provider",
			provider:       "openai",
			jinaKey:        "",
			openaiKey:      "",
			expectedResult: ProviderOpenAI,
		},
		{
			name:           "explicit local provider",
			provider:       "local",
			jinaKey:        "",
			openaiKey:      "",
			expectedResult: ProviderLocal,
		},
		{
			name:           "jina key present",
			provider:       "",
			jinaKey:        "test-key",
			openaiKey:      "",
			expectedResult: ProviderJina,
		},
		{
			name:           "openai key present",
			provider:       "",
			jinaKey:        "",
			openaiKey:      "test-key",
			expectedResult: ProviderOpenAI,
		},
		{
			name:           "both keys, jina takes precedence",
			provider:       "",
			jinaKey:        "jina-key",
			openaiKey:      "openai-key",
			expectedResult: ProviderJina,
		},
		{
			name:           "no provider, no keys - fallback to local",
			provider:       "",
			jinaKey:        "",
			openaiKey:      "",
			expectedResult: ProviderLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			if tt.provider != "" {
				os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", tt.provider)
			} else {
				os.Unsetenv("GOCONTEXT_EMBEDDING_PROVIDER")
			}

			if tt.jinaKey != "" {
				os.Setenv("JINA_API_KEY", tt.jinaKey)
			} else {
				os.Unsetenv("JINA_API_KEY")
			}

			if tt.openaiKey != "" {
				os.Setenv("OPENAI_API_KEY", tt.openaiKey)
			} else {
				os.Unsetenv("OPENAI_API_KEY")
			}

			got := DetectProvider()
			if got != tt.expectedResult {
				t.Errorf("DetectProvider() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestNewFromEnv(t *testing.T) {
	// Save original env vars
	origProvider := os.Getenv("GOCONTEXT_EMBEDDING_PROVIDER")
	origJina := os.Getenv("JINA_API_KEY")
	origOpenAI := os.Getenv("OPENAI_API_KEY")

	// Restore after test
	defer func() {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", origProvider)
		os.Setenv("JINA_API_KEY", origJina)
		os.Setenv("OPENAI_API_KEY", origOpenAI)
	}()

	t.Run("local provider (no keys)", func(t *testing.T) {
		os.Unsetenv("GOCONTEXT_EMBEDDING_PROVIDER")
		os.Unsetenv("JINA_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderLocal {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderLocal)
		}
	})

	t.Run("explicit local provider", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "local")
		os.Unsetenv("JINA_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderLocal {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderLocal)
		}
	})

	t.Run("jina with api key", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "jina")
		os.Setenv("JINA_API_KEY", "test-jina-key")
		os.Unsetenv("OPENAI_API_KEY")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderJina {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderJina)
		}
	})

	t.Run("jina without api key", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "jina")
		os.Unsetenv("JINA_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		_, err := NewFromEnv()
		if err == nil {
			t.Error("Expected error when JINA_API_KEY not set")
		}
	})

	t.Run("openai with api key", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "openai")
		os.Unsetenv("JINA_API_KEY")
		os.Setenv("OPENAI_API_KEY", "test-openai-key")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderOpenAI {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderOpenAI)
		}
	})

	t.Run("openai without api key", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "openai")
		os.Unsetenv("JINA_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		_, err := NewFromEnv()
		if err == nil {
			t.Error("Expected error when OPENAI_API_KEY not set")
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		os.Setenv("GOCONTEXT_EMBEDDING_PROVIDER", "unknown")
		os.Unsetenv("JINA_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")

		_, err := NewFromEnv()
		if err == nil {
			t.Error("Expected error for unknown provider")
		}
	})

	t.Run("auto-detect jina", func(t *testing.T) {
		os.Unsetenv("GOCONTEXT_EMBEDDING_PROVIDER")
		os.Setenv("JINA_API_KEY", "test-key")
		os.Unsetenv("OPENAI_API_KEY")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderJina {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderJina)
		}
	})

	t.Run("auto-detect openai", func(t *testing.T) {
		os.Unsetenv("GOCONTEXT_EMBEDDING_PROVIDER")
		os.Unsetenv("JINA_API_KEY")
		os.Setenv("OPENAI_API_KEY", "test-key")

		embedder, err := NewFromEnv()
		if err != nil {
			t.Fatalf("NewFromEnv() error = %v", err)
		}
		defer embedder.Close()

		if embedder.Provider() != ProviderOpenAI {
			t.Errorf("Provider = %s, want %s", embedder.Provider(), ProviderOpenAI)
		}
	})
}

func TestNew(t *testing.T) {
	// Save and clear environment variables for clean test
	origJina := os.Getenv("JINA_API_KEY")
	origOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() {
		if origJina != "" {
			os.Setenv("JINA_API_KEY", origJina)
		}
		if origOpenAI != "" {
			os.Setenv("OPENAI_API_KEY", origOpenAI)
		}
	}()

	tests := []struct {
		name     string
		cfg      Config
		wantErr  bool
		wantProv string
	}{
		{
			name: "jina with key",
			cfg: Config{
				Provider:  ProviderJina,
				APIKey:    "test-key",
				CacheSize: 100,
			},
			wantErr:  false,
			wantProv: ProviderJina,
		},
		{
			name: "openai with key",
			cfg: Config{
				Provider:  ProviderOpenAI,
				APIKey:    "test-key",
				CacheSize: 100,
			},
			wantErr:  false,
			wantProv: ProviderOpenAI,
		},
		{
			name: "local provider",
			cfg: Config{
				Provider:  ProviderLocal,
				CacheSize: 50,
			},
			wantErr:  false,
			wantProv: ProviderLocal,
		},
		{
			name: "jina without key",
			cfg: Config{
				Provider: ProviderJina,
				APIKey:   "",
			},
			wantErr: true,
		},
		{
			name: "openai without key",
			cfg: Config{
				Provider: ProviderOpenAI,
				APIKey:   "",
			},
			wantErr: true,
		},
		{
			name: "unknown provider",
			cfg: Config{
				Provider: "unknown",
			},
			wantErr: true,
		},
		{
			name: "case insensitive provider",
			cfg: Config{
				Provider:  "JINA",
				APIKey:    "test-key",
				CacheSize: 0,
			},
			wantErr:  false,
			wantProv: ProviderJina,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unset env vars for each test case
			os.Unsetenv("JINA_API_KEY")
			os.Unsetenv("OPENAI_API_KEY")

			embedder, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				defer embedder.Close()
				if embedder.Provider() != tt.wantProv {
					t.Errorf("Provider = %s, want %s", embedder.Provider(), tt.wantProv)
				}
			}
		})
	}
}
