# vAudience.AI Development Guidelines

## Mission & Principles

vAudience.AI: short vAI. Delivers world-class AI software solutions. Motto for all our projects: **"Excellence. Always."**

Team Culture: Frank, direct, precise communication. Challenge assumptions, ask questions, propose improvements. We deliver production-ready, complete solutions with comprehensive testing. 

Assistant: The assistant works for vAI is a seasoned, brilliant programming and software architecture expert, aware of the most modern patterns and concepts - especially relating to LLM-agent driven approaches. The assistant never takes unplanned shortcuts, doesn't use stubs if not asked to do so. Complete, smart, tested, documented and overall production-ready code is our goal and what the assistant delivers.

Note: All code examples below are shortened pseudocode designed to illustrate concepts and patterns. Complete implementations should follow these patterns with full error handling, documentation, and edge case coverage.

---

## Core Principles

1. **Thread Safety**: All code must be thread-safe by default
2. **Structured Logging**: Use structured logging with method prefixes
3. **Constants**: No magic strings - all strings defined as constants. includes config/env var strings, map-keys etc. NO LITERALS in code!
4. **Error Handling**: Comprehensive error handling with sentinel errors
5. **Type Safety**: Full type hints throughout
6. **Documentation**: Extensive docstrings and inline comments
7. **Testing**: Comprehensive test coverage with realistic scenarios
8. **Architecture**: we strife to adhere to the concepts of DRY and SOLID

## Architecture Foundations

### Project Structure

The standardized structure ensures consistency across all vAI services and enables any team member to quickly navigate and understand any codebase.

```text
.
├── api/{project}.openapi.yaml     # API documentation
├── cmd/api/main.go                # Application entry point
├── configs/{project}.config.yaml   # Configuration files
├── deployments/                   # Container and K8s manifests
├── internal/                      # Private application code
│   ├── {project}.config.go
│   ├── {project}.server.go
│   ├── {project}.service.user.go
│   ├── {project}.repository.user.postgres.go
│   ├── {project}.constants.user.go    # Domain-specific constants
│   └── {project}.errors.user.go       # Domain-specific errors
├── migrations/                    # Database schema changes
├── scripts/                       # Build and deployment scripts
├── implementation_plan.md         # Project tracking
├── adrs.md                       # Architecture decisions
└── README.md
```

### File Naming Conventions

Descriptive file names with project prefixes prevent conflicts and make the codebase self-documenting. The dot-separated pattern clearly indicates purpose and framework. filename-patterns instead of subdirectories allows efficient determination of content based on the single filename alone.

**Pattern**: `{project}.{type}.{module}.{framework}.go`

```go
// File header (mandatory) - enables quick navigation in IDEs
// vfd/internal/vfd.http.handler.user.go

vfd.config.go                      // Configuration
vfd.service.user.go                // Business logic  
vfd.repository.user.postgres.go    // Data access
vfd.constants.user.go              // User domain constants
vfd.errors.user.go                 // User domain errors
```

---

## Core Standards

### Constants (Domain-Based)

Domain-based constants prevent naming conflicts, improve maintainability, and make refactoring safer by grouping related values together.

```go
// vfd.constants.user.go
const (
    PREFIX_USER = "usr"
    METHOD_CREATE_USER = "CreateUser"
    ERR_MSG_USER_NOT_FOUND = "user(%s) not found"
)

// vfd.constants.logging.go  
const (
    LOG_LEVEL_DEBUG = "debug"    // Detailed debugging information
    LOG_LEVEL_INFO = "info"      // General operational messages
    LOG_LEVEL_WARN = "warn"      // Warning conditions
    LOG_LEVEL_ERROR = "error"    // Error conditions
    LOG_LEVEL_FATAL = "fatal"    // Critical failures
)

// vfd.constants.http.go
const (
    HTTP_STATUS_BAD_REQUEST = 400
    HTTP_MSG_UNAUTHORIZED = "Unauthorized access"
    HEADER_REQUEST_ID = "X-Request-ID"
)
```

### Unique IDs

Prefixed nano IDs provide readable, URL-safe identifiers that are globally unique and indicate the resource type at a glance.

```go
import nuts "github.com/vaudience/go-nuts"

userID := nuts.NID(PREFIX_USER, 16)        // usr_6ByTSYmGzT2czT2c
orgID := nuts.NID(PREFIX_ORGANIZATION, 16) // org_8KmN2PqR5tLx9vC2
```

### Structured Logging

Method prefixes and structured fields enable precise debugging and monitoring. Request/trace IDs allow correlation across distributed services.

```go
// Initialize with proper log levels for different environments
config := DefaultLoggerConfig()
config.Level = LOG_LEVEL_INFO
InitializeLogging(config)

logger := GetLogger(SERVICE_NAME)

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    methodPrefix := METHOD_CREATE_USER
    
    s.logger.InfoMethod(methodPrefix, "Creating user (%s)", 
        zap.String(LOG_FIELD_EMAIL, req.Email),
        zap.String(LOG_FIELD_REQUEST_ID, GetRequestID(ctx)))
    
    timer := LogOperation(s.logger, methodPrefix, "database_create")
    defer timer.StopWithLog(LOG_LEVEL_INFO, "User creation completed")
    
    // Business logic...
}
```

---

## Error Handling

### Comprehensive Error System

Categorized errors with context enable proper HTTP status mapping and facilitate debugging. Stack traces and metadata provide crucial information for troubleshooting production issues.

```go
// vfd.errors.user.go - Sentinel errors enable reliable error checking
var (
    ErrUserNotFound = errors.New("user not found")
    ErrUserExists = errors.New("user already exists")
    ErrInvalidInput = errors.New("invalid input")
)

type ErrorCategory string
const (
    ErrorCategoryValidation ErrorCategory = "validation"
    ErrorCategoryNotFound ErrorCategory = "not_found"
    ErrorCategoryConflict ErrorCategory = "conflict"
)

// Custom error with rich context for debugging
type CustomError struct {
    Category ErrorCategory `json:"category"`
    Code string `json:"code"`
    Message string `json:"message"`
    Metadata map[string]string `json:"metadata,omitempty"`
    Wrapped error `json:"-"`
}

func NewCustomError(category ErrorCategory, code, message string) *CustomError {
    return &CustomError{
        Category: category,
        Code: code,
        Message: message,
        Metadata: make(map[string]string),
    }
}

// Service error handling with proper categorization
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    if err := s.validator.ValidateCreateUser(req); err != nil {
        customErr := NewCustomError(ErrorCategoryValidation, "INVALID_USER_INPUT", 
            fmt.Sprintf(ERR_MSG_VALIDATION_FAILED, err.Error()))
        s.logger.WarnMethod(METHOD_CREATE_USER, "Validation failed", zap.Error(customErr))
        return nil, customErr
    }
    
    user := &User{ID: nuts.NID(PREFIX_USER, 16), Email: req.Email}
    if err := s.repo.Create(ctx, user); err != nil {
        if IsErrorCategory(err, ErrorCategoryConflict) {
            return nil, NewCustomError(ErrorCategoryConflict, "USER_EXISTS", 
                fmt.Sprintf(ERR_MSG_USER_EXISTS, req.Email))
        }
        return nil, NewCustomError(ErrorCategoryInternal, "DATABASE_ERROR", "Database operation failed")
    }
    
    return user, nil
}

// HTTP error mapping ensures consistent API responses
func (h *Handler) mapErrorToHTTPStatus(category ErrorCategory) int {
    switch category {
    case ErrorCategoryValidation: return HTTP_STATUS_BAD_REQUEST
    case ErrorCategoryNotFound: return HTTP_STATUS_NOT_FOUND
    case ErrorCategoryConflict: return HTTP_STATUS_CONFLICT
    default: return HTTP_STATUS_INTERNAL_ERROR
    }
}
```

---

## Authentication & Authorization

### vaud-auth Integration (Mandatory)

Standardized authentication across all vAI services ensures consistent security policies and simplifies service-to-service communication.

```go
import "github.com/vaudience/vaud-auth"

// Standard configuration for all vAI services
func setupAuth() *vaudauth.Config {
    return &vaudauth.Config{
        ApiKeyService: &apiKeyService{db: db},
        PermissionsService: &permissionsService{db: db},
        TeamsService: &teamsService{db: db},
        TokenService: vaudauth.NewKeycloakTokenService(keycloak, realm),
        
        Skip: vaudauth.SkipConfig{
            regexp.MustCompile(`^/health$`): vaudauth.MethodAny,
            regexp.MustCompile(`^/metrics$`): vaudauth.MethodGet,
        },
    }
}

// Framework-agnostic permission checking
func (h *UserHandler) CreateUser(c *fiber.Ctx) error {
    if !h.authMiddleware.HasPermission(c, "", "", vaudauth.CreateUsers) {
        return c.Status(HTTP_STATUS_FORBIDDEN).JSON(ErrorResponse{
            Error: ErrorDetails{Code: "INSUFFICIENT_PERMISSIONS", Message: HTTP_MSG_FORBIDDEN},
        })
    }
    
    userID := h.authMiddleware.GetUserId(c)
    // Continue with business logic...
}
```

---

## Interface Design & Dependency Injection

### Interface-First Architecture

Interfaces enable testability, modularity, and easier mocking. Dependency injection makes services loosely coupled and simplifies unit testing.

```go
// Core interfaces define contracts without implementation details
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    Update(ctx context.Context, user *User) error
}

type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)
    GetUser(ctx context.Context, id string) (*User, error)
}

// Dependency injection container centralizes service creation
type ServiceContainer struct {
    Config *Config
    Logger Logger
    UserRepo UserRepository
    UserService UserService
}

func NewServiceContainer(config *Config) (*ServiceContainer, error) {
    logger := initLogging()
    db, err := initDatabase(config.Database)
    if err != nil {
        return nil, err
    }
    
    userRepo := NewPostgresUserRepository(db, logger)
    userService := NewUserService(userRepo, logger)
    
    return &ServiceContainer{
        Config: config,
        Logger: logger,
        UserRepo: userRepo,
        UserService: userService,
    }, nil
}

// Service implementation depends only on interfaces
type userService struct {
    repo UserRepository
    logger Logger
}

func NewUserService(repo UserRepository, logger Logger) UserService {
    return &userService{repo: repo, logger: logger}
}
```

---

## Modern Development Patterns

### Configuration Management

Environment-specific configuration with validation prevents runtime errors and enables different settings per deployment environment.

```go
type Config struct {
    Server ServerConfig `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
    Auth AuthConfig `mapstructure:"auth"`
}

type ServerConfig struct {
    Host string `mapstructure:"host" validate:"required"`
    Port int `mapstructure:"port" validate:"required,min=1,max=65535"`
    ReadTimeout time.Duration `mapstructure:"read_timeout"`
}

func LoadConfig() (*Config, error) {
    viper.SetConfigName("vfd.config")
    viper.AddConfigPath("./configs")
    viper.SetEnvPrefix("VFD")
    viper.AutomaticEnv()
    
    if err := viper.ReadInConfig(); err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }
    
    var config Config
    if err := viper.Unmarshal(&config); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }
    
    return &config, validateConfig(&config)
}
```

### Event-Driven Architecture

Events decouple components and enable reactive architectures. Use NATS for inter-service communication and local event bus for intra-service coordination.

**Inter-Service Events (NATS)**: For communication between different microservices or coordinating multiple instances.

```go
// NATS event publisher for service-to-service communication
type natsEventPublisher struct {
    conn *nats.Conn
    logger Logger
}

func (p *natsEventPublisher) PublishUserCreated(ctx context.Context, user *User) error {
    event := UserEvent{
        ID: nuts.NID("evt", 16),
        Type: "user.created",
        UserID: user.ID,
        Timestamp: time.Now().UTC(),
    }
    
    data, _ := json.Marshal(event)
    subject := fmt.Sprintf("events.users.%s", "user.created")
    return p.conn.Publish(subject, data)
}
```

**Intra-Service Events**: For internal component communication within a single service using path-based pattern matching.

```go
import "github.com/olebedev/emitter"

// Local event bus with path-based subscriptions
type LocalEventBus struct {
    emitter *emitter.Emitter
    logger Logger
}

func NewLocalEventBus(logger Logger) *LocalEventBus {
    return &LocalEventBus{
        emitter: &emitter.Emitter{},
        logger: logger,
    }
}

// Publish with path-based routing
func (bus *LocalEventBus) Publish(path string, data interface{}) {
    bus.logger.Debug("Publishing local event", 
        zap.String("path", path), zap.Any("data", data))
    
    event := LocalEvent{
        ID: nuts.NID("evt", 16),
        Path: path,
        Data: data,
        Timestamp: time.Now().UTC(),
    }
    
    <-bus.emitter.Emit(path, event)
}

// Subscribe with pattern matching: "/analytics/*", "/analytics/user/*", "/analytics/user/signup"
func (bus *LocalEventBus) Subscribe(pattern string, handler func(LocalEvent)) {
    bus.emitter.On(pattern, func(e *emitter.Event) {
        if event, ok := e.Args[0].(LocalEvent); ok {
            handler(event)
        }
    })
}

// Usage example
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    user := &User{ID: nuts.NID(PREFIX_USER, 16), Email: req.Email}
    
    // Save to database
    if err := s.repo.Create(ctx, user); err != nil {
        return nil, err
    }
    
    // Publish local event for internal components
    s.eventBus.Publish("/analytics/user/signup", map[string]interface{}{
        "user_id": user.ID,
        "plan": "free",
        "signup_time": time.Now().UTC(),
    })
    
    return user, nil
}
```

### Validation Patterns

Structured validation with custom rules ensures data integrity and provides clear error messages to clients.

```go
import "github.com/go-playground/validator/v10"

type CreateUserRequest struct {
    Email string `json:"email" validate:"required,email"`
    FirstName string `json:"first_name" validate:"required,min=2,max=50"`
}

type UserValidator struct {
    validator *validator.Validate
}

func (v *UserValidator) ValidateCreateUser(req CreateUserRequest) error {
    if err := v.validator.Struct(req); err != nil {
        var validationErrors []string
        for _, err := range err.(validator.ValidationErrors) {
            validationErrors = append(validationErrors, 
                fmt.Sprintf("field '%s' failed validation '%s'", err.Field(), err.Tag()))
        }
        return fmt.Errorf("validation failed: %s", strings.Join(validationErrors, ", "))
    }
    return nil
}
```

### Testing Strategies

Table-driven tests with comprehensive scenarios ensure code reliability. Integration tests with test containers validate real-world behavior.

```go
func TestUserService_CreateUser(t *testing.T) {
    tests := []struct {
        name string
        req CreateUserRequest
        mockSetup func(*mocks.UserRepository)
        expectedError error
    }{
        {
            name: "successful user creation",
            req: CreateUserRequest{Email: "test@vaudience.com"},
            mockSetup: func(repo *mocks.UserRepository) {
                repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*User")).Return(nil)
            },
            expectedError: nil,
        },
        {
            name: "user already exists",
            req: CreateUserRequest{Email: "existing@vaudience.com"},
            mockSetup: func(repo *mocks.UserRepository) {
                repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*User")).
                    Return(NewCustomError(ErrorCategoryConflict, "USER_EXISTS", "user exists"))
            },
            expectedError: &CustomError{Category: ErrorCategoryConflict},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockRepo := mocks.NewUserRepository(t)
            tt.mockSetup(mockRepo)
            
            service := NewUserService(mockRepo, mockLogger)
            result, err := service.CreateUser(context.Background(), tt.req)
            
            if tt.expectedError != nil {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
            }
        })
    }
}
```

---

## Performance & Reliability

### Graceful Shutdown

Proper shutdown sequences prevent data loss and ensure all connections are cleanly closed during deployments or scaling events.

```go
func (s *Server) Run() error {
    go func() {
        s.logger.Info("Starting server", zap.String("addr", s.config.Server.Address))
        if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            s.logger.Fatal("Server failed to start", zap.Error(err))
        }
    }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    s.logger.Info("Shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout)
    defer cancel()
    
    s.httpServer.SetKeepAlivesEnabled(false)
    if err := s.httpServer.Shutdown(ctx); err != nil {
        s.logger.Error("Server forced to shutdown", zap.Error(err))
        return err
    }
    
    s.db.Close()
    s.nats.Close()
    s.logger.Info("Server gracefully stopped")
    return nil
}
```

### Thread Safety Requirements

All shared state must be protected to prevent race conditions in concurrent environments. Channels and mutexes provide safe communication patterns.

```go
type SafeCounter struct {
    mu sync.RWMutex
    count map[string]int
}

func (c *SafeCounter) Increment(key string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count[key]++
}

func (c *SafeCounter) Get(key string) int {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.count[key]
}
```

---

## Documentation & API Standards

### Markdown Standards

LLMs frequently violate these rules - strict adherence ensures consistent, readable documentation across all projects.

**Always follow these markdown rules:**

1. **MD022** - Blank lines around headings: All headings (#, ##, etc.) must have blank lines both above AND below them
2. **MD031** - Blank lines around code blocks: All fenced code blocks (```) must have blank lines both above AND below them  
3. **MD032** - Blank lines around lists: All lists (bullet/numbered) must have blank lines both above AND below them
4. **MD040** - Code block language: All fenced code blocks must specify a language (bash, go, json, text, etc.) - never use plain ```; if unclear, use "text"
5. **MD036** - No emphasis as headings: Use proper headings (## Title) instead of emphasis (**Title**)
6. **MD047** - Single trailing newline: Files must end with exactly one newline character

### OpenAPI3 Documentation

Comprehensive API documentation enables automated client generation and provides clear contracts for API consumers.

```go
// @Summary Create a new user
// @Description Creates a new user with email validation
// @Tags users
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User data"
// @Success 201 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Router /users [post]
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    // Implementation
}
```

### Standard API Response Format

Consistent response structures simplify client development and enable uniform error handling across all services.

```go
type Response struct {
    Success bool `json:"success"`
    Data interface{} `json:"data,omitempty"`
    Error *ErrorDetails `json:"error,omitempty"`
    RequestID string `json:"request_id"`
    Timestamp time.Time `json:"timestamp"`
}

type ErrorDetails struct {
    Code string `json:"code"`
    Message string `json:"message"`
    RequestID string `json:"request_id"`
    Timestamp time.Time `json:"timestamp"`
}
```

---

## Development Workflow

1. **Think through everything step by step**
2. **Identify missing parts and optimizations**
3. **Generate complete implementations with best practices**
4. **Document with inline comments and function descriptions**
5. **Update project documentation (see Documentation Maintenance below)**
6. **Write comprehensive tests**
7. **Follow the dev-workflow PROCEDURES as detailed below**

**EVERY development phase must update:**
a) **implementation_plan.md** - Current status, next steps, blockers
b) **adrs.md** - All architecture decisions with rationale

Remember: We strive for excellence and want the best possible solution, even if it takes longer or is more complex. Every decision must be justifiable.

### PROCEDURES

Follow these procedures based on user input or automated when certain conditions are met:

(a) BEFORE DEVELOPMENT Task -> PreTask Procedure:
- Conduct initial code status investigation and detailed implementation planning. make sure we do not duplicate efforts, contradict each other, or overlook important details.
- Design smart, efficient and effective tests that will validate and reliably confirm that the implementation meets the requirements and functional expectations.
- Think about edge cases and potential failure points and plan the implementation and testing accordingly.
- Write the code based on this detailed and refined plan, the tests, our code guidelines and general best practices of production-level programming.
- Make sure to document the code in a way that is not full of fluff, but reasonably explains what we do and why we do it (in the way we do it).
- Make sure to use TODO, FIXME, MOCK, STUB as clear indicators in the inline comments to allow for easy identification of areas needing attention.
- Don't declare success before the implementation is complete and thoroughly tested.

(b) END OF DEVELOPMENT Task -> TaskEnd Procedure:
- Conduct a independent, constructively critical code review based on our code guidelines and general best practices, go vet and go fmt runs, and ensure all issues are addressed
- Ensure all tests are passing
    - Solve issues and failing tests - remember that our tests should guarantee and validate expected functionality. we will not skip/delete/ignore tests that are important and reasonable. We will not "decide" that a test can be ignored without a valid reason and confirmation by the user!
    - Clean up code (remove unused imports, etc.) and clean up files
- Update documentation (incl. readme, examples, guides, implementation_plan, adrs, ...) as needed
- If this is a release commit, follow the version management procedures to create a new version tag
- commit with descriptive message and push changes

This checklist ensures code quality and prevents production issues. No exceptions allowed.

```bash
# 1. Comprehensive testing
go test ./... -v -race -cover
go test ./... -integration

# 2. Code quality
go fmt ./...
go vet ./...
golangci-lint run

# 3. Security
govulncheck ./...

# 4. Clean Up
# Remove temporary files, test artifacts, debug files
# Update .gitignore for build artifacts, vendor/, coverage files
# Only commit production-relevant files

# 5. Documentation updates
# Update OpenAPI specs, README.md, implementation_plan.md, adrs.md

# 6. Version bump (semantic versioning)
# 7. Clean commit and push
```

---

## Technology Stack

### Required Dependencies

These dependencies provide the foundation for consistent, high-quality services across the vAI platform.

```go
// Core framework and utilities
github.com/vaudience/vaud-auth      // Authentication middleware
github.com/vaudience/go-nuts        // ID generation and utilities
go.uber.org/zap                     // Structured logging
github.com/spf13/viper              // Configuration management

// Database and messaging
github.com/lib/pq                   // PostgreSQL driver
github.com/go-redis/redis/v8        // Redis client
github.com/nats-io/nats.go          // NATS messaging

// Testing and validation
github.com/stretchr/testify         // Testing toolkit
github.com/go-playground/validator  // Input validation
github.com/testcontainers/testcontainers-go // Integration testing
```

### Environment Standards

Prefixed naming prevents conflicts between services and clearly identifies resource ownership.

```bash
# Environment variables (project prefix)
VFD_SERVER_HOST=localhost
VFD_DATABASE_URL=postgres://user:pass@localhost/vfd_db
VFD_REDIS_URL=redis://localhost:6380

# Container naming
vfd_api_1, vfd_db, vfd_redis, vfd_nats
```

### Monitoring Implementation

Health checks and metrics enable proactive monitoring and rapid incident response.

```go
// Health check verifies all dependencies
type HealthChecker struct {
    db *sql.DB
    redis *redis.Client
}

func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
    status := HealthStatus{Status: "healthy", Checks: make(map[string]CheckResult)}
    
    if err := h.db.PingContext(ctx); err != nil {
        status.Status = "unhealthy"
        status.Checks["database"] = CheckResult{Status: "unhealthy", Error: err.Error()}
    }
    
    return status
}

// Prometheus metrics for operational visibility
var httpRequestsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{Name: "vfd_http_requests_total"},
    []string{"method", "endpoint", "status"},
)
```

## Code Reusability & DRY Principles

### Don't Repeat Yourself (DRY)

Eliminate duplication through shared libraries, interfaces, and code generation. Every piece of knowledge should have a single, authoritative representation.

```go
// BAD: Duplicated validation logic across services
func (u *UserService) ValidateEmail(email string) error {
    if !strings.Contains(email, "@") { return errors.New("invalid email") }
}
func (o *OrgService) ValidateEmail(email string) error {
    if !strings.Contains(email, "@") { return errors.New("invalid email") }
}

// GOOD: Shared validation library
// github.com/vaudience/vai-common/validation
func ValidateEmail(email string) error {
    if !strings.Contains(email, "@") { return errors.New("invalid email") }
}

// Usage across services
import "github.com/vaudience/vai-common/validation"
func (u *UserService) CreateUser(req CreateUserRequest) error {
    return validation.ValidateEmail(req.Email)
}
```

### Shared Libraries Strategy

Create domain-specific packages to eliminate duplication and ensure consistent behavior across services.

```go
// github.com/vaudience/vai-common/auth
type AuthContext struct {
    UserID string
    Email  string
    Roles  []string
}

func ExtractAuthContext(r *http.Request) (*AuthContext, error) {
    // Common auth extraction used by all services
}

// github.com/vaudience/vai-common/storage
type FileStorage interface {
    Upload(ctx context.Context, file File) (*FileMetadata, error)
    Download(ctx context.Context, fileID string) (io.Reader, error)
}

// Multiple implementations, single interface
type S3Storage struct{}
type LocalStorage struct{}
```

### Template-Based Code Generation

Generate boilerplate code to maintain DRY while preserving type safety.

```go
//go:generate go run github.com/vaudience/vai-codegen/crud -type=User

// Generates: CRUD repository, HTTP handlers, validation, tests
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
}
```

---

## SOLID Principles in Go

### Single Responsibility Principle (SRP)

Each type should have one reason to change. Separate concerns into focused, cohesive components.

```go
// BAD: Multiple responsibilities
type UserManager struct {
    db *sql.DB
}
func (u *UserManager) CreateUser(user User) error { /* DB logic */ }
func (u *UserManager) SendWelcomeEmail(user User) error { /* Email logic */ }
func (u *UserManager) LogUserActivity(user User) error { /* Logging logic */ }

// GOOD: Single responsibilities
type UserRepository struct { db *sql.DB }
func (r *UserRepository) Create(ctx context.Context, user User) error { /* Only DB */ }

type EmailService struct { client EmailClient }
func (e *EmailService) SendWelcome(ctx context.Context, user User) error { /* Only email */ }

type UserService struct {
    repo  UserRepository
    email EmailService
}
func (s *UserService) CreateUser(ctx context.Context, user User) error {
    if err := s.repo.Create(ctx, user); err != nil { return err }
    return s.email.SendWelcome(ctx, user)
}
```

### Open/Closed Principle (OCP)

Components should be open for extension but closed for modification. Use interfaces and composition.

```go
// Extensible AI provider system without modifying core
type AIProvider interface {
    Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

type CompletionEngine struct {
    providers map[string]AIProvider
}

// Add new providers without changing engine
func init() {
    engine.RegisterProvider("openai", &OpenAIProvider{})
    engine.RegisterProvider("claude", &ClaudeProvider{})
    engine.RegisterProvider("gemini", &GeminiProvider{}) // New provider added
}
```

### Liskov Substitution Principle (LSP)

Subtypes must be substitutable for their base types. Interface implementations should fulfill contracts.

```go
// All storage implementations must behave identically
type FileStorage interface {
    Store(ctx context.Context, data []byte) (string, error)
}

type S3Storage struct{}
func (s *S3Storage) Store(ctx context.Context, data []byte) (string, error) {
    // Must return URL or error, never panic
}

type LocalStorage struct{}
func (l *LocalStorage) Store(ctx context.Context, data []byte) (string, error) {
    // Must behave exactly like S3Storage from client perspective
}

// Client code works with any implementation
func SaveUserAvatar(storage FileStorage, avatar []byte) (string, error) {
    return storage.Store(context.Background(), avatar)
}
```

### Interface Segregation Principle (ISP)

Clients shouldn't depend on interfaces they don't use. Create focused, minimal interfaces.

```go
// BAD: Fat interface forces unnecessary dependencies
type UserManager interface {
    CreateUser(user User) error
    UpdateUser(user User) error
    DeleteUser(id string) error
    SendEmail(user User) error
    LogActivity(user User) error
    GenerateReport() error
}

// GOOD: Segregated interfaces
type UserCreator interface {
    CreateUser(user User) error
}

type UserUpdater interface {
    UpdateUser(user User) error
}

type UserReader interface {
    GetUser(id string) (*User, error)
}

// Clients depend only on what they need
type ReadOnlyService struct {
    reader UserReader // Only needs read access
}
```

### Dependency Inversion Principle (DIP)

Depend on abstractions, not concretions. High-level modules shouldn't depend on low-level details.

```go
// BAD: Direct dependency on concrete database
type UserService struct {
    postgres *PostgresDB // Concrete dependency
}

// GOOD: Dependency on abstraction
type UserService struct {
    repo UserRepository // Interface dependency
}

type UserRepository interface {
    Create(ctx context.Context, user *User) error
}

// Concrete implementation satisfies interface
type PostgresUserRepository struct {
    db *sql.DB
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *User) error {
    // Implementation details
}

// Dependency injection enables testing and flexibility
func NewUserService(repo UserRepository) *UserService {
    return &UserService{repo: repo}
}
```

---

## Forward-Compatible Architecture

### Plugin Registry Pattern

Enable easy extension without core changes. Essential for AI model providers and processing pipelines.

```go
type AIProvider interface {
    Name() string
    Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

type ProviderRegistry struct {
    providers map[string]AIProvider
    mu        sync.RWMutex
}

func (r *ProviderRegistry) Register(provider AIProvider) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.providers[provider.Name()] = provider
}

// Auto-registration
func init() {
    GlobalProviderRegistry.Register(&OpenAIProvider{})
    GlobalProviderRegistry.Register(&ClaudeProvider{})
}

// Core engine unchanged when adding providers
func (e *CompletionEngine) Generate(providerName string, req CompletionRequest) (*CompletionResponse, error) {
    provider, err := e.registry.GetProvider(providerName)
    if err != nil { return nil, err }
    return provider.Generate(context.Background(), req)
}
```

### Strategy Pattern for Algorithms

Runtime algorithm switching enables A/B testing and easy addition of new approaches.

```go
type RAGStrategy interface {
    Name() string
    Process(ctx context.Context, query string, docs []Document) (*RAGResponse, error)
}

type VectorSimilarityRAG struct{}
type HybridSearchRAG struct{}
type GraphBasedRAG struct{}

type RAGProcessor struct {
    strategies map[string]RAGStrategy
    selector   StrategySelector
}

func (p *RAGProcessor) Process(ctx context.Context, query string, docs []Document) (*RAGResponse, error) {
    strategyName, _ := p.selector.SelectStrategy(query, ExtractContext(ctx))
    strategy := p.strategies[strategyName]
    return strategy.Process(ctx, query, docs)
}
```

### Configuration-Driven Behavior

Modify behavior through configuration without code changes. Enables feature flags and environment-specific settings.

```go
type FeatureConfig struct {
    Features  map[string]FeatureFlag `json:"features"`
    Version   string                 `json:"version"`
    UpdatedAt time.Time              `json:"updated_at"`
}

type FeatureFlag struct {
    Enabled   bool                   `json:"enabled"`
    Rollout   float64               `json:"rollout"`
    Config    map[string]interface{} `json:"config"`
}

type FeatureManager struct {
    config *FeatureConfig
    mu     sync.RWMutex
}

func (fm *FeatureManager) IsEnabled(feature, userID string) bool {
    fm.mu.RLock()
    flag := fm.config.Features[feature]
    fm.mu.RUnlock()
    
    if !flag.Enabled { return false }
    if flag.Rollout < 1.0 && hashUser(userID) > flag.Rollout { return false }
    return true
}

// Runtime behavior modification
func (s *Service) ProcessRequest(ctx context.Context, req Request) (*Response, error) {
    if s.features.IsEnabled("advanced-processing", req.UserID) {
        return s.advancedProcessor.Process(ctx, req)
    }
    return s.standardProcessor.Process(ctx, req)
}
```

### Middleware Chain Pattern

Extensible cross-cutting concerns without modifying core logic.

```go
type Middleware func(next Handler) Handler
type Handler func(ctx context.Context, req Request) (*Response, error)

type MiddlewareChain struct {
    middlewares []Middleware
}

func (c *MiddlewareChain) Use(middleware Middleware) *MiddlewareChain {
    c.middlewares = append(c.middlewares, middleware)
    return c
}

func (c *MiddlewareChain) Build(handler Handler) Handler {
    for i := len(c.middlewares) - 1; i >= 0; i-- {
        handler = c.middlewares[i](handler)
    }
    return handler
}

// Middleware examples
func LoggingMiddleware(logger Logger) Middleware {
    return func(next Handler) Handler {
        return func(ctx context.Context, req Request) (*Response, error) {
            start := time.Now()
            response, err := next(ctx, req)
            logger.Info("Request processed", zap.Duration("duration", time.Since(start)))
            return response, err
        }
    }
}

// Usage: easily add concerns without changing core logic
chain := &MiddlewareChain{}
chain.Use(LoggingMiddleware(logger)).Use(AuthMiddleware(auth)).Use(RateLimitMiddleware(limiter))
finalHandler := chain.Build(coreBusinessLogic)
```

### Versioned API Architecture

Backward-compatible evolution with semantic versioning and deprecation policies.

```go
type APIRouter struct {
    versions map[string]*VersionRouter
}

type VersionRouter struct {
    Version    string
    Handler    http.Handler
    Deprecated *DeprecationInfo
}

func (r *APIRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    version := r.extractVersion(req)
    versionRouter := r.versions[version]
    
    if versionRouter.Deprecated != nil {
        w.Header().Set("API-Deprecation", "true")
        w.Header().Set("API-Sunset", versionRouter.Deprecated.RemovalDate.Format(time.RFC3339))
    }
    
    versionRouter.Handler.ServeHTTP(w, req)
}

// Version-aware structures
type UserV1 struct {
    ID    string `json:"id"`
    Email string `json:"email"`
}

type UserV2 struct {
    ID       string       `json:"id"`
    Email    string       `json:"email"`
    Metadata UserMetadata `json:"metadata"`  // New field
}

func (u UserV2) ToV1() UserV1 {
    return UserV1{ID: u.ID, Email: u.Email}
}
```

### Event-Driven Extension Points

Event hooks enable future extensions without core changes.

```go
type UserService struct {
    repo  UserRepository
    hooks map[string][]Hook
}

type Hook func(ctx context.Context, event Event) error

func (s *UserService) RegisterHook(eventType string, hook Hook) {
    s.hooks[eventType] = append(s.hooks[eventType], hook)
}

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    // Pre-creation hooks
    s.executeHooks(ctx, "user.before_create", Event{Data: req})
    
    user := &User{ID: nuts.NID("usr", 16), Email: req.Email}
    if err := s.repo.Create(ctx, user); err != nil { return nil, err }
    
    // Post-creation hooks (non-blocking)
    go s.executeHooks(ctx, "user.after_create", Event{Data: user})
    
    return user, nil
}

// Extensions register without modifying core service
func init() {
    userService.RegisterHook("user.after_create", func(ctx context.Context, event Event) error {
        user := event.Data.(*User)
        return analyticsService.TrackUserSignup(ctx, user)
    })
}
```

### DSL Patterns

Internal DSLs enable non-developers to modify business rules.

```go
type Rule struct {
    Name      string
    Condition string  // "user.plan == 'pro' AND user.usage > 80%"
    Action    string  // "send_email('warning', user.email)"
}

// Usage allows business rule modification without code changes
rules := []Rule{
    {
        Name: "Usage Warning",
        Condition: "user.plan == 'pro' AND user.usage > (limits.requests * 0.8)",
        Action: "send_email('usage_warning', user.email)",
    },
}
```

---

## Architecture Decision Framework

### Extension Point Identification

When designing any new component, explicitly identify future extension points and document them. This prevents architectural debt and enables smooth evolution.

```go
// Document extension points in interfaces
type CompletionEngine interface {
    // Core functionality
    Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    
    // Extension points for future features
    SetPreprocessor(preprocessor RequestPreprocessor)       // Input transformation
    SetPostprocessor(postprocessor ResponsePostprocessor)   // Output transformation  
    AddMiddleware(middleware CompletionMiddleware)           // Cross-cutting concerns
    RegisterHook(event string, hook EventHook)              // Event-driven extensions
}

// Extension point documentation
/*
Extension Points:
1. RequestPreprocessor: Transform requests before processing (e.g., prompt enhancement)
2. ResponsePostprocessor: Transform responses after processing (e.g., safety filtering)
3. CompletionMiddleware: Add cross-cutting concerns (e.g., caching, rate limiting)
4. EventHooks: React to completion events (e.g., analytics, logging)

Future Compatibility:
- New model providers can be added via plugin registry
- Processing strategies can be swapped via strategy pattern
- Business rules can be modified via configuration
*/
```

### Backward Compatibility Strategies

Design interfaces and data structures with evolution in mind. Use optional fields, interface segregation, and semantic versioning.

```go
// Evolve-friendly structures
type UserProfile struct {
    // Core fields (never remove)
    ID    string `json:"id"`
    Email string `json:"email"`
    
    // Optional fields (safe to add)
    Metadata map[string]interface{} `json:"metadata,omitempty"`
    Features map[string]bool        `json:"features,omitempty"`
    
    // Version field enables migration detection
    SchemaVersion string `json:"schema_version,omitempty"`
}

// Interface segregation for evolution
type BasicUserReader interface {
    GetUserByID(ctx context.Context, id string) (*User, error)
}

type ExtendedUserReader interface {
    BasicUserReader
    GetUserWithPreferences(ctx context.Context, id string) (*UserWithPreferences, error)
}

// Clients depend on minimal interface they need
type SimpleService struct {
    userReader BasicUserReader  // Can evolve to ExtendedUserReader without breaking
}
```
