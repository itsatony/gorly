// config_loader.go
package ratelimit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigLoader provides functionality to load configuration from various sources
type ConfigLoader struct {
	// Default configuration to merge with loaded config
	defaults *Config
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		defaults: DefaultConfig(),
	}
}

// NewConfigLoaderWithDefaults creates a new configuration loader with custom defaults
func NewConfigLoaderWithDefaults(defaults *Config) *ConfigLoader {
	return &ConfigLoader{
		defaults: defaults,
	}
}

// LoadFromFile loads configuration from a file (supports JSON, YAML, TOML based on extension)
func (cl *ConfigLoader) LoadFromFile(filename string) (*Config, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %s: %w", filename, err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".json":
		return cl.LoadFromJSON(file)
	case ".yaml", ".yml":
		return cl.LoadFromYAML(file)
	default:
		return nil, fmt.Errorf("unsupported file format: %s (supported: .json, .yaml, .yml)", ext)
	}
}

// LoadFromJSON loads configuration from JSON reader
func (cl *ConfigLoader) LoadFromJSON(reader io.Reader) (*Config, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON data: %w", err)
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return cl.parseConfig(rawConfig)
}

// LoadFromYAML loads configuration from YAML reader
func (cl *ConfigLoader) LoadFromYAML(reader io.Reader) (*Config, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML data: %w", err)
	}

	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return cl.parseConfig(rawConfig)
}

// LoadFromEnv loads configuration from environment variables
func (cl *ConfigLoader) LoadFromEnv() (*Config, error) {
	config := cl.copyDefaults()

	// Load basic settings from environment
	if val := os.Getenv("GORLY_ENABLED"); val != "" {
		config.Enabled = strings.ToLower(val) == "true"
	}

	if val := os.Getenv("GORLY_ALGORITHM"); val != "" {
		config.Algorithm = val
	}

	if val := os.Getenv("GORLY_STORE"); val != "" {
		config.Store = val
	}

	if val := os.Getenv("GORLY_KEY_PREFIX"); val != "" {
		config.KeyPrefix = val
	}

	if val := os.Getenv("GORLY_ENABLE_METRICS"); val != "" {
		config.EnableMetrics = strings.ToLower(val) == "true"
	}

	if val := os.Getenv("GORLY_METRICS_PREFIX"); val != "" {
		config.MetricsPrefix = val
	}

	// Redis configuration
	if val := os.Getenv("GORLY_REDIS_ADDRESS"); val != "" {
		config.Redis.Address = val
	}

	if val := os.Getenv("GORLY_REDIS_PASSWORD"); val != "" {
		config.Redis.Password = val
	}

	if val := os.Getenv("GORLY_REDIS_DATABASE"); val != "" {
		var db int
		if _, err := fmt.Sscanf(val, "%d", &db); err == nil {
			config.Redis.Database = db
		}
	}

	// Default limits from environment (simplified format)
	if val := os.Getenv("GORLY_DEFAULT_LIMIT"); val != "" {
		if requests, window, err := ParseRateString(val); err == nil {
			if config.DefaultLimits == nil {
				config.DefaultLimits = make(map[string]RateLimit)
			}
			config.DefaultLimits[ScopeGlobal] = RateLimit{
				Requests: requests,
				Window:   window,
			}
		}
	}

	return config, nil
}

// LoadFromMultipleSources loads configuration from multiple sources in priority order
// Later sources override earlier ones
func (cl *ConfigLoader) LoadFromMultipleSources(sources ...ConfigSource) (*Config, error) {
	config := cl.copyDefaults()

	for _, source := range sources {
		loadedConfig, err := source.Load(cl)
		if err != nil {
			// If source is not required and fails, continue
			if source.IsRequired() {
				return nil, fmt.Errorf("required config source failed: %w", err)
			}
			continue
		}

		// Merge loaded config into our config
		if err := cl.mergeConfigs(config, loadedConfig); err != nil {
			return nil, fmt.Errorf("failed to merge config from source: %w", err)
		}
	}

	return config, nil
}

// parseConfig converts raw configuration map to Config struct
func (cl *ConfigLoader) parseConfig(raw map[string]interface{}) (*Config, error) {
	config := cl.copyDefaults()

	// Basic settings
	if val, ok := raw["enabled"].(bool); ok {
		config.Enabled = val
	}

	if val, ok := raw["algorithm"].(string); ok {
		config.Algorithm = val
	}

	if val, ok := raw["store"].(string); ok {
		config.Store = val
	}

	if val, ok := raw["keyPrefix"].(string); ok {
		config.KeyPrefix = val
	}

	if val, ok := raw["enableMetrics"].(bool); ok {
		config.EnableMetrics = val
	}

	if val, ok := raw["metricsPrefix"].(string); ok {
		config.MetricsPrefix = val
	}

	// Parse timeouts
	if val, ok := raw["operationTimeout"].(string); ok {
		if timeout, err := time.ParseDuration(val); err == nil {
			config.OperationTimeout = timeout
		}
	}

	// Parse Redis config
	if redisRaw, ok := raw["redis"].(map[string]interface{}); ok {
		if err := cl.parseRedisConfig(&config.Redis, redisRaw); err != nil {
			return nil, fmt.Errorf("failed to parse Redis config: %w", err)
		}
	}

	// Parse default limits
	if limitsRaw, ok := raw["defaultLimits"].(map[string]interface{}); ok {
		limits, err := cl.parseRateLimits(limitsRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse default limits: %w", err)
		}
		config.DefaultLimits = limits
	}

	// Parse scope limits
	if limitsRaw, ok := raw["scopeLimits"].(map[string]interface{}); ok {
		limits, err := cl.parseRateLimits(limitsRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scope limits: %w", err)
		}
		config.ScopeLimits = limits
	}

	// Parse tier limits
	if tiersRaw, ok := raw["tierLimits"].(map[string]interface{}); ok {
		tiers, err := cl.parseTierLimits(tiersRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tier limits: %w", err)
		}
		config.TierLimits = tiers
	}

	// Parse entity overrides
	if overridesRaw, ok := raw["entityOverrides"].(map[string]interface{}); ok {
		overrides, err := cl.parseEntityOverrides(overridesRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse entity overrides: %w", err)
		}
		config.EntityOverrides = overrides
	}

	return config, nil
}

// parseRedisConfig parses Redis configuration from raw map
func (cl *ConfigLoader) parseRedisConfig(redis *RedisConfig, raw map[string]interface{}) error {
	if val, ok := raw["address"].(string); ok {
		redis.Address = val
	}

	if val, ok := raw["password"].(string); ok {
		redis.Password = val
	}

	if val, ok := raw["database"]; ok {
		if db, ok := val.(int); ok {
			redis.Database = db
		} else if dbFloat, ok := val.(float64); ok {
			redis.Database = int(dbFloat)
		}
	}

	if val, ok := raw["poolSize"]; ok {
		if size, ok := val.(int); ok {
			redis.PoolSize = size
		} else if sizeFloat, ok := val.(float64); ok {
			redis.PoolSize = int(sizeFloat)
		}
	}

	if val, ok := raw["minIdleConn"]; ok {
		if conn, ok := val.(int); ok {
			redis.MinIdleConn = conn
		} else if connFloat, ok := val.(float64); ok {
			redis.MinIdleConn = int(connFloat)
		}
	}

	if val, ok := raw["maxRetries"]; ok {
		if retries, ok := val.(int); ok {
			redis.MaxRetries = retries
		} else if retriesFloat, ok := val.(float64); ok {
			redis.MaxRetries = int(retriesFloat)
		}
	}

	if val, ok := raw["timeout"].(string); ok {
		if timeout, err := time.ParseDuration(val); err == nil {
			redis.Timeout = timeout
		}
	}

	if val, ok := raw["tls"].(bool); ok {
		redis.TLS = val
	}

	return nil
}

// parseRateLimits parses rate limits from raw map
func (cl *ConfigLoader) parseRateLimits(raw map[string]interface{}) (map[string]RateLimit, error) {
	limits := make(map[string]RateLimit)

	for scope, limitRaw := range raw {
		switch v := limitRaw.(type) {
		case string:
			// Parse rate string like "100/min"
			requests, window, err := ParseRateString(v)
			if err != nil {
				return nil, fmt.Errorf("invalid rate limit for scope %s: %w", scope, err)
			}
			limits[scope] = RateLimit{
				Requests: requests,
				Window:   window,
			}

		case map[string]interface{}:
			// Parse detailed rate limit object
			limit, err := cl.parseRateLimit(v)
			if err != nil {
				return nil, fmt.Errorf("invalid rate limit config for scope %s: %w", scope, err)
			}
			limits[scope] = limit

		default:
			return nil, fmt.Errorf("invalid rate limit format for scope %s", scope)
		}
	}

	return limits, nil
}

// parseRateLimit parses a single rate limit from raw map
func (cl *ConfigLoader) parseRateLimit(raw map[string]interface{}) (RateLimit, error) {
	var limit RateLimit

	if val, ok := raw["requests"]; ok {
		if requests, ok := val.(int); ok {
			limit.Requests = int64(requests)
		} else if requestsFloat, ok := val.(float64); ok {
			limit.Requests = int64(requestsFloat)
		} else {
			return limit, fmt.Errorf("invalid requests value")
		}
	}

	if val, ok := raw["window"].(string); ok {
		window, err := time.ParseDuration(val)
		if err != nil {
			return limit, fmt.Errorf("invalid window duration: %w", err)
		}
		limit.Window = window
	}

	return limit, nil
}

// parseTierLimits parses tier limits from raw map
func (cl *ConfigLoader) parseTierLimits(raw map[string]interface{}) (map[string]TierConfig, error) {
	tiers := make(map[string]TierConfig)

	for tier, tierRaw := range raw {
		tierMap, ok := tierRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid tier config for tier %s", tier)
		}

		tierConfig := TierConfig{
			DefaultLimits: make(map[string]RateLimit),
			ScopeLimits:   make(map[string]RateLimit),
		}

		if limitsRaw, ok := tierMap["defaultLimits"].(map[string]interface{}); ok {
			limits, err := cl.parseRateLimits(limitsRaw)
			if err != nil {
				return nil, fmt.Errorf("failed to parse default limits for tier %s: %w", tier, err)
			}
			tierConfig.DefaultLimits = limits
		}

		if limitsRaw, ok := tierMap["scopeLimits"].(map[string]interface{}); ok {
			limits, err := cl.parseRateLimits(limitsRaw)
			if err != nil {
				return nil, fmt.Errorf("failed to parse scope limits for tier %s: %w", tier, err)
			}
			tierConfig.ScopeLimits = limits
		}

		tiers[tier] = tierConfig
	}

	return tiers, nil
}

// parseEntityOverrides parses entity overrides from raw map
func (cl *ConfigLoader) parseEntityOverrides(raw map[string]interface{}) (map[string]EntityConfig, error) {
	overrides := make(map[string]EntityConfig)

	for entity, entityRaw := range raw {
		entityMap, ok := entityRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid entity override config for entity %s", entity)
		}

		entityConfig := EntityConfig{
			Limits:   make(map[string]RateLimit),
			Enabled:  true, // Default to enabled
			Metadata: make(map[string]interface{}),
		}

		// Parse limits
		if limitsRaw, exists := entityMap["limits"]; exists {
			if limitsMap, ok := limitsRaw.(map[string]interface{}); ok {
				limits, err := cl.parseRateLimits(limitsMap)
				if err != nil {
					return nil, fmt.Errorf("failed to parse limits for entity %s: %w", entity, err)
				}
				entityConfig.Limits = limits
			}
		} else {
			// If no "limits" key, treat the whole map as rate limits (backward compatibility)
			limits, err := cl.parseRateLimits(entityMap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse entity overrides for entity %s: %w", entity, err)
			}
			entityConfig.Limits = limits
		}

		// Parse algorithm override
		if algorithm, ok := entityMap["algorithm"].(string); ok {
			entityConfig.Algorithm = algorithm
		}

		// Parse enabled flag
		if enabled, ok := entityMap["enabled"].(bool); ok {
			entityConfig.Enabled = enabled
		}

		// Parse metadata
		if metadata, ok := entityMap["metadata"].(map[string]interface{}); ok {
			entityConfig.Metadata = metadata
		}

		overrides[entity] = entityConfig
	}

	return overrides, nil
}

// mergeConfigs merges source config into destination config
func (cl *ConfigLoader) mergeConfigs(dest, src *Config) error {
	// Basic settings - overwrite if different from defaults
	if src.Enabled != cl.defaults.Enabled {
		dest.Enabled = src.Enabled
	}

	if src.Algorithm != cl.defaults.Algorithm {
		dest.Algorithm = src.Algorithm
	}

	if src.Store != cl.defaults.Store {
		dest.Store = src.Store
	}

	if src.KeyPrefix != cl.defaults.KeyPrefix {
		dest.KeyPrefix = src.KeyPrefix
	}

	if src.EnableMetrics != cl.defaults.EnableMetrics {
		dest.EnableMetrics = src.EnableMetrics
	}

	if src.MetricsPrefix != cl.defaults.MetricsPrefix {
		dest.MetricsPrefix = src.MetricsPrefix
	}

	if src.OperationTimeout != cl.defaults.OperationTimeout {
		dest.OperationTimeout = src.OperationTimeout
	}

	if src.MaxConcurrentRequests != cl.defaults.MaxConcurrentRequests {
		dest.MaxConcurrentRequests = src.MaxConcurrentRequests
	}

	// Merge Redis config
	cl.mergeRedisConfig(&dest.Redis, &src.Redis)

	// Merge rate limits maps
	cl.mergeRateLimitMaps(dest.DefaultLimits, src.DefaultLimits)
	cl.mergeRateLimitMaps(dest.ScopeLimits, src.ScopeLimits)

	// Merge tier limits
	for tier, tierConfig := range src.TierLimits {
		if dest.TierLimits == nil {
			dest.TierLimits = make(map[string]TierConfig)
		}
		dest.TierLimits[tier] = tierConfig
	}

	// Merge entity overrides
	for entity, entityConfig := range src.EntityOverrides {
		if dest.EntityOverrides == nil {
			dest.EntityOverrides = make(map[string]EntityConfig)
		}
		dest.EntityOverrides[entity] = entityConfig
	}

	return nil
}

// mergeRedisConfig merges Redis configurations
func (cl *ConfigLoader) mergeRedisConfig(dest, src *RedisConfig) {
	if src.Address != cl.defaults.Redis.Address {
		dest.Address = src.Address
	}
	if src.Password != cl.defaults.Redis.Password {
		dest.Password = src.Password
	}
	if src.Database != cl.defaults.Redis.Database {
		dest.Database = src.Database
	}
	if src.PoolSize != cl.defaults.Redis.PoolSize {
		dest.PoolSize = src.PoolSize
	}
	if src.MinIdleConn != cl.defaults.Redis.MinIdleConn {
		dest.MinIdleConn = src.MinIdleConn
	}
	if src.MaxRetries != cl.defaults.Redis.MaxRetries {
		dest.MaxRetries = src.MaxRetries
	}
	if src.Timeout != cl.defaults.Redis.Timeout {
		dest.Timeout = src.Timeout
	}
	if src.TLS != cl.defaults.Redis.TLS {
		dest.TLS = src.TLS
	}
}

// mergeRateLimitMaps merges rate limit maps
func (cl *ConfigLoader) mergeRateLimitMaps(dest, src map[string]RateLimit) {
	for key, value := range src {
		dest[key] = value
	}
}

// copyDefaults creates a deep copy of the default configuration
func (cl *ConfigLoader) copyDefaults() *Config {
	config := *cl.defaults

	// Deep copy maps
	if cl.defaults.DefaultLimits != nil {
		config.DefaultLimits = make(map[string]RateLimit)
		for k, v := range cl.defaults.DefaultLimits {
			config.DefaultLimits[k] = v
		}
	}

	if cl.defaults.ScopeLimits != nil {
		config.ScopeLimits = make(map[string]RateLimit)
		for k, v := range cl.defaults.ScopeLimits {
			config.ScopeLimits[k] = v
		}
	}

	if cl.defaults.TierLimits != nil {
		config.TierLimits = make(map[string]TierConfig)
		for k, v := range cl.defaults.TierLimits {
			newTierConfig := TierConfig{
				DefaultLimits: make(map[string]RateLimit),
				ScopeLimits:   make(map[string]RateLimit),
			}
			for dk, dv := range v.DefaultLimits {
				newTierConfig.DefaultLimits[dk] = dv
			}
			for sk, sv := range v.ScopeLimits {
				newTierConfig.ScopeLimits[sk] = sv
			}
			config.TierLimits[k] = newTierConfig
		}
	}

	if cl.defaults.EntityOverrides != nil {
		config.EntityOverrides = make(map[string]EntityConfig)
		for k, v := range cl.defaults.EntityOverrides {
			newEntityConfig := EntityConfig{
				Limits:    make(map[string]RateLimit),
				Algorithm: v.Algorithm,
				Enabled:   v.Enabled,
				Metadata:  make(map[string]interface{}),
			}
			for lk, lv := range v.Limits {
				newEntityConfig.Limits[lk] = lv
			}
			for mk, mv := range v.Metadata {
				newEntityConfig.Metadata[mk] = mv
			}
			config.EntityOverrides[k] = newEntityConfig
		}
	}

	return &config
}

// ConfigSource represents a source of configuration
type ConfigSource interface {
	Load(loader *ConfigLoader) (*Config, error)
	IsRequired() bool
}

// FileConfigSource loads config from a file
type FileConfigSource struct {
	Filename string
	Required bool
}

func (f *FileConfigSource) Load(loader *ConfigLoader) (*Config, error) {
	return loader.LoadFromFile(f.Filename)
}

func (f *FileConfigSource) IsRequired() bool {
	return f.Required
}

// EnvConfigSource loads config from environment variables
type EnvConfigSource struct {
	Required bool
}

func (e *EnvConfigSource) Load(loader *ConfigLoader) (*Config, error) {
	return loader.LoadFromEnv()
}

func (e *EnvConfigSource) IsRequired() bool {
	return e.Required
}

// ReaderConfigSource loads config from an io.Reader with specified format
type ReaderConfigSource struct {
	Reader   io.Reader
	Format   string // "json" or "yaml"
	Required bool
}

func (r *ReaderConfigSource) Load(loader *ConfigLoader) (*Config, error) {
	switch strings.ToLower(r.Format) {
	case "json":
		return loader.LoadFromJSON(r.Reader)
	case "yaml", "yml":
		return loader.LoadFromYAML(r.Reader)
	default:
		return nil, fmt.Errorf("unsupported format: %s", r.Format)
	}
}

func (r *ReaderConfigSource) IsRequired() bool {
	return r.Required
}
