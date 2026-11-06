# Phase 1 currency foundation — 6 November 2026

The project owner confirmed the roadmap recommendations for D1, D2 and D3.

- Each license has authoritative, explicit INR/USD prices. Upload can auto-fill the other price from an identified FX snapshot; the seller can edit it.
- Buyer preference follows profile, cookie, then country, exposed through a visible switcher. A missing selected-currency price must be handled clearly. Any native-price fallback must show and charge that native currency unchanged.
- Marketplace rounding belongs only to editable upload auto-fill. Checkout must charge the stored integer minor-unit price without live conversion or .99 rounding.

## Implemented in this increment

`GET /config/currencies` returns representation metadata and the payment registry's configured routing. Fields are `code`, `symbol`, `minor_unit_scale`, `locale`, and nullable `gateway`. Responses are public and cacheable for five minutes. No provider secrets are exposed. A configured gateway name is routing metadata, not evidence that provider credentials or live checkout are operational.

The UI loads and validates this configuration before rendering. HTTP failure, malformed configuration or a three-second timeout retains centralized INR/USD compatibility metadata with null gateway names. The configuration service exposes loaded/unavailable state. This fallback formats existing records; it does not enable another payment provider.

Money rendering uses a shared formatter for integer minor units, configured scales/locales, and ISO metadata for historical currencies. Zero remains zero even if a legacy raw price is nonzero. Unsafe integers and malformed codes render an em dash. Compact dashboard amounts, catalog prices, upload previews, cart money pipes, Studio orders and admin formatting share these rules. Display formatting never changes stored monetary values.

## Remaining Phase 1 work

The existing `license_option_prices` table is not yet wired into catalog writes, reads and order creation. Existing catalog conversion and checkout selection remain legacy behavior; approving these rules and exposing configuration does **not** implement task 1.2. That migration must update all three paths together and test displayed amount/currency against the order and provider request.

Do not backfill a second currency with the legacy static FX fallback. Before migration, obtain a reviewed dated FX snapshot, preserve native prices including zero, distinguish explicit seller prices from generated backfills, and define a reversible migration that does not overwrite explicit prices. Daily FX history/refresh is task 1.3. The profile/cookie switcher is task 1.5, and two-price upload editing is task 1.6. Multi-item checkout, provider completion and refund/idempotency work remain separate.

## Validation

Unit and HTTP tests cover metadata, configured routing snapshots, missing routing, malformed configuration, network failure, unknown currency preservation, non-cent scales and zero-value prices. Run `go test ./...`, `make coverage-check COVERAGE_THRESHOLD=90`, `npm run test:coverage -- --maxWorkers=4`, and `npm run build -- --configuration development`. The standalone tracker's progress migration is tested with `node redwave-project-tracker/verify-progress.cjs` from the projects folder.

Verified results: 1,015 UI tests and 1,303 backend tests pass. Backend coverage is 94.38% against its 90% gate. UI coverage is 94.36% lines, 93.78% statements, 95.19% functions and 89.03% branches; all configured gates pass. The Angular development build, both repositories' diff whitespace checks and the tracker migration checks pass. Live payment-provider checkout was not exercised.
