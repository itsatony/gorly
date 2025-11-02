// middleware/http_test.go
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itsatony/gorly"
	"github.com/itsatony/gorly/stores"
)

// ============================================================================
// TEST FIXTURES AND HELPERS
// ============================================================================

// mockLimiter implements RateLimiter for testing
type mockLimiter struct {
	checkFunc func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error)
	allowFunc func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error)
	resetFunc func(ctx context.Context, rlCtx ratelimit.Identity) error
	statsFunc func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error)
	closeFunc func() error
}

func (m *mockLimiter) Check(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
	if m.checkFunc != nil {
		return m.checkFunc(ctx, rlCtx)
	}
	return &ratelimit.Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		Used:      0, // Check doesn't consume
		ResetAt:   time.Now().Add(time.Hour),
		Window:    time.Hour,
	}, nil
}

func (m *mockLimiter) Allow(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
	if m.allowFunc != nil {
		return m.allowFunc(ctx, rlCtx)
	}
	return &ratelimit.Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		Used:      1,
		ResetAt:   time.Now().Add(time.Hour),
		Window:    time.Hour,
	}, nil
}

func (m *mockLimiter) AllowN(ctx context.Context, rlCtx ratelimit.Identity, n int64) (*ratelimit.Result, error) {
	return m.Allow(ctx, rlCtx)
}

func (m *mockLimiter) Reset(ctx context.Context, rlCtx ratelimit.Identity) error {
	if m.resetFunc != nil {
		return m.resetFunc(ctx, rlCtx)
	}
	return nil
}

func (m *mockLimiter) Stats(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, rlCtx)
	}
	return &ratelimit.Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 50,
		Used:      50,
	}, nil
}

func (m *mockLimiter) Health(ctx context.Context) error {
	return nil // Always healthy for testing
}

func (m *mockLimiter) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// createTestLimiter creates a real rate limiter for integration tests
func createTestLimiter(t *testing.T, limit int64, window time.Duration) ratelimit.RateLimiter {
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}

	limiter, err := ratelimit.NewSimple(store, limit, window)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	return limiter
}

// testHandler returns a simple test handler
func testHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
}

// ============================================================================
// CONSTRUCTOR TESTS
// ============================================================================

func TestNewHTTPMiddleware_Success(t *testing.T) {
	limiter := &mockLimiter{}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
	}

	middleware, err := NewHTTPMiddleware(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if middleware == nil {
		t.Fatal("Expected middleware to be created")
	}

	// Verify limiter was set (can't directly compare due to interface)
	if middleware.limiter == nil {
		t.Error("Limiter not set")
	}

	if middleware.extractor == nil {
		t.Error("Extractor should default to IP extractor")
	}
}

func TestNewHTTPMiddleware_NilConfig(t *testing.T) {
	_, err := NewHTTPMiddleware(nil)
	if err == nil {
		t.Fatal("Expected error for nil config")
	}

	if !strings.Contains(err.Error(), "config is required") {
		t.Errorf("Expected config required error, got: %v", err)
	}
}

func TestNewHTTPMiddleware_NilLimiter(t *testing.T) {
	config := &HTTPMiddlewareConfig{}

	_, err := NewHTTPMiddleware(config)
	if err == nil {
		t.Fatal("Expected error for nil limiter")
	}

	if !strings.Contains(err.Error(), "limiter is required") {
		t.Errorf("Expected limiter required error, got: %v", err)
	}
}

func TestNewHTTPMiddleware_DefaultsSet(t *testing.T) {
	limiter := &mockLimiter{}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
	}

	middleware, err := NewHTTPMiddleware(config)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify defaults were set
	if config.ContextExtractor == nil {
		t.Error("Default context extractor not set")
	}

	if config.ErrorHandler == nil {
		t.Error("Default error handler not set")
	}

	if middleware.extractor == nil {
		t.Error("Extractor not initialized from config")
	}
}

// ============================================================================
// MIDDLEWARE EXECUTION TESTS
// ============================================================================

func TestMiddleware_AllowedRequest(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{
				Allowed:   true,
				Limit:     100,
				Remaining: 99,
				Used:      1,
				ResetAt:   time.Now().Add(time.Hour),
				Window:    time.Hour,
			}, nil
		},
	}

	config := &HTTPMiddlewareConfig{
		Limiter:    limiter,
		AddHeaders: true,
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got %s", w.Body.String())
	}

	// Check headers
	if w.Header().Get("X-RateLimit-Limit") != "100" {
		t.Errorf("Expected X-RateLimit-Limit header to be '100', got %s", w.Header().Get("X-RateLimit-Limit"))
	}

	if w.Header().Get("X-RateLimit-Remaining") != "99" {
		t.Errorf("Expected X-RateLimit-Remaining header to be '99', got %s", w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestMiddleware_DeniedRequest(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{
				Allowed:    false,
				Limit:      100,
				Remaining:  0,
				Used:       100,
				ResetAt:    time.Now().Add(time.Hour),
				Window:     time.Hour,
				RetryAfter: 30 * time.Second,
			}, nil
		},
	}

	config := &HTTPMiddlewareConfig{
		Limiter:    limiter,
		AddHeaders: true,
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}

	// Check Retry-After header
	if w.Header().Get("Retry-After") != "30" {
		t.Errorf("Expected Retry-After header to be '30', got %s", w.Header().Get("Retry-After"))
	}

	// Check JSON response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["error"] != "Rate limit exceeded" {
		t.Errorf("Expected error message, got %v", response["error"])
	}
}

func TestMiddleware_CustomResponse(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{
				Allowed:   false,
				Limit:     100,
				Remaining: 0,
			}, nil
		},
	}

	customBody := map[string]string{
		"message": "Too many requests, please slow down",
		"code":    "RATE_LIMIT_EXCEEDED",
	}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		CustomResponse: &HTTPRateLimitResponse{
			StatusCode: http.StatusServiceUnavailable,
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
			},
			Body: customBody,
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	if w.Header().Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header not set")
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["message"] != customBody["message"] {
		t.Errorf("Expected custom message, got %v", response["message"])
	}
}

func TestMiddleware_CustomResponseByteBody(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{Allowed: false}, nil
		},
	}

	customBody := []byte("Rate limit exceeded - plain text")

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		CustomResponse: &HTTPRateLimitResponse{
			StatusCode: http.StatusTooManyRequests,
			Body:       customBody,
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Body.String() != string(customBody) {
		t.Errorf("Expected custom byte body, got %s", w.Body.String())
	}
}

// TestHTTPMiddleware_CustomResponse_IncludesHeaders verifies P0-2 fix:
// Standard rate limit headers MUST be included even when using custom responses.
// This is an API contract requirement - clients expect these headers regardless of custom response.
func TestHTTPMiddleware_CustomResponse_IncludesHeaders(t *testing.T) {
	resetTime := time.Now().Add(time.Hour)
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{
				Allowed:    false,
				Limit:      1000,
				Remaining:  0,
				Used:       1000,
				ResetAt:    resetTime,
				Window:     time.Hour,
				RetryAfter: 30 * time.Second,
			}, nil
		},
	}

	customBody := map[string]string{
		"error":   "Custom rate limit message",
		"details": "Please upgrade your plan",
	}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		CustomResponse: &HTTPRateLimitResponse{
			StatusCode: http.StatusPaymentRequired, // 402 - custom status
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
				"X-Upgrade-URL":   "https://example.com/upgrade",
			},
			Body: customBody,
		},
	}

	middleware, err := NewHTTPMiddleware(config)
	if err != nil {
		t.Fatalf("Failed to create middleware: %v", err)
	}

	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify custom status code
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("Expected custom status 402, got %d", w.Code)
	}

	// Verify custom headers are present
	if w.Header().Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header X-Custom-Header not set correctly")
	}

	if w.Header().Get("X-Upgrade-URL") != "https://example.com/upgrade" {
		t.Error("Custom header X-Upgrade-URL not set correctly")
	}

	// P0-2 FIX VERIFICATION: Standard rate limit headers MUST be present
	// These headers are required by the API contract regardless of custom response
	if w.Header().Get("X-RateLimit-Limit") != "1000" {
		t.Errorf("P0-2 FAILURE: Standard header X-RateLimit-Limit missing or incorrect, got: %s",
			w.Header().Get("X-RateLimit-Limit"))
	}

	if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("P0-2 FAILURE: Standard header X-RateLimit-Remaining missing or incorrect, got: %s",
			w.Header().Get("X-RateLimit-Remaining"))
	}

	expectedReset := strconv.FormatInt(resetTime.Unix(), 10)
	if w.Header().Get("X-RateLimit-Reset") != expectedReset {
		t.Errorf("P0-2 FAILURE: Standard header X-RateLimit-Reset missing or incorrect, got: %s, expected: %s",
			w.Header().Get("X-RateLimit-Reset"), expectedReset)
	}

	if w.Header().Get("Retry-After") != "30" {
		t.Errorf("P0-2 FAILURE: Standard header Retry-After missing or incorrect, got: %s",
			w.Header().Get("Retry-After"))
	}

	// Verify custom body is present
	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode custom response body: %v", err)
	}

	if response["error"] != customBody["error"] {
		t.Errorf("Custom body not preserved, got: %v", response["error"])
	}

	if response["details"] != customBody["details"] {
		t.Errorf("Custom body not preserved, got: %v", response["details"])
	}
}

// TestHTTPMiddleware_CustomResponse_HeadersWithoutCustomHeaders verifies that
// standard rate limit headers are present even when NO custom headers are specified
func TestHTTPMiddleware_CustomResponse_HeadersWithoutCustomHeaders(t *testing.T) {
	resetTime := time.Now().Add(time.Minute)
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return &ratelimit.Result{
				Allowed:    false,
				Limit:      100,
				Remaining:  5,
				Used:       95,
				ResetAt:    resetTime,
				Window:     time.Minute,
				RetryAfter: 10 * time.Second,
			}, nil
		},
	}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		CustomResponse: &HTTPRateLimitResponse{
			StatusCode: http.StatusTooManyRequests,
			Headers:    nil, // No custom headers
			Body:       "Rate limit exceeded",
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify all standard headers are present
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("Standard header X-RateLimit-Limit is missing")
	}

	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("Standard header X-RateLimit-Remaining is missing")
	}

	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("Standard header X-RateLimit-Reset is missing")
	}

	if w.Header().Get("Retry-After") == "" {
		t.Error("Standard header Retry-After is missing")
	}
}

func TestMiddleware_SkipPath(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			// This should not be called for skipped paths
			t.Error("Limiter should not be called for skipped path")
			return nil, fmt.Errorf("should not be called")
		},
	}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		SkipPaths: []string{
			"/health",
			"/metrics",
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	tests := []struct {
		path       string
		shouldSkip bool
	}{
		{"/health", true},
		{"/metrics", true},
		{"/health/detailed", true}, // Prefix match
		{"/api/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// Reset limiter function for non-skipped paths
			if !tt.shouldSkip {
				limiter.allowFunc = func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
					return &ratelimit.Result{Allowed: true, Limit: 100, Remaining: 99}, nil
				}
			} else {
				limiter.allowFunc = func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
					t.Error("Limiter should not be called for skipped path")
					return nil, fmt.Errorf("should not be called")
				}
			}

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestMiddleware_ExtractorError(t *testing.T) {
	limiter := &mockLimiter{}

	errorHandlerCalled := false
	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		ContextExtractor: func(r *http.Request) (ratelimit.Identity, error) {
			return nil, fmt.Errorf("extraction failed")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result) {
			errorHandlerCalled = true
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !errorHandlerCalled {
		t.Error("Error handler should have been called")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestMiddleware_LimiterError(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, rlCtx ratelimit.Identity) (*ratelimit.Result, error) {
			return nil, fmt.Errorf("limiter error")
		},
	}

	errorHandlerCalled := false
	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result) {
			errorHandlerCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"limiter error"}`))
		},
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !errorHandlerCalled {
		t.Error("Error handler should have been called")
	}

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestMiddleware_WithoutHeaders(t *testing.T) {
	limiter := &mockLimiter{}

	config := &HTTPMiddlewareConfig{
		Limiter:    limiter,
		AddHeaders: false, // Disable headers
	}

	middleware, _ := NewHTTPMiddleware(config)
	handler := middleware.Middleware(testHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Headers should not be present
	if w.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("X-RateLimit-Limit header should not be set")
	}
}

func TestMiddlewareFunc(t *testing.T) {
	limiter := &mockLimiter{}

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
	}

	middleware, _ := NewHTTPMiddleware(config)

	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Handler called"))
	}

	wrappedFunc := middleware.MiddlewareFunc(handlerFunc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	wrappedFunc(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "Handler called" {
		t.Errorf("Expected handler to be called, got %s", w.Body.String())
	}
}

// ============================================================================
// CONTEXT EXTRACTOR TESTS
// ============================================================================

func TestDefaultIPContextExtractor_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	ctx, err := DefaultIPContextExtractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Identity() != "192.168.1.1" {
		t.Errorf("Expected IP 192.168.1.1, got %s", ctx.Identity())
	}
}

func TestDefaultIPContextExtractor_NoIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "" // No IP

	_, err := DefaultIPContextExtractor(req)
	if err == nil {
		t.Fatal("Expected error for missing IP")
	}

	// SECURITY: Verify we use generic error message, not internal details
	if !strings.Contains(err.Error(), "unable to process request") {
		t.Errorf("Expected generic error message, got: %v", err)
	}

	// SECURITY: Verify we DON'T leak internal implementation details
	if strings.Contains(err.Error(), "client IP") || strings.Contains(err.Error(), "IP address") {
		t.Errorf("Error message leaks internal implementation details: %v", err)
	}
}

func TestAPIKeyContextExtractor_Success(t *testing.T) {
	getTier := func(apiKey string) string {
		if apiKey == "premium-key" {
			return ratelimit.TierPremium
		}
		return ratelimit.TierFree
	}

	extractor := APIKeyContextExtractor("X-API-Key", getTier)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "premium-key")

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Identity() != "premium-key" {
		t.Errorf("Expected identity 'premium-key', got %s", ctx.Identity())
	}

	if ctx.Tier() != ratelimit.TierPremium {
		t.Errorf("Expected tier premium, got %s", ctx.Tier())
	}
}

func TestAPIKeyContextExtractor_MissingKey(t *testing.T) {
	extractor := APIKeyContextExtractor("X-API-Key", nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No API key header set

	_, err := extractor(req)
	if err == nil {
		t.Fatal("Expected error for missing API key")
	}

	// SECURITY: Verify we use generic error message, not internal details
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected generic authentication error, got: %v", err)
	}

	// SECURITY: Verify we DON'T leak which header we're looking for
	if strings.Contains(err.Error(), "X-API-Key") || strings.Contains(err.Error(), "header") {
		t.Errorf("Error message leaks internal implementation details: %v", err)
	}
}

func TestAPIKeyContextExtractor_DefaultTier(t *testing.T) {
	extractor := APIKeyContextExtractor("X-API-Key", nil) // No tier function

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "test-key")

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Tier() != ratelimit.TierFree {
		t.Errorf("Expected default tier to be free, got %s", ctx.Tier())
	}
}

// testUser implements user interface for testing
type testUser struct {
	id   string
	tier string
}

func (u *testUser) ID() string {
	return u.id
}

func (u *testUser) Tier() string {
	return u.tier
}

func TestUserContextExtractor_WithInterface(t *testing.T) {
	user := &testUser{id: "user123", tier: ratelimit.TierEnterprise}

	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), "user", user)
	req = req.WithContext(ctx)

	rlCtx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rlCtx.Identity() != "user123" {
		t.Errorf("Expected identity 'user123', got %s", rlCtx.Identity())
	}

	if rlCtx.Tier() != ratelimit.TierEnterprise {
		t.Errorf("Expected tier enterprise, got %s", rlCtx.Tier())
	}
}

func TestUserContextExtractor_WithMap(t *testing.T) {
	userMap := map[string]interface{}{
		"id":   "user456",
		"tier": ratelimit.TierPremium,
	}

	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), "user", userMap)
	req = req.WithContext(ctx)

	rlCtx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rlCtx.Identity() != "user456" {
		t.Errorf("Expected identity 'user456', got %s", rlCtx.Identity())
	}

	if rlCtx.Tier() != ratelimit.TierPremium {
		t.Errorf("Expected tier premium, got %s", rlCtx.Tier())
	}
}

func TestUserContextExtractor_MapDefaultTier(t *testing.T) {
	userMap := map[string]interface{}{
		"id": "user789",
		// No tier provided
	}

	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), "user", userMap)
	req = req.WithContext(ctx)

	rlCtx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rlCtx.Tier() != ratelimit.TierFree {
		t.Errorf("Expected default tier to be free, got %s", rlCtx.Tier())
	}
}

func TestUserContextExtractor_MissingUser(t *testing.T) {
	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No user in context

	_, err := extractor(req)
	if err == nil {
		t.Fatal("Expected error for missing user")
	}

	// SECURITY: Verify we use generic error message, not internal details
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected generic authentication error, got: %v", err)
	}

	// SECURITY: Verify we DON'T leak that we're looking for "user information in context"
	if strings.Contains(err.Error(), "user") || strings.Contains(err.Error(), "context") {
		t.Errorf("Error message leaks internal implementation details: %v", err)
	}
}

func TestUserContextExtractor_MapNoID(t *testing.T) {
	userMap := map[string]interface{}{
		"tier": ratelimit.TierPremium,
		// No ID
	}

	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), "user", userMap)
	req = req.WithContext(ctx)

	_, err := extractor(req)
	if err == nil {
		t.Fatal("Expected error for missing user ID")
	}

	// SECURITY: Verify we use generic error message, not internal details
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected generic authentication error, got: %v", err)
	}

	// SECURITY: Verify we DON'T leak that we're checking for "user ID"
	if strings.Contains(err.Error(), "user ID") || strings.Contains(err.Error(), "ID") {
		t.Errorf("Error message leaks internal implementation details: %v", err)
	}
}

func TestUserContextExtractor_InvalidFormat(t *testing.T) {
	extractor := UserContextExtractor("user")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), "user", "invalid-string") // Invalid format
	req = req.WithContext(ctx)

	_, err := extractor(req)
	if err == nil {
		t.Fatal("Expected error for invalid format")
	}

	// SECURITY: Verify we use generic error message
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected generic authentication error, got: %v", err)
	}

	// SECURITY: Verify we DON'T leak implementation details about expected format
	if strings.Contains(err.Error(), "format") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "string") {
		t.Errorf("Error message leaks internal implementation details: %v", err)
	}
}

func TestPathScopeContextExtractor_ExactMatch(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	pathMappings := map[string]string{
		"/api/search": ratelimit.ScopeSearch,
		"/api/write":  ratelimit.ScopeDatabase,
	}

	extractor := PathScopeContextExtractor(pathMappings, baseExtractor)

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Scope() != ratelimit.ScopeSearch {
		t.Errorf("Expected scope 'search', got %s", ctx.Scope())
	}
}

func TestPathScopeContextExtractor_PrefixMatch(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	pathMappings := map[string]string{
		"/api/admin": ratelimit.ScopeAdmin,
	}

	extractor := PathScopeContextExtractor(pathMappings, baseExtractor)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Scope() != ratelimit.ScopeAdmin {
		t.Errorf("Expected scope 'admin', got %s", ctx.Scope())
	}
}

func TestPathScopeContextExtractor_DefaultScope(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	pathMappings := map[string]string{
		"/api/search": ratelimit.ScopeSearch,
	}

	extractor := PathScopeContextExtractor(pathMappings, baseExtractor)

	req := httptest.NewRequest(http.MethodGet, "/api/other", nil)

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Scope() != ratelimit.ScopeGlobal {
		t.Errorf("Expected default scope 'global', got %s", ctx.Scope())
	}
}

func TestPathScopeContextExtractor_BaseExtractorError(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return nil, fmt.Errorf("base extractor error")
	}

	extractor := PathScopeContextExtractor(map[string]string{}, baseExtractor)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := extractor(req)
	if err == nil {
		t.Fatal("Expected error from base extractor")
	}

	if !strings.Contains(err.Error(), "base extractor error") {
		t.Errorf("Expected base extractor error, got: %v", err)
	}
}

func TestMethodScopeContextExtractor_WriteMethod(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	extractor := MethodScopeContextExtractor(baseExtractor)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)

			ctx, err := extractor(req)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if ctx.Scope() != "write" {
				t.Errorf("Expected scope 'write' for %s, got %s", method, ctx.Scope())
			}
		})
	}
}

func TestMethodScopeContextExtractor_DeleteMethod(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	extractor := MethodScopeContextExtractor(baseExtractor)

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Scope() != "delete" {
		t.Errorf("Expected scope 'delete', got %s", ctx.Scope())
	}
}

func TestMethodScopeContextExtractor_ReadMethod(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	extractor := MethodScopeContextExtractor(baseExtractor)

	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)

			ctx, err := extractor(req)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if ctx.Scope() != "read" {
				t.Errorf("Expected scope 'read' for %s, got %s", method, ctx.Scope())
			}
		})
	}
}

func TestMethodScopeContextExtractor_UnknownMethod(t *testing.T) {
	baseExtractor := func(r *http.Request) (ratelimit.Identity, error) {
		return ratelimit.NewIPContext("192.168.1.1"), nil
	}

	extractor := MethodScopeContextExtractor(baseExtractor)

	req := httptest.NewRequest("CUSTOM", "/test", nil)

	ctx, err := extractor(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if ctx.Scope() != ratelimit.ScopeGlobal {
		t.Errorf("Expected default scope 'global', got %s", ctx.Scope())
	}
}

// ============================================================================
// IP EXTRACTION TESTS
// ============================================================================

func TestGetClientIP_CloudflareHeaders(t *testing.T) {
	tests := []struct {
		name      string
		headerKey string
		headerVal string
		expected  string
	}{
		{"CF-Connecting-IP", "CF-Connecting-IP", "1.2.3.4", "1.2.3.4"},
		{"True-Client-IP", "True-Client-IP", "5.6.7.8", "5.6.7.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(tt.headerKey, tt.headerVal)

			ip := getClientIP(req)
			if ip != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2, 10.0.0.3")

	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("Expected first IP 10.0.0.1, got %s", ip)
	}
}

func TestGetClientIP_XForwardedForWithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "  10.0.0.1  , 10.0.0.2")

	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("Expected IP 10.0.0.1 (trimmed), got %s", ip)
	}
}

func TestGetClientIP_UnknownValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "unknown")

	// Should fallback to RemoteAddr
	req.RemoteAddr = "192.168.1.1:12345"

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("Expected fallback IP 192.168.1.1, got %s", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	ip := getClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("Expected IP 192.168.1.100, got %s", ip)
	}
}

func TestGetClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.200"

	ip := getClientIP(req)
	if ip != "192.168.1.200" {
		t.Errorf("Expected IP 192.168.1.200, got %s", ip)
	}
}

func TestGetClientIP_NoIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "" // Explicitly clear RemoteAddr
	// No headers, no RemoteAddr

	ip := getClientIP(req)
	if ip != "" {
		t.Errorf("Expected empty IP, got %s", ip)
	}
}

func TestGetClientIP_HeaderPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("CF-Connecting-IP", "1.1.1.1")
	req.Header.Set("X-Real-IP", "2.2.2.2")
	req.Header.Set("X-Forwarded-For", "3.3.3.3")
	req.RemoteAddr = "4.4.4.4:12345"

	// Should use CF-Connecting-IP (highest priority)
	ip := getClientIP(req)
	if ip != "1.1.1.1" {
		t.Errorf("Expected IP 1.1.1.1 (Cloudflare header), got %s", ip)
	}
}

// ============================================================================
// ERROR HANDLER TESTS - Removed deprecated DefaultHTTPErrorHandler
// NewSecureHTTPErrorHandler is now tested in secure error handling tests
// ============================================================================

// ============================================================================
// DEFAULT CONFIG TESTS
// ============================================================================

func TestDefaultHTTPMiddlewareConfig(t *testing.T) {
	limiter := &mockLimiter{}

	config := DefaultHTTPMiddlewareConfig(limiter)

	if config.Limiter != limiter {
		t.Error("Limiter not set correctly")
	}

	if config.ContextExtractor == nil {
		t.Error("Context extractor not set")
	}

	if config.ErrorHandler == nil {
		t.Error("Error handler not set")
	}

	if !config.AddHeaders {
		t.Error("AddHeaders should be true by default")
	}

	if len(config.SkipPaths) == 0 {
		t.Error("SkipPaths should have default values")
	}

	// Verify default skip paths
	expectedSkipPaths := []string{"/health", "/metrics", "/ready"}
	if len(config.SkipPaths) != len(expectedSkipPaths) {
		t.Errorf("Expected %d skip paths, got %d", len(expectedSkipPaths), len(config.SkipPaths))
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestHTTPMiddleware_RealLimiter_Integration(t *testing.T) {
	limiter := createTestLimiter(t, 3, time.Minute)
	defer limiter.Close()

	config := &HTTPMiddlewareConfig{
		Limiter:    limiter,
		AddHeaders: true,
	}

	middleware, err := NewHTTPMiddleware(config)
	if err != nil {
		t.Fatalf("Failed to create middleware: %v", err)
	}

	handler := middleware.Middleware(testHandler())

	// Make 3 requests - should all be allowed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i, w.Code)
		}
	}

	// 4th request should be denied
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: expected status 429, got %d", w.Code)
	}

	// Check rate limit headers
	if w.Header().Get("X-RateLimit-Limit") != "3" {
		t.Errorf("Expected limit header '3', got %s", w.Header().Get("X-RateLimit-Limit"))
	}

	if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("Expected remaining header '0', got %s", w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestHTTPMiddleware_DifferentIPs_Integration(t *testing.T) {
	limiter := createTestLimiter(t, 2, time.Minute)
	defer limiter.Close()

	config := &HTTPMiddlewareConfig{
		Limiter: limiter,
	}

	middleware, err := NewHTTPMiddleware(config)
	if err != nil {
		t.Fatalf("Failed to create middleware: %v", err)
	}

	handler := middleware.Middleware(testHandler())

	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Each IP should be able to make 2 requests
	for _, ip := range ips {
		for i := 1; i <= 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = fmt.Sprintf("%s:12345", ip)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("IP %s request %d: expected status 200, got %d", ip, i, w.Code)
			}
		}
	}

	// 3rd request from each IP should be denied
	for _, ip := range ips {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = fmt.Sprintf("%s:12345", ip)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("IP %s 3rd request: expected status 429, got %d", ip, w.Code)
		}
	}
}
