package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	version "github.com/itsatony/go-version"
)

func TestInitializeVersion(t *testing.T) {
	defer ResetVersion()

	err := InitializeVersion()
	if err != nil {
		t.Fatalf("InitializeVersion failed: %v", err)
	}

	if versionInfo == nil {
		t.Error("versionInfo should be initialized")
	}
}

func TestGetVersion(t *testing.T) {
	defer ResetVersion()

	v := GetVersion()
	if v == "" {
		t.Error("GetVersion returned empty string")
	}
	if v == "unknown" {
		t.Error("GetVersion returned 'unknown', expected valid version")
	}

	// Should be semver format
	if !strings.Contains(v, ".") {
		t.Errorf("GetVersion returned invalid semver format: %s", v)
	}
}

func TestGetVersionInfo(t *testing.T) {
	defer ResetVersion()

	info := GetVersionInfo()
	if info == nil {
		t.Fatal("GetVersionInfo returned nil")
	}

	if info.Project.Name != "gorly" {
		t.Errorf("Expected project name 'gorly', got '%s'", info.Project.Name)
	}

	if info.Project.Version == "" {
		t.Error("Project version should not be empty")
	}
}

func TestMustGetVersionInfo(t *testing.T) {
	defer ResetVersion()

	// Should not panic with valid initialization
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustGetVersionInfo panicked unexpectedly: %v", r)
		}
	}()

	info := MustGetVersionInfo()
	if info == nil {
		t.Error("MustGetVersionInfo returned nil without panicking")
	}
}

func TestGetProjectName(t *testing.T) {
	defer ResetVersion()

	name := GetProjectName()
	if name == "" {
		t.Error("GetProjectName returned empty string")
	}
	if name != "gorly" {
		t.Errorf("Expected 'gorly', got '%s'", name)
	}
}

func TestGetGitCommit(t *testing.T) {
	defer ResetVersion()

	commit := GetGitCommit()
	if commit == "" {
		t.Error("GetGitCommit returned empty string")
	}
	// Should return "unknown" or a valid commit hash
	if commit != "unknown" && len(commit) < 7 {
		t.Errorf("GetGitCommit returned invalid commit: %s", commit)
	}
}

func TestGetAPIVersion(t *testing.T) {
	defer ResetVersion()

	tests := []struct {
		name        string
		apiName     string
		expectError bool
	}{
		{
			name:        "valid API",
			apiName:     "http_middleware",
			expectError: false,
		},
		{
			name:        "non-existent API",
			apiName:     "nonexistent_api",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := GetAPIVersion(tt.apiName)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for non-existent API")
				}
			} else {
				if err != nil {
					t.Errorf("GetAPIVersion failed: %v", err)
				}
				if v == "" {
					t.Error("API version should not be empty")
				}
			}
		})
	}
}

func TestGetComponentVersion(t *testing.T) {
	defer ResetVersion()

	tests := []struct {
		name          string
		componentName string
		expectError   bool
	}{
		{
			name:          "algorithms component",
			componentName: "algorithms",
			expectError:   false,
		},
		{
			name:          "stores component",
			componentName: "stores",
			expectError:   false,
		},
		{
			name:          "middleware component",
			componentName: "middleware",
			expectError:   false,
		},
		{
			name:          "non-existent component",
			componentName: "nonexistent",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := GetComponentVersion(tt.componentName)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for non-existent component")
				}
			} else {
				if err != nil {
					t.Errorf("GetComponentVersion failed: %v", err)
				}
				if v == "" {
					t.Error("Component version should not be empty")
				}
			}
		})
	}
}

func TestGetSchemaVersion(t *testing.T) {
	defer ResetVersion()

	tests := []struct {
		name        string
		schemaName  string
		expectError bool
	}{
		{
			name:        "rate_limit_config schema",
			schemaName:  "rate_limit_config",
			expectError: false,
		},
		{
			name:        "non-existent schema",
			schemaName:  "nonexistent_schema",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := GetSchemaVersion(tt.schemaName)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error for non-existent schema")
				}
			} else {
				if err != nil {
					t.Errorf("GetSchemaVersion failed: %v", err)
				}
				if v == "" {
					t.Error("Schema version should not be empty")
				}
			}
		})
	}
}

func TestFormatVersionString(t *testing.T) {
	defer ResetVersion()

	formatted := FormatVersionString()
	if formatted == "" {
		t.Error("FormatVersionString returned empty string")
	}

	// Should contain project name
	if !strings.Contains(formatted, "gorly") {
		t.Errorf("Expected formatted string to contain 'gorly', got: %s", formatted)
	}

	// Should contain version number
	if !strings.Contains(formatted, "v") {
		t.Errorf("Expected formatted string to contain version, got: %s", formatted)
	}
}

func TestVersionHandler(t *testing.T) {
	defer ResetVersion()

	handler := VersionHandler()
	if handler == nil {
		t.Fatal("VersionHandler returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Response body should not be empty")
	}

	// Should contain JSON with version info
	if !strings.Contains(body, "version") {
		t.Errorf("Expected JSON response to contain 'version', got: %s", body)
	}
}

func TestVersionHandlerFunc(t *testing.T) {
	defer ResetVersion()

	handlerFunc := VersionHandlerFunc()
	if handlerFunc == nil {
		t.Fatal("VersionHandlerFunc returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()

	handlerFunc(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestVersionMiddleware(t *testing.T) {
	defer ResetVersion()

	// Create a test handler
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := VersionMiddleware(nextHandler)
	if middleware == nil {
		t.Fatal("VersionMiddleware returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHealthHandler(t *testing.T) {
	defer ResetVersion()

	handler := HealthHandler()
	if handler == nil {
		t.Fatal("HealthHandler returned nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Response body should not be empty")
	}
}

func TestValidateVersions(t *testing.T) {
	defer ResetVersion()

	ctx := context.Background()

	// Test with valid validators
	t.Run("valid component validator", func(t *testing.T) {
		validator := version.NewComponentValidator("algorithms", "0.1.0")
		err := ValidateVersions(ctx, validator)
		if err != nil {
			t.Errorf("ValidateVersions failed with valid validator: %v", err)
		}
	})

	// Test with strict version requirement
	t.Run("component with minimum version", func(t *testing.T) {
		validator := version.NewComponentValidator("stores", "0.5.0")
		err := ValidateVersions(ctx, validator)
		if err != nil {
			t.Errorf("ValidateVersions failed: %v", err)
		}
	})
}

func TestResetVersion(t *testing.T) {
	// Initialize version first
	_ = InitializeVersion()
	if versionInfo == nil {
		t.Fatal("versionInfo should be initialized")
	}

	// Reset
	ResetVersion()

	if versionInfo != nil {
		t.Error("versionInfo should be nil after Reset")
	}
	if initError != nil {
		t.Error("initError should be nil after Reset")
	}
}

func TestVersionInitializationLazy(t *testing.T) {
	defer ResetVersion()

	// Don't explicitly initialize, let it happen lazily
	v := GetVersion()
	if v == "" || v == "unknown" {
		t.Error("Lazy initialization should have occurred")
	}

	if versionInfo == nil {
		t.Error("versionInfo should be initialized after lazy load")
	}
}

func TestMultipleInitializations(t *testing.T) {
	defer ResetVersion()

	// First initialization
	err1 := InitializeVersion()
	if err1 != nil {
		t.Fatalf("First initialization failed: %v", err1)
	}

	v1 := GetVersion()

	// Reset before second initialization (singleton pattern requires this)
	ResetVersion()

	// Second initialization (after reset)
	err2 := InitializeVersion()
	if err2 != nil {
		t.Errorf("Second initialization failed: %v", err2)
	}

	v2 := GetVersion()

	if v1 != v2 {
		t.Error("Version should be consistent across multiple initializations")
	}
}
