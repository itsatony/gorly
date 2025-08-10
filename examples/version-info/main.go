// examples/simple/version.go - Version information example
package main

import (
	"fmt"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🔍 Gorly Version Information Example")
	fmt.Println("===================================")

	// Simple version string
	fmt.Printf("📦 Library Version: %s\n\n", ratelimit.VersionString())

	// Comprehensive version info
	info := ratelimit.Info()
	fmt.Printf("📋 Detailed Version Info:\n%s\n\n", info.String())

	// Styled banner (great for CLI tools)
	fmt.Println("🎨 Styled Version Banner:")
	fmt.Print(info.Banner())

	// Usage in application context
	fmt.Printf("✨ Example Usage:\n")
	fmt.Printf("   Starting rate limiter with %s v%s\n", info.Name, info.Version)
	fmt.Printf("   Built with %s\n", info.GoVersion)

	if info.GitCommit != "unknown" {
		fmt.Printf("   Git commit: %s\n", info.GitCommit)
	}

	if info.BuildTime != "unknown" {
		fmt.Printf("   Built at: %s\n", info.BuildTime)
	}

	// Demonstrate using version in actual rate limiting
	limiter := ratelimit.IPLimit("10/minute")
	fmt.Printf("\n🚀 Rate limiter created successfully using %s!\n", info.Name)

	// Access version constants directly
	fmt.Printf("\n📊 Version Constants:\n")
	fmt.Printf("   Name: %s\n", ratelimit.Name)
	fmt.Printf("   Version: %s\n", ratelimit.Version)
	fmt.Printf("   Description: %s\n", ratelimit.Description)

	_ = limiter // Avoid unused variable warning
}
