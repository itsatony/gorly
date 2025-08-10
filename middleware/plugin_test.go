// middleware/plugin_test.go
package middleware

import (
	"context"
	"testing"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

func TestPluginRegistry(t *testing.T) {
	// Test that all expected plugins are registered
	expectedPlugins := []string{"gin", "echo", "fiber", "chi"}

	registeredPlugins := List()
	if len(registeredPlugins) == 0 {
		t.Fatal("No plugins registered")
	}

	for _, expected := range expectedPlugins {
		found := false
		for _, registered := range registeredPlugins {
			if registered == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Plugin %s not found in registered plugins: %v", expected, registeredPlugins)
		}
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Test get functionality
	ginPlugin, exists := Get("gin")
	if !exists {
		t.Error("Gin plugin not found in registry")
	}
	if ginPlugin == nil {
		t.Error("Gin plugin is nil")
	}
	if ginPlugin.Name() != "gin" {
		t.Errorf("Expected plugin name 'gin', got '%s'", ginPlugin.Name())
	}

	// Test non-existent plugin
	_, exists = Get("nonexistent")
	if exists {
		t.Error("Non-existent plugin should not be found")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Check that default extractors are set
	if config.EntityExtractor == nil {
		t.Error("EntityExtractor is nil")
	}
	if config.ScopeExtractor == nil {
		t.Error("ScopeExtractor is nil")
	}
	if config.TierExtractor == nil {
		t.Error("TierExtractor is nil")
	}

	// Check response config defaults
	if config.ResponseConfig.RateLimitedStatusCode != 429 {
		t.Errorf("Expected rate limited status code 429, got %d", config.ResponseConfig.RateLimitedStatusCode)
	}
	if config.ResponseConfig.ErrorStatusCode != 500 {
		t.Errorf("Expected error status code 500, got %d", config.ResponseConfig.ErrorStatusCode)
	}
}

func TestRequestInfoCreation(t *testing.T) {
	req := &RequestInfo{
		Method:     "GET",
		Path:       "/api/test",
		RemoteAddr: "127.0.0.1",
		UserAgent:  "Test-Agent/1.0",
		Headers:    map[string][]string{"X-API-Key": {"test-key"}},
		Context:    context.Background(),
		Requests:   1,
		Metadata:   map[string]interface{}{"test": "value"},
	}

	if req.Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", req.Method)
	}
	if req.Path != "/api/test" {
		t.Errorf("Expected path '/api/test', got '%s'", req.Path)
	}
	if req.Requests != 1 {
		t.Errorf("Expected requests 1, got %d", req.Requests)
	}
}

func TestDefaultEntityExtractor(t *testing.T) {
	extractor := &DefaultEntityExtractor{
		APIKeyHeaders: []string{"X-API-Key"},
		UserIDHeaders: []string{"X-User-ID"},
		UseIPFallback: true,
	}

	// Test API key extraction
	req := &RequestInfo{
		Headers: map[string][]string{"X-API-Key": {"test-api-key"}},
	}
	entityID, entityType, err := extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if entityID != "apikey:test-api-key" {
		t.Errorf("Expected entity ID 'apikey:test-api-key', got '%s'", entityID)
	}
	if entityType != ratelimit.EntityTypeAPIKey {
		t.Errorf("Expected entity type '%s', got '%s'", ratelimit.EntityTypeAPIKey, entityType)
	}

	// Test user ID extraction
	req = &RequestInfo{
		Headers: map[string][]string{"X-User-ID": {"user123"}},
	}
	entityID, entityType, err = extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if entityID != "user:user123" {
		t.Errorf("Expected entity ID 'user:user123', got '%s'", entityID)
	}
	if entityType != ratelimit.EntityTypeUser {
		t.Errorf("Expected entity type '%s', got '%s'", ratelimit.EntityTypeUser, entityType)
	}

	// Test IP fallback
	req = &RequestInfo{
		Headers:    map[string][]string{},
		RemoteAddr: "192.168.1.1:8080",
	}
	entityID, entityType, err = extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if entityID != "ip:192.168.1.1:8080" {
		t.Errorf("Expected entity ID 'ip:192.168.1.1:8080', got '%s'", entityID)
	}
	if entityType != ratelimit.EntityTypeIP {
		t.Errorf("Expected entity type '%s', got '%s'", ratelimit.EntityTypeIP, entityType)
	}
}

func TestDefaultScopeExtractor(t *testing.T) {
	extractor := &DefaultScopeExtractor{
		PathScopes: map[string]string{
			"/api/search": ratelimit.ScopeSearch,
			"/api/admin":  ratelimit.ScopeAdmin,
		},
		MethodScopes: map[string]string{
			"POST": "write",
			"GET":  "read",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}

	// Test path-based scope extraction
	req := &RequestInfo{
		Path:   "/api/search/query",
		Method: "GET",
	}
	scope, err := extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if scope != ratelimit.ScopeSearch {
		t.Errorf("Expected scope '%s', got '%s'", ratelimit.ScopeSearch, scope)
	}

	// Test method-based scope extraction
	req = &RequestInfo{
		Path:   "/api/other",
		Method: "POST",
	}
	scope, err = extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if scope != "write" {
		t.Errorf("Expected scope 'write', got '%s'", scope)
	}

	// Test default scope
	req = &RequestInfo{
		Path:   "/other/path",
		Method: "OPTIONS",
	}
	scope, err = extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if scope != ratelimit.ScopeGlobal {
		t.Errorf("Expected scope '%s', got '%s'", ratelimit.ScopeGlobal, scope)
	}
}

func TestDefaultTierExtractor(t *testing.T) {
	extractor := &DefaultTierExtractor{
		TierHeaders: []string{"X-User-Tier"},
		DefaultTier: ratelimit.TierFree,
	}

	// Test tier from header
	req := &RequestInfo{
		Headers: map[string][]string{"X-User-Tier": {"premium"}},
	}
	tier, err := extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if tier != "premium" {
		t.Errorf("Expected tier 'premium', got '%s'", tier)
	}

	// Test default tier
	req = &RequestInfo{
		Headers: map[string][]string{},
	}
	tier, err = extractor.Extract(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if tier != ratelimit.TierFree {
		t.Errorf("Expected tier '%s', got '%s'", ratelimit.TierFree, tier)
	}
}

func TestProcessRequestWithSkip(t *testing.T) {
	// Create a simple in-memory rate limiter for testing
	config := ratelimit.DefaultConfig()
	config.Store = "memory"

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	middlewareConfig := DefaultConfig()
	middlewareConfig.Limiter = limiter
	middlewareConfig.SkipFunc = func(req *RequestInfo) bool {
		return req.Path == "/health"
	}

	// Test skipped request
	req := &RequestInfo{
		Method:     "GET",
		Path:       "/health",
		RemoteAddr: "127.0.0.1",
		Context:    context.Background(),
		Requests:   1,
		Headers:    make(map[string][]string),
		Metadata:   make(map[string]interface{}),
	}

	result, err := ProcessRequest(req, middlewareConfig)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("Skipped request should be allowed")
	}

	// Test non-skipped request
	req.Path = "/api/test"
	result, err = ProcessRequest(req, middlewareConfig)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("First request should be allowed")
	}
	if result.Remaining != 99 {
		t.Errorf("Expected 99 remaining requests, got %d", result.Remaining)
	}
}

func TestBuildResponseHeaders(t *testing.T) {
	result := &ratelimit.Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      10,
		Used:       10,
		RetryAfter: 60 * time.Second,
		Algorithm:  "sliding_window",
	}

	config := &ResponseConfig{
		IncludeHeaders: true,
		HeaderPrefix:   "X-RateLimit-",
		CustomHeaders:  map[string]string{"Custom": "value"},
	}

	headers := BuildResponseHeaders(result, config)

	expectedHeaders := map[string]string{
		"Custom":                  "value",
		"X-RateLimit-Limit":       "10",
		"X-RateLimit-Remaining":   "0",
		"X-RateLimit-Used":        "10",
		"X-RateLimit-Retry-After": "60",
		"Retry-After":             "60",
		"X-RateLimit-Algorithm":   "sliding_window",
	}

	for key, expectedValue := range expectedHeaders {
		if headers[key] != expectedValue {
			t.Errorf("Header %s: expected %s, got %s", key, expectedValue, headers[key])
		}
	}
}
