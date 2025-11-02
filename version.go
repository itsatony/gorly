// Package ratelimit version information using go-version
package ratelimit

import (
	"context"
	"fmt"
	"net/http"

	version "github.com/itsatony/go-version"
)

var (
	// versionInfo stores the lazily initialized version information
	versionInfo *version.Info

	// initError stores any initialization error
	initError error
)

// InitializeVersion initializes the version information from the manifest
// This should be called once during application startup
func InitializeVersion(opts ...version.Option) error {
	// Add manifest path and git/build info by default
	defaultOpts := []version.Option{
		version.WithManifestPath("versions.yaml"),
		version.WithGitInfo(),
		version.WithBuildInfo(),
	}

	// Append user-provided options
	allOpts := append(defaultOpts, opts...)

	err := version.Initialize(allOpts...)
	if err != nil {
		initError = err
		return fmt.Errorf("failed to initialize version: %w", err)
	}

	versionInfo = version.MustGet()
	return nil
}

// GetVersion returns the current version string
func GetVersion() string {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo != nil {
		return versionInfo.Project.Version
	}
	return "unknown"
}

// GetVersionInfo returns comprehensive version information
// Initializes version info if not already done
func GetVersionInfo() *version.Info {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	return versionInfo
}

// MustGetVersionInfo returns version information or panics if unavailable
func MustGetVersionInfo() *version.Info {
	info := GetVersionInfo()
	if info == nil {
		panic("version information not available")
	}
	return info
}

// GetProjectName returns the project name
func GetProjectName() string {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo != nil {
		return versionInfo.Project.Name
	}
	return "gorly"
}

// GetGitCommit returns the git commit hash
func GetGitCommit() string {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo != nil && versionInfo.Git.Commit != "" {
		return versionInfo.Git.Commit
	}
	return "unknown"
}

// GetAPIVersion returns the version of a specific API
func GetAPIVersion(apiName string) (string, error) {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo == nil {
		return "", fmt.Errorf("version information not available")
	}

	v, exists := versionInfo.GetAPIVersion(apiName)
	if !exists {
		return "", fmt.Errorf("API '%s' not found in manifest", apiName)
	}

	return v, nil
}

// GetComponentVersion returns the version of a specific component
func GetComponentVersion(componentName string) (string, error) {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo == nil {
		return "", fmt.Errorf("version information not available")
	}

	v, exists := versionInfo.GetComponentVersion(componentName)
	if !exists {
		return "", fmt.Errorf("component '%s' not found in manifest", componentName)
	}

	return v, nil
}

// GetSchemaVersion returns the version of a specific schema
func GetSchemaVersion(schemaName string) (string, error) {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo == nil {
		return "", fmt.Errorf("version information not available")
	}

	v, exists := versionInfo.GetSchemaVersion(schemaName)
	if !exists {
		return "", fmt.Errorf("schema '%s' not found in manifest", schemaName)
	}

	return v, nil
}

// FormatVersionString returns a formatted version string with additional info
func FormatVersionString() string {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo == nil {
		return "gorly version unknown"
	}

	result := fmt.Sprintf("%s v%s", versionInfo.Project.Name, versionInfo.Project.Version)

	if versionInfo.Git.Commit != "" && versionInfo.Git.Commit != "unknown" {
		// Show first 8 characters of commit hash
		commitHash := versionInfo.Git.Commit
		if len(commitHash) > 8 {
			commitHash = commitHash[:8]
		}
		result += fmt.Sprintf(" (commit: %s)", commitHash)
	}

	if versionInfo.Build.Time != "" {
		result += fmt.Sprintf(" built: %s", versionInfo.Build.Time)
	}

	return result
}

// VersionHandler returns an HTTP handler that exposes version information
// This uses the go-version package's built-in handler
func VersionHandler() http.Handler {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	return version.Handler()
}

// VersionHandlerFunc returns an HTTP handler function that exposes version information
func VersionHandlerFunc() http.HandlerFunc {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	return version.HandlerFunc()
}

// VersionMiddleware returns middleware that adds version information to the context
func VersionMiddleware(next http.Handler) http.Handler {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	return version.Middleware(next)
}

// HealthHandler returns an HTTP handler for health checks with version info
func HealthHandler() http.Handler {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	return version.HealthHandler()
}

// ValidateVersions validates that all components meet minimum version requirements
// This is useful for ensuring compatibility during initialization
func ValidateVersions(ctx context.Context, validators ...version.Validator) error {
	if versionInfo == nil {
		_ = InitializeVersion()
	}
	if versionInfo == nil {
		return fmt.Errorf("version information not available")
	}

	for _, validator := range validators {
		if err := validator.Validate(ctx, versionInfo); err != nil {
			return fmt.Errorf("version validation failed: %w", err)
		}
	}

	return nil
}

// ResetVersion resets the version information (useful for testing)
func ResetVersion() {
	version.Reset()
	versionInfo = nil
	initError = nil
}
