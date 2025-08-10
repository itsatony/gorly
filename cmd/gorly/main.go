// cmd/gorly/main.go
package main

import (
	"flag"
	"fmt"
	"runtime"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "show version information")
		showHelp    = flag.Bool("help", false, "show help information")
	)

	flag.Parse()

	if *showVersion {
		printVersion()
		return
	}

	if *showHelp {
		printHelp()
		return
	}

	fmt.Println("Gorly Rate Limiting Library")
	fmt.Println("This is a library package. Import it in your Go projects:")
	fmt.Println("")
	fmt.Println("  import \"github.com/itsatony/gorly\"")
	fmt.Println("")
	fmt.Println("For examples and documentation, visit:")
	fmt.Println("  https://github.com/itsatony/gorly")
	fmt.Println("")
	fmt.Println("Use --help for more information or --version to see version details.")
}

func printVersion() {
	fmt.Printf("gorly version %s\n", version)
	fmt.Printf("  commit: %s\n", commit)
	fmt.Printf("  date: %s\n", date)
	fmt.Printf("  go: %s\n", runtime.Version())
	fmt.Printf("  platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func printHelp() {
	fmt.Println("Gorly - World-class Go Rate Limiting Library")
	fmt.Println("")
	fmt.Println("DESCRIPTION:")
	fmt.Println("  Gorly is a high-performance, Redis-backed rate limiting library")
	fmt.Println("  for Go applications with support for multiple algorithms:")
	fmt.Println("")
	fmt.Println("  • Token Bucket - Allows bursts up to bucket capacity")
	fmt.Println("  • Sliding Window - Precise tracking with nanosecond accuracy")
	fmt.Println("  • GCRA - Generic Cell Rate Algorithm (coming soon)")
	fmt.Println("")
	fmt.Println("FEATURES:")
	fmt.Println("  • Multiple storage backends (Memory, Redis)")
	fmt.Println("  • Tier-based rate limiting (Free, Pro, Enterprise)")
	fmt.Println("  • Scope-based limits (Global, Memory, Search, etc.)")
	fmt.Println("  • Prometheus metrics integration")
	fmt.Println("  • HTTP middleware for popular frameworks")
	fmt.Println("  • Comprehensive test coverage with race detection")
	fmt.Println("")
	fmt.Println("USAGE:")
	fmt.Println("  This is a library package. Import it in your Go code:")
	fmt.Println("")
	fmt.Println("    import \"github.com/itsatony/gorly\"")
	fmt.Println("")
	fmt.Println("  Basic example:")
	fmt.Println("    config := gorly.DefaultConfig()")
	fmt.Println("    limiter, err := gorly.NewRateLimiter(config)")
	fmt.Println("    if err != nil {")
	fmt.Println("        log.Fatal(err)")
	fmt.Println("    }")
	fmt.Println("    defer limiter.Close()")
	fmt.Println("")
	fmt.Println("    entity := gorly.NewDefaultAuthEntity(\"user123\", gorly.EntityTypeUser, gorly.TierFree)")
	fmt.Println("    result, err := limiter.Allow(ctx, entity, gorly.ScopeGlobal)")
	fmt.Println("    if err != nil {")
	fmt.Println("        log.Fatal(err)")
	fmt.Println("    }")
	fmt.Println("    if !result.Allowed {")
	fmt.Println("        fmt.Print(\\\"Rate limited. Retry after: \\\", result.RetryAfter)")
	fmt.Println("    }")
	fmt.Println("")
	fmt.Println("DOCUMENTATION:")
	fmt.Println("  https://github.com/itsatony/gorly")
	fmt.Println("  https://pkg.go.dev/github.com/itsatony/gorly")
	fmt.Println("")
	fmt.Println("OPTIONS:")
	fmt.Println("  --version    Show version information")
	fmt.Println("  --help       Show this help message")
	fmt.Println("")
	fmt.Println("LICENSE:")
	fmt.Println("  MIT License - see LICENSE file for details")
}
