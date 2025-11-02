package stores

import (
	"context"
	"strings"
	"testing"

	ratelimit "github.com/itsatony/gorly"
)

// TestMemoryStore_KeyLengthValidation tests key length validation for memory store
func TestMemoryStore_KeyLengthValidation(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	// Test valid key (255 bytes - should pass)
	validKey := strings.Repeat("a", 255)
	err = store.Set(ctx, validKey, []byte("value"), 0)
	if err != nil {
		t.Errorf("Valid key (255 bytes) should not return error, got: %v", err)
	}

	// Test max valid key (256 bytes - should pass)
	maxValidKey := strings.Repeat("b", 256)
	err = store.Set(ctx, maxValidKey, []byte("value"), 0)
	if err != nil {
		t.Errorf("Max valid key (256 bytes) should not return error, got: %v", err)
	}

	// Test oversized key (257 bytes - should fail)
	oversizedKey := strings.Repeat("c", 257)
	err = store.Set(ctx, oversizedKey, []byte("value"), 0)
	if err == nil {
		t.Error("Oversized key (257 bytes) should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true, got error: %v", err)
	}

	// Test Get with oversized key
	_, err = store.Get(ctx, oversizedKey)
	if err == nil {
		t.Error("Get with oversized key should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true for Get, got error: %v", err)
	}

	// Test Increment with oversized key
	_, err = store.Increment(ctx, oversizedKey, 0)
	if err == nil {
		t.Error("Increment with oversized key should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true for Increment, got error: %v", err)
	}

	// Test Delete with oversized key
	err = store.Delete(ctx, oversizedKey)
	if err == nil {
		t.Error("Delete with oversized key should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true for Delete, got error: %v", err)
	}

	// Test Exists with oversized key
	_, err = store.Exists(ctx, oversizedKey)
	if err == nil {
		t.Error("Exists with oversized key should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true for Exists, got error: %v", err)
	}
}

// TestMemoryStore_KeyLengthBoundary tests boundary conditions
func TestMemoryStore_KeyLengthBoundary(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name      string
		keyLength int
		shouldErr bool
	}{
		{"empty key", 0, false},
		{"1 byte", 1, false},
		{"255 bytes", 255, false},
		{"256 bytes (max)", 256, false},
		{"257 bytes (over)", 257, true},
		{"500 bytes (way over)", 500, true},
		{"1000 bytes (way over)", 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := strings.Repeat("x", tt.keyLength)
			err := store.Set(ctx, key, []byte("value"), 0)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for key length %d", tt.keyLength)
				}
				if !ratelimit.IsKeyTooLong(err) {
					t.Errorf("Expected IsKeyTooLong to be true, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for key length %d: %v", tt.keyLength, err)
				}
			}
		})
	}
}

// TestRedisStore_KeyLengthValidation tests key length validation for Redis store
// This uses a mock-like approach - adjust if you have Redis integration tests
func TestRedisStore_KeyLengthValidation(t *testing.T) {
	// Skip if no Redis available - this would be part of integration tests
	t.Skip("Redis integration test - run with -tags=integration")

	ctx := context.Background()

	// This would require actual Redis connection
	config := DefaultRedisStoreConfig()
	config.Address = "localhost:6379"

	store, err := NewRedisStore(config)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer store.Close()

	// Test oversized key
	oversizedKey := strings.Repeat("d", 257)
	err = store.Set(ctx, oversizedKey, []byte("value"), 0)
	if err == nil {
		t.Error("Oversized key should return error for Redis store")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true, got error: %v", err)
	}

	// Test ExecuteScript with oversized keys
	keys := []string{
		strings.Repeat("e", 257),
		strings.Repeat("f", 300),
	}
	_, err = store.ExecuteScript(ctx, "return 1", keys)
	if err == nil {
		t.Error("ExecuteScript with oversized keys should return error")
	}
	if !ratelimit.IsKeyTooLong(err) {
		t.Errorf("Expected IsKeyTooLong to be true for ExecuteScript, got error: %v", err)
	}
}

// TestValidateKeyLength tests the validation function directly
func TestValidateKeyLength(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		shouldErr bool
	}{
		{"empty", "", false},
		{"single char", "a", false},
		{"normal key", "user:123:rate", false},
		{"255 bytes", strings.Repeat("a", 255), false},
		{"256 bytes (max)", strings.Repeat("b", 256), false},
		{"257 bytes", strings.Repeat("c", 257), true},
		{"1024 bytes", strings.Repeat("d", 1024), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ratelimit.ValidateKeyLength(tt.key)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for key length %d", len(tt.key))
				}
				if !ratelimit.IsKeyTooLong(err) {
					t.Errorf("Expected IsKeyTooLong to be true, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for key length %d: %v", len(tt.key), err)
				}
			}
		})
	}
}

// TestKeyLengthDOSPrevention verifies DOS attack prevention
func TestKeyLengthDOSPrevention(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	// Simulate DOS attack with extremely large keys
	attackSizes := []int{1024, 10240, 102400, 1048576} // 1KB, 10KB, 100KB, 1MB

	for _, size := range attackSizes {
		t.Run("attack_"+string(rune(size))+"_bytes", func(t *testing.T) {
			attackKey := strings.Repeat("ATTACK", size/6) // Create large key
			err := store.Set(ctx, attackKey, []byte("value"), 0)

			if err == nil {
				t.Errorf("DOS attack with %d byte key should be blocked", size)
			}

			if !ratelimit.IsKeyTooLong(err) {
				t.Errorf("Expected IsKeyTooLong error for DOS attack, got: %v", err)
			}
		})
	}
}
