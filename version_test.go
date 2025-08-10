package ratelimit

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetVersion(t *testing.T) {
	version := GetVersion()

	if version == "" {
		t.Error("Version should not be empty")
	}

	// Version should follow semantic versioning pattern
	if !strings.Contains(version, ".") {
		t.Error("Version should contain dots (semantic versioning)")
	}

	if version != Version {
		t.Errorf("GetVersion() should return Version constant, got %s, expected %s", version, Version)
	}
}

func TestGetVersionInfo(t *testing.T) {
	info := GetVersionInfo()

	if info == nil {
		t.Fatal("GetVersionInfo() should not return nil")
	}

	// Check required fields
	if info.Version == "" {
		t.Error("VersionInfo.Version should not be empty")
	}

	if info.Name == "" {
		t.Error("VersionInfo.Name should not be empty")
	}

	if info.Description == "" {
		t.Error("VersionInfo.Description should not be empty")
	}

	if info.GoVersion == "" {
		t.Error("VersionInfo.GoVersion should not be empty")
	}

	// Verify Go version format
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Error("GoVersion should start with 'go'")
	}

	// Check that GoVersion matches runtime
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion should match runtime.Version(), got %s, expected %s",
			info.GoVersion, runtime.Version())
	}
}

func TestVersionInfoString(t *testing.T) {
	info := GetVersionInfo()
	str := info.String()

	if str == "" {
		t.Error("VersionInfo.String() should not be empty")
	}

	// Should contain key information
	if !strings.Contains(str, info.Name) {
		t.Error("String representation should contain name")
	}

	if !strings.Contains(str, info.Version) {
		t.Error("String representation should contain version")
	}

	if !strings.Contains(str, info.GoVersion) {
		t.Error("String representation should contain Go version")
	}
}

func TestVersionInfoBanner(t *testing.T) {
	info := GetVersionInfo()
	banner := info.Banner()

	if banner == "" {
		t.Error("VersionInfo.Banner() should not be empty")
	}

	// Should contain key information
	if !strings.Contains(banner, info.Name) {
		t.Error("Banner should contain name")
	}

	if !strings.Contains(banner, info.Version) {
		t.Error("Banner should contain version")
	}

	if !strings.Contains(banner, info.Description) {
		t.Error("Banner should contain description")
	}

	// Should contain emoji/styling
	if !strings.Contains(banner, "🚀") {
		t.Error("Banner should contain rocket emoji for styling")
	}

	if !strings.Contains(banner, "✨") {
		t.Error("Banner should contain sparkles emoji")
	}
}

func TestVersionConstants(t *testing.T) {
	// Test that constants are set to reasonable values
	if Version == "" {
		t.Error("Version constant should not be empty")
	}

	if Name == "" {
		t.Error("Name constant should not be empty")
	}

	if Description == "" {
		t.Error("Description constant should not be empty")
	}

	// Version should follow semantic versioning
	parts := strings.Split(Version, ".")
	if len(parts) != 3 {
		t.Errorf("Version should have 3 parts (semantic versioning), got %d parts in %s", len(parts), Version)
	}
}

func TestBuildTimeVariables(t *testing.T) {
	// These will be "unknown" in tests since they're set at build time
	commit := GetGitCommit()
	buildTime := GetBuildTime()
	buildUser := GetBuildUser()

	// Should at least return something (even if "unknown")
	if commit == "" {
		t.Error("GetGitCommit() should return a value (even if 'unknown')")
	}

	if buildTime == "" {
		t.Error("GetBuildTime() should return a value (even if 'unknown')")
	}

	if buildUser == "" {
		t.Error("GetBuildUser() should return a value (even if 'unknown')")
	}
}

func TestVersionStringFunction(t *testing.T) {
	// Test the exported VersionString function variable
	version := VersionString()

	if version != GetVersion() {
		t.Errorf("VersionString() should return same as GetVersion(), got %s, expected %s",
			version, GetVersion())
	}
}

func TestInfoFunction(t *testing.T) {
	// Test the exported Info function variable
	info := Info()

	if info == nil {
		t.Fatal("Info() should not return nil")
	}

	expected := GetVersionInfo()
	if info.Version != expected.Version {
		t.Errorf("Info() should return same as GetVersionInfo(), got version %s, expected %s",
			info.Version, expected.Version)
	}
}

// Test version functionality under different build scenarios
func TestVersionWithBuildInfo(t *testing.T) {
	// Save original values
	originalCommit := gitCommit
	originalBuildTime := buildTime
	originalBuildUser := buildUser

	// Test with simulated build info
	gitCommit = "abc123def456"
	buildTime = "2024-01-15T10:30:00Z"
	buildUser = "ci-system"

	info := GetVersionInfo()

	if info.GitCommit != "abc123def456" {
		t.Errorf("Expected GitCommit to be abc123def456, got %s", info.GitCommit)
	}

	if info.BuildTime != "2024-01-15T10:30:00Z" {
		t.Errorf("Expected BuildTime to be 2024-01-15T10:30:00Z, got %s", info.BuildTime)
	}

	if info.BuildUser != "ci-system" {
		t.Errorf("Expected BuildUser to be ci-system, got %s", info.BuildUser)
	}

	// Test string representation includes build info
	str := info.String()
	if !strings.Contains(str, "abc123d") { // Shortened commit
		t.Error("String representation should contain shortened commit hash")
	}

	if !strings.Contains(str, "2024-01-15T10:30:00Z") {
		t.Error("String representation should contain build time")
	}

	if !strings.Contains(str, "ci-system") {
		t.Error("String representation should contain build user")
	}

	// Test banner includes build info
	banner := info.Banner()
	if !strings.Contains(banner, "abc123d") {
		t.Error("Banner should contain shortened commit hash")
	}

	// Restore original values
	gitCommit = originalCommit
	buildTime = originalBuildTime
	buildUser = originalBuildUser
}
