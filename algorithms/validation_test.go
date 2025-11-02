// algorithms/validation_test.go
package algorithms

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// VALIDATION FUNCTION TESTS
// ============================================================================

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name      string
		limit     int64
		wantError bool
	}{
		{
			name:      "valid limit - minimum",
			limit:     MinLimit,
			wantError: false,
		},
		{
			name:      "valid limit - normal",
			limit:     1000,
			wantError: false,
		},
		{
			name:      "valid limit - maximum",
			limit:     MaxLimit,
			wantError: false,
		},
		{
			name:      "invalid limit - zero",
			limit:     0,
			wantError: true,
		},
		{
			name:      "invalid limit - negative",
			limit:     -1,
			wantError: true,
		},
		{
			name:      "invalid limit - too large",
			limit:     MaxLimit + 1,
			wantError: true,
		},
		{
			name:      "invalid limit - max int64",
			limit:     math.MaxInt64,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLimit(tt.limit)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateLimit() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateWindow(t *testing.T) {
	tests := []struct {
		name      string
		window    time.Duration
		wantError bool
	}{
		{
			name:      "valid window - minimum",
			window:    MinWindow,
			wantError: false,
		},
		{
			name:      "valid window - normal",
			window:    time.Minute,
			wantError: false,
		},
		{
			name:      "valid window - maximum",
			window:    MaxWindow,
			wantError: false,
		},
		{
			name:      "invalid window - zero",
			window:    0,
			wantError: true,
		},
		{
			name:      "invalid window - negative",
			window:    -time.Second,
			wantError: true,
		},
		{
			name:      "invalid window - too small",
			window:    time.Millisecond,
			wantError: true,
		},
		{
			name:      "invalid window - too large",
			window:    MaxWindow + time.Second,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWindow(tt.window)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateWindow() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateRequestCount(t *testing.T) {
	tests := []struct {
		name      string
		n         int64
		wantError bool
	}{
		{
			name:      "valid count - one",
			n:         1,
			wantError: false,
		},
		{
			name:      "valid count - normal",
			n:         100,
			wantError: false,
		},
		{
			name:      "valid count - maximum",
			n:         MaxRequestCount,
			wantError: false,
		},
		{
			name:      "invalid count - zero",
			n:         0,
			wantError: true,
		},
		{
			name:      "invalid count - negative",
			n:         -1,
			wantError: true,
		},
		{
			name:      "invalid count - too large",
			n:         MaxRequestCount + 1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestCount(tt.n)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateRequestCount() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateBurst(t *testing.T) {
	tests := []struct {
		name      string
		burst     int64
		limit     int64
		wantError bool
	}{
		{
			name:      "valid burst - zero",
			burst:     0,
			limit:     100,
			wantError: false,
		},
		{
			name:      "valid burst - equal to limit",
			burst:     100,
			limit:     100,
			wantError: false,
		},
		{
			name:      "valid burst - 10x limit",
			burst:     1000,
			limit:     100,
			wantError: false,
		},
		{
			name:      "invalid burst - negative",
			burst:     -1,
			limit:     100,
			wantError: true,
		},
		{
			name:      "invalid burst - too large",
			burst:     100000,
			limit:     10,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBurst(tt.burst, tt.limit)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateBurst() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantError bool
	}{
		{
			name:      "valid key - short",
			key:       "user123",
			wantError: false,
		},
		{
			name:      "valid key - with colons",
			key:       "user:123:api",
			wantError: false,
		},
		{
			name:      "valid key - maximum length",
			key:       strings.Repeat("a", 256),
			wantError: false,
		},
		{
			name:      "invalid key - empty",
			key:       "",
			wantError: true,
		},
		{
			name:      "invalid key - too long",
			key:       strings.Repeat("a", 257),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateKey() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateAllowInputs(t *testing.T) {
	tests := []struct {
		name      string
		limit     int64
		window    time.Duration
		n         int64
		wantError bool
	}{
		{
			name:      "valid inputs - normal",
			limit:     100,
			window:    time.Minute,
			n:         1,
			wantError: false,
		},
		{
			name:      "valid inputs - n equals limit",
			limit:     100,
			window:    time.Minute,
			n:         100,
			wantError: false,
		},
		{
			name:      "invalid inputs - n exceeds limit",
			limit:     100,
			window:    time.Minute,
			n:         101,
			wantError: true,
		},
		{
			name:      "invalid inputs - limit too low",
			limit:     0,
			window:    time.Minute,
			n:         1,
			wantError: true,
		},
		{
			name:      "invalid inputs - window too small",
			limit:     100,
			window:    time.Millisecond,
			n:         1,
			wantError: true,
		},
		{
			name:      "invalid inputs - n zero",
			limit:     100,
			window:    time.Minute,
			n:         0,
			wantError: true,
		},
		{
			name:      "valid inputs - large but safe",
			limit:     10000,
			window:    time.Hour,
			n:         1,
			wantError: false,
		},
		{
			name:      "invalid inputs - overflow risk",
			limit:     MaxLimit,
			window:    MaxWindow,
			n:         1,
			wantError: true, // Correctly detects overflow risk with max values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllowInputs(tt.limit, tt.window, tt.n)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateAllowInputs() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// ============================================================================
// ALGORITHM VALIDATION INTEGRATION TESTS
// ============================================================================

func TestTokenBucketAlgorithm_ValidationErrors(t *testing.T) {
	store := &mockStore{}
	algo := NewTokenBucketAlgorithm()
	ctx := context.Background()

	tests := []struct {
		name    string
		key     string
		limit   int64
		window  time.Duration
		n       int64
		wantErr string
	}{
		{
			name:    "empty key",
			key:     "",
			limit:   100,
			window:  time.Minute,
			n:       1,
			wantErr: "key cannot be empty",
		},
		{
			name:    "limit too low",
			key:     "test",
			limit:   0,
			window:  time.Minute,
			n:       1,
			wantErr: "limit must be at least",
		},
		{
			name:    "limit too high",
			key:     "test",
			limit:   MaxLimit + 1,
			window:  time.Minute,
			n:       1,
			wantErr: "limit must not exceed",
		},
		{
			name:    "window too small",
			key:     "test",
			limit:   100,
			window:  time.Millisecond,
			n:       1,
			wantErr: "window must be at least",
		},
		{
			name:    "window too large",
			key:     "test",
			limit:   100,
			window:  MaxWindow + time.Second,
			n:       1,
			wantErr: "window must not exceed",
		},
		{
			name:    "n zero",
			key:     "test",
			limit:   100,
			window:  time.Minute,
			n:       0,
			wantErr: "request count must be positive",
		},
		{
			name:    "n negative",
			key:     "test",
			limit:   100,
			window:  time.Minute,
			n:       -1,
			wantErr: "request count must be positive",
		},
		{
			name:    "n exceeds limit",
			key:     "test",
			limit:   100,
			window:  time.Minute,
			n:       101,
			wantErr: "request count (101) cannot exceed limit (100)",
		},
		{
			name:    "n too large",
			key:     "test",
			limit:   MaxRequestCount,
			window:  time.Minute,
			n:       MaxRequestCount + 1,
			wantErr: "request count must not exceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := algo.Allow(ctx, store, tt.key, tt.limit, tt.window, tt.n)
			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestSlidingWindowAlgorithm_ValidationErrors(t *testing.T) {
	store := &mockStore{}
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	tests := []struct {
		name    string
		key     string
		limit   int64
		window  time.Duration
		n       int64
		wantErr string
	}{
		{
			name:    "empty key",
			key:     "",
			limit:   100,
			window:  time.Minute,
			n:       1,
			wantErr: "key cannot be empty",
		},
		{
			name:    "limit negative",
			key:     "test",
			limit:   -1,
			window:  time.Minute,
			n:       1,
			wantErr: "limit must be at least",
		},
		{
			name:    "n exceeds limit",
			key:     "test",
			limit:   50,
			window:  time.Minute,
			n:       51,
			wantErr: "request count (51) cannot exceed limit (50)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := algo.Allow(ctx, store, tt.key, tt.limit, tt.window, tt.n)
			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// ============================================================================
// OVERFLOW PREVENTION TESTS
// ============================================================================

func TestValidation_OverflowPrevention(t *testing.T) {
	tests := []struct {
		name      string
		limit     int64
		window    time.Duration
		wantError bool
		reason    string
	}{
		{
			name:      "safe values",
			limit:     1000,
			window:    time.Hour,
			wantError: false,
			reason:    "Normal values should pass",
		},
		{
			name:      "large safe values",
			limit:     10000,
			window:    24 * time.Hour,
			wantError: false,
			reason:    "Large but safe values should pass",
		},
		{
			name:      "max values cause overflow",
			limit:     MaxLimit,
			window:    MaxWindow,
			wantError: true,
			reason:    "Maximum limit and window together cause overflow",
		},
		{
			name:      "prevent limit overflow",
			limit:     math.MaxInt64,
			window:    time.Second,
			wantError: true,
			reason:    "Excessive limit should be rejected",
		},
		{
			name:      "prevent window overflow",
			limit:     100,
			window:    time.Duration(math.MaxInt64),
			wantError: true,
			reason:    "Excessive window should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllowInputs(tt.limit, tt.window, 1)
			if (err != nil) != tt.wantError {
				t.Errorf("%s: ValidateAllowInputs() error = %v, wantError %v", tt.reason, err, tt.wantError)
			}
		})
	}
}

// ============================================================================
// P1-1 KEY LENGTH VALIDATION TESTS
// Tests to prevent memory DOS attacks via oversized keys
// ============================================================================

// TestValidateKey_P1_1_KeyLengthLimits verifies P1-1 fix:
// Key length must be limited to MaxKeyLength (256 bytes) to prevent memory DOS
func TestValidateKey_P1_1_KeyLengthLimits(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
		description string
	}{
		{
			name:        "empty key",
			key:         "",
			expectError: true,
			description: "Empty keys should be rejected",
		},
		{
			name:        "short valid key",
			key:         "user:123:api",
			expectError: false,
			description: "Short keys should be accepted",
		},
		{
			name:        "exactly 256 bytes",
			key:         string(make([]byte, 256)),
			expectError: false,
			description: "Keys exactly at MaxKeyLength should be accepted",
		},
		{
			name:        "257 bytes - just over limit",
			key:         string(make([]byte, 257)),
			expectError: true,
			description: "P1-1: Keys exceeding MaxKeyLength by 1 byte should be rejected",
		},
		{
			name:        "512 bytes - double limit",
			key:         string(make([]byte, 512)),
			expectError: true,
			description: "P1-1: Keys double the limit should be rejected",
		},
		{
			name:        "1024 bytes - old limit",
			key:         string(make([]byte, 1024)),
			expectError: true,
			description: "P1-1: Keys at old limit (1024) should now be rejected",
		},
		{
			name:        "10KB - DOS attempt",
			key:         string(make([]byte, 10240)),
			expectError: true,
			description: "P1-1: Large keys (DOS attempt) should be rejected",
		},
		{
			name:        "1MB - extreme DOS attempt",
			key:         string(make([]byte, 1048576)),
			expectError: true,
			description: "P1-1: Extremely large keys should be rejected",
		},
		{
			name:        "typical rate limit key",
			key:         "ratelimit:user:abc123:api:2024-11-02",
			expectError: false,
			description: "Typical rate limit keys should be accepted",
		},
		{
			name:        "complex key with metadata",
			key:         "gorly:tenant:acme-corp:user:john-doe:scope:global:tier:premium:timestamp:1699000000",
			expectError: false,
			description: "Complex keys with metadata should be accepted if under limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)

			if tt.expectError && err == nil {
				t.Errorf("P1-1 FAILURE: Expected error for key of length %d bytes, but got nil - %s",
					len(tt.key), tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for key of length %d bytes, but got: %v - %s",
					len(tt.key), err, tt.description)
			}

			// P1-1: Verify error message mentions memory DOS prevention
			if tt.expectError && err != nil && len(tt.key) > MaxKeyLength {
				errMsg := err.Error()
				if !containsAny(errMsg, []string{"memory DOS", "prevents memory", "MaxKeyLength"}) {
					t.Errorf("P1-1: Error message should explain DOS prevention, got: %s", errMsg)
				}
			}
		})
	}
}

// TestValidateKey_P1_1_MultibyteCharacters verifies that key length
// is measured in bytes, not Unicode characters
func TestValidateKey_P1_1_MultibyteCharacters(t *testing.T) {
	// Create strings with multibyte UTF-8 characters
	// "日" = 3 bytes each in UTF-8
	utf8Char := "日"

	tests := []struct {
		name        string
		key         string
		expectError bool
		description string
	}{
		{
			name:        "ASCII characters at limit",
			key:         strings.Repeat("a", 256), // 256 bytes of ASCII
			expectError: false,
			description: "256 ASCII characters = 256 bytes, should be accepted",
		},
		{
			name:        "UTF-8 at limit",
			key:         strings.Repeat(utf8Char, 85), // 85 * 3 bytes = 255 bytes
			expectError: false,
			description: "85 UTF-8 characters (255 bytes total) should be accepted",
		},
		{
			name:        "UTF-8 just over limit",
			key:         strings.Repeat(utf8Char, 86), // 86 * 3 bytes = 258 bytes
			expectError: true,
			description: "P1-1: 86 UTF-8 characters (258 bytes total) should be rejected",
		},
		{
			name:        "UTF-8 far over limit",
			key:         strings.Repeat(utf8Char, 200), // 200 * 3 bytes = 600 bytes
			expectError: true,
			description: "P1-1: 200 UTF-8 characters (600 bytes total) should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			keyBytes := len(tt.key)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for key with %d bytes, but got nil - %s",
					keyBytes, tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for key with %d bytes, but got: %v - %s",
					keyBytes, err, tt.description)
			}
		})
	}
}

// TestValidateKey_P1_1_MemoryDOSPrevention simulates a memory DOS attack
// by attempting to validate many oversized keys
func TestValidateKey_P1_1_MemoryDOSPrevention(t *testing.T) {
	// Simulate attacker trying to exhaust memory with large keys
	attackKeys := []string{
		string(make([]byte, 1024)),     // 1 KB
		string(make([]byte, 10240)),    // 10 KB
		string(make([]byte, 102400)),   // 100 KB
		string(make([]byte, 1048576)),  // 1 MB
		string(make([]byte, 10485760)), // 10 MB (extreme)
	}

	for i, key := range attackKeys {
		t.Run(fmt.Sprintf("DOS_attempt_%d_bytes", len(key)), func(t *testing.T) {
			err := ValidateKey(key)

			// P1-1: All oversized keys should be rejected BEFORE processing
			if err == nil {
				t.Errorf("P1-1 FAILURE: DOS attack key of %d bytes was not rejected", len(key))
			}

			// Verify rejection happens quickly (validation, not processing)
			// The validation should be O(1) - just checking length
			start := time.Now()
			for j := 0; j < 1000; j++ {
				ValidateKey(key)
			}
			elapsed := time.Since(start)

			// 1000 validations should complete in well under 100ms
			if elapsed > 100*time.Millisecond {
				t.Errorf("Validation too slow for attack key %d: %v (expected <100ms for 1000 ops)",
					i, elapsed)
			}
		})
	}
}

// TestValidateKey_P1_1_ConstantConsistency verifies that MaxKeyLength
// constant is consistent with parent package
func TestValidateKey_P1_1_ConstantConsistency(t *testing.T) {
	// This test documents the expected constant value
	// If this fails, the parent package constant may have changed
	expectedMaxKeyLength := 256

	if MaxKeyLength != expectedMaxKeyLength {
		t.Errorf("P1-1: MaxKeyLength constant mismatch: got %d, expected %d (should match parent package)",
			MaxKeyLength, expectedMaxKeyLength)
	}

	// Verify the constant is actually enforced
	justOverLimit := string(make([]byte, MaxKeyLength+1))
	err := ValidateKey(justOverLimit)

	if err == nil {
		t.Errorf("P1-1 FAILURE: Key of length MaxKeyLength+1 (%d bytes) was not rejected",
			MaxKeyLength+1)
	}

	// Verify keys at the limit are accepted
	atLimit := string(make([]byte, MaxKeyLength))
	err = ValidateKey(atLimit)

	if err != nil {
		t.Errorf("Key at exactly MaxKeyLength (%d bytes) should be accepted, got error: %v",
			MaxKeyLength, err)
	}
}

// Helper function to check if string contains any of the given substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(substr) > 0 && len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
