package sample

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestHashPassword_UsesBcrypt tests that hashPassword uses bcrypt algorithm.
// Regression test for US1: Ensures passwords are hashed with cryptographically secure bcrypt.
// Bug fixed: Replaced SHA256 with bcrypt for password hashing with proper salt.
func TestHashPassword_UsesBcrypt(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "simple password",
			password: "password123",
		},
		{
			name:     "complex password",
			password: "P@ssw0rd!ComplexWith$pecialChars",
		},
		{
			name:     "long password",
			password: strings.Repeat("a", 72), // bcrypt max is 72 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashPassword(tt.password)

			// Note: bcrypt can hash empty passwords (though in production this might be disallowed)
			if hash == "" {
				t.Errorf("hashPassword returned empty hash for password %q", tt.password)
				return
			}

			// Bcrypt hashes start with $2a$, $2b$, or $2y$ followed by cost parameter
			// Format: $2a$10$... (where 10 is the default cost)
			if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
				t.Errorf("Hash does not start with bcrypt identifier ($2a$, $2b$, or $2y$), got: %s", hash)
			}

			// Bcrypt hash should be exactly 60 characters for bcrypt with cost 10-31
			if len(hash) != 60 {
				t.Errorf("Hash length = %d, expected 60 (standard bcrypt hash length)", len(hash))
			}

			// Verify the hash can be verified by bcrypt
			err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(tt.password))
			if err != nil {
				t.Errorf("bcrypt.CompareHashAndPassword failed: %v (hash may not be valid bcrypt)", err)
			}
		})
	}
}

// TestHashPassword_GeneratesUniqueSalts tests that same password generates different hashes.
// Regression test for US1: Ensures bcrypt generates unique salts for each hash.
// Bug fixed: SHA256 produced identical hashes for same password; bcrypt generates unique salts.
func TestHashPassword_GeneratesUniqueSalts(t *testing.T) {
	password := "test-password-123"

	// Generate multiple hashes of the same password
	hash1 := hashPassword(password)
	hash2 := hashPassword(password)
	hash3 := hashPassword(password)

	// All should be valid bcrypt hashes
	if hash1 == "" || hash2 == "" || hash3 == "" {
		t.Fatal("One or more hashes are empty")
	}

	// All should be different (bcrypt uses random salts)
	if hash1 == hash2 {
		t.Errorf("hash1 == hash2, but bcrypt should generate unique salts")
	}
	if hash2 == hash3 {
		t.Errorf("hash2 == hash3, but bcrypt should generate unique salts")
	}
	if hash1 == hash3 {
		t.Errorf("hash1 == hash3, but bcrypt should generate unique salts")
	}

	// But all should verify against the same password
	err := bcrypt.CompareHashAndPassword([]byte(hash1), []byte(password))
	if err != nil {
		t.Errorf("hash1 failed verification: %v", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash2), []byte(password))
	if err != nil {
		t.Errorf("hash2 failed verification: %v", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash3), []byte(password))
	if err != nil {
		t.Errorf("hash3 failed verification: %v", err)
	}
}

// TestVerifyPassword_CorrectlyValidates tests verifyPassword function.
// Regression test for US1: Ensures password verification works correctly with bcrypt.
// Bug fixed: Verification now uses bcrypt.CompareHashAndPassword instead of string comparison.
func TestVerifyPassword_CorrectlyValidates(t *testing.T) {
	tests := []struct {
		name         string
		password     string
		testPassword string
		shouldMatch  bool
	}{
		{
			name:         "Correct password",
			password:     "correct-password",
			testPassword: "correct-password",
			shouldMatch:  true,
		},
		{
			name:         "Wrong password",
			password:     "correct-password",
			testPassword: "wrong-password",
			shouldMatch:  false,
		},
		{
			name:         "Case sensitive",
			password:     "Password123",
			testPassword: "password123",
			shouldMatch:  false,
		},
		{
			name:         "Empty password comparison",
			password:     "",
			testPassword: "",
			shouldMatch:  true, // bcrypt allows empty passwords
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Hash the original password
			hash := hashPassword(tt.password)

			if hash == "" {
				t.Fatalf("hashPassword returned empty hash for password %q", tt.password)
			}

			// Verify the test password
			result := verifyPassword(hash, tt.testPassword)

			if result != tt.shouldMatch {
				t.Errorf("verifyPassword(%q, %q) = %v, want %v",
					hash, tt.testPassword, result, tt.shouldMatch)
			}
		})
	}
}

// TestVerifyPassword_RejectsWrongPasswords tests that wrong passwords are always rejected.
// Regression test for US1: Prevents authentication bypass.
// Bug fixed: Ensures bcrypt comparison prevents timing attacks and brute force.
func TestVerifyPassword_RejectsWrongPasswords(t *testing.T) {
	correctPassword := "correct-password-123"
	hash := hashPassword(correctPassword)

	wrongPasswords := []string{
		"wrong-password",
		"correct-password-12",   // Missing last char
		"correct-password-1234", // Extra char
		"CORRECT-PASSWORD-123",  // Different case
		"",                      // Empty
		"x" + correctPassword,   // Prefix
		correctPassword + "x",   // Suffix
	}

	for _, wrong := range wrongPasswords {
		t.Run("reject_"+wrong, func(t *testing.T) {
			result := verifyPassword(hash, wrong)
			if result {
				t.Errorf("verifyPassword should reject wrong password %q, but it matched", wrong)
			}
		})
	}

	// Correct password should still work
	result := verifyPassword(hash, correctPassword)
	if !result {
		t.Errorf("verifyPassword should accept correct password, but it rejected it")
	}
}
