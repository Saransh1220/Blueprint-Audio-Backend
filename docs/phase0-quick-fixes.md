# Phase 0 implementation notes

The quick-fix pass covers the Go backend and the Angular UI in the adjacent
`red-wave-app` repository. The active progress tracker is
`redwave-project-tracker/Redwave-Project-Tracker.html` in the shared workspace.

## Delivered behavior

- Header notifications contain only API records. The badge uses the server's
  unread count, including records outside the loaded page.
- Studio Tracks loads the authenticated producer's catalog. Owner-only Edit and
  confirmed Delete actions share one Angular command component, also used on
  beat details. Editing preserves custom licenses, zero prices and currencies.
- Carts survive refresh, discard invalid stored snapshots and avoid a combined
  mixed-currency total. Multi-item checkout is explicitly unavailable until the
  Phase 1 order contract is implemented. Single-beat purchases remain available.
- Fabricated statistics, transactions, payout balances, comments, reviews,
  audience panels and generated waveforms are removed from reachable pages.
- Loops, packs, battles, messages, earnings and the unsupported Studio settings
  route are gated centrally. Models, repository contracts and useful feature
  components remain for their planned implementation phases.
- Terms, Privacy and Licensing have explicit routes before the beat short-code
  route. The footer links to them. These are initial product explanations and
  require owner/legal review before a public launch.
- Public producer URLs resolve the stored username. Ambiguous/missing/inactive
  handles return not found. Catalog DTOs expose the canonical producer handle.
  The profile uses the API total and loads additional pages rather than labeling
  first-page plays/favorites as whole-profile totals.
- Unknown sample-pack slugs do not fall back to the first mock pack.
- The confirmed Waveyard identity is centralized in the UI brand configuration,
  including the browser title. Decision D8 is complete.
- Unused DirectoryService and producer-row implementations were removed. Producer
  models remain. Local logs are ignored and backend.log is untracked. Executables
  and coverage output were already ignored and are not tracked source files.

## WebSocket authentication

1. The signed-in browser calls `POST /ws/ticket` using the normal HTTP
   Authorization interceptor.
2. The API returns a random, single-use ticket with a 30-second lifetime and
   `Cache-Control: no-store`.
3. The browser opens `/ws?ticket=...`. Access tokens are never placed in the URL
   or printed to the browser console. The application access logger redacts
   ticket query parameters, including failed handshakes.
4. The API atomically consumes the ticket and rechecks account state before
   handing off to the existing notification hub.

The ticket store is bounded and process-local. Before deploying multiple API
instances, provide a shared atomic ticket store or validated sticky routing.
Distributed rate limits remain a later roadmap item.

## Operational changes

- `/metrics` requires authentication plus `admin:analytics:read`. Configure the
  monitoring scraper with an authorized bearer credential through its secret
  management mechanism. The previous unauthenticated scrape configuration will
  receive 401 until that credential is configured.
- Login/Google login, registration, token refresh, public plays and upload
  operations are rate-limited. Existing email action limits remain.
- By default the limiter uses the connection peer address, including IPv6, and
  ignores caller-supplied forwarding headers. Set `TRUST_PROXY_HEADERS=true`
  only behind a trusted proxy that overwrites X-Forwarded-For and prevents direct
  untrusted access. This is an explicit deployment setting, not a default trust
  of arbitrary client headers.

## Validation

Verified locally on 6 November 2026:

- Backend: 1,301 tests passed; the repository's coverage check passed at 94.45%
  against its 90% threshold.
- UI: 1,008 tests passed; CI coverage thresholds passed (94.32% lines, 93.75%
  statements, 95.13% functions and 89.02% branches).
- Angular development build and Git whitespace checks passed.
- The tracker migration preserves existing notes, custom subtasks, unrelated
  task progress and pending decisions, and does not duplicate updates on reload.
- Phase 0 now has 13 completed tasks. The project owner confirmed Waveyard on
  6 November 2026; decision D8 and branding task 0.12 are complete. Browser title
  also uses the shared UI brand setting. Task 0.9 remains In review for legal
  copy approval.

Run `go test ./...` in the backend and `npm test -- --run --maxWorkers=4` plus
`npm run build -- --configuration development` in the UI. Regression coverage
includes single-use/expired tickets, spoofed forwarding headers and IPv6,
unique public handles, owner commands, custom zero-priced licenses, cart
persistence, API unread counts, honest empty states and gated navigation.

Payment-provider live checkout, email delivery and production proxy/monitoring
credentials require the deployed integrations and were not exercised against
live services during this local change.
