// config.go
package ratelimit

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config represents the configuration for the rate limiter
type Config struct {
	// Global settings
	Enabled   bool   `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Algorithm string `yaml:"algorithm" json:"algorithm" mapstructure:"algorithm"` // "token_bucket", "sliding_window", "gcra"
	Store     string `yaml:"store" json:"store" mapstructure:"store"`             // "redis", "memory"
	KeyPrefix string `yaml:"key_prefix" json:"key_prefix" mapstructure:"key_prefix"`

	// Store configuration
	Redis  RedisConfig  `yaml:"redis" json:"redis" mapstructure:"redis"`
	Memory MemoryConfig `yaml:"memory" json:"memory" mapstructure:"memory"`

	// Default rate limits
	DefaultLimits map[string]RateLimit `yaml:"default_limits" json:"default_limits" mapstructure:"default_limits"`

	// Entity-specific overrides
	EntityOverrides map[string]EntityConfig `yaml:"entity_overrides" json:"entity_overrides" mapstructure:"entity_overrides"`

	// Scope-specific rate limits
	ScopeLimits map[string]RateLimit `yaml:"scope_limits" json:"scope_limits" mapstructure:"scope_limits"`

	// Tier-specific rate limits
	TierLimits map[string]TierConfig `yaml:"tier_limits" json:"tier_limits" mapstructure:"tier_limits"`

	// Metrics and monitoring
	EnableMetrics  bool          `yaml:"enable_metrics" json:"enable_metrics" mapstructure:"enable_metrics"`
	MetricsPrefix  string        `yaml:"metrics_prefix" json:"metrics_prefix" mapstructure:"metrics_prefix"`
	StatsRetention time.Duration `yaml:"stats_retention" json:"stats_retention" mapstructure:"stats_retention"`

	// Performance settings
	MaxConcurrentRequests int           `yaml:"max_concurrent_requests" json:"max_concurrent_requests" mapstructure:"max_concurrent_requests"`
	OperationTimeout      time.Duration `yaml:"operation_timeout" json:"operation_timeout" mapstructure:"operation_timeout"`
	CleanupInterval       time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" mapstructure:"cleanup_interval"`
}

// RedisConfig configures Redis store settings
type RedisConfig struct {
	Address     string        `yaml:"address" json:"address" mapstructure:"address"`
	Password    string        `yaml:"password" json:"password" mapstructure:"password"`
	Database    int           `yaml:"database" json:"database" mapstructure:"database"`
	PoolSize    int           `yaml:"pool_size" json:"pool_size" mapstructure:"pool_size"`
	MinIdleConn int           `yaml:"min_idle_conn" json:"min_idle_conn" mapstructure:"min_idle_conn"`
	MaxRetries  int           `yaml:"max_retries" json:"max_retries" mapstructure:"max_retries"`
	Timeout     time.Duration `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
	TLS         bool          `yaml:"tls" json:"tls" mapstructure:"tls"`
}

// MemoryConfig configures in-memory store settings
type MemoryConfig struct {
	MaxKeys         int           `yaml:"max_keys" json:"max_keys" mapstructure:"max_keys"`
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" mapstructure:"cleanup_interval"`
	ShardCount      int           `yaml:"shard_count" json:"shard_count" mapstructure:"shard_count"`
}

// RateLimit represents a rate limit configuration
type RateLimit struct {
	// Rate specification
	Requests int64         `yaml:"requests" json:"requests" mapstructure:"requests"`
	Window   time.Duration `yaml:"window" json:"window" mapstructure:"window"`

	// Token bucket specific settings
	BurstSize int64 `yaml:"burst_size,omitempty" json:"burst_size,omitempty" mapstructure:"burst_size"`

	// Algorithm override for this specific limit
	Algorithm string `yaml:"algorithm,omitempty" json:"algorithm,omitempty" mapstructure:"algorithm"`

	// Human-readable rate string (e.g., "100/1m", "1000/1h")
	RateString string `yaml:"rate_string,omitempty" json:"rate_string,omitempty" mapstructure:"rate_string"`
}

// EntityConfig represents entity-specific configuration
type EntityConfig struct {
	// Custom limits for this entity
	Limits map[string]RateLimit `yaml:"limits" json:"limits" mapstructure:"limits"`

	// Override algorithm for this entity
	Algorithm string `yaml:"algorithm,omitempty" json:"algorithm,omitempty" mapstructure:"algorithm"`

	// Entity-specific settings
	Enabled  bool                   `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Metadata map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty" mapstructure:"metadata"`
}

// TierConfig represents tier-specific configuration
type TierConfig struct {
	// Default limits for this tier
	DefaultLimits map[string]RateLimit `yaml:"default_limits" json:"default_limits" mapstructure:"default_limits"`

	// Scope-specific limits for this tier
	ScopeLimits map[string]RateLimit `yaml:"scope_limits" json:"scope_limits" mapstructure:"scope_limits"`

	// Features enabled for this tier
	Features []string `yaml:"features,omitempty" json:"features,omitempty" mapstructure:"features"`

	// Burst multiplier for token bucket algorithm
	BurstMultiplier float64 `yaml:"burst_multiplier,omitempty" json:"burst_multiplier,omitempty" mapstructure:"burst_multiplier"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:   true,
		Algorithm: "token_bucket",
		Store:     "redis",
		KeyPrefix: "m3mo:ratelimit",

		Redis: RedisConfig{
			Address:     "localhost:6379",
			Database:    0,
			PoolSize:    10,
			MinIdleConn: 2,
			MaxRetries:  3,
			Timeout:     5 * time.Second,
		},

		Memory: MemoryConfig{
			MaxKeys:         100000,
			CleanupInterval: 5 * time.Minute,
			ShardCount:      16,
		},

		DefaultLimits: map[string]RateLimit{
			ScopeGlobal: {
				Requests:   1000,
				Window:     time.Hour,
				BurstSize:  100,
				RateString: "1000/1h",
			},
		},

		ScopeLimits: map[string]RateLimit{
			ScopeMemory: {
				Requests:   500,
				Window:     time.Hour,
				BurstSize:  50,
				RateString: "500/1h",
			},
			ScopeSearch: {
				Requests:   200,
				Window:     time.Hour,
				BurstSize:  20,
				RateString: "200/1h",
			},
			ScopeAnalytics: {
				Requests:   100,
				Window:     time.Hour,
				BurstSize:  10,
				RateString: "100/1h",
			},
		},

		TierLimits: map[string]TierConfig{
			TierFree: {
				DefaultLimits: map[string]RateLimit{
					ScopeGlobal: {
						Requests:   100,
						Window:     time.Hour,
						BurstSize:  10,
						RateString: "100/1h",
					},
				},
				BurstMultiplier: 1.0,
				Features:        []string{"basic"},
			},
			TierPremium: {
				DefaultLimits: map[string]RateLimit{
					ScopeGlobal: {
						Requests:   1000,
						Window:     time.Hour,
						BurstSize:  100,
						RateString: "1000/1h",
					},
				},
				BurstMultiplier: 2.0,
				Features:        []string{"basic", "advanced"},
			},
			TierEnterprise: {
				DefaultLimits: map[string]RateLimit{
					ScopeGlobal: {
						Requests:   10000,
						Window:     time.Hour,
						BurstSize:  1000,
						RateString: "10000/1h",
					},
				},
				BurstMultiplier: 5.0,
				Features:        []string{"basic", "advanced", "enterprise"},
			},
		},

		EnableMetrics:         true,
		MetricsPrefix:         "m3mo_ratelimit",
		StatsRetention:        24 * time.Hour,
		MaxConcurrentRequests: 1000,
		OperationTimeout:      5 * time.Second,
		CleanupInterval:       10 * time.Minute,
	}
}

// ParseRateString parses a rate string like "100/1m" or "1000/1h" into requests and window
func ParseRateString(rateStr string) (int64, time.Duration, error) {
	parts := strings.Split(rateStr, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid rate string format: %s (expected format: requests/duration)", rateStr)
	}

	requests, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid requests value in rate string: %s", parts[0])
	}

	window, err := parseDurationString(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid duration in rate string: %s", parts[1])
	}

	return requests, window, nil
}

// parseDurationString parses duration strings like "1m", "1h", "30s"
func parseDurationString(durationStr string) (time.Duration, error) {
	originalStr := durationStr

	// Check for day or week suffixes and convert
	if strings.HasSuffix(durationStr, "d") {
		numStr := strings.TrimSuffix(durationStr, "d")
		if num, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			durationStr = fmt.Sprintf("%dh", num*24)
		}
	} else if strings.HasSuffix(durationStr, "w") {
		numStr := strings.TrimSuffix(durationStr, "w")
		if num, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			durationStr = fmt.Sprintf("%dh", num*24*7)
		}
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s", originalStr)
	}

	return duration, nil
}

// ApplyRateString updates the RateLimit with parsed values from RateString
func (rl *RateLimit) ApplyRateString() error {
	if rl.RateString == "" {
		return nil
	}

	requests, window, err := ParseRateString(rl.RateString)
	if err != nil {
		return err
	}

	rl.Requests = requests
	rl.Window = window

	// Set default burst size if not specified
	if rl.BurstSize == 0 {
		rl.BurstSize = requests / 10 // Default to 10% of requests as burst
		if rl.BurstSize < 1 {
			rl.BurstSize = 1
		}
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{
		"token_bucket":   true,
		"sliding_window": true,
		"gcra":           true,
	}
	if !validAlgorithms[c.Algorithm] {
		return fmt.Errorf("invalid algorithm: %s", c.Algorithm)
	}

	// Validate store
	validStores := map[string]bool{
		"redis":  true,
		"memory": true,
	}
	if !validStores[c.Store] {
		return fmt.Errorf("invalid store: %s", c.Store)
	}

	// Validate Redis config if using Redis
	if c.Store == "redis" {
		if c.Redis.Address == "" {
			return fmt.Errorf("redis address is required when using redis store")
		}
		if c.Redis.PoolSize <= 0 {
			c.Redis.PoolSize = 10
		}
		if c.Redis.Timeout <= 0 {
			c.Redis.Timeout = 5 * time.Second
		}
	}

	// Validate and apply rate strings
	for scope, limit := range c.DefaultLimits {
		if err := limit.ApplyRateString(); err != nil {
			return fmt.Errorf("invalid rate string in default_limits[%s]: %v", scope, err)
		}
		c.DefaultLimits[scope] = limit
	}

	for scope, limit := range c.ScopeLimits {
		if err := limit.ApplyRateString(); err != nil {
			return fmt.Errorf("invalid rate string in scope_limits[%s]: %v", scope, err)
		}
		c.ScopeLimits[scope] = limit
	}

	// Validate tier limits
	for tier, tierConfig := range c.TierLimits {
		for scope, limit := range tierConfig.DefaultLimits {
			if err := limit.ApplyRateString(); err != nil {
				return fmt.Errorf("invalid rate string in tier_limits[%s].default_limits[%s]: %v", tier, scope, err)
			}
			tierConfig.DefaultLimits[scope] = limit
		}
		for scope, limit := range tierConfig.ScopeLimits {
			if err := limit.ApplyRateString(); err != nil {
				return fmt.Errorf("invalid rate string in tier_limits[%s].scope_limits[%s]: %v", tier, scope, err)
			}
			tierConfig.ScopeLimits[scope] = limit
		}
		c.TierLimits[tier] = tierConfig
	}

	// Validate entity overrides
	for entityID, entityConfig := range c.EntityOverrides {
		for scope, limit := range entityConfig.Limits {
			if err := limit.ApplyRateString(); err != nil {
				return fmt.Errorf("invalid rate string in entity_overrides[%s].limits[%s]: %v", entityID, scope, err)
			}
			entityConfig.Limits[scope] = limit
		}
		c.EntityOverrides[entityID] = entityConfig
	}

	// Set defaults for optional fields
	if c.KeyPrefix == "" {
		c.KeyPrefix = "m3mo:ratelimit"
	}
	if c.MetricsPrefix == "" {
		c.MetricsPrefix = "m3mo_ratelimit"
	}
	if c.StatsRetention <= 0 {
		c.StatsRetention = 24 * time.Hour
	}
	if c.MaxConcurrentRequests <= 0 {
		c.MaxConcurrentRequests = 1000
	}
	if c.OperationTimeout <= 0 {
		c.OperationTimeout = 5 * time.Second
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = 10 * time.Minute
	}

	return nil
}

// GetRateLimit returns the appropriate rate limit for the given entity and scope
func (c *Config) GetRateLimit(entity AuthEntity, scope string) RateLimit {
	// Check entity-specific overrides first
	entityKey := entity.Type() + ":" + entity.ID()
	if entityConfig, exists := c.EntityOverrides[entityKey]; exists {
		if limit, exists := entityConfig.Limits[scope]; exists {
			return limit
		}
	}

	// Check tier-specific limits
	if tierConfig, exists := c.TierLimits[entity.Tier()]; exists {
		// First check scope-specific limits for this tier
		if limit, exists := tierConfig.ScopeLimits[scope]; exists {
			// Apply burst multiplier if using token bucket
			if c.Algorithm == "token_bucket" && tierConfig.BurstMultiplier > 0 {
				limit.BurstSize = int64(float64(limit.BurstSize) * tierConfig.BurstMultiplier)
			}
			return limit
		}
		// Then check default limits for this tier
		if limit, exists := tierConfig.DefaultLimits[scope]; exists {
			if c.Algorithm == "token_bucket" && tierConfig.BurstMultiplier > 0 {
				limit.BurstSize = int64(float64(limit.BurstSize) * tierConfig.BurstMultiplier)
			}
			return limit
		}
	}

	// Check scope-specific limits
	if limit, exists := c.ScopeLimits[scope]; exists {
		return limit
	}

	// Fall back to default global limit
	if limit, exists := c.DefaultLimits[ScopeGlobal]; exists {
		return limit
	}

	// Ultimate fallback
	return RateLimit{
		Requests:   100,
		Window:     time.Hour,
		BurstSize:  10,
		Algorithm:  c.Algorithm,
		RateString: "100/1h",
	}
}

// ============================================================================
// Configuration Loading Convenience Functions
// ============================================================================

// LoadConfigFromFile loads configuration from a file using ConfigLoader
func LoadConfigFromFile(filename string) (*Config, error) {
	loader := NewConfigLoader()
	return loader.LoadFromFile(filename)
}

// LoadConfigFromEnv loads configuration from environment variables using ConfigLoader
func LoadConfigFromEnv() (*Config, error) {
	loader := NewConfigLoader()
	return loader.LoadFromEnv()
}

// LoadConfigFromSources loads configuration from multiple sources in priority order
func LoadConfigFromSources(sources ...ConfigSource) (*Config, error) {
	loader := NewConfigLoader()
	return loader.LoadFromMultipleSources(sources...)
}

// LoadConfigWithDefaults loads configuration with custom defaults
func LoadConfigWithDefaults(defaults *Config, sources ...ConfigSource) (*Config, error) {
	loader := NewConfigLoaderWithDefaults(defaults)
	return loader.LoadFromMultipleSources(sources...)
}

// NewRateLimiterFromFile creates a new rate limiter from a configuration file
func NewRateLimiterFromFile(filename string) (RateLimiter, error) {
	config, err := LoadConfigFromFile(filename)
	if err != nil {
		return nil, err
	}
	return NewRateLimiter(config)
}

// NewRateLimiterFromEnv creates a new rate limiter from environment variables
func NewRateLimiterFromEnv() (RateLimiter, error) {
	config, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewRateLimiter(config)
}

// NewRateLimiterFromSources creates a new rate limiter from multiple configuration sources
func NewRateLimiterFromSources(sources ...ConfigSource) (RateLimiter, error) {
	config, err := LoadConfigFromSources(sources...)
	if err != nil {
		return nil, err
	}
	return NewRateLimiter(config)
}
