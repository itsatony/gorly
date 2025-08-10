// algorithms/sliding_window.go
package algorithms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// SlidingWindowAlgorithm implements the sliding window rate limiting algorithm
// This provides more accurate rate limiting by tracking individual requests
// within a rolling time window
type SlidingWindowAlgorithm struct {
	name string
}

// NewSlidingWindowAlgorithm creates a new sliding window algorithm
func NewSlidingWindowAlgorithm() *SlidingWindowAlgorithm {
	return &SlidingWindowAlgorithm{
		name: "sliding_window",
	}
}

// Name returns the algorithm name
func (sw *SlidingWindowAlgorithm) Name() string {
	return sw.name
}

// SlidingWindowState represents the current state of a sliding window
type SlidingWindowState struct {
	// Array of request timestamps within the current window
	Requests []int64 `json:"requests"`

	// Total number of requests processed (lifetime)
	TotalRequests int64 `json:"total_requests"`

	// Total requests denied (lifetime)
	DeniedRequests int64 `json:"denied_requests"`

	// Window duration in nanoseconds
	WindowNano int64 `json:"window_nano"`

	// Last cleanup timestamp in nanoseconds
	LastCleanup int64 `json:"last_cleanup"`

	// Limit for this window
	Limit int64 `json:"limit"`
}

// Allow checks if N requests are allowed within the sliding window
func (sw *SlidingWindowAlgorithm) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error) {
	if n <= 0 {
		return &Result{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: 0,
			ResetTime:  time.Time{},
			Limit:      limit,
			Window:     window,
			Used:       0,
			Algorithm:  sw.name,
		}, NewRateLimitError("validation", "request count must be greater than 0", nil)
	}

	now := time.Now()
	nowNano := now.UnixNano()
	windowNano := int64(window.Nanoseconds())

	// Get current state
	state, err := sw.getState(ctx, store, key, limit, windowNano)
	if err != nil {
		return nil, err
	}

	// Clean up old requests outside the current window
	state = sw.cleanupExpiredRequests(state, nowNano)

	// Calculate current usage
	currentUsage := int64(len(state.Requests))
	remaining := limit - currentUsage

	// Check if request can be allowed
	allowed := remaining >= n

	var retryAfter time.Duration
	var resetTime time.Time

	if allowed {
		// Add the new requests to the window
		for i := int64(0); i < n; i++ {
			state.Requests = append(state.Requests, nowNano)
		}
		state.TotalRequests += n
		remaining -= n
		currentUsage += n
	} else {
		// Request denied - calculate retry after time
		state.DeniedRequests += n

		if len(state.Requests) > 0 {
			// Find the oldest request that will expire
			oldestRequest := state.Requests[0]
			retryAfter = time.Duration(oldestRequest + windowNano - nowNano)
		} else {
			// No requests in window, can retry immediately
			retryAfter = 0
		}
	}

	// Calculate reset time (when the window will have capacity again)
	if len(state.Requests) > 0 {
		// Reset time is when the oldest request expires
		oldestRequest := state.Requests[0]
		resetTime = time.Unix(0, oldestRequest+windowNano)
	} else {
		resetTime = now.Add(window)
	}

	// Update last cleanup time
	state.LastCleanup = nowNano

	// Save state back to store
	if err := sw.saveState(ctx, store, key, state, window); err != nil {
		return nil, err
	}

	return &Result{
		Allowed:    allowed,
		Remaining:  remaining,
		RetryAfter: retryAfter,
		ResetTime:  resetTime,
		Limit:      limit,
		Window:     window,
		Used:       currentUsage,
		Algorithm:  sw.name,
	}, nil
}

// Reset clears all requests for a specific key
func (sw *SlidingWindowAlgorithm) Reset(ctx context.Context, store Store, key string) error {
	return store.Delete(ctx, key)
}

// GetWindowInfo returns information about the current window state
func (sw *SlidingWindowAlgorithm) GetWindowInfo(ctx context.Context, store Store, key string, limit int64, window time.Duration) (map[string]interface{}, error) {
	windowNano := int64(window.Nanoseconds())
	state, err := sw.getState(ctx, store, key, limit, windowNano)
	if err != nil {
		return nil, err
	}

	nowNano := time.Now().UnixNano()
	state = sw.cleanupExpiredRequests(state, nowNano)

	// Calculate request distribution over time
	requestTimes := make([]time.Time, len(state.Requests))
	for i, ts := range state.Requests {
		requestTimes[i] = time.Unix(0, ts)
	}

	// Calculate average request rate
	var avgRate float64
	if len(state.Requests) > 1 && window.Seconds() > 0 {
		timespan := float64(state.Requests[len(state.Requests)-1] - state.Requests[0])
		if timespan > 0 {
			avgRate = float64(len(state.Requests)) / (timespan / 1e9) // Convert nanoseconds to seconds
		}
	}

	return map[string]interface{}{
		"algorithm":         sw.name,
		"limit":             limit,
		"window":            window,
		"current_requests":  len(state.Requests),
		"remaining":         limit - int64(len(state.Requests)),
		"total_requests":    state.TotalRequests,
		"denied_requests":   state.DeniedRequests,
		"window_start_time": time.Unix(0, nowNano-windowNano),
		"window_end_time":   time.Unix(0, nowNano),
		"request_times":     requestTimes,
		"average_rate":      avgRate,
		"oldest_request": func() *time.Time {
			if len(state.Requests) > 0 {
				t := time.Unix(0, state.Requests[0])
				return &t
			}
			return nil
		}(),
		"newest_request": func() *time.Time {
			if len(state.Requests) > 0 {
				t := time.Unix(0, state.Requests[len(state.Requests)-1])
				return &t
			}
			return nil
		}(),
	}, nil
}

// GetMetrics returns detailed metrics about the sliding window
func (sw *SlidingWindowAlgorithm) GetMetrics(ctx context.Context, store Store, key string, limit int64, window time.Duration) (*WindowMetrics, error) {
	windowNano := int64(window.Nanoseconds())
	state, err := sw.getState(ctx, store, key, limit, windowNano)
	if err != nil {
		return nil, err
	}

	nowNano := time.Now().UnixNano()
	state = sw.cleanupExpiredRequests(state, nowNano)

	metrics := &WindowMetrics{
		WindowKey:       key,
		Algorithm:       sw.name,
		Limit:           limit,
		Window:          window,
		CurrentRequests: int64(len(state.Requests)),
		TotalRequests:   state.TotalRequests,
		DeniedRequests:  state.DeniedRequests,
		WindowStart:     time.Unix(0, nowNano-windowNano),
		WindowEnd:       time.Unix(0, nowNano),
	}

	// Calculate request distribution (requests per time bucket)
	if len(state.Requests) > 0 {
		bucketSize := windowNano / 10 // 10 buckets
		if bucketSize < 1 {
			bucketSize = 1
		}

		buckets := make([]int, 10)
		for _, reqTime := range state.Requests {
			bucketIndex := int((nowNano - reqTime) / bucketSize)
			if bucketIndex >= 0 && bucketIndex < 10 {
				buckets[9-bucketIndex]++ // Reverse order for chronological display
			}
		}
		metrics.RequestDistribution = buckets
	}

	return metrics, nil
}

// ValidateConfig validates the sliding window configuration
func (sw *SlidingWindowAlgorithm) ValidateConfig(limit int64, window time.Duration, maxRequests int64) error {
	if limit <= 0 {
		return NewRateLimitError("config", "limit must be greater than 0", nil)
	}

	if window <= 0 {
		return NewRateLimitError("config", "window must be greater than 0", nil)
	}

	// Check for reasonable window sizes
	if window < time.Second {
		return NewRateLimitError("config", "window should be at least 1 second", nil)
	}

	if window > 24*time.Hour {
		return NewRateLimitError("config", "window should not exceed 24 hours", nil)
	}

	// Check for reasonable limits
	if limit > 1000000 {
		return NewRateLimitError("config", "limit should not exceed 1,000,000 requests", nil)
	}

	// Check that we won't have too many individual requests to track
	if maxRequests > 0 && limit > maxRequests {
		return NewRateLimitError("config",
			fmt.Sprintf("limit %d exceeds maximum trackable requests %d", limit, maxRequests), nil)
	}

	return nil
}

// WindowMetrics contains detailed metrics for the sliding window
type WindowMetrics struct {
	WindowKey           string        `json:"window_key"`
	Algorithm           string        `json:"algorithm"`
	Limit               int64         `json:"limit"`
	Window              time.Duration `json:"window"`
	CurrentRequests     int64         `json:"current_requests"`
	TotalRequests       int64         `json:"total_requests"`
	DeniedRequests      int64         `json:"denied_requests"`
	WindowStart         time.Time     `json:"window_start"`
	WindowEnd           time.Time     `json:"window_end"`
	RequestDistribution []int         `json:"request_distribution"` // Distribution across time buckets
}

// getState retrieves the current sliding window state from storage
func (sw *SlidingWindowAlgorithm) getState(ctx context.Context, store Store, key string, limit, windowNano int64) (*SlidingWindowState, error) {
	data, err := store.Get(ctx, key)
	if err != nil {
		// Key doesn't exist, create new state
		return &SlidingWindowState{
			Requests:       make([]int64, 0),
			TotalRequests:  0,
			DeniedRequests: 0,
			WindowNano:     windowNano,
			LastCleanup:    time.Now().UnixNano(),
			Limit:          limit,
		}, nil
	}

	var state SlidingWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, NewRateLimitError("store", "failed to unmarshal sliding window state", err)
	}

	// Update window configuration if it has changed
	state.WindowNano = windowNano
	state.Limit = limit

	return &state, nil
}

// saveState saves the sliding window state to storage
func (sw *SlidingWindowAlgorithm) saveState(ctx context.Context, store Store, key string, state *SlidingWindowState, window time.Duration) error {
	data, err := json.Marshal(state)
	if err != nil {
		return NewRateLimitError("store", "failed to marshal sliding window state", err)
	}

	// Set expiration to window + buffer for cleanup
	expiration := window + time.Hour
	return store.Set(ctx, key, data, expiration)
}

// cleanupExpiredRequests removes requests that are outside the current window
func (sw *SlidingWindowAlgorithm) cleanupExpiredRequests(state *SlidingWindowState, nowNano int64) *SlidingWindowState {
	if len(state.Requests) == 0 {
		return state
	}

	windowStart := nowNano - state.WindowNano

	// Find the first request that is still within the window
	cutoffIndex := sort.Search(len(state.Requests), func(i int) bool {
		return state.Requests[i] >= windowStart
	})

	// Remove expired requests
	if cutoffIndex > 0 {
		state.Requests = state.Requests[cutoffIndex:]
	}

	return state
}

// GetRequestPattern analyzes the request pattern within the window
func (sw *SlidingWindowAlgorithm) GetRequestPattern(ctx context.Context, store Store, key string, limit int64, window time.Duration) (*RequestPattern, error) {
	windowNano := int64(window.Nanoseconds())
	state, err := sw.getState(ctx, store, key, limit, windowNano)
	if err != nil {
		return nil, err
	}

	nowNano := time.Now().UnixNano()
	state = sw.cleanupExpiredRequests(state, nowNano)

	pattern := &RequestPattern{
		TotalRequests: int64(len(state.Requests)),
		WindowStart:   time.Unix(0, nowNano-windowNano),
		WindowEnd:     time.Unix(0, nowNano),
	}

	if len(state.Requests) == 0 {
		return pattern, nil
	}

	// Calculate statistics
	requests := state.Requests

	// Sort to ensure chronological order (should already be sorted, but ensure it)
	sort.Slice(requests, func(i, j int) bool {
		return requests[i] < requests[j]
	})

	// Calculate intervals between requests
	if len(requests) > 1 {
		intervals := make([]time.Duration, len(requests)-1)
		for i := 1; i < len(requests); i++ {
			intervals[i-1] = time.Duration(requests[i] - requests[i-1])
		}

		// Calculate average interval
		var totalInterval time.Duration
		for _, interval := range intervals {
			totalInterval += interval
		}
		pattern.AverageInterval = totalInterval / time.Duration(len(intervals))

		// Find min and max intervals
		pattern.MinInterval = intervals[0]
		pattern.MaxInterval = intervals[0]
		for _, interval := range intervals[1:] {
			if interval < pattern.MinInterval {
				pattern.MinInterval = interval
			}
			if interval > pattern.MaxInterval {
				pattern.MaxInterval = interval
			}
		}
	}

	// Calculate request rate (requests per second)
	timespan := requests[len(requests)-1] - requests[0]
	if timespan > 0 {
		pattern.RequestRate = float64(len(requests)) / (float64(timespan) / 1e9) // Convert nanoseconds to seconds
	}

	// Detect bursts (sequences of requests with small intervals)
	burstThreshold := time.Second // Requests within 1 second are considered a burst
	var burstCount int
	var currentBurstSize int

	for i := 1; i < len(requests); i++ {
		interval := time.Duration(requests[i] - requests[i-1])
		if interval <= burstThreshold {
			if currentBurstSize == 0 {
				currentBurstSize = 2 // Start of a burst
			} else {
				currentBurstSize++
			}
		} else {
			if currentBurstSize >= 3 { // Only count bursts of 3+ requests
				burstCount++
			}
			currentBurstSize = 0
		}
	}
	if currentBurstSize >= 3 {
		burstCount++
	}

	pattern.BurstCount = burstCount

	return pattern, nil
}

// RequestPattern contains analysis of request patterns within a sliding window
type RequestPattern struct {
	TotalRequests   int64         `json:"total_requests"`
	WindowStart     time.Time     `json:"window_start"`
	WindowEnd       time.Time     `json:"window_end"`
	AverageInterval time.Duration `json:"average_interval"`
	MinInterval     time.Duration `json:"min_interval"`
	MaxInterval     time.Duration `json:"max_interval"`
	RequestRate     float64       `json:"request_rate"` // Requests per second
	BurstCount      int           `json:"burst_count"`  // Number of burst sequences detected
}
