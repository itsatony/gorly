// internal/middleware/auto.go - Auto-detecting middleware
package middleware

import (
	"context"
	"net/http"
	"reflect"
	"strconv"

	"github.com/itsatony/gorly/internal/core"
)

// New creates middleware that automatically detects the framework
func New(limiter core.Limiter, config *core.Config) interface{} {
	// Create a universal middleware that can be used directly with any framework
	return &UniversalMiddleware{
		limiter: limiter,
		config:  config,
	}
}

// UniversalMiddleware is the magic middleware that works with any framework
type UniversalMiddleware struct {
	limiter core.Limiter
	config  *core.Config
}

// =============================================================================
// Universal Middleware - Works with ANY Go web framework! 🎯
// =============================================================================

// FrameworkType represents different web framework types
type FrameworkType int

const (
	FrameworkGin FrameworkType = iota
	FrameworkEcho
	FrameworkFiber
	FrameworkChi
	FrameworkHTTP
	FrameworkAuto // Auto-detect
)

// For creates middleware for a specific framework type
func (um *UniversalMiddleware) For(framework FrameworkType) interface{} {
	switch framework {
	case FrameworkGin:
		return um.ginHandler()
	case FrameworkEcho:
		return um.echoHandler()
	case FrameworkFiber:
		return um.fiberHandler()
	case FrameworkChi:
		return um.chiHandler()
	case FrameworkHTTP:
		return um.httpHandler()
	case FrameworkAuto:
		return um // Return self for auto-detection
	default:
		return um.httpHandler() // Default to HTTP
	}
}

// ServeHTTP implements http.Handler for direct use and auto-detection
func (um *UniversalMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	um.checkRateLimit(w, r)
}

// ginHandler returns a Gin-compatible middleware
func (um *UniversalMiddleware) ginHandler() interface{} {
	return func(c interface{}) {
		ctx := reflect.ValueOf(c)

		// Extract request and writer from Gin context
		request := ctx.FieldByName("Request").Interface().(*http.Request)
		writer := ctx.MethodByName("Writer").Call(nil)[0].Interface().(http.ResponseWriter)

		if !um.checkRateLimit(writer, request) {
			ctx.MethodByName("Abort").Call(nil)
			return
		}

		ctx.MethodByName("Next").Call(nil)
	}
}

// echoHandler returns an Echo-compatible middleware
func (um *UniversalMiddleware) echoHandler() interface{} {
	return func(next interface{}) interface{} {
		return func(c interface{}) error {
			ctx := reflect.ValueOf(c)
			request := ctx.MethodByName("Request").Call(nil)[0].Interface().(*http.Request)
			response := ctx.MethodByName("Response").Call(nil)[0]
			writer := response.MethodByName("Writer").Call(nil)[0].Interface().(http.ResponseWriter)

			if !um.checkRateLimit(writer, request) {
				return nil
			}

			nextFunc := reflect.ValueOf(next)
			results := nextFunc.Call([]reflect.Value{ctx})
			if len(results) > 0 && !results[0].IsNil() {
				return results[0].Interface().(error)
			}
			return nil
		}
	}
}

// fiberHandler returns a Fiber-compatible middleware
func (um *UniversalMiddleware) fiberHandler() interface{} {
	return func(c interface{}) error {
		ctx := reflect.ValueOf(c)

		// Extract basic info from Fiber context
		method := ctx.MethodByName("Method").Call(nil)[0].String()
		path := ctx.MethodByName("Path").Call(nil)[0].String()
		ip := ctx.MethodByName("IP").Call(nil)[0].String()

		// Create minimal HTTP request for rate limiting
		req, _ := http.NewRequest(method, path, nil)
		req.RemoteAddr = ip + ":0"

		// Add common headers
		userAgent := ctx.MethodByName("Get").Call([]reflect.Value{reflect.ValueOf("User-Agent")})[0].String()
		req.Header = make(http.Header)
		req.Header.Set("User-Agent", userAgent)

		if !um.checkRateLimit(nil, req) {
			ctx.MethodByName("Status").Call([]reflect.Value{reflect.ValueOf(429)})
			body := map[string]string{"error": "Rate limit exceeded"}
			return ctx.MethodByName("JSON").Call([]reflect.Value{reflect.ValueOf(body)})[0].Interface().(error)
		}

		return ctx.MethodByName("Next").Call(nil)[0].Interface().(error)
	}
}

// chiHandler returns a Chi-compatible middleware
func (um *UniversalMiddleware) chiHandler() interface{} {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !um.checkRateLimit(w, r) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// httpHandler returns a standard HTTP middleware
func (um *UniversalMiddleware) httpHandler() interface{} {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !um.checkRateLimit(w, r) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Rate Limit Check Logic
// =============================================================================

// checkRateLimit performs the actual rate limit check
func (um *UniversalMiddleware) checkRateLimit(w http.ResponseWriter, r *http.Request) bool {
	// Extract entity using the configured extractor
	entity := um.config.ExtractorFunc(r)
	if entity == "" {
		entity = "anonymous"
	}

	// Extract scope using the configured scope function (if any)
	scope := "global"
	if um.config.ScopeFunc != nil {
		if s := um.config.ScopeFunc(r); s != "" {
			scope = s
		}
	}

	// Perform rate limit check
	result, err := um.limiter.Check(r.Context(), entity, scope)
	if err != nil {
		// Handle error
		if um.config.ErrorHandler != nil {
			um.config.ErrorHandler(err)
		}

		if w != nil {
			http.Error(w, "Rate limiting service unavailable", http.StatusInternalServerError)
		}
		return false
	}

	// Add rate limit headers if we have a response writer
	if w != nil {
		w.Header().Set("X-RateLimit-Limit", toString(result.Limit))
		w.Header().Set("X-RateLimit-Remaining", toString(result.Remaining))
		w.Header().Set("X-RateLimit-Used", toString(result.Used))
		w.Header().Set("X-RateLimit-Window", result.Window.String())

		if !result.Allowed {
			w.Header().Set("X-RateLimit-Retry-After", toString(int64(result.RetryAfter.Seconds())))
			w.Header().Set("Retry-After", toString(int64(result.RetryAfter.Seconds())))
		}
	}

	// Check if request is allowed
	if !result.Allowed {
		if um.config.DeniedHandler != nil && w != nil {
			um.config.DeniedHandler(w, r, result)
		} else if w != nil {
			// Default denied response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"Rate limit exceeded","retry_after_seconds":` + toString(int64(result.RetryAfter.Seconds())) + `}`))
		}
		return false
	}

	// Add rate limit info to request context for downstream handlers
	ctx := context.WithValue(r.Context(), "gorly_result", result)
	ctx = context.WithValue(ctx, "gorly_entity", entity)
	ctx = context.WithValue(ctx, "gorly_scope", scope)
	*r = *r.WithContext(ctx)

	return true
}

// toString converts int64 to string
func toString(n int64) string {
	return strconv.FormatInt(n, 10)
}
