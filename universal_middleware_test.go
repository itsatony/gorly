// universal_middleware_test.go - Tests for universal middleware system
package ratelimit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/itsatony/gorly/internal/middleware"
)

func TestUniversalMiddleware(t *testing.T) {
	// Create a simple rate limiter
	limiter := IPLimit("3/minute")

	// Test auto-detecting middleware
	autoMiddleware := limiter.Middleware()
	if autoMiddleware == nil {
		t.Fatal("Auto-detecting middleware should not be nil")
	}

	// Verify it's the right type
	if _, ok := autoMiddleware.(*middleware.UniversalMiddleware); !ok {
		t.Errorf("Expected *middleware.UniversalMiddleware, got %T", autoMiddleware)
	}

	t.Log("✅ Auto-detecting middleware working")
}

func TestFrameworkSpecificMiddleware(t *testing.T) {
	limiter := IPLimit("3/minute")

	tests := []struct {
		name      string
		framework middleware.FrameworkType
		expected  string
	}{
		{"Gin", Gin, "func(interface {})"},
		{"Echo", Echo, "func(interface {}) interface {}"},
		{"Fiber", Fiber, "func(interface {}) error"},
		{"Chi", Chi, "func(http.Handler) http.Handler"},
		{"HTTP", HTTP, "func(http.Handler) http.Handler"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := limiter.For(tt.framework)
			if mw == nil {
				t.Fatalf("Middleware for %s should not be nil", tt.name)
			}

			actualType := fmt.Sprintf("%T", mw)
			if actualType != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, actualType)
			}

			t.Logf("✅ %s middleware: %s", tt.name, actualType)
		})
	}
}

func TestHTTPMiddlewareIntegration(t *testing.T) {
	// Create rate limiter with a very low limit for testing
	limiter := IPLimit("2/minute")

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Get HTTP middleware
	middlewareFunc := limiter.For(HTTP).(func(http.Handler) http.Handler)
	wrappedHandler := middlewareFunc(handler)

	// Test multiple requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345" // Same IP for all requests
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if i < 2 {
			// First 2 requests should be allowed
			if rec.Code != http.StatusOK {
				t.Errorf("Request %d should be allowed, got status %d", i+1, rec.Code)
			}

			// Check rate limit headers
			if limit := rec.Header().Get("X-RateLimit-Limit"); limit == "" {
				t.Error("X-RateLimit-Limit header should be present")
			}
			if remaining := rec.Header().Get("X-RateLimit-Remaining"); remaining == "" {
				t.Error("X-RateLimit-Remaining header should be present")
			}

		} else {
			// 3rd request should be denied
			if rec.Code != http.StatusTooManyRequests {
				t.Errorf("Request %d should be denied with 429, got status %d", i+1, rec.Code)
			}

			// Check retry-after header
			if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
				t.Error("Retry-After header should be present for denied requests")
			}
		}
	}

	t.Log("✅ HTTP middleware integration working correctly")
}

func TestMiddlewareHeaders(t *testing.T) {
	limiter := IPLimit("5/minute")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	middlewareFunc := limiter.For(HTTP).(func(http.Handler) http.Handler)
	wrappedHandler := middlewareFunc(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.200:12345"
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Verify standard rate limit headers are present
	headers := []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Used",
		"X-RateLimit-Window",
	}

	for _, header := range headers {
		if value := rec.Header().Get(header); value == "" {
			t.Errorf("Header %s should be present", header)
		} else {
			t.Logf("✅ Header %s: %s", header, value)
		}
	}
}

func TestRateLimitContextValues(t *testing.T) {
	limiter := IPLimit("10/minute")

	var contextEntity, contextScope string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract context values set by middleware
		if entity := r.Context().Value("gorly_entity"); entity != nil {
			contextEntity = entity.(string)
		}
		if scope := r.Context().Value("gorly_scope"); scope != nil {
			contextScope = scope.(string)
		}
		w.Write([]byte("OK"))
	})

	middlewareFunc := limiter.For(HTTP).(func(http.Handler) http.Handler)
	wrappedHandler := middlewareFunc(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.300:12345"
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Verify context values were set
	if contextEntity == "" {
		t.Error("Context entity should be set")
	}
	if contextScope != "global" {
		t.Errorf("Expected scope 'global', got '%s'", contextScope)
	}

	t.Logf("✅ Context values: entity=%s, scope=%s", contextEntity, contextScope)
}
