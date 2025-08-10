// examples/middleware/universal/main.go - Universal middleware demonstration
package main

import (
	"fmt"
	"log"
	"net/http"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🎯 Universal Middleware Demo - Works with ANY Go web framework!")
	fmt.Println("===============================================================")

	// Create a rate limiter
	limiter := ratelimit.IPLimit("5/minute")

	fmt.Println("\n✅ 1. Auto-detecting middleware (works with any framework):")
	middleware := limiter.Middleware()
	fmt.Printf("   Type: %T\n", middleware)

	fmt.Println("\n✅ 2. Framework-specific middleware:")
	ginMW := limiter.For(ratelimit.Gin)
	echoMW := limiter.For(ratelimit.Echo)
	fiberMW := limiter.For(ratelimit.Fiber)
	chiMW := limiter.For(ratelimit.Chi)
	httpMW := limiter.For(ratelimit.HTTP)

	fmt.Printf("   Gin:   %T\n", ginMW)
	fmt.Printf("   Echo:  %T\n", echoMW)
	fmt.Printf("   Fiber: %T\n", fiberMW)
	fmt.Printf("   Chi:   %T\n", chiMW)
	fmt.Printf("   HTTP:  %T\n", httpMW)

	fmt.Println("\n✅ 3. Standard HTTP server example:")
	mux := http.NewServeMux()

	// Add a simple endpoint
	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Success! Rate limiting is working!"}`))
	})

	// Wrap with rate limiting using standard HTTP middleware pattern
	httpMiddleware := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)
	handler := httpMiddleware(mux)

	fmt.Println("\n🚀 Server starting on http://localhost:8081")
	fmt.Println("   Try: curl http://localhost:8081/api/test")
	fmt.Println("   Rate limit: 5 requests per minute per IP")
	fmt.Println("   After 5 requests, you'll get HTTP 429 Too Many Requests")

	log.Fatal(http.ListenAndServe(":8081", handler))
}

/*
Usage examples for different frameworks:

// Gin
r := gin.Default()
r.Use(limiter.For(ratelimit.Gin).(gin.HandlerFunc))

// Echo
e := echo.New()
e.Use(limiter.For(ratelimit.Echo).(echo.MiddlewareFunc))

// Fiber
app := fiber.New()
app.Use(limiter.For(ratelimit.Fiber).(fiber.Handler))

// Chi
r := chi.NewRouter()
r.Use(limiter.For(ratelimit.Chi).(func(http.Handler) http.Handler))

// Standard HTTP
mux := http.NewServeMux()
handler := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(mux)
*/
