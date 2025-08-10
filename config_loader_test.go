// config_loader_test.go
package ratelimit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigLoader_LoadFromJSON(t *testing.T) {
	jsonConfig := `{
		"enabled": true,
		"algorithm": "sliding_window",
		"store": "redis",
		"keyPrefix": "test:",
		"enableMetrics": true,
		"metricsPrefix": "gorly_test_",
		"operationTimeout": "10s",
		"redis": {
			"address": "localhost:6379",
			"password": "secret",
			"database": 1,
			"poolSize": 20,
			"timeout": "5s",
			"tls": true
		},
		"defaultLimits": {
			"global": "100/1m"
		},
		"scopeLimits": {
			"memory": {
				"requests": 50,
				"window": "30s"
			}
		},
		"tierLimits": {
			"premium": {
				"defaultLimits": {
					"global": "1000/1m"
				}
			}
		},
		"entityOverrides": {
			"user:vip123": {
				"global": "500/1m"
			}
		}
	}`

	loader := NewConfigLoader()
	config, err := loader.LoadFromJSON(strings.NewReader(jsonConfig))
	if err != nil {
		t.Fatalf("Failed to load JSON config: %v", err)
	}

	// Test basic settings
	if !config.Enabled {
		t.Error("Expected enabled to be true")
	}

	if config.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window', got '%s'", config.Algorithm)
	}

	if config.Store != "redis" {
		t.Errorf("Expected store 'redis', got '%s'", config.Store)
	}

	if config.KeyPrefix != "test:" {
		t.Errorf("Expected keyPrefix 'test:', got '%s'", config.KeyPrefix)
	}

	if !config.EnableMetrics {
		t.Error("Expected enableMetrics to be true")
	}

	if config.MetricsPrefix != "gorly_test_" {
		t.Errorf("Expected metricsPrefix 'gorly_test_', got '%s'", config.MetricsPrefix)
	}

	if config.OperationTimeout != 10*time.Second {
		t.Errorf("Expected operationTimeout 10s, got %v", config.OperationTimeout)
	}

	// Test Redis config
	if config.Redis.Address != "localhost:6379" {
		t.Errorf("Expected Redis address 'localhost:6379', got '%s'", config.Redis.Address)
	}

	if config.Redis.Password != "secret" {
		t.Errorf("Expected Redis password 'secret', got '%s'", config.Redis.Password)
	}

	if config.Redis.Database != 1 {
		t.Errorf("Expected Redis database 1, got %d", config.Redis.Database)
	}

	if config.Redis.PoolSize != 20 {
		t.Errorf("Expected Redis poolSize 20, got %d", config.Redis.PoolSize)
	}

	if config.Redis.Timeout != 5*time.Second {
		t.Errorf("Expected Redis timeout 5s, got %v", config.Redis.Timeout)
	}

	if !config.Redis.TLS {
		t.Error("Expected Redis TLS to be true")
	}

	// Test default limits
	if globalLimit, exists := config.DefaultLimits[ScopeGlobal]; !exists {
		t.Error("Expected global default limit to exist")
	} else {
		if globalLimit.Requests != 100 {
			t.Errorf("Expected global limit 100 requests, got %d", globalLimit.Requests)
		}
		if globalLimit.Window != time.Minute {
			t.Errorf("Expected global limit window 1m, got %v", globalLimit.Window)
		}
	}

	// Test scope limits
	if memoryLimit, exists := config.ScopeLimits[ScopeMemory]; !exists {
		t.Error("Expected memory scope limit to exist")
	} else {
		if memoryLimit.Requests != 50 {
			t.Errorf("Expected memory limit 50 requests, got %d", memoryLimit.Requests)
		}
		if memoryLimit.Window != 30*time.Second {
			t.Errorf("Expected memory limit window 30s, got %v", memoryLimit.Window)
		}
	}

	// Test tier limits
	if premiumTier, exists := config.TierLimits["premium"]; !exists {
		t.Error("Expected premium tier to exist")
	} else {
		if globalLimit, exists := premiumTier.DefaultLimits[ScopeGlobal]; !exists {
			t.Error("Expected premium tier global limit to exist")
		} else {
			if globalLimit.Requests != 1000 {
				t.Errorf("Expected premium global limit 1000 requests, got %d", globalLimit.Requests)
			}
		}
	}

	// Test entity overrides
	if userOverride, exists := config.EntityOverrides["user:vip123"]; !exists {
		t.Error("Expected user:vip123 overrides to exist")
	} else {
		if globalLimit, exists := userOverride.Limits[ScopeGlobal]; !exists {
			t.Error("Expected user:vip123 global override to exist")
		} else {
			if globalLimit.Requests != 500 {
				t.Errorf("Expected user:vip123 global override 500 requests, got %d", globalLimit.Requests)
			}
		}
	}
}

func TestConfigLoader_LoadFromYAML(t *testing.T) {
	yamlConfig := `
enabled: true
algorithm: token_bucket
store: memory
keyPrefix: "yaml_test:"
enableMetrics: false
redis:
  address: "redis.example.com:6380"
  database: 2
  tls: false
defaultLimits:
  global: "200/1h"
  memory: "50/1m"
scopeLimits:
  search:
    requests: 25
    window: "15s"
`

	loader := NewConfigLoader()
	config, err := loader.LoadFromYAML(strings.NewReader(yamlConfig))
	if err != nil {
		t.Fatalf("Failed to load YAML config: %v", err)
	}

	// Test basic settings
	if !config.Enabled {
		t.Error("Expected enabled to be true")
	}

	if config.Algorithm != "token_bucket" {
		t.Errorf("Expected algorithm 'token_bucket', got '%s'", config.Algorithm)
	}

	if config.Store != "memory" {
		t.Errorf("Expected store 'memory', got '%s'", config.Store)
	}

	if config.KeyPrefix != "yaml_test:" {
		t.Errorf("Expected keyPrefix 'yaml_test:', got '%s'", config.KeyPrefix)
	}

	if config.EnableMetrics {
		t.Error("Expected enableMetrics to be false")
	}

	// Test Redis config
	if config.Redis.Address != "redis.example.com:6380" {
		t.Errorf("Expected Redis address 'redis.example.com:6380', got '%s'", config.Redis.Address)
	}

	if config.Redis.Database != 2 {
		t.Errorf("Expected Redis database 2, got %d", config.Redis.Database)
	}

	if config.Redis.TLS {
		t.Error("Expected Redis TLS to be false")
	}

	// Test default limits
	if globalLimit, exists := config.DefaultLimits[ScopeGlobal]; !exists {
		t.Error("Expected global default limit to exist")
	} else {
		if globalLimit.Requests != 200 {
			t.Errorf("Expected global limit 200 requests, got %d", globalLimit.Requests)
		}
		if globalLimit.Window != time.Hour {
			t.Errorf("Expected global limit window 1h, got %v", globalLimit.Window)
		}
	}

	if memoryLimit, exists := config.DefaultLimits[ScopeMemory]; !exists {
		t.Error("Expected memory default limit to exist")
	} else {
		if memoryLimit.Requests != 50 {
			t.Errorf("Expected memory limit 50 requests, got %d", memoryLimit.Requests)
		}
		if memoryLimit.Window != time.Minute {
			t.Errorf("Expected memory limit window 1m, got %v", memoryLimit.Window)
		}
	}

	// Test scope limits
	if searchLimit, exists := config.ScopeLimits[ScopeSearch]; !exists {
		t.Error("Expected search scope limit to exist")
	} else {
		if searchLimit.Requests != 25 {
			t.Errorf("Expected search limit 25 requests, got %d", searchLimit.Requests)
		}
		if searchLimit.Window != 15*time.Second {
			t.Errorf("Expected search limit window 15s, got %v", searchLimit.Window)
		}
	}
}

func TestConfigLoader_LoadFromEnv(t *testing.T) {
	// Set environment variables
	oldVars := map[string]string{
		"GORLY_ENABLED":        os.Getenv("GORLY_ENABLED"),
		"GORLY_ALGORITHM":      os.Getenv("GORLY_ALGORITHM"),
		"GORLY_STORE":          os.Getenv("GORLY_STORE"),
		"GORLY_KEY_PREFIX":     os.Getenv("GORLY_KEY_PREFIX"),
		"GORLY_ENABLE_METRICS": os.Getenv("GORLY_ENABLE_METRICS"),
		"GORLY_REDIS_ADDRESS":  os.Getenv("GORLY_REDIS_ADDRESS"),
		"GORLY_REDIS_DATABASE": os.Getenv("GORLY_REDIS_DATABASE"),
		"GORLY_DEFAULT_LIMIT":  os.Getenv("GORLY_DEFAULT_LIMIT"),
	}

	// Cleanup function
	defer func() {
		for key, value := range oldVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("GORLY_ENABLED", "true")
	os.Setenv("GORLY_ALGORITHM", "sliding_window")
	os.Setenv("GORLY_STORE", "redis")
	os.Setenv("GORLY_KEY_PREFIX", "env_test:")
	os.Setenv("GORLY_ENABLE_METRICS", "true")
	os.Setenv("GORLY_REDIS_ADDRESS", "env.redis:6379")
	os.Setenv("GORLY_REDIS_DATABASE", "3")
	os.Setenv("GORLY_DEFAULT_LIMIT", "75/1m")

	loader := NewConfigLoader()
	config, err := loader.LoadFromEnv()
	if err != nil {
		t.Fatalf("Failed to load env config: %v", err)
	}

	// Test basic settings
	if !config.Enabled {
		t.Error("Expected enabled to be true")
	}

	if config.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window', got '%s'", config.Algorithm)
	}

	if config.Store != "redis" {
		t.Errorf("Expected store 'redis', got '%s'", config.Store)
	}

	if config.KeyPrefix != "env_test:" {
		t.Errorf("Expected keyPrefix 'env_test:', got '%s'", config.KeyPrefix)
	}

	if !config.EnableMetrics {
		t.Error("Expected enableMetrics to be true")
	}

	// Test Redis config
	if config.Redis.Address != "env.redis:6379" {
		t.Errorf("Expected Redis address 'env.redis:6379', got '%s'", config.Redis.Address)
	}

	if config.Redis.Database != 3 {
		t.Errorf("Expected Redis database 3, got %d", config.Redis.Database)
	}

	// Test default limit
	if globalLimit, exists := config.DefaultLimits[ScopeGlobal]; !exists {
		t.Error("Expected global default limit to exist")
	} else {
		if globalLimit.Requests != 75 {
			t.Errorf("Expected global limit 75 requests, got %d", globalLimit.Requests)
		}
		if globalLimit.Window != time.Minute {
			t.Errorf("Expected global limit window 1m, got %v", globalLimit.Window)
		}
	}
}

func TestConfigLoader_LoadFromFile(t *testing.T) {
	// Test JSON file
	jsonContent := `{"enabled": true, "algorithm": "token_bucket"}`
	jsonFile := createTempFile(t, "config.json", jsonContent)
	defer os.Remove(jsonFile.Name())

	loader := NewConfigLoader()
	config, err := loader.LoadFromFile(jsonFile.Name())
	if err != nil {
		t.Fatalf("Failed to load JSON file: %v", err)
	}

	if !config.Enabled {
		t.Error("Expected enabled to be true")
	}

	if config.Algorithm != "token_bucket" {
		t.Errorf("Expected algorithm 'token_bucket', got '%s'", config.Algorithm)
	}

	// Test YAML file
	yamlContent := `enabled: false
algorithm: sliding_window`
	yamlFile := createTempFile(t, "config.yaml", yamlContent)
	defer os.Remove(yamlFile.Name())

	config, err = loader.LoadFromFile(yamlFile.Name())
	if err != nil {
		t.Fatalf("Failed to load YAML file: %v", err)
	}

	if config.Enabled {
		t.Error("Expected enabled to be false")
	}

	if config.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window', got '%s'", config.Algorithm)
	}

	// Test unsupported file extension
	txtFile := createTempFile(t, "config.txt", "some content")
	defer os.Remove(txtFile.Name())

	_, err = loader.LoadFromFile(txtFile.Name())
	if err == nil {
		t.Error("Expected error for unsupported file format")
	}

	// Test non-existent file
	_, err = loader.LoadFromFile("non_existent_file.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestConfigLoader_LoadFromMultipleSources(t *testing.T) {
	// Create base config file
	baseContent := `{
		"enabled": true,
		"algorithm": "token_bucket",
		"defaultLimits": {
			"global": "100/1m"
		}
	}`
	baseFile := createTempFile(t, "base.json", baseContent)
	defer os.Remove(baseFile.Name())

	// Create override config file
	overrideContent := `{
		"algorithm": "sliding_window",
		"defaultLimits": {
			"memory": "50/1m"
		}
	}`
	overrideFile := createTempFile(t, "override.json", overrideContent)
	defer os.Remove(overrideFile.Name())

	loader := NewConfigLoader()

	sources := []ConfigSource{
		&FileConfigSource{Filename: baseFile.Name(), Required: true},
		&FileConfigSource{Filename: overrideFile.Name(), Required: false},
	}

	config, err := loader.LoadFromMultipleSources(sources...)
	if err != nil {
		t.Fatalf("Failed to load from multiple sources: %v", err)
	}

	// Should have enabled from base
	if !config.Enabled {
		t.Error("Expected enabled to be true from base config")
	}

	// Should have algorithm from override (last wins)
	if config.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window' from override, got '%s'", config.Algorithm)
	}

	// Should have both limits merged
	if _, exists := config.DefaultLimits[ScopeGlobal]; !exists {
		t.Error("Expected global limit from base config")
	}

	if _, exists := config.DefaultLimits[ScopeMemory]; !exists {
		t.Error("Expected memory limit from override config")
	}
}

func TestConfigLoader_MergeConfigs(t *testing.T) {
	loader := NewConfigLoader()

	base := DefaultConfig()
	base.Algorithm = "token_bucket"
	base.DefaultLimits = map[string]RateLimit{
		ScopeGlobal: {Requests: 100, Window: time.Minute},
	}

	override := DefaultConfig()
	override.Algorithm = "sliding_window"
	override.DefaultLimits = map[string]RateLimit{
		ScopeMemory: {Requests: 50, Window: 30 * time.Second},
	}

	err := loader.mergeConfigs(base, override)
	if err != nil {
		t.Fatalf("Failed to merge configs: %v", err)
	}

	// Algorithm should be overridden
	if base.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window', got '%s'", base.Algorithm)
	}

	// Both limits should exist
	if _, exists := base.DefaultLimits[ScopeGlobal]; !exists {
		t.Error("Expected global limit to be preserved")
	}

	if _, exists := base.DefaultLimits[ScopeMemory]; !exists {
		t.Error("Expected memory limit to be added")
	}
}

func TestConfigLoader_ErrorHandling(t *testing.T) {
	loader := NewConfigLoader()

	// Test invalid JSON
	_, err := loader.LoadFromJSON(strings.NewReader(`{"invalid": json}`))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}

	// Test invalid YAML
	_, err = loader.LoadFromYAML(strings.NewReader("invalid:\n  yaml:\n    missing: [unclosed"))
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}

	// Test empty filename
	_, err = loader.LoadFromFile("")
	if err == nil {
		t.Error("Expected error for empty filename")
	}

	// Test required source failure
	sources := []ConfigSource{
		&FileConfigSource{Filename: "non_existent.json", Required: true},
	}

	_, err = loader.LoadFromMultipleSources(sources...)
	if err == nil {
		t.Error("Expected error for failed required source")
	}
}

// Helper function to create temporary files for testing
func createTempFile(t *testing.T, name, content string) *os.File {
	// Create temp file with proper extension
	tempDir := t.TempDir()
	fullPath := filepath.Join(tempDir, name)

	file, err := os.Create(fullPath)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := file.WriteString(content); err != nil {
		file.Close()
		t.Fatalf("Failed to write temp file: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Reopen for reading
	file, err = os.Open(fullPath)
	if err != nil {
		t.Fatalf("Failed to reopen temp file: %v", err)
	}

	return file
}

func TestConfigSource_Implementations(t *testing.T) {
	// Test FileConfigSource
	jsonContent := `{"enabled": true}`
	jsonFile := createTempFile(t, "source.json", jsonContent)
	defer os.Remove(jsonFile.Name())

	fileSource := &FileConfigSource{
		Filename: jsonFile.Name(),
		Required: true,
	}

	loader := NewConfigLoader()
	config, err := fileSource.Load(loader)
	if err != nil {
		t.Fatalf("FileConfigSource load failed: %v", err)
	}

	if !config.Enabled {
		t.Error("Expected enabled to be true")
	}

	if !fileSource.IsRequired() {
		t.Error("Expected FileConfigSource to be required")
	}

	// Test EnvConfigSource
	os.Setenv("GORLY_ENABLED", "false")
	defer os.Unsetenv("GORLY_ENABLED")

	envSource := &EnvConfigSource{Required: false}
	config, err = envSource.Load(loader)
	if err != nil {
		t.Fatalf("EnvConfigSource load failed: %v", err)
	}

	if config.Enabled {
		t.Error("Expected enabled to be false from env")
	}

	if envSource.IsRequired() {
		t.Error("Expected EnvConfigSource to not be required")
	}

	// Test ReaderConfigSource
	readerSource := &ReaderConfigSource{
		Reader:   strings.NewReader(`{"algorithm": "sliding_window"}`),
		Format:   "json",
		Required: true,
	}

	config, err = readerSource.Load(loader)
	if err != nil {
		t.Fatalf("ReaderConfigSource load failed: %v", err)
	}

	if config.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm 'sliding_window', got '%s'", config.Algorithm)
	}

	if !readerSource.IsRequired() {
		t.Error("Expected ReaderConfigSource to be required")
	}

	// Test ReaderConfigSource with invalid format
	invalidReaderSource := &ReaderConfigSource{
		Reader:   strings.NewReader("content"),
		Format:   "invalid",
		Required: true,
	}

	_, err = invalidReaderSource.Load(loader)
	if err == nil {
		t.Error("Expected error for invalid format")
	}
}
