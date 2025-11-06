package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	internalMCP "github.com/dshills/gocontext-mcp/internal/mcp"
)

// Note: These tests focus on validation logic and error constants
// Full integration testing of MCP handlers is done in integration tests

// TestErrorCodes verifies MCP error codes are defined correctly
func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"ErrorCodeInvalidParams", internalMCP.ErrorCodeInvalidParams},
		{"ErrorCodeInternalError", internalMCP.ErrorCodeInternalError},
		{"ErrorCodeProjectNotFound", internalMCP.ErrorCodeProjectNotFound},
		{"ErrorCodeIndexingInProgress", internalMCP.ErrorCodeIndexingInProgress},
		{"ErrorCodeNotIndexed", internalMCP.ErrorCodeNotIndexed},
		{"ErrorCodeEmptyQuery", internalMCP.ErrorCodeEmptyQuery},
	}

	// Verify error codes are unique and in expected range
	seenCodes := make(map[int]string)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check code is in valid range
			if tt.code > 0 || tt.code < -40000 {
				t.Errorf("%s has invalid code %d (should be negative and > -40000)", tt.name, tt.code)
			}

			// Check for duplicates
			if existing, found := seenCodes[tt.code]; found {
				t.Errorf("%s has duplicate code %d (already used by %s)", tt.name, tt.code, existing)
			}
			seenCodes[tt.code] = tt.name
		})
	}
}

// TestMCPError tests the MCPError type
func TestMCPError(t *testing.T) {
	tests := []struct {
		name          string
		code          int
		message       string
		data          interface{}
		expectedError string
	}{
		{
			name:          "SimpleError",
			code:          -32602,
			message:       "invalid params",
			data:          nil,
			expectedError: "MCP error -32602: invalid params",
		},
		{
			name:    "ErrorWithData",
			code:    -32001,
			message: "project not found",
			data: map[string]interface{}{
				"path": "/test/path",
			},
			expectedError: "MCP error -32001: project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &internalMCP.MCPError{
				Code:    tt.code,
				Message: tt.message,
				Data:    tt.data,
			}

			if err.Error() != tt.expectedError {
				t.Errorf("expected error message %q, got %q", tt.expectedError, err.Error())
			}

			if err.Code != tt.code {
				t.Errorf("expected code %d, got %d", tt.code, err.Code)
			}

			if err.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, err.Message)
			}
		})
	}
}

// TestPathValidationErrors tests the validation error types
func TestPathValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrPathRequired", internalMCP.ErrPathRequired},
		{"ErrPathNotAbsolute", internalMCP.ErrPathNotAbsolute},
		{"ErrPathNotFound", internalMCP.ErrPathNotFound},
		{"ErrPathNotReadable", internalMCP.ErrPathNotReadable},
		{"ErrNotDirectory", internalMCP.ErrNotDirectory},
		{"ErrNoGoFiles", internalMCP.ErrNoGoFiles},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}

			if tt.err.Error() == "" {
				t.Errorf("%s has empty error message", tt.name)
			}
		})
	}
}

// TestGetTestdataPath is a helper to create test fixtures
func TestGetTestdataPath(t *testing.T) {
	// Get testdata path
	testdataPath, err := filepath.Abs("../../../tests/testdata/fixtures")
	if err != nil {
		t.Fatalf("failed to get testdata path: %v", err)
	}

	// Verify it exists
	info, err := os.Stat(testdataPath)
	if err != nil {
		t.Skipf("testdata path does not exist: %s", testdataPath)
	}

	if !info.IsDir() {
		t.Errorf("testdata path is not a directory: %s", testdataPath)
	}
}

// TestServerConstants tests server name and version constants
func TestServerConstants(t *testing.T) {
	if internalMCP.ServerName == "" {
		t.Error("ServerName should not be empty")
	}

	if internalMCP.ServerVersion == "" {
		t.Error("ServerVersion should not be empty")
	}

	if internalMCP.DefaultDBPath == "" {
		t.Error("DefaultDBPath should not be empty")
	}
}
