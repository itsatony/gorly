// Package ratelimit provides testing utilities for rate limiting configurations
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// TestHelper provides utilities for testing rate limiting configurations
type TestHelper struct {
	limiter Limiter
	mu      sync.RWMutex
	stats   TestStats
}

// TestStats tracks testing statistics
type TestStats struct {
	TotalRequests   int64         `json:"total_requests"`
	AllowedRequests int64         `json:"allowed_requests"`
	DeniedRequests  int64         `json:"denied_requests"`
	SuccessRate     float64       `json:"success_rate"`
	AverageLatency  time.Duration `json:"average_latency"`
}

// TestScenario defines a testing scenario
type TestScenario struct {
	Name        string        `json:"name"`
	Entity      string        `json:"entity"`
	Scope       string        `json:"scope"`
	Requests    int           `json:"requests"`
	Interval    time.Duration `json:"interval"`
	ExpectAllow int           `json:"expect_allow"`
	ExpectDeny  int           `json:"expect_deny"`
}

// TestResult contains the results of a test scenario
type TestResult struct {
	Scenario       TestScenario  `json:"scenario"`
	ActualAllow    int           `json:"actual_allow"`
	ActualDeny     int           `json:"actual_deny"`
	Success        bool          `json:"success"`
	Duration       time.Duration `json:"duration"`
	AverageLatency time.Duration `json:"average_latency"`
	Error          string        `json:"error,omitempty"`
}

// NewTestHelper creates a new test helper for the given limiter
func NewTestHelper(limiter Limiter) *TestHelper {
	return &TestHelper{
		limiter: limiter,
	}
}

// TestLimit tests a specific limit configuration
func (th *TestHelper) TestLimit(ctx context.Context, entity, scope string, requests int, interval time.Duration) *TestResult {
	start := time.Now()
	var allowed, denied int64
	var totalLatency time.Duration

	for i := 0; i < requests; i++ {
		requestStart := time.Now()

		result, err := th.limiter.Check(ctx, entity, scope)
		if err != nil {
			return &TestResult{
				Error: fmt.Sprintf("Error checking limit: %v", err),
			}
		}

		requestLatency := time.Since(requestStart)
		totalLatency += requestLatency

		if result.Allowed {
			atomic.AddInt64(&allowed, 1)
		} else {
			atomic.AddInt64(&denied, 1)
		}

		// Update stats
		atomic.AddInt64(&th.stats.TotalRequests, 1)
		atomic.AddInt64(&th.stats.AllowedRequests, allowed)
		atomic.AddInt64(&th.stats.DeniedRequests, denied)

		if i < requests-1 {
			time.Sleep(interval)
		}
	}

	duration := time.Since(start)
	avgLatency := totalLatency / time.Duration(requests)

	return &TestResult{
		ActualAllow:    int(allowed),
		ActualDeny:     int(denied),
		Duration:       duration,
		AverageLatency: avgLatency,
		Success:        true,
	}
}

// RunScenario executes a test scenario
func (th *TestHelper) RunScenario(ctx context.Context, scenario TestScenario) *TestResult {
	result := th.TestLimit(ctx, scenario.Entity, scenario.Scope, scenario.Requests, scenario.Interval)

	result.Scenario = scenario

	// Check if results match expectations
	if scenario.ExpectAllow > 0 {
		allowTolerance := int(float64(scenario.ExpectAllow) * 0.1) // 10% tolerance
		if result.ActualAllow < scenario.ExpectAllow-allowTolerance ||
			result.ActualAllow > scenario.ExpectAllow+allowTolerance {
			result.Success = false
			result.Error = fmt.Sprintf("Expected ~%d allowed requests, got %d",
				scenario.ExpectAllow, result.ActualAllow)
		}
	}

	if scenario.ExpectDeny > 0 {
		denyTolerance := int(float64(scenario.ExpectDeny) * 0.1) // 10% tolerance
		if result.ActualDeny < scenario.ExpectDeny-denyTolerance ||
			result.ActualDeny > scenario.ExpectDeny+denyTolerance {
			result.Success = false
			result.Error = fmt.Sprintf("Expected ~%d denied requests, got %d",
				scenario.ExpectDeny, result.ActualDeny)
		}
	}

	return result
}

// RunConcurrentTest runs concurrent requests to test race conditions
func (th *TestHelper) RunConcurrentTest(ctx context.Context, entity, scope string,
	goroutines, requestsPerGoroutine int) *ConcurrentTestResult {

	start := time.Now()
	var wg sync.WaitGroup
	var totalAllowed, totalDenied int64

	results := make(chan *TestResult, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			entityID := fmt.Sprintf("%s-goroutine-%d", entity, id)
			result := th.TestLimit(ctx, entityID, scope, requestsPerGoroutine, 0)

			atomic.AddInt64(&totalAllowed, int64(result.ActualAllow))
			atomic.AddInt64(&totalDenied, int64(result.ActualDeny))

			results <- result
		}(i)
	}

	wg.Wait()
	close(results)

	var individualResults []*TestResult
	for result := range results {
		individualResults = append(individualResults, result)
	}

	return &ConcurrentTestResult{
		Goroutines:           goroutines,
		RequestsPerGoroutine: requestsPerGoroutine,
		TotalAllowed:         int(totalAllowed),
		TotalDenied:          int(totalDenied),
		Duration:             time.Since(start),
		IndividualResults:    individualResults,
	}
}

// ConcurrentTestResult contains results of concurrent testing
type ConcurrentTestResult struct {
	Goroutines           int           `json:"goroutines"`
	RequestsPerGoroutine int           `json:"requests_per_goroutine"`
	TotalAllowed         int           `json:"total_allowed"`
	TotalDenied          int           `json:"total_denied"`
	Duration             time.Duration `json:"duration"`
	IndividualResults    []*TestResult `json:"individual_results"`
}

// BenchmarkLimiter benchmarks the limiter performance
func (th *TestHelper) BenchmarkLimiter(ctx context.Context, entity, scope string, duration time.Duration) *BenchmarkResult {
	start := time.Now()
	var requests, allowed, denied int64

	for time.Since(start) < duration {
		requestStart := time.Now()
		result, err := th.limiter.Check(ctx, entity, scope)
		requestLatency := time.Since(requestStart)

		atomic.AddInt64(&requests, 1)

		if err != nil {
			continue
		}

		if result.Allowed {
			atomic.AddInt64(&allowed, 1)
		} else {
			atomic.AddInt64(&denied, 1)
		}

		// Track latency
		th.mu.Lock()
		th.stats.AverageLatency = (th.stats.AverageLatency*time.Duration(requests-1) + requestLatency) / time.Duration(requests)
		th.mu.Unlock()
	}

	actualDuration := time.Since(start)
	rps := float64(requests) / actualDuration.Seconds()

	return &BenchmarkResult{
		Duration:          actualDuration,
		TotalRequests:     int(requests),
		AllowedRequests:   int(allowed),
		DeniedRequests:    int(denied),
		RequestsPerSecond: rps,
		AverageLatency:    th.stats.AverageLatency,
	}
}

// BenchmarkResult contains benchmark results
type BenchmarkResult struct {
	Duration          time.Duration `json:"duration"`
	TotalRequests     int           `json:"total_requests"`
	AllowedRequests   int           `json:"allowed_requests"`
	DeniedRequests    int           `json:"denied_requests"`
	RequestsPerSecond float64       `json:"requests_per_second"`
	AverageLatency    time.Duration `json:"average_latency"`
}

// MockHTTPTest provides utilities for testing HTTP middleware
type MockHTTPTest struct {
	limiter Limiter
	handler http.Handler
}

// NewMockHTTPTest creates a new HTTP test helper
func NewMockHTTPTest(limiter Limiter) *MockHTTPTest {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply rate limiting middleware
	middleware := limiter.For(HTTP).(func(http.Handler) http.Handler)
	handler := middleware(mux)

	return &MockHTTPTest{
		limiter: limiter,
		handler: handler,
	}
}

// TestHTTPRequests tests HTTP requests with rate limiting
func (mht *MockHTTPTest) TestHTTPRequests(requests int, headers map[string]string) *HTTPTestResult {
	var allowed, denied int
	var responses []HTTPResponse

	for i := 0; i < requests; i++ {
		req := httptest.NewRequest("GET", "/test", nil)

		// Add custom headers
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		w := httptest.NewRecorder()
		mht.handler.ServeHTTP(w, req)

		response := HTTPResponse{
			StatusCode: w.Code,
			Headers:    make(map[string]string),
		}

		// Capture rate limiting headers
		for key, values := range w.Header() {
			if len(values) > 0 {
				response.Headers[key] = values[0]
			}
		}

		responses = append(responses, response)

		if w.Code == http.StatusOK {
			allowed++
		} else if w.Code == http.StatusTooManyRequests {
			denied++
		}
	}

	return &HTTPTestResult{
		TotalRequests: requests,
		Allowed:       allowed,
		Denied:        denied,
		Responses:     responses,
	}
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
}

// HTTPTestResult contains HTTP test results
type HTTPTestResult struct {
	TotalRequests int            `json:"total_requests"`
	Allowed       int            `json:"allowed"`
	Denied        int            `json:"denied"`
	Responses     []HTTPResponse `json:"responses"`
}

// AssertLimitBehavior provides assertion utilities for tests
type AssertLimitBehavior struct {
	th *TestHelper
}

// NewAssertLimitBehavior creates assertion utilities
func NewAssertLimitBehavior(limiter Limiter) *AssertLimitBehavior {
	return &AssertLimitBehavior{
		th: NewTestHelper(limiter),
	}
}

// AssertAllowed asserts that a request should be allowed
func (alb *AssertLimitBehavior) AssertAllowed(ctx context.Context, entity, scope string) error {
	result, err := alb.th.limiter.Check(ctx, entity, scope)
	if err != nil {
		return fmt.Errorf("error checking limit: %v", err)
	}

	if !result.Allowed {
		return fmt.Errorf("expected request to be allowed, but it was denied")
	}

	return nil
}

// AssertDenied asserts that a request should be denied
func (alb *AssertLimitBehavior) AssertDenied(ctx context.Context, entity, scope string) error {
	result, err := alb.th.limiter.Check(ctx, entity, scope)
	if err != nil {
		return fmt.Errorf("error checking limit: %v", err)
	}

	if result.Allowed {
		return fmt.Errorf("expected request to be denied, but it was allowed")
	}

	return nil
}

// AssertRemainingCount asserts the remaining count
func (alb *AssertLimitBehavior) AssertRemainingCount(ctx context.Context, entity, scope string, expected int64) error {
	result, err := alb.th.limiter.Check(ctx, entity, scope)
	if err != nil {
		return fmt.Errorf("error checking limit: %v", err)
	}

	if result.Remaining != expected {
		return fmt.Errorf("expected %d remaining requests, got %d", expected, result.Remaining)
	}

	return nil
}

// GetStats returns current test statistics
func (th *TestHelper) GetStats() TestStats {
	th.mu.RLock()
	defer th.mu.RUnlock()

	stats := th.stats
	if stats.TotalRequests > 0 {
		stats.SuccessRate = float64(stats.AllowedRequests) / float64(stats.TotalRequests) * 100
	}

	return stats
}

// ResetStats resets test statistics
func (th *TestHelper) ResetStats() {
	th.mu.Lock()
	defer th.mu.Unlock()

	th.stats = TestStats{}
}
