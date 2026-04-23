# Staged Email Feature Deep Guide

## Scope

This guide explains the currently staged changes in `blueprint-backend` as one connected feature set.

The staged branch is not just "send some emails". It introduces a broader authentication and notification architecture made of three user-facing capabilities:

1. Email verification for password-based signup.
2. Password reset using one-time email codes.
3. Payment receipt emails after successful payment verification.

It also introduces the shared infrastructure needed to support those flows cleanly:

1. Config and environment variables for email and frontend links.
2. A reusable email sender abstraction.
3. Reusable HTML and text email template builders.
4. Database support for user verification state and single-use email action tokens.
5. Dependency injection wiring from `cmd/server/main.go` down into modules.

---

## The Big Architectural Idea

The code follows a layered/module-oriented design:

1. `cmd/server/main.go`
Composition root. This is where concrete dependencies are created and injected.

2. `internal/shared/infrastructure/*`
Reusable infrastructure used by multiple modules. Email sender and config live here because they are cross-cutting concerns, not auth-specific logic.

3. `internal/modules/auth/domain/*`
Core auth business types and interfaces. This layer describes what auth needs, not how Postgres or HTTP works.

4. `internal/modules/auth/infrastructure/persistence/postgres/*`
Concrete Postgres implementations of auth repositories.

5. `internal/modules/auth/application/auth_service.go`
Use-case orchestration. This is where register, login, verify-email, forgot-password, and reset-password flows are coordinated.

6. `internal/modules/auth/interfaces/http/auth_handler.go`
HTTP transport adapter. It converts JSON requests into application requests and maps domain/application errors to HTTP status codes.

7. `internal/modules/payment/application/service.go`
Business logic for payment verification now also triggers receipt email sending.

8. `internal/gateway/routes.go`
The gateway registers the new auth endpoints.

9. `db/migrations/*`
Schema changes required to persist verification state and email action tokens.

This is a classic ports-and-adapters / dependency-inversion style:

1. Domain/application depend on interfaces.
2. Infrastructure implements those interfaces.
3. `main.go` composes the concrete graph.

---

## Which Architecture Pattern Are We Following Here

Short answer:

This codebase is mostly a mix of:

1. layered architecture,
2. modular monolith structure,
3. ports-and-adapters,
4. dependency injection,
5. repository pattern.

It is not a textbook perfect Clean Architecture repo, but it clearly follows many of the same ideas.

### The closest label

If I had to name it simply, I would call this:

`A modular layered monolith using ports-and-adapters and repository-based persistence`

That description matches the actual code better than forcing a pure marketing label on it.

### What that means in our app

1. `internal/modules/auth`, `payment`, `catalog`, `user`:
   modular monolith slices by business area.
2. `domain`:
   business types and interfaces.
3. `application`:
   use-case orchestration.
4. `infrastructure/persistence/postgres`:
   adapter implementations for storage.
5. `interfaces/http`:
   adapter implementations for HTTP transport.
6. `cmd/server/main.go`:
   composition root for wiring.
7. `internal/shared/infrastructure/*`:
   reusable technical capabilities.

### So are we using the Repository Pattern?

Yes, definitely.

Examples:

1. `domain.UserRepository`
2. `domain.EmailActionTokenRepository`
3. payment/order/license repositories

The repository pattern here means:

1. application services do not directly write SQL,
2. they call interfaces,
3. concrete Postgres repos implement those interfaces.

### Are we using DI?

Yes.

Examples:

1. `main.go` creates `emailSender`
2. `main.go` passes it into auth and payment modules
3. module constructors pass concrete dependencies into services

That is constructor-based dependency injection.

### Are we using Ports And Adapters?

Yes.

Examples:

1. `Sender` is a port, `resendSender` is an adapter.
2. `UserRepository` is a port, `PgUserRepository` is an adapter.
3. auth HTTP handler is an adapter from HTTP into application logic.

### Are we using Clean Architecture?

Partially in spirit, yes.

It shares these Clean Architecture traits:

1. business logic is separated from frameworks,
2. dependencies point inward toward interfaces,
3. infrastructure is replaceable,
4. handlers are thin.

But it is not organized in a strict concentric-circle style everywhere, and some older code still reads env directly in service construction paths, so it is more practical than doctrinaire.

---

## Popular Architecture Patterns And How Ours Compares

Below are common patterns you will hear about, and where this app fits relative to them.

### 1. Layered Architecture

Typical layers:

1. presentation,
2. application/service,
3. domain/business,
4. persistence/infrastructure.

### Folder shape often looks like

```text
app/
  controllers/
  services/
  repositories/
  models/
```

### Our app version

```text
internal/modules/auth/
  domain/
  application/
  infrastructure/persistence/postgres/
  interfaces/http/
```

### Comparison

We absolutely use layered architecture, but instead of one global `services/` and `repositories/` folder for the whole backend, we group layers inside each module. That is stronger than a simple flat layered app because it preserves business boundaries.

### 2. Repository Pattern

Typical idea:

1. application code depends on repository interfaces,
2. repositories hide DB details.

### Folder shape often looks like

```text
repositories/
  user_repository.go
  order_repository.go
```

### Our app version

Interfaces are close to the domain:

```text
internal/modules/auth/domain/user.go
internal/modules/auth/domain/email_token.go
```

Concrete repos are in infrastructure:

```text
internal/modules/auth/infrastructure/persistence/postgres/user_repo.go
internal/modules/auth/infrastructure/persistence/postgres/email_token_repo.go
```

### Comparison

We are using the repository pattern in a cleaner form than many apps because:

1. interfaces live with the business domain,
2. implementations live in infrastructure,
3. services depend on interfaces, not concrete DB code.

### 3. Clean Architecture

Typical idea:

1. entities at the core,
2. use cases around them,
3. interfaces/adapters outside,
4. frameworks at the edge.

### Folder shape often looks like

```text
internal/
  entity/
  usecase/
  interface/
  infrastructure/
```

### Our app version

We do something similar, but per module:

```text
internal/modules/auth/
  domain/
  application/
  infrastructure/
  interfaces/
```

### Comparison

We are very close in spirit to Clean Architecture, but with module-first organization instead of one global set of layers.

### 4. Hexagonal Architecture / Ports And Adapters

Typical idea:

1. define ports as interfaces,
2. plug adapters into them.

### Folder shape often looks like

```text
core/
  ports/
adapters/
  http/
  postgres/
  email/
```

### Our app version

Ports are usually in `domain`, adapters are split across:

1. `interfaces/http`
2. `infrastructure/persistence/postgres`
3. `internal/shared/infrastructure/email`

### Comparison

This app strongly follows ports-and-adapters, just without naming folders literally `ports` and `adapters`.

### 5. Feature-First Modular Monolith

Typical idea:

1. organize by business capability first,
2. keep all pieces of a feature together,
3. stay in one deployable app.

### Folder shape often looks like

```text
modules/
  auth/
  payment/
  catalog/
```

### Our app version

Exactly this, plus inner layering:

```text
internal/modules/
  auth/
  payment/
  user/
  catalog/
```

### Comparison

This is probably the single best top-level description of the repo shape: a modular monolith.

### 6. Traditional MVC

Typical idea:

1. models,
2. views,
3. controllers.

### Folder shape often looks like

```text
controllers/
models/
views/
```

### Comparison with our app

We are not really following MVC.

Why:

1. there is no MVC-style controller/model split driving everything,
2. domain/application/repository separation is stronger,
3. HTTP handlers are thinner than traditional controllers,
4. persistence is abstracted through repos/interfaces.

---

## Folder Structure Comparison

### A flat layered backend might look like this

```text
internal/
  handlers/
  services/
  repositories/
  models/
  db/
```

### Problem with that shape

Over time:

1. auth code spreads across many top-level folders,
2. payment code spreads across many top-level folders,
3. finding one feature's full flow gets harder,
4. cross-feature boundaries become fuzzy.

### Our app structure

```text
cmd/server/main.go

internal/
  gateway/
  modules/
    auth/
      domain/
      application/
      infrastructure/persistence/postgres/
      interfaces/http/
      module.go
    payment/
      domain/
      application/
      infrastructure/persistence/postgres/
      interfaces/http/
      module.go
  shared/
    infrastructure/
      config/
      database/
      email/
```

### Why our structure is stronger

1. Business modules are obvious.
2. Each module keeps its own layers together.
3. Shared technical capabilities are separated from business modules.
4. Wiring is centralized in `main.go`.
5. It scales better than a flat folder-per-layer approach.

### One-line summary of the folder philosophy

Top level:

1. organize by business module first.

Inside each module:

1. organize by architectural layer.

Shared non-business capabilities:

1. keep under `internal/shared/infrastructure`.

---

## What The Feature Actually Changes

### Before

1. Password signup created a user and the user could log in immediately.
2. No verification state was stored on the `users` table.
3. No generic token table existed for one-time email actions.
4. No shared email infrastructure existed.
5. Payment verification ended after license creation.

### After

1. Password signup creates an unverified user.
2. A six-digit verification code is generated, stored as a digest, and emailed.
3. Login for password users is blocked until `email_verified` is true.
4. A generic `email_action_tokens` table stores single-use verification and reset tokens.
5. Forgot-password creates a reset token and sends a reset email.
6. Reset-password consumes the token, updates the password hash, and revokes all sessions.
7. Google login users are created as already verified because Google provides verified identity claims.
8. Successful payment verification now also sends a receipt email.

---

## Composition Root And DI

### File: `cmd/server/main.go`

This is the most important DI file in the whole feature.

It now creates:

1. `emailSender := sharedemail.NewSender(sharedemail.Config{...})`
2. `auth.NewModule(..., emailSender, cfg.AppBaseURL)`
3. `payment.NewModule(..., authModule.UserFinder(), ..., emailSender, cfg.AppBaseURL)`

### Why `main.go` is the DI location

`main.go` is the composition root because:

1. It knows about real config values.
2. It knows which concrete implementations to use.
3. It is allowed to connect modules together.
4. Lower layers should not instantiate their own infrastructure dependencies.

If `AuthService` created its own email sender internally, auth would be tightly coupled to one transport and tests would be harder. Instead, `main.go` injects the dependency.

### Actual injected dependencies

#### Into auth

1. `userRepo`
2. `sessionRepo`
3. `tokenRepo`
4. `emailSender`
5. `appBaseURL`
6. JWT config

#### Into payment

1. `orderRepo`
2. `paymentRepo`
3. `licenseRepo`
4. `specFinder`
5. `userFinder`
6. `fileService`
7. `emailSender`
8. `appBaseURL`

### Why `payment` receives `authModule.UserFinder()`

Payment needs buyer information for receipts, but it should not depend on auth internals like handlers or services. So it depends only on the small domain capability it needs: `UserFinder`.

That is a good DI choice because:

1. It narrows the dependency surface.
2. It prevents service-to-service entanglement.
3. It keeps payment focused on a capability, not another module's internal implementation.

---

## Shared Infrastructure

## Config

### File: `internal/shared/infrastructure/config/config.go`

New config additions:

1. `Email EmailConfig`
2. `AppBaseURL string`

`EmailConfig` contains:

1. `ResendAPIKey`
2. `From`
3. `ReplyTo`
4. `Enabled`

### Why config lives here

This package is the infrastructure boundary for environment loading. The app should read env vars once, normalize them into typed config, and then inject the result downstream.

### Related env wiring files

1. `.env.example`
2. `README.md`
3. `docker-compose.yml`

These changes ensure the new config is not only present in Go code, but also documented and passed into Docker runtime.

---

## Shared Email Sender

### File: `internal/shared/infrastructure/email/email.go`

This file defines the transport abstraction:

1. `type Message struct`
2. `type Sender interface`
3. `type noopSender struct`
4. `type resendSender struct`
5. `func NewSender(cfg Config) Sender`

### Why the interface exists

`Sender` is the port:

```go
type Sender interface {
    Send(ctx context.Context, msg Message) error
}
```

This lets auth and payment depend on an email capability, not on Resend-specific code.

### Why `noopSender` exists

`NewSender` returns a no-op implementation if email is disabled or config is incomplete.

That gives the system:

1. Safe local development.
2. Simple tests.
3. No `nil` checks spread throughout business code.

This is a Null Object pattern.

### Why this is infrastructure, not application logic

Sending HTTP requests to Resend is transport work:

1. Build JSON payload.
2. Create HTTP request.
3. Set headers.
4. Send request.
5. Parse failure conditions.

That is not auth business logic and not payment business logic, so it belongs in shared infrastructure.

### Go concepts used here

1. `interface`
Defines behavior without fixing implementation.

2. `struct`
Holds config and sender state.

3. method receivers
`func (s *resendSender) Send(...) error`

4. `context.Context`
Carries cancellation and deadlines into outgoing HTTP calls.

5. `http.Client`
Reusable HTTP transport client.

6. `json.Marshal`
Serializes the API payload.

7. `io.ReadAll`
Used for limited response-body diagnostics on failure.

8. `strings.TrimSpace`, `strings.TrimRight`
Normalizes config and URLs.

9. `defer resp.Body.Close()`
Ensures resource cleanup.

---

## Shared Email Templates

### Files

1. `internal/shared/infrastructure/email/templates.go`
2. `internal/shared/infrastructure/email/templates/layout.html`
3. `internal/shared/infrastructure/email/templates/payment-receipt.html`
4. `internal/shared/infrastructure/email/templates/reset-password.html`
5. `internal/shared/infrastructure/email/templates/verify-email.html`

### Why templates are separate from the sender

Template generation and transport are different responsibilities.

1. Template builder decides message content.
2. Sender decides how content is delivered.

This separation makes it easy to:

1. reuse the same sender for many email types,
2. test rendering separately,
3. change providers without rewriting content logic.

### What `templates.go` does

It builds strongly shaped email messages for:

1. verification,
2. password reset,
3. payment receipt.

It also centralizes link building with:

1. `buildLink(baseURL, path, params)`

That matters because the frontend routes must be consistent across all email flows.

### Why `//go:embed` is used

`templates.go` uses:

```go
//go:embed templates/*.html
var templateFS embed.FS
```

This embeds template files into the binary at build time.

Benefits:

1. No separate template deployment problem.
2. No filesystem dependency at runtime.
3. Easier container deployment.

### Why both `HTML` and `Text` are built

`Message` supports both:

1. HTML for rich email clients.
2. Plain text for compatibility and accessibility.

---

## Auth Domain Layer

### File: `internal/modules/auth/domain/user.go`

The `User` entity gains:

1. `EmailVerified bool`
2. `EmailVerifiedAt *time.Time`

### Why this belongs on the user entity

Email verification is user identity state, not transient request state.

It affects:

1. whether login is allowed,
2. whether verification resend should do anything,
3. whether external identity providers should mark a user verified.

### File: `internal/modules/auth/domain/email_token.go`

Adds:

1. `TokenPurpose`
2. `TokenPurposeVerifyEmail`
3. `TokenPurposeResetPassword`
4. `EmailActionToken`
5. `EmailActionTokenRepository`

### Why a dedicated token domain type was added

This model represents a real business concept:

1. a one-time code,
2. tied to a user and email,
3. scoped by purpose,
4. expiring,
5. single-use.

This is richer than just "string code in memory".

### Why `purpose` is explicit

The same code storage mechanism is reused for two flows:

1. email verification,
2. password reset.

Explicit purpose prevents cross-use. A reset code cannot verify email, and a verification code cannot reset a password.

### File: `internal/modules/auth/domain/errors.go`

New domain errors:

1. `ErrEmailNotVerified`
2. `ErrInvalidOrExpiredCode`

Why they are domain errors:

1. They represent business outcomes.
2. HTTP can map them to status codes.
3. Tests can assert them precisely.

---

## Auth Persistence Layer

### File: `internal/modules/auth/infrastructure/persistence/postgres/user_repo.go`

Changes:

1. `Create` now inserts `email_verified` and `email_verified_at`.
2. Adds `MarkEmailVerified`.
3. Adds `UpdatePassword`.

### Why these methods belong in the user repo

They mutate durable user state in the `users` table:

1. verification state,
2. password hash.

That is repository work.

### File: `internal/modules/auth/infrastructure/persistence/postgres/email_token_repo.go`

This is the concrete implementation of `EmailActionTokenRepository`.

Methods:

1. `Create`
2. `Consume`
3. `InvalidateActive`

### Important behaviors

#### Codes are stored as digests

`digestEmailActionCode` hashes the raw code with SHA-256 before storage.

Reason:

1. plaintext recovery from DB should not reveal valid codes,
2. one-time secrets should be treated like credentials,
3. the system can still verify by digesting incoming code and comparing.

#### `InvalidateActive`

Before a new token is created for a user/purpose pair, all active unconsumed tokens for that pair are marked consumed.

Why:

1. prevents multiple valid codes existing at once,
2. simplifies UX,
3. reduces confusion,
4. reduces token replay surface.

#### `Consume`

This is an atomic "check-and-use" SQL update:

1. match email,
2. match purpose,
3. match digest,
4. ensure not consumed,
5. ensure not expired,
6. mark consumed,
7. return the token row.

This is safer than "select then update" because it shrinks race windows.

---

## Auth Application Layer

### File: `internal/modules/auth/application/auth_service.go`

This is the core orchestration file.

New service fields:

1. `tokenRepo`
2. `emailSender`
3. `appBaseURL`

New request DTOs:

1. `VerifyEmailRequest`
2. `ResendVerificationRequest`
3. `ForgotPasswordRequest`
4. `ResetPasswordRequest`

### Why this file changed the most

Because application services are where use cases are coordinated. They:

1. validate inputs,
2. call repositories,
3. construct business state,
4. trigger side effects,
5. map security/business rules into behavior.

### Constructor changes

`NewAuthService(...)` now receives more dependencies through constructor injection.

This is a standard Go DI pattern:

1. concrete values are passed in,
2. the service stores interfaces,
3. the service does not create its own infrastructure.

### Registration flow

`Register` now:

1. validates request,
2. hashes password with bcrypt,
3. creates user with `EmailVerified: false`,
4. persists the user,
5. triggers `sendVerificationCode`.

### Why registration still succeeds if email sending fails

The code logs send failure but does not roll back user creation.

That design says:

1. account creation is the critical write,
2. notification delivery is important but secondary,
3. user can still request resend later.

### Login flow

`Login` now:

1. validates fields,
2. fetches user,
3. checks password hash,
4. blocks login if `!user.EmailVerified`,
5. creates session tokens only after verification passes.

### Why verification check belongs in login

Because login is the policy gate for session issuance.

### Google login flow

Google-created users are now marked:

1. `EmailVerified: true`
2. `EmailVerifiedAt: timePtr(time.Now())`

Why:

1. Google identity token already includes verified identity context.
2. Requiring email verification again would duplicate trust checks.

### Verify email flow

`VerifyEmail`:

1. validates request,
2. consumes a verify-email token,
3. marks the user verified.

### Why consume comes before mark-verified

Because the code itself is the proof. If consumption fails, verification must not happen.

### Resend verification flow

`ResendVerification`:

1. validates email,
2. fetches user by email,
3. returns success even if user does not exist,
4. returns success if user is already verified,
5. otherwise sends a new verification code.

### Why generic success is used

This avoids account enumeration. An attacker should not learn whether an email exists.

### Forgot password flow

`ForgotPassword`:

1. validates email,
2. fetches user,
3. returns success even if user does not exist,
4. creates a reset token,
5. sends reset email.

### Reset password flow

`ResetPassword`:

1. validates fields,
2. consumes reset token,
3. bcrypt-hashes the new password,
4. updates password in DB,
5. revokes all sessions for the user.

### Why revoke all sessions

After a password reset, old sessions may no longer be trustworthy. Revoking them is the safer security posture.

### Internal helper methods

1. `sendVerificationCode`
2. `createEmailActionToken`
3. `generateEmailCode`
4. `timePtr`

`generateEmailCode` uses `crypto/rand` and `math/big`, which is important because verification/reset codes are security-sensitive and should come from cryptographic randomness rather than predictable pseudo-random generators.

---

## Auth HTTP Layer

### File: `internal/modules/auth/interfaces/http/auth_handler.go`

The handler interface expands with:

1. `VerifyEmail`
2. `ResendVerification`
3. `ForgotPassword`
4. `ResetPassword`

### Handler responsibility

The handler is not the place for business rules. It should:

1. decode JSON,
2. call the service,
3. map service/domain errors to HTTP status codes,
4. encode JSON response.

### New HTTP error semantics

1. login with unverified email returns `403 Forbidden`
2. invalid or expired verification/reset code returns `401 Unauthorized`
3. malformed payload returns `400 Bad Request`

### Why error mapping is done here

Because HTTP is a transport concern. Domain/application should not know about status codes.

---

## Gateway Layer

### File: `internal/gateway/routes.go`

New routes:

1. `POST /auth/verify-email`
2. `POST /auth/resend-verification`
3. `POST /auth/forgot-password`
4. `POST /auth/reset-password`

### Why routes live here

This file is the gateway registration point. It is responsible for exposing handler methods as actual HTTP endpoints.

---

## Payment Integration

### Files

1. `internal/modules/payment/module.go`
2. `internal/modules/payment/application/service.go`

### What changed in payment module wiring

`payment.NewModule(...)` now requires:

1. `userFinder`
2. `emailSender`
3. `appBaseURL`

### Why payment gets shared email infrastructure directly

Payment needs to send a receipt, but receipt sending is not an auth concern. So payment uses the same shared sender, not the auth service.

That avoids:

1. fake cross-module dependencies,
2. service-to-service coupling,
3. turning auth into a general email relay.

### What changed in payment service

New fields:

1. `userFinder`
2. `emailSender`
3. `appBaseURL`

New behavior:

1. after successful payment verification,
2. after payment persistence,
3. after order status update,
4. after license issuance,
5. send receipt email via `sendReceiptEmail`.

### Why receipt failure does not fail payment verification

`VerifyPayment` logs receipt send failure but still returns success after license issuance.

That design says:

1. payment capture + order + license are business-critical,
2. receipt delivery is important but should not invalidate a successful purchase.

### `sendReceiptEmail`

This method:

1. gets the buyer via `userFinder`,
2. prefers payment email if present,
3. derives spec title from order notes,
4. formats amount,
5. builds the receipt message with shared template builders,
6. sends it through the shared sender.

---

## Database Changes

### File: `db/migrations/000025_add_email_verification_to_users.up.sql`

Adds:

1. `email_verified BOOLEAN NOT NULL DEFAULT false`
2. `email_verified_at TIMESTAMP WITH TIME ZONE`

Then backfills existing users to verified.

### Why old users are backfilled as verified

Without this backfill, every existing account would suddenly be locked out on deploy.

### File: `db/migrations/000026_create_email_action_tokens.up.sql`

Creates `email_action_tokens` with:

1. `id`
2. `user_id`
3. `email`
4. `purpose`
5. `code_digest`
6. `expires_at`
7. `consumed_at`
8. timestamps

Indexes:

1. lookup index on `(email, purpose, code_digest)`
2. active-token invalidation index on `(user_id, purpose)`

### Why a separate table exists

Because one-time action codes are not user state. They are short-lived workflow artifacts.

Separating them gives:

1. cleaner data modeling,
2. support for multiple token types,
3. single-use tracking,
4. auditability,
5. expiry semantics.

---

## File-By-File Staged Inventory

This section lists every staged file and why it exists in this change set.

### Config, bootstrap, runtime docs

1. `.env.example`
Documents new email-related env vars and frontend base URL.

2. `README.md`
Adds email config documentation.

3. `docker-compose.yml`
Passes email/app URL env vars into the API container.

4. `cmd/server/main.go`
Composition root that creates `emailSender` and injects it into auth and payment.

5. `docs/backend-email-architecture.md`
Large architecture narrative already staged in the branch.

6. `docs/email-auth-curls.md`
Manual API walkthrough for register, verify, forgot-password, reset-password.

### Database

7. `db/migrations/000025_add_email_verification_to_users.up.sql`
Adds verification columns and backfills old users.

8. `db/migrations/000025_add_email_verification_to_users.down.sql`
Rollback for the verification columns.

9. `db/migrations/000026_create_email_action_tokens.up.sql`
Creates token table and indexes.

10. `db/migrations/000026_create_email_action_tokens.down.sql`
Rollback for the token table.

### Shared infrastructure

11. `internal/shared/infrastructure/config/config.go`
Typed config loading for email and app base URL.

12. `internal/shared/infrastructure/email/email.go`
Shared sender abstraction and Resend transport.

13. `internal/shared/infrastructure/email/templates.go`
Reusable message builders and link generation.

14. `internal/shared/infrastructure/email/templates/layout.html`
Base HTML email layout.

15. `internal/shared/infrastructure/email/templates/payment-receipt.html`
Receipt-specific template body.

16. `internal/shared/infrastructure/email/templates/reset-password.html`
Password reset template body.

17. `internal/shared/infrastructure/email/templates/verify-email.html`
Email verification template body.

18. `internal/shared/infrastructure/email/templates_test.go`
Unit tests for template rendering and content.

### Auth domain, persistence, application, transport

19. `internal/modules/auth/domain/user.go`
Adds verification state to the user entity and user repository contract.

20. `internal/modules/auth/domain/email_token.go`
Defines token domain model and token repository contract.

21. `internal/modules/auth/domain/errors.go`
Adds auth-specific business errors for unverified email and invalid/expired codes.

22. `internal/modules/auth/infrastructure/persistence/postgres/user_repo.go`
Persists verification flags and supports password/verification updates.

23. `internal/modules/auth/infrastructure/persistence/postgres/email_token_repo.go`
Postgres implementation for email action tokens.

24. `internal/modules/auth/application/auth_service.go`
Core orchestration for register/login/verify/resend/forgot/reset.

25. `internal/modules/auth/interfaces/http/auth_handler.go`
HTTP endpoints and error mapping for the new auth flows.

26. `internal/modules/auth/module.go`
Auth module wiring now includes token repo, sender, and app URL.

27. `internal/gateway/routes.go`
Exposes the new auth endpoints.

### Payment integration

28. `internal/modules/payment/application/service.go`
Adds receipt sending after successful payment verification.

29. `internal/modules/payment/module.go`
Injects user finder, sender, and app URL into payment service.

### Tests updated for contract changes

30. `internal/modules/auth/application/auth_service_test.go`
Adds mocks for token repo and email sender and tests new auth flows.

31. `internal/modules/auth/interfaces/http/auth_handler_test.go`
Extends the mock auth service interface for new methods.

32. `internal/modules/auth/interfaces/http/auth_handler_more_test.go`
Adds endpoint and login-branch tests for new HTTP behavior.

33. `internal/modules/auth/module_test.go`
Updates auth module constructor tests for new injected dependencies.

34. `internal/modules/payment/application/service_test.go`
Adds user finder and email sender mocks and verifies receipt flow.

35. `internal/modules/payment/module_test.go`
Updates payment module constructor tests for new DI.

36. `internal/modules/user/application/user_service_test.go`
Extends mock user repo to satisfy the expanded auth user repository contract.

37. `internal/modules/user/module_test.go`
Same contract adaptation for user module tests.

38. `internal/shared/utils/coverage_smoke_test.go`
Updates smoke test construction to satisfy auth module DI changes.

---

## Core Go Concepts Used In This Feature

### `package`

Defines the namespace/module file belongs to. It is how the codebase separates shared infrastructure, auth, payment, gateway, and server bootstrap.

### `import`

Pulls in dependencies from the standard library and internal packages.

### `type struct`

Used for:

1. entities like `User`,
2. services like `AuthService`,
3. config like `EmailConfig`,
4. DTOs like `ResetPasswordRequest`,
5. adapters like `resendSender`.

### `type interface`

Used heavily for dependency inversion:

1. `Sender`
2. `EmailActionTokenRepository`
3. `UserRepository`
4. `UserFinder`

### method receivers

Example:

```go
func (s *AuthService) ResetPassword(...)
```

This binds behavior to a type.

### pointers

Used when:

1. the receiver should mutate or avoid copying large state,
2. a field is optional, like `*time.Time`,
3. a repository returns an entity pointer.

### `context.Context`

Flows from HTTP handlers into services and repos and then into database or HTTP calls. It supports cancellation and timeout propagation.

### `error`

Go's explicit error model is used throughout instead of exceptions.

### sentinel errors

Examples:

1. `domain.ErrEmailNotVerified`
2. `domain.ErrInvalidOrExpiredCode`

These are compared with `errors.Is`.

### constructors

Examples:

1. `NewSender`
2. `NewAuthService`
3. `NewModule`

These make dependency injection explicit.

### DTOs

Request structs such as `VerifyEmailRequest` and `ForgotPasswordRequest` are transport/application boundary shapes.

### repository pattern

Repository interfaces abstract persistence operations away from application logic.

### composition root

`cmd/server/main.go` builds the full dependency graph.

### Null Object pattern

`noopSender` implements `Sender` and safely does nothing.

### embedded assets

`embed.FS` compiles HTML templates into the Go binary.

### crypto primitives

1. `bcrypt` for password hashing,
2. `sha256` for token digest storage,
3. `crypto/rand` for secure code generation,
4. HMAC-SHA256 for Razorpay payment signature verification.

### UUID v7

The code uses `uuid.NewV7()` for sortable unique identifiers with modern time-ordered properties.

### tags

Struct tags like:

1. ``json:"email"``
2. ``db:"email"``

control JSON serialization and SQL mapping.

### `map[string]any`

Used for flexible note payloads and JSON-like data where a fixed struct is not used.

---

## End-To-End Runtime Walkthrough

### Register to verify

1. Client calls register.
2. Auth handler decodes JSON.
3. Auth service validates input and hashes password.
4. User repo creates user with `email_verified=false`.
5. Auth service invalidates old verification tokens.
6. Auth service generates six-digit code.
7. Token repo stores digest in `email_action_tokens`.
8. Shared email template builder creates verification message.
9. Shared sender sends email through Resend or noop sender.
10. Later client calls `/auth/verify-email`.
11. Token repo atomically consumes the token.
12. User repo marks the user verified.

### Forgot password to reset

1. Client calls `/auth/forgot-password`.
2. Service returns generic success for missing users.
3. For real users, a reset token is created and emailed.
4. Client calls `/auth/reset-password`.
5. Service consumes reset token.
6. Service bcrypt-hashes new password.
7. User repo updates the password.
8. Session repo revokes all sessions.

### Payment to receipt

1. Client completes payment externally.
2. Backend verifies Razorpay signature and capture status.
3. Payment record is persisted.
4. Order status changes to paid.
5. License is issued.
6. Payment service fetches buyer via `userFinder`.
7. Shared template builder creates receipt email.
8. Shared sender sends it.

---

## Design Principles Used

1. Shared infrastructure should be reusable across modules.
2. Modules should depend on capabilities, not on each other's internal services.
3. Application services should own orchestration.
4. HTTP handlers should stay thin.
5. Security-sensitive codes should not be stored in plaintext.
6. Notification failure should not destroy core business success paths.
7. Existing users should not be broken by new verification requirements.

---

## Final Mental Model

Think of the staged branch as adding a new cross-cutting capability: transactional email with secure action tokens.

Auth uses it for identity workflows.
Payment uses it for commerce workflows.
`main.go` wires it once.
Shared infrastructure provides sender and templates.
Domain and repositories protect the security model.
Handlers expose the flows as HTTP APIs.

That is why the feature touches so many files: it is not a single endpoint change, it is a full-stack backend capability addition.

---

## How To Add A New Feature From Scratch In This Codebase

This section is the practical blueprint for building a new backend feature here.

Do not start with handlers first.
Do not start with `main.go` first.
Do not start with random repo methods first.

Start from the business capability and move outward in layers.

---

## The Correct Feature-Build Order

When adding a new feature, use this order:

1. Define the user/business outcome.
2. Identify which module owns the feature.
3. Decide whether new state must be persisted.
4. Model domain types and interfaces.
5. add or update migrations if storage changes.
6. implement repository methods.
7. implement application service/use-case orchestration.
8. add shared infrastructure only if the feature is cross-cutting.
9. wire the module constructor.
10. wire `main.go` only after constructor dependencies are clear.
11. expose HTTP handlers and routes.
12. add config/env only if runtime behavior truly needs it.
13. update tests at each layer.
14. add docs/manual curl examples if externally visible.

That order matters because each layer answers a different question.

1. Domain answers: what is the concept?
2. Repository answers: how is it persisted?
3. Service answers: what is the workflow?
4. Handler answers: how does HTTP reach it?
5. `main.go` answers: what concrete things must be injected?

---

## Step 1: Define The Feature In One Sentence

Before touching code, write the feature as:

1. Actor
2. action
3. success result
4. failure rules
5. side effects

Example:

"A signed-in user can request export generation, the system stores an export job, processes it asynchronously, and optionally notifies the user when complete."

This tells you immediately:

1. which module probably owns it,
2. whether DB changes are needed,
3. whether background processing is needed,
4. whether shared infra like email/queue/storage is needed.

If you cannot express the feature clearly, you are not ready to implement it.

---

## Step 2: Choose The Owning Module

Ask:

1. Which business area owns the rule?
2. Which module's language matches the feature?
3. Which module should answer "is this allowed?" and "what happens next?"

Examples:

1. login, verification, password reset -> `auth`
2. purchase verification, license issuance -> `payment`
3. spec listing, licensing options -> `catalog`
4. reusable email transport -> `internal/shared/infrastructure`

### Rule

Put the feature where the business rule lives, not where it is easiest to hack.

Bad example:

1. putting email receipt logic into auth just because auth already has email code.

Good example:

1. payment owns receipt sending, but uses shared email infrastructure.

---

## Step 3: Decide Whether New Persistent State Exists

Ask:

1. Is the feature purely computational?
2. Does the system need to remember something later?
3. Does the state survive process restarts?
4. Do we need history, expiry, or auditability?

If yes, you likely need:

1. a migration,
2. domain model updates,
3. repository methods.

### When to create a new table

Create a new table when the concept is its own lifecycle object.

Examples:

1. email action tokens,
2. export jobs,
3. notification deliveries,
4. audit logs.

### When to add columns to an existing table

Add columns when the new data is part of the entity itself.

Examples:

1. `users.email_verified`,
2. `users.email_verified_at`.

### Practical question

Ask: "If I deleted this row/field, what concept disappears?"

1. If the answer is "part of the user identity", add columns.
2. If the answer is "a separate event/workflow object", create a table.

---

## Step 4: Write Domain First

In this codebase, domain is where you describe the business shape before transport or infrastructure.

Write first:

1. entity/value object types,
2. enums/constants,
3. repository interfaces,
4. capability interfaces for other modules,
5. domain errors.

### Example checklist

If adding `export jobs`, domain might need:

1. `ExportJob struct`
2. `ExportStatus` constants
3. `ExportRepository interface`
4. `ErrExportNotFound`
5. maybe `Exporter interface` if generation is pluggable

### Why this comes before repository implementation

Because implementation should satisfy business contracts, not invent them as it goes.

---

## Step 5: Add Migrations Early

If the feature needs schema changes, write migrations before deep service work.

Why:

1. schema shape influences repo design,
2. repo shape influences service logic,
3. it prevents accidental design drift.

### Migration design checklist

1. What table/column/index is required?
2. What constraints enforce correctness?
3. What indexes are required for lookup patterns?
4. Do existing rows need backfill?
5. What should happen on delete, cascade, or nullability?

### Example from this branch

1. `users` got verification columns.
2. `email_action_tokens` got indexes shaped around lookup and invalidation patterns.

That is a strong pattern: indexes follow read/write behavior, not just data existence.

---

## Step 6: Implement Repository Methods Next

Once domain and schema are clear, implement persistence in the module's infrastructure layer.

For auth, that is:

1. `internal/modules/auth/infrastructure/persistence/postgres/*`

### Repo design rule

Repositories should do database work, not orchestration.

Good repo responsibilities:

1. insert/update/select/delete,
2. atomic SQL transitions,
3. mapping SQL rows to domain models.

Bad repo responsibilities:

1. sending emails,
2. deciding whether login should be allowed,
3. calling external providers,
4. composing HTTP responses.

### What to write first inside a repo

1. constructor
2. smallest required query methods
3. atomic update methods
4. error mapping for "not found" style cases

### Important question

Ask: "Can this be one SQL operation instead of select-then-update?"

If yes, prefer the atomic version when correctness matters.

---

## Step 7: Write Application Service Flow After Repos

Now build the use case in the application layer.

This is where the feature's real behavior lives.

### Service checklist

1. validate input,
2. load required state,
3. enforce business rules,
4. call repositories,
5. call external capabilities if needed,
6. return domain/application errors,
7. keep transport details out.

### What to write first in the service

1. request/response DTOs if needed,
2. constructor signature,
3. main use-case method,
4. small private helpers for repeated logic.

### Where side effects should happen

Side effects like email, storage upload, queue publish, webhook, etc. should usually be orchestrated in the application service, because that is where the workflow is coordinated.

---

## Step 8: Decide If The Dependency Is Shared Infrastructure Or Module-Specific

This is one of the most important architecture decisions.

Ask:

1. Will only one module ever need this?
2. Is it a domain concept or a technical capability?
3. Could another module reuse it without knowing business rules?

### Put it in shared infrastructure when

1. it is a technical capability,
2. it can be reused across modules,
3. it should not encode one module's business logic.

Examples:

1. email sender,
2. config loader,
3. DB connection,
4. storage client.

### Put it in a module when

1. it is business-specific,
2. it speaks the language of that module,
3. it owns that module's use cases.

Examples:

1. auth verification policy,
2. payment license issuance,
3. catalog search rules.

---

## Step 9: How To Know What To Inject

Only inject dependencies that a constructor truly needs to do its job.

### Constructor rule

A constructor parameter should exist if:

1. the service/module uses it directly,
2. it is stable enough to be passed in once,
3. it is not just incidental configuration hidden elsewhere.

### In practice

Inject:

1. repositories,
2. shared infrastructure capabilities,
3. narrow interfaces from other modules,
4. typed config values,
5. clocks/clients when needed for testing or integration.

Do not inject:

1. whole modules if you only need one method,
2. giant god-objects,
3. raw environment access from deep layers.

### Good dependency shape

Instead of injecting "AuthModule", inject:

1. `UserFinder`

Instead of injecting "everything storage can do forever", inject the smallest interface the feature uses.

This is how you know what to inject:

Ask: "What capability does this service need, in the smallest stable shape?"

---

## Step 10: How To Know When `main.go` Must Change

`main.go` changes only when:

1. a module constructor gained a new dependency,
2. a new shared infrastructure object must be created,
3. a new config value must be loaded and passed in,
4. a new module must be wired into the app graph.

### What should never happen

You should never start by editing `main.go` and hoping the rest falls into place.

`main.go` is last-mile wiring, not where features are designed.

### Mental model

1. domain defines contracts,
2. service needs dependencies,
3. module constructor exposes those needs,
4. `main.go` satisfies them.

---

## Step 11: How To Know Whether Another Module Is Needed

Ask:

1. Does the feature need business data owned by another module?
2. Does it need only a small read capability?
3. Is there already an interface exposed for that?

### Preferred pattern

Expose a narrow domain interface from the owning module.

Example:

1. payment needs user data,
2. auth owns users,
3. auth exposes `UserFinder`,
4. payment depends on `UserFinder`, not on auth service internals.

### If no suitable interface exists

Add one in the owning module's domain layer.

For example:

1. `type UserFinder interface { FindByID(...); Exists(...) }`

This keeps module boundaries clean.

---

## Step 12: Add HTTP Only After Service Logic Is Solid

After service logic works, add transport.

### Handler checklist

1. decode request body,
2. validate transport-level issues,
3. call application service,
4. map business errors to status codes,
5. encode response.

### Route checklist

1. add handler method,
2. register route in `internal/gateway/routes.go`,
3. protect with middleware if needed.

### Middleware question

Ask:

1. Is this public?
2. Does this require auth?
3. Does it require role checks?
4. Does it need rate-limiting or special middleware?

---

## Step 13: Add Config Only When Runtime Needs It

Do not invent config just because a feature exists.

Add config when:

1. environment-specific values vary by deployment,
2. secrets are needed,
3. external URLs or provider IDs are needed,
4. behavior needs toggling.

### Config flow

1. add typed field in `config.go`,
2. load from env,
3. document in `.env.example`,
4. document in `README.md`,
5. pass in Docker if containerized,
6. inject into `main.go`,
7. pass into module/service constructor.

### Bad pattern

Reading `os.Getenv` deep inside business logic.

### Better pattern

Load once in config and inject the needed value.

Exception:

Some older code may already read env in-place, like payment's Razorpay client. When adding new work, prefer the cleaner injected approach instead of spreading the older pattern further.

---

## Step 14: Write Tests In The Same Layered Order

The easiest way to avoid architecture drift is to test layer by layer.

### Test order

1. domain/helpers if needed,
2. repository behavior,
3. application service orchestration,
4. handler status-code mapping,
5. module constructor wiring smoke tests,
6. shared infrastructure tests if introduced.

### What service tests should prove

1. happy path,
2. validation path,
3. security path,
4. "not found" path,
5. external dependency failure path,
6. side-effect behavior.

### What handler tests should prove

1. request decoding,
2. status mapping,
3. response structure.

---

## Step 15: Add Docs For External Flows

If the feature creates a user-visible or integrator-visible API, add docs.

Examples:

1. curl examples,
2. architecture notes,
3. setup steps,
4. environment requirements.

This branch already models that well with:

1. `docs/backend-email-architecture.md`
2. `docs/email-auth-curls.md`

---

## A Concrete Template You Can Reuse

When building a new feature, ask these in order:

1. What business problem am I solving?
2. Which module owns that problem?
3. What state must be stored?
4. Do I need a migration?
5. What domain types/errors/interfaces are needed?
6. What repo methods are required?
7. What application method orchestrates the flow?
8. Do I need a new shared capability?
9. What dependencies must the constructor receive?
10. Does `main.go` need new wiring?
11. Do I need new env/config?
12. What HTTP endpoints/routes are required?
13. Which middleware applies?
14. Which tests prove correctness?
15. What docs/examples should be added?

If you follow that sequence, you usually will not get lost.

---

## What To Write First, Literally

If you are starting from an empty feature idea, write files in roughly this order:

1. migration files if schema changes are obvious,
2. domain types/interfaces/errors,
3. repo methods,
4. service method and DTOs,
5. shared infra if required,
6. module constructor changes,
7. `main.go` wiring,
8. handler methods,
9. routes,
10. config/docs/env,
11. tests,
12. curl docs.

### Why this order works

Because each step reduces ambiguity for the next one.

---

## How To Avoid Common Mistakes

### Mistake 1: Starting from HTTP

Problem:

1. you end up designing around payload shape instead of business model.

Fix:

1. start from domain and service.

### Mistake 2: Injecting whole modules

Problem:

1. massive coupling,
2. unclear ownership,
3. hard tests.

Fix:

1. inject narrow interfaces like `UserFinder`.

### Mistake 3: Putting reusable infra in a module

Problem:

1. other modules become forced to depend on the wrong business area.

Fix:

1. move technical capability to `internal/shared/infrastructure`.

### Mistake 4: Putting business logic in repos

Problem:

1. orchestration becomes hidden in SQL layer,
2. testing gets awkward,
3. business rules scatter.

Fix:

1. repos persist,
2. services orchestrate.

### Mistake 5: Reading env vars everywhere

Problem:

1. hard to test,
2. hard to reason about,
3. config becomes invisible.

Fix:

1. load once,
2. inject typed config.

### Mistake 6: Not thinking about existing data

Problem:

1. deploy breaks older users or rows.

Fix:

1. backfill in migrations when introducing new required business rules.

---

## The Short Heuristic

When you are unsure, use this:

1. If it is business meaning, put it in domain/application.
2. If it is storage, put it in repo/infrastructure.
3. If it is HTTP, put it in handlers/routes.
4. If it is reusable technical plumbing, put it in shared infrastructure.
5. If it is object creation/wiring, put it in `main.go`.
6. If another module is needed, depend on a small interface from that module's domain.

---

## Final Build Playbook

For a new feature in this repo, the safe workflow is:

1. define the feature clearly,
2. pick the owning module,
3. model the data and rules,
4. add migrations if state changes,
5. add repo methods,
6. add service orchestration,
7. add shared infra only if reusable,
8. update module constructors,
9. wire dependencies in `main.go`,
10. expose handlers and routes,
11. add config only where needed,
12. test each layer,
13. document the API/setup.

That is the repeatable path you should follow here almost every time.
