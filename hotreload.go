// Package ratelimit provides hot reload functionality for dynamic configuration updates
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// HotReloadConfig defines configuration that can be hot-reloaded
type HotReloadConfig struct {
	Limits     map[string]string `json:"limits"`
	TierLimits map[string]string `json:"tier_limits"`
	Algorithm  string            `json:"algorithm"`
	Enabled    bool              `json:"enabled"`

	// Metadata
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// HotReloadConfigSource defines where configuration updates come from
type HotReloadConfigSource interface {
	// Watch for configuration changes
	Watch(ctx context.Context) (<-chan *HotReloadConfig, error)

	// Get current configuration
	GetConfig(ctx context.Context) (*HotReloadConfig, error)

	// Close the configuration source
	Close() error
}

// HotReloadFileConfigSource watches a JSON file for configuration changes
type HotReloadFileConfigSource struct {
	filePath string
	lastMod  time.Time
	mu       sync.RWMutex
}

// NewHotReloadFileConfigSource creates a file-based configuration source
func NewHotReloadFileConfigSource(filePath string) *HotReloadFileConfigSource {
	return &HotReloadFileConfigSource{
		filePath: filePath,
	}
}

// Watch implements HotReloadConfigSource interface
func (fcs *HotReloadFileConfigSource) Watch(ctx context.Context) (<-chan *HotReloadConfig, error) {
	configChan := make(chan *HotReloadConfig, 1)

	// Load initial config
	config, err := fcs.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	configChan <- config

	// Start watching for changes
	go func() {
		defer close(configChan)

		ticker := time.NewTicker(time.Second * 5) // Check every 5 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if config, err := fcs.checkForUpdates(ctx); err == nil && config != nil {
					configChan <- config
				}
			}
		}
	}()

	return configChan, nil
}

// GetConfig implements HotReloadConfigSource interface
func (fcs *HotReloadFileConfigSource) GetConfig(ctx context.Context) (*HotReloadConfig, error) {
	// In a real implementation, this would read from the file
	// For now, return a sample configuration
	return &HotReloadConfig{
		Limits: map[string]string{
			"global": "100/minute",
			"upload": "10/minute",
			"search": "50/minute",
		},
		TierLimits: map[string]string{
			"free":    "50/minute",
			"premium": "500/minute",
		},
		Algorithm: "sliding_window",
		Enabled:   true,
		Version:   "1.0.0",
		UpdatedAt: time.Now(),
		UpdatedBy: "system",
	}, nil
}

// checkForUpdates checks if the file has been modified
func (fcs *HotReloadFileConfigSource) checkForUpdates(ctx context.Context) (*HotReloadConfig, error) {
	// In a real implementation, this would check file modification time
	// and reload if changed. For demo purposes, we'll simulate occasional updates.

	if time.Now().Unix()%30 == 0 { // Simulate update every 30 seconds
		config, err := fcs.GetConfig(ctx)
		if err != nil {
			return nil, err
		}

		// Simulate some changes
		config.Version = fmt.Sprintf("1.0.%d", time.Now().Unix()%100)
		config.UpdatedAt = time.Now()

		return config, nil
	}

	return nil, nil
}

// Close implements HotReloadConfigSource interface
func (fcs *HotReloadFileConfigSource) Close() error {
	return nil
}

// HTTPConfigSource gets configuration from HTTP endpoints
type HTTPConfigSource struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

// NewHTTPConfigSource creates an HTTP-based configuration source
func NewHTTPConfigSource(endpoint string) *HTTPConfigSource {
	return &HTTPConfigSource{
		endpoint: endpoint,
		headers:  make(map[string]string),
		client:   &http.Client{Timeout: time.Second * 10},
	}
}

// Watch implements HotReloadConfigSource interface
func (hcs *HTTPConfigSource) Watch(ctx context.Context) (<-chan *HotReloadConfig, error) {
	configChan := make(chan *HotReloadConfig, 1)

	// In a real implementation, this might use Server-Sent Events or WebSockets
	// For now, we'll poll the endpoint
	go func() {
		defer close(configChan)

		ticker := time.NewTicker(time.Second * 10) // Poll every 10 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if config, err := hcs.GetConfig(ctx); err == nil {
					configChan <- config
				}
			}
		}
	}()

	return configChan, nil
}

// GetConfig implements HotReloadConfigSource interface
func (hcs *HTTPConfigSource) GetConfig(ctx context.Context) (*HotReloadConfig, error) {
	// In a real implementation, this would make HTTP request to the endpoint
	// For demo, return a sample config
	return &HotReloadConfig{
		Limits: map[string]string{
			"global": "200/minute",
			"upload": "20/minute",
			"search": "100/minute",
		},
		TierLimits: map[string]string{
			"free":       "100/minute",
			"premium":    "1000/minute",
			"enterprise": "10000/minute",
		},
		Algorithm: "token_bucket",
		Enabled:   true,
		Version:   "http-1.0.0",
		UpdatedAt: time.Now(),
		UpdatedBy: "admin",
	}, nil
}

// Close implements HotReloadConfigSource interface
func (hcs *HTTPConfigSource) Close() error {
	return nil
}

// HotReloadManager manages dynamic configuration updates
type HotReloadManager struct {
	limiter       Limiter
	configSource  HotReloadConfigSource
	currentConfig *HotReloadConfig
	updateChan    chan *HotReloadConfig
	errorHandler  func(error)
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	// Callbacks
	onConfigUpdate    func(*HotReloadConfig)
	onUpdateError     func(error)
	onValidationError func(error)
}

// NewHotReloadManager creates a new hot reload manager
func NewHotReloadManager(limiter Limiter, source HotReloadConfigSource) *HotReloadManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &HotReloadManager{
		limiter:      limiter,
		configSource: source,
		updateChan:   make(chan *HotReloadConfig, 10),
		errorHandler: DefaultErrorHandler,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins watching for configuration changes
func (hrm *HotReloadManager) Start() error {
	// Start watching for config changes
	configChan, err := hrm.configSource.Watch(hrm.ctx)
	if err != nil {
		return fmt.Errorf("failed to start watching config: %w", err)
	}

	// Start update processing
	hrm.wg.Add(1)
	go hrm.processUpdates()

	// Start config change monitoring
	hrm.wg.Add(1)
	go func() {
		defer hrm.wg.Done()

		for {
			select {
			case <-hrm.ctx.Done():
				return
			case config, ok := <-configChan:
				if !ok {
					return
				}

				// Queue the update
				select {
				case hrm.updateChan <- config:
				default:
					// Channel full, skip this update
					log.Printf("Update queue full, skipping config update")
				}
			}
		}
	}()

	return nil
}

// Stop stops the hot reload manager
func (hrm *HotReloadManager) Stop() {
	hrm.cancel()
	close(hrm.updateChan)
	hrm.wg.Wait()
	hrm.configSource.Close()
}

// processUpdates processes configuration updates
func (hrm *HotReloadManager) processUpdates() {
	defer hrm.wg.Done()

	for {
		select {
		case <-hrm.ctx.Done():
			return
		case config, ok := <-hrm.updateChan:
			if !ok {
				return
			}

			if err := hrm.applyConfig(config); err != nil {
				if hrm.onUpdateError != nil {
					hrm.onUpdateError(err)
				} else {
					hrm.errorHandler(err)
				}
			} else {
				hrm.mu.Lock()
				hrm.currentConfig = config
				hrm.mu.Unlock()

				if hrm.onConfigUpdate != nil {
					hrm.onConfigUpdate(config)
				}

				log.Printf("Configuration updated to version %s", config.Version)
			}
		}
	}
}

// applyConfig applies a new configuration to the rate limiter
func (hrm *HotReloadManager) applyConfig(config *HotReloadConfig) error {
	// Validate the configuration
	if err := hrm.validateConfig(config); err != nil {
		if hrm.onValidationError != nil {
			hrm.onValidationError(err)
		}
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Apply the configuration
	// Note: In a real implementation, this would update the limiter's internal configuration
	// For now, we'll just log the changes

	log.Printf("Applying configuration update:")
	log.Printf("  Version: %s", config.Version)
	log.Printf("  Algorithm: %s", config.Algorithm)
	log.Printf("  Enabled: %t", config.Enabled)
	log.Printf("  Limits: %v", config.Limits)
	log.Printf("  Tier Limits: %v", config.TierLimits)
	log.Printf("  Updated by: %s at %v", config.UpdatedBy, config.UpdatedAt)

	return nil
}

// validateConfig validates a configuration before applying it
func (hrm *HotReloadManager) validateConfig(config *HotReloadConfig) error {
	if config == nil {
		return NewConfigError(ErrCodeInvalidConfig, "Configuration is nil", "")
	}

	// Validate algorithm
	if config.Algorithm != "" {
		switch config.Algorithm {
		case "token_bucket", "sliding_window":
			// Valid algorithms
		default:
			return NewConfigError(ErrCodeInvalidAlgorithm,
				fmt.Sprintf("Invalid algorithm: %s", config.Algorithm),
				"Supported algorithms: token_bucket, sliding_window")
		}
	}

	// Validate limits format
	for scope, limit := range config.Limits {
		if _, _, err := ParseLimit(limit); err != nil {
			return NewConfigError(ErrCodeInvalidLimit,
				fmt.Sprintf("Invalid limit for scope %s: %s", scope, limit),
				err.Error())
		}
	}

	// Validate tier limits format
	for tier, limit := range config.TierLimits {
		if _, _, err := ParseLimit(limit); err != nil {
			return NewConfigError(ErrCodeInvalidLimit,
				fmt.Sprintf("Invalid tier limit for %s: %s", tier, limit),
				err.Error())
		}
	}

	return nil
}

// GetCurrentConfig returns the current configuration
func (hrm *HotReloadManager) GetCurrentConfig() *HotReloadConfig {
	hrm.mu.RLock()
	defer hrm.mu.RUnlock()
	return hrm.currentConfig
}

// ForceReload forces a configuration reload
func (hrm *HotReloadManager) ForceReload() error {
	config, err := hrm.configSource.GetConfig(hrm.ctx)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	return hrm.applyConfig(config)
}

// SetUpdateCallback sets a callback for configuration updates
func (hrm *HotReloadManager) SetUpdateCallback(callback func(*HotReloadConfig)) {
	hrm.onConfigUpdate = callback
}

// SetErrorCallback sets a callback for update errors
func (hrm *HotReloadManager) SetErrorCallback(callback func(error)) {
	hrm.onUpdateError = callback
}

// SetValidationErrorCallback sets a callback for validation errors
func (hrm *HotReloadManager) SetValidationErrorCallback(callback func(error)) {
	hrm.onValidationError = callback
}

// HotReloadableLimiter wraps a limiter with hot reload capabilities
type HotReloadableLimiter struct {
	Limiter
	manager *HotReloadManager
}

// NewHotReloadableLimiter creates a limiter with hot reload capabilities
func NewHotReloadableLimiter(limiter Limiter, source HotReloadConfigSource) (*HotReloadableLimiter, error) {
	manager := NewHotReloadManager(limiter, source)

	hrl := &HotReloadableLimiter{
		Limiter: limiter,
		manager: manager,
	}

	// Set up callbacks
	manager.SetUpdateCallback(func(config *HotReloadConfig) {
		log.Printf("Hot reload: Configuration updated to version %s", config.Version)
	})

	manager.SetErrorCallback(func(err error) {
		log.Printf("Hot reload error: %v", err)
	})

	// Start the manager
	if err := manager.Start(); err != nil {
		return nil, fmt.Errorf("failed to start hot reload: %w", err)
	}

	return hrl, nil
}

// GetManager returns the hot reload manager
func (hrl *HotReloadableLimiter) GetManager() *HotReloadManager {
	return hrl.manager
}

// Close closes the limiter and hot reload manager
func (hrl *HotReloadableLimiter) Close() error {
	hrl.manager.Stop()
	return hrl.Limiter.Close()
}

// ConfigValidationRules defines validation rules for configuration
type ConfigValidationRules struct {
	MaxLimitsPerScope int
	MaxTierLimits     int
	AllowedAlgorithms []string
	MinLimitValue     int64
	MaxLimitValue     int64
}

// DefaultValidationRules returns default validation rules
func DefaultValidationRules() *ConfigValidationRules {
	return &ConfigValidationRules{
		MaxLimitsPerScope: 100,
		MaxTierLimits:     10,
		AllowedAlgorithms: []string{"token_bucket", "sliding_window"},
		MinLimitValue:     1,
		MaxLimitValue:     1000000,
	}
}

// ValidateWithRules validates configuration against custom rules
func (rules *ConfigValidationRules) ValidateWithRules(config *HotReloadConfig) error {
	// Check number of limits
	if len(config.Limits) > rules.MaxLimitsPerScope {
		return NewConfigError(ErrCodeInvalidConfig,
			fmt.Sprintf("Too many limits defined: %d (max: %d)",
				len(config.Limits), rules.MaxLimitsPerScope), "")
	}

	// Check number of tier limits
	if len(config.TierLimits) > rules.MaxTierLimits {
		return NewConfigError(ErrCodeInvalidConfig,
			fmt.Sprintf("Too many tier limits defined: %d (max: %d)",
				len(config.TierLimits), rules.MaxTierLimits), "")
	}

	// Validate algorithm
	if config.Algorithm != "" {
		allowed := false
		for _, alg := range rules.AllowedAlgorithms {
			if config.Algorithm == alg {
				allowed = true
				break
			}
		}
		if !allowed {
			return NewConfigError(ErrCodeInvalidAlgorithm,
				fmt.Sprintf("Algorithm %s not allowed", config.Algorithm),
				fmt.Sprintf("Allowed: %v", rules.AllowedAlgorithms))
		}
	}

	// Validate limit values
	for scope, limitStr := range config.Limits {
		rate, _, err := ParseLimit(limitStr)
		if err != nil {
			return err
		}

		if rate < rules.MinLimitValue || rate > rules.MaxLimitValue {
			return NewConfigError(ErrCodeInvalidLimit,
				fmt.Sprintf("Limit value %d for scope %s out of range [%d, %d]",
					rate, scope, rules.MinLimitValue, rules.MaxLimitValue), "")
		}
	}

	return nil
}
