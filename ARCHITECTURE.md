# Blueprint-Audio-Backend - Architecture Documentation

## Table of Contents
1. [Executive Summary](#executive-summary)
2. [System Overview](#system-overview)
3. [Architecture Philosophy](#architecture-philosophy)
4. [Module Structure](#module-structure)
5. [Component Interactions](#component-interactions)
6. [Interface & Pattern Analysis](#interface--pattern-analysis)
7. [Design Decisions & Rationale](#design-decisions--rationale)
8. [Data Flow Diagrams](#data-flow-diagrams)
9. [Component Details](#component-details)
10. [Communication Patterns](#communication-patterns)

---

## Executive Summary

**Architecture Pattern:** Modular Monolith with DDD (Domain-Driven Design) and Gateway Pattern

**Key Components:**
- **6 Feature Modules** (Auth, Catalog, Payment, User, Analytics, FileStorage)
- **Gateway Layer** (Router, Middleware, Server)
- **Shared Infrastructure** (Database, Redis, Config)
- **Domain Layer** (Business entities and interfaces)

**Lines of Code:** ~2,700+ lines (main + modules + gateway)

**Design Goals:**
1. ✅ **Separation of Concerns** — Each module owns its domain, services, and persistence
2. ✅ **Dependency Injection via Go Interfaces** — Modules depend on abstractions, not concrete implementations
3. ✅ **Gateway as Orchestration Layer** — Routing and middleware are consolidated into a single gateway
4. ✅ **Shared Infrastructure** — Common services (DB, Redis, Config) shared across modules
5. ✅ **Testability** — Each module can be tested independently with mockable interfaces

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     Client Request (HTTP)                        │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│                  Gateway Layer                            │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Router & Middleware                │   │
│  └──────────────────────────────────────────────────────┘   │
│                      ↓                                     │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Application Modules                │   │
│  │  ┌─────────┬─────────┬──────────┬────┐ │
│  │  │ Auth   │ Catalog │ Payment │ User │
│  │  └───────┴─────────┴──────────┴────┘   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
         ↓               ↓                 ↓               ↓
┌─────────────────────────────────────────────────────────────────┐
│            Shared Infrastructure                           │
│  ┌─────────────────┬─────────────┬──────────────┐ │
│  │   Database   │  Config    │ Redis (Cache)  │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Architecture Philosophy

### 1. Domain-Driven Design (DDD)
Each business domain (Auth, Catalog, Payment, User, Analytics, FileStorage) has:
- **Domain models** — Core business entities (User, Spec, Order, License, etc.)
- **Repositories** — Data access abstractions (UserRepository, SpecRepository, etc.)
- **Services** — Business logic layer (UserService, SpecService, PaymentService, etc.)
- **HTTP Handlers** — External API adapters
- **Module** — Public API for initializing the module

### 2. Dependency Inversion Principle (DIP)
**High-level modules depend on LOW-LEVEL ABSTRACTIONS:**
- Application modules depend on *interfaces* (repository interfaces, service abstractions)
- They do NOT depend on concrete implementations
- This enables:
  - Independent testing with mocks
  - Swapping implementations (PostgreSQL → MongoDB for tests)
  - Easy modification without affecting other modules

### 3. Gateway Pattern
**Gateway acts as the orchestration layer:**
- Router maps HTTP paths to handlers
- Middleware chains (Auth, CORS, Prometheus) wrap handlers
- Server manages HTTP lifecycle (graceful shutdown)

### 4. Clear Separation of Concerns
| Layer | Responsibility |
|-------|---------------|
| **Domain** | Business entities, repository interfaces |
| **Application** | Business logic services |
| **Infrastructure** | Database, Redis, Config, S3 storage |
| **Interfaces** | HTTP handlers, DTOs |
| **Gateway** | Routing, middleware, server |

---

## Module Structure

```
Blueprint-Audio-Backend/
├── cmd/server/           # Application entrypoint
├── internal/
│   ├── gateway/         # Gateway layer (orchestration)
│   │   ├── middleware/    # Auth, CORS, Prometheus
│   │   ├── routes.go       # Route definitions
│   │   ├── router.go       # Server wrapper
│   │   └── server.go       # HTTP server
│   └── modules/         # Business domains
│       ├── auth/         # Authentication module
│       │   ├── domain/      # User entity, errors
│       │   ├── application/  # AuthService
│       │   ├── infrastructure/
│       │   │   ├── jwt/        # JWT provider
│       │   │   ├── persistence/postgres/  # UserRepository
│       │   └── interfaces/http/  # AuthHandler
│       ├── catalog/      # Beats/samples catalog
│       │   ├── domain/      # Spec, Genre, License models
│       │   ├── application/  # SpecService
│       │   ├── infrastructure/persistence/postgres/
│       │   │   ├── spec_repo.go      # SpecRepository
│       │   │   ├── spec_finder.go    # SpecFinder interface
│       │   └── interfaces/http/      # SpecHandler
│       ├── payment/      # Orders, payments, licenses
│       │   ├── domain/      # Order, License models
│       │   ├── application/  # PaymentService
│       │   ├── infrastructure/persistence/postgres/
│       │   │   ├── pg_license_repo.go
│       │   │   ├── pg_order_repo.go
│       │   │   ├── pg_payment_repo.go
│       │   └── interfaces/http/  # PaymentHandler
│       ├── user/         # User profiles
│       │   ├── application/  # UserService
│       │   └── interfaces/http/  # UserHandler
│       ├── analytics/    # Statistics, tracking
│       │   ├── domain/      # Analytics models
│       │   ├── application/  # AnalyticsService
│       │   ├── infrastructure/persistence/postgres/
│       │   │   ├── pg_analytics_repo.go
│       │   └── interfaces/http/  # AnalyticsHandler
│       └── filestorage/  # File upload, S3, local storage
│           ├── domain/      # File interfaces
│           ├── application/  # FileService
│           ├── infrastructure/
│           │   ├── local/  # Local file storage
│           │   └── s3/    # AWS S3/Cloudflare R2 storage
│           └── module.go
└── shared/            # Cross-cutting concerns
    ├── infrastructure/
    │   ├── config/     # Environment configuration
    │   ├── database/   # PostgreSQL connection
    │   └── redis/       # Redis connection
    └── utils/           # Shared utilities (JWT, validators, response helpers)
└── go.mod, go.sum   # Go dependencies
```

---

## Component Interactions

### Main.go → Gateway Layer

```
┌─────────────┐                          main.go
│             │
┌────────────┴────────────┐
│   Gateway Layer           │
└────────────┬────────────┘
    │                │
┌───┴───────┬───────────────┬───────────────┬─────────────┐
│   Router   │ AuthMiddleware │ CORSMiddleware│PrometheusMiddleware│
└───┬────────┴────┴────────────┴────────────┴────────────┴────┘
    │
    │
    ↓
┌───┴──────────┬───────────────┬─────────────┬───────────────┬─────────────┐
│  AuthHandler │ CatalogHandler │ UserHandler  │ PaymentHandler │ AnalyticsHandler │
└───┬─────────┴────────────┴────────────┴────────────┴────────────┘
```

### Gateway → Application Modules

**Router Setup:**
```go
mux := gateway.SetupRoutes(RouterConfig{
    AuthHandler:      authModule.HTTPHandler(),
    AuthMiddleware:   authMiddleware,
    SpecHandler:      catalogModule.HTTPHandler(),
    UserHandler:      userModule.HTTPHandler(),
    PaymentHandler:   paymentModule.HTTPHandler(),
    AnalyticsHandler: analyticsModule.AnalyticsHandler,
})

// Apply middleware chain
handler := CORSMiddleware(mux, allowedOrigins)
handler = PrometheusMiddleware(handler)
```

**Middleware Chain:**
```
HTTP Request → CORS → Prometheus → Auth → Handler → Response
```

---

## Interface & Pattern Analysis

### 1. Repository Interfaces (Why? How?)

**Pattern:** Define behavior, hide implementation details.

**Example - Catalog Module:**
```go
// Domain layer defines the interface
type SpecFinder interface {
    FindByID(ctx, id) (*Spec, error)
    List(ctx, filter) ([]Spec, int, error)
}

// Service layer uses the interface (not concrete repository)
type SpecService struct {
    specFinder SpecFinder  // ← Depends on ABSTRACTION
}
```

**Why use interfaces:**
1. ✅ **Loose coupling** — Service depends on behavior, not implementation
2. ✅ **Testability** — Can inject mock repository during tests
3. ✅ **Flexibility** — Swap implementations without changing service code
4. ✅ **Single Responsibility** — Repository owns data, Service owns logic

### 2. Module Public API (Why exposed?)

**Each module exposes a `Module` struct:**
```go
type Module struct {
    service    *ApplicationService    // Business logic
    repository *RepositoryImpl        // Data access
    handler    *HTTPHandler          // API adapter
}

func (m *Module) HTTPHandler() *HTTPHandler {
    return m.handler
}

func (m *Module) Service() *ApplicationService {
    return m.service
}
```

**Why expose via methods:**
1. ✅ **Controlled access** — Other modules can only get what module provides
2. ✅ **Encapsulation** — Internal implementation hidden
3. ✅ **Explicit dependencies** — Required dependencies passed to `NewModule()`

### 3. Context Pattern (How & Why?)

**Context values injected via middleware:**
```go
// Gateway middleware injects values into request context
const (
    ContextKeyUserId contextKey = "user_id"
    ContextKeyRole   contextKey = "role"
)

// Handler retrieves values
userID, ok := r.Context().Value(gatewayMiddleware.ContextKeyUserId).(uuid.UUID)
```

**Why use context:**
1. ✅ **Request-scoped data** — Each request gets its own context
2. ✅ **Type-safe retrieval** — Explicit type assertion with ok check
3. ✅ **Propagation** — Values flow through middleware chain to handlers

### 4. Configuration Pattern (How? Where?)

**Loaded once, passed to modules:**
```go
// main.go loads and passes config
cfg := config.Load()

// Passed to each module
authModule := auth.NewModule(db, cfg.JWT.Secret, cfg.JWT.Expiry, fsModule.Service())
```

**Why centralized:**
1. ✅ **Single source of truth** — All config in one place
2. ✅ **Easy testing** — Inject test config easily
3. ✅ **Security** — Secrets managed centrally

### 5. Dependency Injection (Manual vs DI Container)

**Current Approach (Manual in main.go):**
```go
// 1. Create repositories
specRepo := catalogPersistence.NewSpecRepository(db)

// 2. Create modules that need them
analyticsModule := analytics.NewModule(db, specRepo, ...)  // ← SpecRepo passed explicitly
catalogModule := catalog.NewModule(db, specRepo, ...)  // ← Same SpecRepo passed again
```

**Issues with manual approach:**
- ❌ **Order matters** — Modules must be initialized in specific order
- ❌ **Fragile** — If specRepo is instantiated twice, get different instances
- ❌ **Implicit coupling** — Hard to track dependencies

**Better approach (DI Container like Wire):**
```go
// Would be cleaner:
//go:generate wire
// Then: app := wire.NewContainer(...)

// Wire automatically resolves dependency graph
```

**Why not using DI yet:**
1. ✅ **Simplicity** — Manual DI is clearer for small codebase
2. ✅ **Visibility** — Dependencies explicit in main.go
3. ✅ **Startup flexibility** — Easier to control initialization order

### 6. Pointer vs Value Semantics (Why pointers?)

**Pointers used for:**
1. ✅ **Optional/nullable fields** — `*string` can be nil
2. ✅ **Large structs** — Avoid copying on assignment
3. ✅ **Interface conformance** — Interfaces expect pointer receivers
4. ✅ **JSON serialization** — `omitempty` works with pointers

**Examples:**
```go
// Domain model - pointer for nullable field
type Spec struct {
    ID             uuid.UUID
    ProducerID     uuid.UUID
    WavURL        *string   // ← Can be nil
    StemsURL      *string   // ← Can be nil
    LicenseOptions  []LicenseOption
}

// Repository method signature uses pointer receiver
func (r *PgSpecRepository) FindByID(ctx context.Context, id uuid.UUID) (*Spec, error)
//                                              ↑
//                                        Pointer enables method calls
```

### 7. Service Layer Pattern

**Services contain business logic:**
```go
type SpecService struct {
    specFinder SpecFinder      // ← Abstraction
    fileService FileService   // ← External dependency
}

func (s *SpecService) CreateSpec(ctx, req) (*Spec, error) {
    // Business logic implementation
    // Delegates to repository for data access
    // Uses file service for storage
}
```

**Why service layer:**
1. ✅ **Orchestration** — Coordinates repository and external services
2. ✅ **Business rules** — Validation, authorization checks
3. ✅ **Transaction management** — Begin/commit/rollback patterns
4. ✅ **Error handling** — Convert technical errors to user-facing errors

---

## Design Decisions & Rationale

### Decision 1: Gateway Orchestration Layer
**Decision:** Consolidate routing and middleware into a `gateway` package.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Separation** | Router and middleware were scattered across codebase. Consolidating provides single entry point. |
| **Testability** | Gateway can be tested independently with mock handlers. |
| **Middleware chain** | Clear order: CORS → Prometheus → Auth → Handler. |
| **Server lifecycle** | Graceful shutdown handled in one place. |

### Decision 2: Module Structure with DDD Boundaries
**Decision:** Each business domain is a self-contained module.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Cohesion** | Related code (Spec, SpecService) stays together. |
| **Loose coupling** | Modules depend on interfaces, not concrete implementations. |
| **Independent testing** | Each module can be tested with mock repositories. |
| **Code organization** | Easy to locate domain-specific code. |

### Decision 3: Repository Interfaces
**Decision:** Define repository interfaces in domain, implement in infrastructure/persistence.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Abstraction** | Hide database details behind interface. |
| **Testability** | Mock repositories for unit tests. |
| **Flexibility** | Can swap PostgreSQL for other storage (MongoDB, Redis) without changing services. |
| **Dependency inversion** | High-level modules depend on abstractions, not implementations. |

### Decision 4: Shared Infrastructure
**Decision:** Database, Redis, Config in `shared/infrastructure`.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Reusability** | All modules can use same connection pool. |
| **Centralized management** | One place to configure DB, Redis. |
| **Lifecycle management** | Connection pooling, graceful shutdown handled centrally. |

### Decision 5: File Storage Abstraction
**Decision:** Interface for file storage with S3 and local implementations.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Swappable storage** | Can use local for development, S3/R2 for production. |
| **Unified API** | All file operations use same interface. |
| **Testing** | Mock file service for unit tests. |

### Decision 6: Context for Authentication
**Decision:** Use `context.Value()` for user ID and role.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Standard pattern** | Go's context package is idiomatic for request-scoped data. |
| **Type safety** | Explicit type assertion prevents panics. |
| **Middleware chain** | Values flow through middleware → handlers. |

### Decision 7: Custom Response Helpers
**Decision:** `response.go` for JSON serialization and error responses.

**Rationale:**
| Consideration | Decision |
|--------------|----------|
| **Consistency** | All endpoints use same error/response format. |
| **HTTP status codes** | Centralized status code management. |
| **Error wrapping** | Wrap technical errors in user-facing messages. |

---

## Data Flow Diagrams

### Request Lifecycle (Complete Flow)

```
┌──────────┐                    ┌──────────┐                    ┌──────────────┐
│  Client  │                    │  Gateway   │                    │ Shared Infra │
└────┬─────┘                    └────┬──────┘                    └────┬───────┘
       │                              │                          │
       │                              │                          │
       ↓                              ↓                          ↓
┌──────────┐                ┌──────────────┐           ┌──────────────┐
│  Middleware   │                │   Context    │           │   Database    │
└────┬─────┘                └────┬──────────┘           └──────┬───────┘
       │                           │                       │
       │                           │                       │
       ↓                           ↓                       ↓
┌──────────────────────┐           ┌──────────────┐           ┌──────────────┐
│  HTTP Handler         │           │   Context    │           │   Repository    │
└────┬────────────────┘           └────┬──────────┘           └──────┬───────┘
       │                           │                       │
       │                           │                       │
       │                           │                       │
       ↓                           ↓                       ↓
┌──────────────────────┐           ┌──────────────┐           ┌──────────────┐
│   Application Service  │           │   Service    │           │   Repository    │
└──────────────────────┘           └──────────────┘           └───────────────┘
       │                           │                       │
       │                           │                       │
       ↓                           ↓                       ↓
┌──────────────────────┐
│   Domain Entity      │
└──────────────────────┘
```

### Module Interactions (Catalog Example)

```
┌──────────────────────────────────────────────────────┐
│              Catalog Module                          │
│  ┌─────────────────────────────────────────────┐ │
│  │ Domain (Spec, Genre, License models)     │ │
│  │ Application (SpecService)                │ │
│  │ Infrastructure (PostgreSQL repository)      │ │
│  │ HTTP Interface (SpecHandler)              │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘

External Dependencies:
├── FileStorage Service (for uploads)
├── Redis (for caching)
└── Config (for settings)
```

### Module Communication (Shared Dependencies)

**Example: Catalog and Analytics sharing SpecRepository**

```
┌──────────────────────────────────────────────────────┐
│                  main.go                          │
│  ┌─────────────────────────────────────────────┐ │
│  │ Create shared resources:                       │ │
│  │   specRepo := catalogPersistence.          │ │
│  │                NewSpecRepository(db)           │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────┐
│     Initialize modules with shared resources:     │
│  ┌─────────────────────────────────────────────┐ │
│  │ catalogModule := catalog.NewModule(        │ │
│  │   db, specRepo, fileService,           │ │
│  │   analyticsService, redisClient)          │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────┐
│              Both modules share specRepo          │
│  ┌─────────────────────────────────────────────┐ │
│  │  catalogModule uses it for:              │ │
│  │   - Finding specs                          │ │
│  └─────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────┐ │
│  │ analyticsModule uses it for:               │ │
│  │   - Tracking plays, downloads              │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
```

---

## Component Details

### Gateway Layer (`internal/gateway`)

#### Router (`router.go`)
```go
type RouterConfig struct {
    AuthHandler      *auth_http.AuthHandler
    AuthMiddleware   *middleware.AuthMiddleWare
    SpecHandler      *catalog_http.SpecHandler
    UserHandler      *user_http.UserHandler
    PaymentHandler   *payment_http.PaymentHandler
    AnalyticsHandler *analytics_http.AnalyticsHandler
}

func SetupRoutes(config RouterConfig) *http.ServeMux
```
**Purpose:** Single entry point for route registration and handler initialization.

#### Middleware (`middleware/`)

**Auth Middleware**
```go
type AuthMiddleWare struct {
    jwtSecret string
}

func (m *AuthMiddleWare) RequireAuth(next http.Handler) http.Handler
```
**Purpose:** Enforce authentication, inject user context.

**CORS Middleware**
```go
func CORSMiddleware(next http.Handler, allowedOrigins string) http.Handler
```
**Purpose:** Enable cross-origin requests, validate allowed origins.

**Prometheus Middleware**
```go
func PrometheusMiddleware(next http.Handler) http.Handler
```
**Purpose:** Track HTTP metrics (request count, status codes, duration).

#### Server (`server.go`)
```go
type Server struct {
    httpServer *http.Server
    port       string
}

func (s *Server) Start() error
```
**Purpose:** Manage HTTP server lifecycle with graceful shutdown (20s timeout).

---

### Auth Module (`internal/modules/auth`)

#### Domain
```go
type User struct {
    ID        uuid.UUID
    Email     string
    Password  string  // bcrypt hashed
    Name      string
    DisplayName string
    Role      string
}
```

#### Repository (`infrastructure/persistence/postgres/user_repo.go`)
```go
type PgUserRepository struct {
    db *sqlx.DB
}

func (r *PgUserRepository) Create(ctx, req) (*domain.User, error)
```
**Purpose:** Direct database access for users.

#### JWT Provider (`infrastructure/jwt/jwt_provider.go`)
```go
type JWTProvider struct {
    secret string
    expiry time.Duration
}

func (p *JWTProvider) GenerateToken(userID, role) (string, error)
```
**Purpose:** JWT token generation and validation.

#### Service (`application/auth_service.go`)
```go
type AuthService struct {
    repo      domain.UserRepository
    jwtSecret string
    jwtExpiry time.Duration
}

func (s *AuthService) Register(ctx, req) (*domain.User, error)
func (s *AuthService) Login(ctx, req) (string, *domain.User, error)
```
**Purpose:** Business logic for registration and authentication.

#### HTTP Handler (`interfaces/http/auth_handler.go`)
```go
type AuthHandler struct {
    service *application.AuthService
}

func (h *AuthHandler) Register(w, r)
func (h *AuthHandler) Login(w, r)
func (h *AuthHandler) Me(w, r)
```
**Purpose:** HTTP request/response handling.

---

### Catalog Module (`internal/modules/catalog`)

#### Domain
```go
type Spec struct {
    ID              uuid.UUID
    ProducerID      uuid.UUID
    ProducerName    string
    Title           string
    Category        Category  // "beat" or "sample"
    Type           string
    BPM             int
    Key             MusicalKey
    ImageUrl        string
    PreviewUrl      string
    WavURL         *string
    StemsURL        *string
    BasePrice       float64
    Description     string
    Duration        int
    FreeMp3Enabled bool
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       *time.Time
    IsDeleted       bool

    // Relations
    Licenses       []LicenseOption
    Genres         []Genre
    Tags            pq.StringArray
}

type LicenseOption struct {
    ID          uuid.UUID
    SpecID       uuid.UUID
    LicenseType LicenseType  // Basic, Premium, Trackout, Unlimited
    Name         string
    Price        float64
    Features     pq.StringArray
    FileTypes    pq.StringArray
}

type Genre struct {
    ID        uuid.UUID
    Name      string
    Slug      string
}
```

#### Interfaces (`interfaces/interfaces.go`)
```go
type SpecFinder interface {
    FindByID(ctx, id) (*Spec, error)
    List(ctx, filter) ([]Spec, int, error)
}

type SpecRepository interface {
    Create(ctx, spec) (*Spec, error)
    Update(ctx, id, data) (*Spec, error)
    Delete(ctx, id, producerID) (*Spec, error)
    FindByProducerID(ctx, producerID, limit, offset) ([]Spec, int, error)
}
```
**Purpose:** Define contracts that modules depend on.

#### Repository (`infrastructure/persistence/postgres/spec_repo.go`)
```go
type PgSpecRepository struct {
    db *sqlx.DB
}

func (r *PgSpecRepository) FindByID(ctx, id) (*Spec, error)
func (r *PgSpecRepository) List(ctx, filter) ([]Spec, int, error)
func (r *PgSpecRepository) Create(ctx, spec) (*Spec, error)
func (r *PgSpecRepository) Update(ctx, id, data) (*Spec, error)
func (r *PgSpecRepository) Delete(ctx, id, producerID) (*Spec, error)
```
**Purpose:** PostgreSQL implementation of SpecRepository.

#### Service (`application/service.go`)
```go
type SpecService struct {
    specFinder    SpecFinder       // ← Interface
    fileService  FileService      // ← External service
}

func (s *SpecService) CreateSpec(ctx, req) (*Spec, error)
func (s *SpecService) ListSpecs(ctx, filter) ([]Spec, int, error)
func (s *SpecService) GetSpec(ctx, id) (*Spec, error)
func (s *SpecService) UpdateSpec(ctx, id, data) (*Spec, error)
func (s *SpecService) DeleteSpec(ctx, id) (*Spec, error)
func (s *SpecService) GetUserSpecs(ctx, userID, page) ([]Spec, int, error)
```
**Purpose:** Business logic for spec operations.

#### HTTP Handler (`interfaces/http/handler.go`)
```go
type SpecHandler struct {
    service       *application.SpecService
    fileService   FileService      // ← Additional dependency
    analyticsService application.AnalyticsService  // ← Additional dependency
    redisClient  *redis.Client       // ← Additional dependency
}
```
**Purpose:** Handle HTTP requests for specs.

---

### Payment Module (`internal/modules/payment`)

#### Domain
```go
type Order struct {
    ID          uuid.UUID
    UserID      uuid.UUID
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Status      OrderStatus
    TotalAmount  float64
}

type License struct {
    ID          uuid.UUID
    OrderID      uuid.UUID
    SpecID       uuid.UUID
    LicenseType LicenseType
    CreatedAt    time.Time
    FileURLs     pq.StringArray  // Download URLs
}
```

#### Interfaces (`interfaces/http/interfaces.go`)
```go
type PaymentService interface {
    CreateOrder(ctx, req) (*domain.Order, error)
    GetUserOrders(ctx, userID, page) (*domain.OrderPagination, error)
    GetOrder(ctx, id) (*domain.Order, error)
    VerifyPayment(ctx, req) (*domain.Order, error)
    GetUserLicenses(ctx, userID, page) (*domain.LicensePagination, error)
    GetLicenseDownloads(ctx, licenseID) ([]string, error)
    GetProducerOrders(ctx, producerID, page) (*domain.OrderPagination, error)
}

type FileService interface {
    GetFileURLs(ctx, fileKeys) (map[string]string, error)
}
```
**Purpose:** Define contracts for payment operations and file access.

#### Service (`application/service.go`)
```go
type PaymentService struct {
    specFinder      catalog_http.SpecFinder    // ← Interface from catalog
    orderRepo       domain.OrderRepository
    licenseRepo      domain.LicenseRepository
    paymentRepo     domain.PaymentRepository
    fileService      FileService      // ← External service
}

func (s *PaymentService) CreateOrder(ctx, req) (*domain.Order, error)
```
**Purpose:** Business logic for orders and payments.

#### HTTP Handler (`interfaces/http/handler.go`)
```go
type PaymentHandler struct {
    service *application.PaymentService
}

func (h *PaymentHandler) CreateOrder(w, r)
func (h *PaymentHandler) GetUserOrders(w, r)
// ... other handlers
```
**Purpose:** Handle HTTP requests for payments.

---

### User Module (`internal/modules/user`)

#### Service (`application/user_service.go`)
```go
type UserService struct {
    repo domain.UserRepository    // ← Interface
}

func (s *UserService) UpdateProfile(ctx, req) (*domain.User, error)
func (s *UserService) UploadAvatar(ctx, req) error
```
**Purpose:** Business logic for user profile operations.

#### HTTP Handler (`interfaces/http/user_handler.go`)
```go
type UserHandler struct {
    service *application.UserService
    fileService FileService      // ← External dependency
}

func (h *UserHandler) UpdateProfile(w, r)
func (h *UserHandler) UploadAvatar(w, r)
func (h *UserHandler) GetPublicProfile(w, r)
func (h *UserHandler) GetUserSpecs(w, r)
```
**Purpose:** Handle HTTP requests for user operations.

---

### Analytics Module (`internal/modules/analytics`)

#### Domain
```go
type SpecAnalytics struct {
    SpecID         uuid.UUID
    PlayCount      int
    FavoriteCount  int
    FreeDownloadCount  int
    TotalPurchaseCount  int
}

type DailyStat struct {
    Date  string
    Count int
}

type TopSpecStat struct {
    SpecID    string
    Title     string
    Plays     int
    Downloads int
    Revenue   float64
}
```

#### Interfaces (`interfaces/http/interfaces.go`)
```go
type SpecRepository interface {
    FindByID(ctx, id) (*Spec, error)
}
```
**Purpose:** Define minimal interface for analytics to access spec data.

#### Service (`application/service.go`)
```go
type AnalyticsService struct {
    specRepo SpecRepository    // ← Interface
    // ... analytics methods
}
```
**Purpose:** Business logic for statistics tracking.

#### HTTP Handler (`interfaces/http/handler.go`)
```go
type AnalyticsHandler struct {
    service *application.AnalyticsService
    specRepo domain.SpecRepository    // ← Additional dependency
}
```
**Purpose:** Handle HTTP requests for analytics.

---

### File Storage Module (`internal/modules/filestorage`)

#### Domain (`domain/file.go`)
```go
type File struct {
    ID        uuid.UUID
    Key       string
    Location  string  // "s3" or "local"
    Size      int64
    MimeType  string
    CreatedAt time.Time
}
```

#### Interfaces (`domain/interfaces.go`)
```go
type FileStorage interface {
    Upload(ctx, file []byte, key, contentType) (*File, error)
    GetFileURLs(ctx, fileKeys) (map[string]string, error)
    Delete(ctx, key) error
}
```
**Purpose:** Define file storage contract.

#### S3 Storage (`infrastructure/s3/s3_storage.go`)
```go
type S3Storage struct {
    client *s3.Client
    bucket string
}

func (s *S3Storage) Upload(ctx, file []byte, key) (*domain.File, error)
func (s *S3Storage) GetFileURLs(ctx, fileKeys) (map[string]string, error)
func (s *S3Storage) Delete(ctx, key) error
```
**Purpose:** AWS S3/Cloudflare R2 implementation.

#### Local Storage (`infrastructure/local/local_storage.go`)
```go
type LocalStorage struct {
    basePath string
}

func (l *LocalStorage) Upload(ctx, file []byte, key) (*domain.File, error)
func (l *LocalStorage) GetFileURLs(ctx, fileKeys) (map[string]string, error)
func (l *LocalStorage) Delete(ctx, key) error
```
**Purpose:** Local file system storage.

#### Service (`application/service.go`)
```go
type FileService struct {
    s3Storage    FileStorage  // ← Interface
    localStorage   FileStorage  // ← Interface
}

func (s *FileService) Upload(ctx, file []byte, key, contentType) (*domain.File, error)
```
**Purpose:** File upload abstraction with S3/local fallback.

---

### Shared Infrastructure

#### Database (`shared/infrastructure/database/postgres.go`)
```go
func NewPostgresDB(cfg Config) (*sqlx.DB, error)
```
**Purpose:** PostgreSQL connection pool with lifecycle management.

#### Redis (`shared/infrastructure/database/redis.go`)
```go
func NewRedisClient(cfg Config) *redis.Client
```
**Purpose:** Redis client creation for caching.

#### Config (`shared/infrastructure/config/config.go`)
```go
type Config struct {
    Server struct
    Database Database
    Redis   Redis
    JWT     JWT
    FileStorage FileStorage
}

func Load() *Config
```
**Purpose:** Environment variable loading and validation.

#### Utils (`shared/utils/`)

**JWT (`jwt.go`)**
```go
type Claims struct {
    UserID uuid.UUID
    Email  string
    Role   string
    jwt.RegisteredClaims
}

func GenerateToken(userID, email, role, secret, expiry) (string, error)
func ValidateToken(tokenString, secret) (*Claims, error)
```
**Purpose:** JWT token generation and validation.

**Response (`response.go`)**
```go
func WriteJSON(w http.ResponseWriter, status int, data interface{})
func WriteError(w http.ResponseWriter, status int, message string)
```
**Purpose:** Standardized JSON response writing.

**Validators (`validators.go`)**
```go
func ValidateEmail(email) error
func ValidatePassword(password) error
```
**Purpose:** Input validation helpers.

---

## Communication Patterns

### 1. Inter-Module Communication

**Pattern:** Modules share dependencies via interfaces.

**Example:** Catalog and Analytics both use SpecRepository
```
┌──────────────────────────────────────────────────────┐
│              main.go                         │
│  ┌─────────────────────────────────────────────┐ │
│  │ specRepo := catalogPersistence.         │
│  │ NewSpecRepository(db)               │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────────────┐
│     Initialize modules:                         │
│  ┌─────────────────────────────────────────────┐ │
│  │ catalogModule := catalog.NewModule(        │ │
│  │ db, specRepo, ...)               │ │
│  └─────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────┐ │
│  │ analyticsModule := analytics.NewModule(      │ │
│  │ db, specRepo, ...)                │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
```

### 2. External Dependencies

**Modules depend on external abstractions:**
- **FileStorage Interface** — Used by Catalog (uploads), Payment (downloads), User (avatar)
- **Config** — Used by all modules
- **Redis** — Used by Catalog (caching spec listings)

### 3. Request Flow

```
HTTP Request
    ↓
CORS Middleware (validate origin, set headers)
    ↓
Prometheus Middleware (record start time, metrics)
    ↓
Auth Middleware (validate JWT, inject user context)
    ↓
HTTP Handler (use service/repository)
    ↓
Application Service (business logic)
    ↓
Repository (database operations)
    ↓
Database (PostgreSQL)
```

### 4. Context Propagation

**Values flow from middleware to handlers:**
```
Request Context
    ↓
CORS: adds headers
    ↓
Prometheus: records timing
    ↓
Auth: adds UserID, Role to context
    ↓
Handler: retrieves values from context
```

---

## Technology Stack

| Layer | Technology | Purpose |
|--------|-----------|---------|
| **Language** | Go 1.25.1 |
| **Web Framework** | Standard library `net/http` |
| **Database** | PostgreSQL (via sqlx) |
| **ORM** | sqlx (type-safe SQL builder) |
| **Caching** | Redis (go-redis/v9) |
| **Object Storage** | AWS S3 SDK v2 (via Cloudflare R2) |
| **Authentication** | JWT (golang-jwt/jwt/v5) |
| **Metrics** | Prometheus (client_golang/prometheus) |
| **Testing** | Standard library `testing`, testify, sqlmock |
| **Payment** | Razorpay (for production) |

---

## Summary of Key Patterns

| Pattern | Description | Where Used |
|----------|-------------|-------------|
| **Interface Segregation** | Separate domain, repository, service layers | All modules |
| **Dependency Injection** | Manual in main.go (shared dependencies passed) | main.go |
| **Context** | Go context for request-scoped data | Middleware → Handlers |
| **Middleware** | Chainable pattern (CORS → Prometheus → Auth) | gateway/middleware/ |
| **Repository** | Interface-based repositories | All modules/infrastructure/persistence |
| **Service** | Business logic with external dependencies | All modules/application |
| **Gateway** | Router + Server abstraction | internal/gateway/ |
| **Shared Infra** | Database, Redis, Config | shared/infrastructure/ |

---

## Future Improvements

### 1. Dependency Injection Container
- **Current:** Manual DI in main.go
- **Improvement:** Use Wire or Fx for compile-time DI
- **Benefit:** Eliminates manual dependency order issues

### 2. Event-Driven Architecture
- **Current:** Direct service calls
- **Improvement:** Event bus for inter-module communication
- **Benefit:** Decouples modules further

### 3. CQRS Pattern
- **Current:** Combined read/write methods in same service
- **Improvement:** Separate Command and Query handlers
- **Benefit:** Better scaling, caching opportunities

### 4. API Versioning
- **Current:** Single API version
- **Improvement:** Version endpoints for breaking changes
- **Benefit:** Safer deployments, backward compatibility

---

## Conclusion

The Blueprint-Audio Backend follows a **Modular Monolith** architecture with clear **DDD boundaries**:

✅ **Modules** are self-contained business domains
✅ **Gateway** provides orchestration and cross-cutting concerns
✅ **Interfaces** enable loose coupling and testability
✅ **Shared Infrastructure** centralizes database, caching, and configuration
✅ **Manual Dependency Injection** provides visibility and control
✅ **Context** provides request-scoped data access

This architecture supports:
- Independent development of modules
- Easy testing with mocks
- Flexible deployment (can swap implementations)
- Clear code organization
- Scalable design for future growth

---

**Document Version:** 1.0
**Generated:** 2026-02-13
**Author:** Blueprint-Audio Backend Team
