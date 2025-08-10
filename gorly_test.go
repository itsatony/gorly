// gorly_test.go - Tests for the new simplified API
package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPLimit(t *testing.T) {
	// Create a simple IP-based rate limiter
	limiter := IPLimit("3/minute")

	// Test the limiter directly
	ctx := context.Background()
	entity := "192.168.1.1"

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, entity)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	allowed, err := limiter.Allow(ctx, entity)
	if err != nil {
		t.Fatalf("Request 4 failed: %v", err)
	}
	if allowed {
		t.Error("Request 4 should be denied")
	}
}

func TestAPIKeyLimit(t *testing.T) {
	// Create an API key-based rate limiter
	limiter := APIKeyLimit("5/minute")

	ctx := context.Background()
	entity := "key-123"

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, entity)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	allowed, err := limiter.Allow(ctx, entity)
	if err != nil {
		t.Fatalf("Request 6 failed: %v", err)
	}
	if allowed {
		t.Error("Request 6 should be denied")
	}
}

func TestFluentBuilder(t *testing.T) {
	// Test fluent builder pattern
	limiter := New().
		Memory().
		Algorithm("sliding_window").
		Limit("global", "100/hour").
		Limit("upload", "10/hour").
		TierLimits(map[string]string{
			"free":    "50/hour",
			"premium": "500/hour",
		}).
		EnableMetrics()

	// Verify the limiter was created
	if limiter == nil {
		t.Fatal("Limiter should not be nil")
	}

	// Test that we can build it
	built, err := limiter.Build()
	if err != nil {
		t.Fatalf("Failed to build limiter: %v", err)
	}

	if built == nil {
		t.Fatal("Built limiter should not be nil")
	}

	// Test health check
	ctx := context.Background()
	if err := built.Health(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestTierLimit(t *testing.T) {
	// Create a tier-based rate limiter
	limiter := TierLimit(map[string]string{
		"free":    "2/minute",
		"premium": "10/minute",
	})

	ctx := context.Background()

	// Test free tier user
	freeUser := "free:user123"
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, freeUser)
		if err != nil {
			t.Fatalf("Free user request %d failed: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("Free user request %d should be allowed", i+1)
		}
	}

	// 3rd request should be denied for free user
	allowed, err := limiter.Allow(ctx, freeUser)
	if err != nil {
		t.Fatalf("Free user request 3 failed: %v", err)
	}
	if allowed {
		t.Error("Free user request 3 should be denied")
	}

	// Test premium user should have higher limit
	premiumUser := "premium:user456"
	for i := 0; i < 5; i++ { // Should be able to make more than free tier
		allowed, err := limiter.Allow(ctx, premiumUser)
		if err != nil {
			t.Fatalf("Premium user request %d failed: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("Premium user request %d should be allowed", i+1)
		}
	}
}

func TestPresets(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
	}{
		{"APIGateway", APIGateway()},
		{"SaaSApp", SaaSApp()},
		{"PublicAPI", PublicAPI()},
		{"Microservice", Microservice()},
		{"WebApp", WebApp()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.builder == nil {
				t.Fatal("Builder should not be nil")
			}

			// Verify we can build the preset
			limiter, err := tt.builder.Build()
			if err != nil {
				t.Errorf("Failed to build %s preset: %v", tt.name, err)
				return
			}

			// Test that it works
			ctx := context.Background()
			allowed, err := limiter.Allow(ctx, "test-entity")
			if err != nil {
				t.Errorf("Failed to check rate limit for %s: %v", tt.name, err)
			}

			// First request should typically be allowed
			if !allowed {
				t.Errorf("First request should be allowed for %s", tt.name)
			}

			// Cleanup
			limiter.Close()
		})
	}
}

func TestHTTPMiddleware(t *testing.T) {
	// Create a simple rate limiter
	limiter := IPLimit("10/minute")

	// Get the middleware - this should work with any framework
	middleware := limiter.Middleware()

	// For testing, we'll check that middleware is returned
	if middleware == nil {
		t.Fatal("Middleware should not be nil")
	}

	// This tests our core rate limiting logic
	ctx := context.Background()
	allowed, err := limiter.Allow(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("Rate limit check failed: %v", err)
	}

	if !allowed {
		t.Error("First request should be allowed")
	}

	// Cleanup
	limiter.Close()
}

func TestEntityExtractors(t *testing.T) {
	tests := []struct {
		name      string
		request   *http.Request
		extractor func(*http.Request) string
		expected  string
	}{
		{
			name:      "ExtractIP",
			request:   createTestRequest("GET", "/", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}),
			extractor: extractIP,
			expected:  "1.2.3.4",
		},
		{
			name:      "ExtractAPIKey_Bearer",
			request:   createTestRequest("GET", "/", map[string]string{"Authorization": "Bearer abc123"}),
			extractor: extractAPIKey,
			expected:  "abc123",
		},
		{
			name:      "ExtractAPIKey_Header",
			request:   createTestRequest("GET", "/", map[string]string{"X-API-Key": "xyz789"}),
			extractor: extractAPIKey,
			expected:  "xyz789",
		},
		{
			name:      "ExtractUserID",
			request:   createTestRequest("GET", "/", map[string]string{"X-User-ID": "user123"}),
			extractor: extractUserID,
			expected:  "user123",
		},
		{
			name:      "ExtractTier",
			request:   createTestRequest("GET", "/", map[string]string{"X-User-Tier": "premium"}),
			extractor: extractTier,
			expected:  "premium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.extractor(tt.request)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestLimitParsing(t *testing.T) {
	tests := []struct {
		limit    string
		requests int64
		duration time.Duration
		hasError bool
	}{
		{"100/hour", 100, time.Hour, false},
		{"10/minute", 10, time.Minute, false},
		{"5/second", 5, time.Second, false},
		{"1000/day", 1000, 24 * time.Hour, false},
		{"50/30s", 50, 30 * time.Second, false},
		{"invalid", 0, 0, true},
		{"100", 0, 0, true},
		{"/hour", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.limit, func(t *testing.T) {
			// We need to access the internal parseLimit function
			// For now, let's test by creating a limiter and seeing if it works
			builder := New().Limit("test", tt.limit)
			_, err := builder.Build()

			if tt.hasError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// Helper function to create test requests
func createTestRequest(method, path string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.168.1.1:12345"

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req
}
