// Package ratelimit version information
package ratelimit

import (
	"fmt"
	"runtime"
)

const (
	// Version is the current version of Gorly
	Version = "1.0.0"

	// Name is the library name
	Name = "Gorly"

	// Description is a short description of the library
	Description = "World-class Go rate limiting library with revolutionary developer experience"
)

// VersionInfo contains comprehensive version information
type VersionInfo struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GoVersion   string `json:"go_version"`
	GitCommit   string `json:"git_commit,omitempty"` // Set at build time
	BuildTime   string `json:"build_time,omitempty"` // Set at build time
	BuildUser   string `json:"build_user,omitempty"` // Set at build time
}

// GetVersion returns the current version string
func GetVersion() string {
	return Version
}

// GetVersionInfo returns comprehensive version information
func GetVersionInfo() *VersionInfo {
	return &VersionInfo{
		Version:     Version,
		Name:        Name,
		Description: Description,
		GoVersion:   runtime.Version(),
		GitCommit:   gitCommit, // Set via ldflags at build time
		BuildTime:   buildTime, // Set via ldflags at build time
		BuildUser:   buildUser, // Set via ldflags at build time
	}
}

// String returns a formatted version string
func (v *VersionInfo) String() string {
	base := fmt.Sprintf("%s v%s (%s)", v.Name, v.Version, v.GoVersion)

	if v.GitCommit != "" {
		if len(v.GitCommit) > 7 {
			base += fmt.Sprintf(" [%s]", v.GitCommit[:7])
		} else {
			base += fmt.Sprintf(" [%s]", v.GitCommit)
		}
	}

	if v.BuildTime != "" {
		base += fmt.Sprintf(" built %s", v.BuildTime)
	}

	if v.BuildUser != "" {
		base += fmt.Sprintf(" by %s", v.BuildUser)
	}

	return base
}

// Banner returns a styled version banner for CLI tools
func (v *VersionInfo) Banner() string {
	return fmt.Sprintf(`
🚀 %s v%s
   %s
   
   Go Version: %s
   Build Info: %s
   
   One line = Magic ✨
`, v.Name, v.Version, v.Description, v.GoVersion, func() string {
		if v.GitCommit != "" || v.BuildTime != "" {
			commit := "unknown"
			if v.GitCommit != "" {
				if len(v.GitCommit) > 7 {
					commit = v.GitCommit[:7]
				} else {
					commit = v.GitCommit
				}
			}

			buildTime := "unknown"
			if v.BuildTime != "" {
				buildTime = v.BuildTime
			}

			return fmt.Sprintf("commit %s, built %s", commit, buildTime)
		}
		return "development build"
	}())
}

// Variables set at build time via ldflags
var (
	gitCommit = "unknown" // Set with: -ldflags "-X github.com/itsatony/gorly.gitCommit=<commit>"
	buildTime = "unknown" // Set with: -ldflags "-X github.com/itsatony/gorly.buildTime=<timestamp>"
	buildUser = "unknown" // Set with: -ldflags "-X github.com/itsatony/gorly.buildUser=<user>"
)

// Build-time information functions for advanced use cases

// GetGitCommit returns the git commit hash (if set at build time)
func GetGitCommit() string {
	return gitCommit
}

// GetBuildTime returns the build timestamp (if set at build time)
func GetBuildTime() string {
	return buildTime
}

// GetBuildUser returns who built the binary (if set at build time)
func GetBuildUser() string {
	return buildUser
}
