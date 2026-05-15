# Cult Beats API and Database Roadmap

Document version: 1.0  
Prepared for: Cult Beats frontend and Blueprint backend alignment  
Scope: API contracts, database changes, data ownership, rollout order, and test strategy  
Non-goal: This document does not implement backend code, frontend code, SQL migrations, or generated types.

## 1. Executive Summary

The Cult Beats frontend has been redesigned around a reference-faithful editorial marketplace experience. The UI now exposes product surfaces for discovery, search, beat metadata, studio analytics, purchases, orders, earnings, profile, notifications, messages, and battles. Some of these surfaces are backed by existing APIs, but many are currently hardcoded, derived locally, or labeled as visual-only because the backend contracts are behind the new product design.

The goal of this roadmap is to replace hardcoded UI data with stable API contracts while preserving the current Go modular-monolith architecture. The backend should continue to follow the existing DDD-style module boundaries: HTTP handler, application service, domain model, repository interface, PostgreSQL repository, and gateway route registration.

Current backend modules:

- Auth
- Catalog
- Payment
- User
- Analytics
- Notification
- FileStorage

Recommended new or expanded modules:

- Earnings
- Messaging
- Battles
- Discovery/Home, optional but recommended
- Currency/Localization, lightweight optional layer

The highest-impact first pass is not a large new feature. It is response consistency and pagination correctness. Several UI issues come from backend endpoints ignoring `per_page`/`limit`, returning incomplete joined metadata, or exposing only aggregated totals where the UI needs richer breakdowns.

## 2. Current API Inventory

### 2.1 Auth

#### `POST /register`

Current purpose: Creates a user account.

Current body:

```json
{
  "email": "artist@example.com",
  "password": "password",
  "name": "Artist Name",
  "role": "artist"
}
```

Current response shape:

- Auth response with token/user data, depending on handler implementation.
- User role is limited by the existing role check: `artist` or `producer`.

Current gaps:

- No country, locale, preferred currency, handle, marketing opt-in, or profile metadata.
- Registration does not collect enough information to support localized currency display.

Recommendation: Upgrade later. Add optional profile/localization fields after currency/localization schema exists.

#### `POST /login`

Current purpose: Authenticates an existing user.

Current body:

```json
{
  "email": "artist@example.com",
  "password": "password"
}
```

Current response shape:

- Auth token and user data.

Current gaps:

- User payload should eventually include `locale`, `country_code`, `preferred_currency`, `handle`, `avatar_url`, and notification/message counts if the frontend wants immediate shell hydration.

Recommendation: Upgrade user response shape after profile schema is expanded.

#### `POST /auth/google`

Current purpose: Authenticates through Google.

Current body:

```json
{
  "credential": "google-id-token"
}
```

Current response shape:

- Auth token and user data.

Current gaps:

- Same localization/profile gaps as login.

Recommendation: Upgrade user response shape after profile schema is expanded.

#### `POST /auth/refresh`

Current purpose: Refreshes auth session.

Current body:

```json
{}
```

Current response shape:

```json
{
  "token": "jwt"
}
```

Current gaps:

- None for UI hardcoding.

Recommendation: Keep as-is.

#### `POST /auth/logout`

Current purpose: Logs out current session.

Current body:

```json
{}
```

Current response shape:

- Empty success response or message.

Current gaps:

- None for UI hardcoding.

Recommendation: Keep as-is.

#### `GET /me`

Current purpose: Returns current authenticated user.

Current response shape currently needs to support:

```json
{
  "id": "uuid",
  "email": "user@example.com",
  "name": "User Name",
  "display_name": "Display Name",
  "role": "producer",
  "bio": "Short bio",
  "avatar_url": "https://...",
  "instagram_url": "https://...",
  "twitter_url": "https://...",
  "youtube_url": "https://...",
  "spotify_url": "https://..."
}
```

Current gaps:

- No handle.
- No banner.
- No location/country/locale/preferred currency.
- No profile tags.
- No follower counts.
- No notification preferences.
- No producer settings.

Recommendation: Upgrade. `GET /me` should be the frontend shell source for user identity and lightweight preferences.

### 2.2 Catalog

#### `GET /specs`

Current purpose: Lists public specs/beats with optional flexible auth.

Current query params:

- `category`
- `genres`
- `tags`
- `search`
- `min_bpm`
- `max_bpm`
- `min_price`
- `max_price`
- `key`
- `page`
- `sort`

Frontend request class also sends:

- `per_page`
- `limit`

Current backend behavior:

- `page` is honored.
- `per_page`/`limit` is currently ignored by the catalog handler, which uses fixed limit `20`.
- Search checks title and tags.
- Sort supports newest, oldest, price, and BPM. It does not fully support plays, downloads, revenue, favorites, trending, or title.

Current response shape:

```json
{
  "data": [
    {
      "id": "uuid",
      "producer_id": "uuid",
      "producer_name": "Producer",
      "title": "Beat Title",
      "category": "beat",
      "type": "beat",
      "bpm": 140,
      "key": "E MINOR",
      "image_url": "https://...",
      "preview_url": "https://...",
      "price": 2499,
      "duration": 162,
      "free_mp3_enabled": false,
      "created_at": "2026-04-23T00:00:00Z",
      "updated_at": "2026-04-23T00:00:00Z",
      "licenses": [],
      "genres": [],
      "tags": [],
      "analytics": {
        "play_count": 0,
        "favorite_count": 0,
        "total_download_count": 0,
        "is_favorited": false
      },
      "processing_status": "completed"
    }
  ],
  "metadata": {
    "total": 0,
    "page": 1,
    "per_page": 20
  }
}
```

Current gaps against redesigned UI:

- Visual-only search filters: license type, moods, extras, stems, tagless preview, sync-ready, new this week, trending, producer.
- Missing searchable description in frontend model.
- Beat duration is not reliably available in list responses. The frontend only discovers duration after audio playback loads metadata, which makes catalog cards, list rows, search results, and beat pages inconsistent.
- Missing mood and usage metadata.
- Missing true `has_stems`.
- Missing draft/live/hidden/archive distinction.
- Missing waveform data for beat details/player.
- Missing producer avatar/handle for richer cards.
- Missing short shareable beat URL/slug. Beat detail currently uses the UUID route, which is not friendly for sharing.
- Missing pagination page size control.

Recommendation: Upgrade, not replace.

#### `GET /specs/{id}`

Current purpose: Returns a single spec with licenses, genres, public analytics, and presigned media URLs.

Current params:

- Path param: `id`

Current response shape:

- Same `SpecResponse` shape as `GET /specs`, plus nested licenses and genres.

Current gaps:

- Missing metadata fields required by beat details.
- Missing real producer public profile block.
- Related tracks are currently fetched separately and filtered client-side.
- Beat details page has hardcoded descriptive copy and waveform.
- Detail route currently depends on UUID. The product should support short slugs and preserve UUID lookup as fallback/backward compatibility.

Recommendation: Upgrade. Add richer spec metadata and optionally embed a `producer` object.

#### `GET /specs/slug/{slug}` or `GET /specs/{identifier}`

Current purpose: Not implemented.

Needed purpose: Load beat detail pages from readable URLs.

Recommended route:

- `GET /specs/{identifier}` can accept UUID, slug, or short code if handler ambiguity is acceptable.

Alternative explicit routes:

- `GET /specs/slug/{slug}`
- `GET /specs/code/{short_code}`

Recommended response shape:

- Same as upgraded `SpecResponse`.

Current UI gap:

- Beat pages currently use UUID URLs, which are long and not share-friendly.

Recommendation: Add. Keep UUID lookup as fallback.

#### `POST /specs`

Current purpose: Creates a spec using multipart upload and async file processing.

Current body:

- `metadata`: JSON string.
- `image`: cover image.
- `preview`: preview audio.
- `wav`: WAV file.
- `stems`: ZIP/RAR stems.

Current metadata fields sent by frontend:

```json
{
  "title": "Beat Title",
  "category": "beat",
  "type": "beat",
  "bpm": 140,
  "key": "E MINOR",
  "price": 2499,
  "tags": ["dark"],
  "genres": [{ "name": "Trap", "slug": "trap" }],
  "description": "Description",
  "free_mp3_enabled": false,
  "licenses": []
}
```

Current gaps:

- Upload UI collects moods locally but does not persist them.
- Royalty split, contract type, and territory are visual-only.
- Draft save is not implemented.
- Sync-ready/tagless flags do not exist.
- Usage rights metadata is not stored.
- Backend requires WAV and stems for beats, while frontend labels WAV/stems optional. This mismatch should be resolved product-wise.

Recommendation: Upgrade. Add draft support and metadata fields.

#### `PATCH /specs/{id}`

Current purpose: Updates spec metadata and optionally replaces cover image.

Current body:

- Multipart form with `metadata`.
- Optional `image`.

Current gaps:

- Cannot update new metadata until schema is added.
- No partial JSON-only update contract.
- License sync exists but no rights metadata contract.

Recommendation: Upgrade. Support the same metadata fields as create.

#### `DELETE /specs/{id}`

Current purpose: Deletes or soft-deletes a spec.

Current behavior:

- Hard delete if no licenses exist.
- Soft delete if purchased licenses exist.

Current gaps:

- UI needs status controls such as hidden, archived, draft, and live without destructive delete.

Recommendation: Keep delete, add status update via `PATCH /specs/{id}`.

#### `POST /specs/{id}/download-free`

Current purpose: Generates a presigned URL for the free MP3 preview and tracks a download event.

Current response shape:

```json
{
  "url": "https://..."
}
```

Current gaps:

- Event metadata is minimal.
- No source/device/referrer/country tracking.

Recommendation: Upgrade analytics event metadata, keep endpoint.

#### `GET /users/{id}/specs`

Current purpose: Lists specs for a producer profile.

Current query params:

- `page`

Current backend behavior:

- Fixed page size `20`.
- No filters/sort.

Current gaps:

- Studio tracks needs owner-only search, per-page, status, sold, revenue, live/draft filters.
- Public producer profile needs public filtering/sorting.

Recommendation: Upgrade with pagination, filters, and an owner-only mode where appropriate.

### 2.3 User

#### `PATCH /users/profile`

Current purpose: Updates authenticated user's profile.

Current body:

```json
{
  "bio": "Bio",
  "avatar_url": "https://...",
  "display_name": "Display Name",
  "instagram_url": "https://...",
  "twitter_url": "https://...",
  "youtube_url": "https://...",
  "spotify_url": "https://..."
}
```

Current gaps:

- Missing handle, location, banner, website, profile tags, country, locale, preferred currency, producer settings, notification preferences.

Recommendation: Upgrade.

#### `POST /users/profile/avatar`

Current purpose: Uploads profile avatar image.

Current body:

- Multipart `avatar`.

Current gaps:

- No banner upload endpoint.

Recommendation: Keep avatar endpoint, add `POST /users/profile/banner`.

#### `GET /users/{id}/public`

Current purpose: Public profile display.

Current response:

- Public user profile fields currently available.

Current gaps:

- No stats such as followers, beats live, total plays, licenses sold.
- No handle.
- No banner.
- No profile tags.

Recommendation: Upgrade, or add `GET /users/{id}/stats` for stats separation.

### 2.4 Payment

#### `POST /orders`

Current purpose: Creates a Razorpay order for a spec/license option.

Current body:

```json
{
  "spec_id": "uuid",
  "license_option_id": "uuid"
}
```

Current response shape:

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "spec_id": "uuid",
  "license_type": "Premium",
  "amount": 249900,
  "currency": "INR",
  "razorpay_order_id": "order_x",
  "status": "pending",
  "notes": {},
  "created_at": "2026-04-23T00:00:00Z",
  "updated_at": "2026-04-23T00:00:00Z",
  "expires_at": "2026-04-23T00:30:00Z"
}
```

Current gaps:

- Currency is hardcoded to INR.
- Response uses `amount`, which is minor units, while some frontend producer order responses use major units.
- No pricing snapshot beyond notes.

Recommendation: Upgrade money naming consistency. Keep INR checkout in v1.

#### `GET /orders`

Current purpose: Lists current user's orders.

Current query params:

- `page`

Current gaps:

- Not paginated with metadata.
- No `limit`.
- No joined spec image/title/license name for dashboard/library use.

Recommendation: Upgrade.

#### `GET /orders/{id}`

Current purpose: Gets one order.

Current gaps:

- Should include joined payment, license, spec, and invoice metadata for receipt/detail UI.

Recommendation: Upgrade later.

#### `GET /orders/producer`

Current purpose: Lists orders for a producer's catalog.

Current query params:

- `page`
- `limit`

Current response shape:

```json
{
  "orders": [
    {
      "id": "uuid",
      "amount": 2499,
      "currency": "INR",
      "status": "paid",
      "created_at": "2026-04-23T00:00:00Z",
      "license_type": "Premium",
      "buyer_name": "Buyer",
      "buyer_email": "buyer@example.com",
      "spec_title": "Beat Title",
      "razorpay_order_id": "order_x"
    }
  ],
  "total": 1,
  "limit": 10,
  "offset": 0
}
```

Current gaps:

- No search.
- No status filter.
- No license filter.
- No date filter.
- No spec image.
- No buyer avatar.
- No payment method.
- Amount is returned in major units while core order model uses minor units.
- No invoice URL.
- No refund status.

Recommendation: Upgrade. Normalize money fields.

#### `POST /payments/verify`

Current purpose: Verifies Razorpay payment, records payment, and issues license.

Current body:

```json
{
  "order_id": "uuid",
  "razorpay_payment_id": "pay_x",
  "razorpay_signature": "signature"
}
```

Current response:

```json
{
  "success": true,
  "license": {},
  "message": "Payment successful! License issued."
}
```

Current gaps:

- Should trigger typed notifications and analytics purchase event.
- Should update earnings ledger/balance once earnings module exists.

Recommendation: Upgrade through events/side effects after earnings/notifications are added.

#### `GET /licenses`

Current purpose: Lists current user's active licenses.

Current query params:

- `page`
- `q`
- `type`

Frontend request class also sends:

- `limit`

Current backend behavior:

- Hardcoded page size `5` in service/handler.
- Search only title.
- Type filter exact-matches license type.

Current response shape:

```json
{
  "data": [
    {
      "id": "uuid",
      "order_id": "uuid",
      "user_id": "uuid",
      "spec_id": "uuid",
      "license_option_id": "uuid",
      "license_type": "Premium",
      "purchase_price": 249900,
      "license_key": "LIC-...",
      "is_active": true,
      "is_revoked": false,
      "downloads_count": 0,
      "last_downloaded_at": null,
      "issued_at": "2026-04-23T00:00:00Z",
      "created_at": "2026-04-23T00:00:00Z",
      "updated_at": "2026-04-23T00:00:00Z",
      "spec_title": "Beat Title",
      "spec_image": "https://..."
    }
  ],
  "metadata": {
    "total": 1,
    "page": 1,
    "per_page": 5
  }
}
```

Current gaps:

- Page size bug.
- Missing producer name.
- Missing BPM/key/genres.
- Missing preview URL.
- Missing license name/file types.
- Missing download limit/remaining.
- Missing currency.
- Missing invoice URL.

Recommendation: Upgrade.

#### `GET /licenses/{id}/downloads`

Current purpose: Returns presigned URLs available for a purchased license and increments download count.

Current response:

```json
{
  "license_id": "uuid",
  "license_type": "Premium",
  "spec_title": "Beat Title",
  "expires_in": 3600,
  "mp3_url": "https://...",
  "wav_url": "https://...",
  "stems_url": "https://..."
}
```

Current gaps:

- No per-file metadata.
- No download limit enforcement.
- No audit row per download.

Recommendation: Upgrade later if licensing terms require download limits.

### 2.5 Analytics

#### `POST /specs/{id}/play`

Current purpose: Increments play count and writes analytics event.

Current body:

- None.

Current gaps:

- Does not capture listener/session/source/referrer/country/device/duration metadata.

Recommendation: Upgrade body to accept optional event metadata.

#### `POST /specs/{id}/favorite`

Current purpose: Toggles favorite.

Current response:

```json
{
  "is_favorited": true
}
```

Current gaps:

- Frontend has fallback logic because `total_count` is not returned.
- Favorite and wishlist are conceptually different in the redesigned UI.

Recommendation: Add `total_count`; add separate wishlist endpoints.

#### `GET /specs/{id}/analytics`

Current purpose: Producer-only analytics for one spec.

Current response currently supports:

- play count
- favorite count
- free download count
- total purchase count
- purchases by license

Current gaps:

- No revenue by day for spec.
- No listener geography/referrers.
- No conversion funnel.
- No cart/wishlist events.

Recommendation: Upgrade or add `GET /analytics/specs/{id}/timeline`.

#### `GET /analytics/overview`

Current purpose: Producer analytics overview.

Current query params:

- `days`
- `sortBy`

Current response:

```json
{
  "total_plays": 0,
  "total_favorites": 0,
  "total_revenue": 0,
  "total_downloads": 0,
  "plays_by_day": [],
  "downloads_by_day": [],
  "revenue_by_day": [],
  "top_specs": [],
  "revenue_by_license": {}
}
```

Current gaps:

- No deltas.
- No unique listeners.
- No conversion rate.
- No average listen time.
- No skip/bounce rate.
- No countries.
- No referrers.
- No hourly heatmap.
- No device breakdown.
- No cart/wishlist/follow counts.
- No recent activity.

Recommendation: Upgrade.

#### `GET /analytics/top-specs`

Current purpose: Returns top specs by plays/revenue/downloads.

Current query params:

- `limit`
- `sortBy`

Current issue:

- Handler currently sets `limit := 5` and does not parse the query limit even though frontend request sends it.

Current gaps:

- No image URL.
- No genre/BPM/key.
- No rank movement.

Recommendation: Upgrade.

### 2.6 Notifications

#### `GET /notifications`

Current purpose: Returns user notifications.

Current query params:

- `limit`
- `offset`

Current response:

```json
{
  "data": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "title": "Upload Complete",
      "message": "Your beat is now live",
      "type": "success",
      "is_read": false,
      "created_at": "2026-04-23T00:00:00Z"
    }
  ]
}
```

Current gaps:

- Header dropdown hardcodes sale/message/cart/follow/milestone/wishlist notifications.
- Notification type is too generic.
- No actor/entity metadata.
- No typed counts.
- No page metadata.

Recommendation: Upgrade.

#### `PATCH /notifications/{id}/read`

Current purpose: Marks one notification as read.

Current response:

- `204 No Content`

Current gaps:

- None significant.

Recommendation: Keep.

#### `PATCH /notifications/read-all`

Current purpose: Marks all user notifications as read.

Current response:

- `204 No Content`

Current gaps:

- None significant.

Recommendation: Keep.

#### `GET /notifications/unread-count`

Current purpose: Returns unread count.

Current response:

```json
{
  "count": 3
}
```

Current gaps:

- No counts by type.

Recommendation: Upgrade or include counts in `GET /notifications`.

#### `GET /ws`

Current purpose: Authenticated notification WebSocket.

Current gaps:

- Event payload should match upgraded typed notification shape.

Recommendation: Upgrade payload type after notification model expands.

## 3. Global Currency and Localization

### 3.1 Recommendation

Use a lightweight display-currency layer first. Do not implement full multi-currency checkout in v1.

Reason:

- Razorpay order creation currently hardcodes `"INR"`.
- License prices are stored as INR-oriented values.
- Orders, payments, licenses, analytics revenue, email receipts, payouts, and refunds all assume INR semantics.
- Real multi-currency would require currency conversion, exchange rate snapshots, settlement currency, refund currency handling, payout conversion, tax rules, and payment gateway support.

### 3.2 V1 Behavior

Checkout currency:

- Keep checkout in INR.
- Keep `orders.currency = 'INR'`.
- Keep `payments.currency = 'INR'`.
- Keep `licenses.purchase_price` in INR minor units.

Display currency:

- Add a frontend `CurrencyService`.
- Display in the user's preferred currency only if conversion support is introduced.
- Until conversion exists, format API money with the response `currency`.
- If no currency is supplied, fallback to `currentUser.preferred_currency`, then browser locale, then INR.

### 3.3 Money Field Standard

All new or upgraded APIs should return money fields like this:

```json
{
  "amount_minor": 249900,
  "amount_major": 2499,
  "currency": "INR"
}
```

Rules:

- `amount_minor` is the source of truth.
- `amount_major` is convenience for UI only.
- `currency` is always ISO 4217.
- Avoid ambiguous `amount` in new contracts.
- Existing fields can remain for compatibility but should be marked legacy.

### 3.4 Optional User Fields

Add to `users`:

| Column | Type | Purpose |
| --- | --- | --- |
| `country_code` | `VARCHAR(2)` | Region hint for localization |
| `locale` | `VARCHAR(20)` | Display formatting, e.g. `en-IN` |
| `preferred_currency` | `VARCHAR(3)` | User display preference |

### 3.5 Future Multi-Currency Requirements

If full multi-currency is desired later:

- Add `license_prices` table keyed by `license_option_id` and `currency`.
- Store exchange rate snapshots on orders.
- Store settlement currency and presentment currency separately.
- Ensure Razorpay/payment provider supports the chosen currencies.
- Update analytics to aggregate by currency or convert using historical exchange rates.
- Update payouts and tax documents per currency/jurisdiction.

## 4. Hardcoded UI Audit

### 4.1 Home

Hardcoded or partially hardcoded:

- Hero search placeholders.
- Trending search suggestions.
- Suggested beat rows.
- Quick shortcuts:
  - under INR 999
  - this week's drops
  - with stems
  - sync-ready
- Hero stats:
  - 1,241+ beats in rotation
  - 40k artists worldwide
- Ticker copy.
- Genre tiles and counts.
- Lab/catalog chips:
  - All beats
  - New this week
  - Trending
  - Under $30
  - Exclusive rights
  - With stems
  - Sub 90 BPM
  - 140+ BPM

Needed backend support:

- `GET /catalog/home`
- `GET /catalog/facets`
- Upgraded `GET /specs` filters and sorts.

### 4.2 Search

Visual-only filters:

- Licenses.
- Moods.
- Extras:
  - has stems
  - tagless preview
  - new this week
  - trending

Other gaps:

- Search placeholder says producers/moods, but backend search does not search producer or mood.
- Per-page selector does not control backend page size.
- Sort options are limited by backend.
- Beat duration should be available before playback, so search rows/cards can show time from API data.

Needed backend support:

- Search filter upgrades.
- Facets endpoint.
- Spec metadata upgrades.

### 4.3 Upload

Hardcoded or visual-only:

- Moods are selected locally but not persisted.
- Royalty split is visual-only.
- Contract type is visual-only.
- Territory is visual-only.
- Save draft only shows a toast.
- Sync/tagless/rights metadata does not exist.

Needed backend support:

- Spec metadata additions.
- Draft release status.
- Optional `POST /specs/drafts` or use `POST /specs` with `release_status = 'draft'`.
- Decide whether WAV/stems are really required for beats, because frontend labels them optional.

### 4.4 Beat Details

Hardcoded or derived:

- Description narrative.
- Waveform bars.
- Producer bio.
- Follow vibe action.
- License comparison helper text.
- Related specs logic is client-side and broad.
- Producer stats are derived from fetched related specs.
- Share URL is UUID-based instead of a readable slug or short code.

Needed backend support:

- Rich spec metadata.
- Producer public profile/stats.
- Related specs endpoint or query mode.
- Follow endpoint.
- Waveform JSON.
- Beat slug and short-code lookup.
- API-provided duration in seconds.

### 4.5 Dashboard

Hardcoded or derived:

- Listener stats.
- Listener library rows.
- Activity feed.
- Community pulse.
- Upcoming Friday drop.
- Messages/wishlist/invite actions.
- Store credit.
- Producer AI cover-art promo.
- Some producer counts like activity and follower state.

Needed backend support:

- `GET /dashboard/summary`.
- `GET /analytics/activity`.
- Purchases/library upgrades.
- Notifications/messages/wishlist/follow support.

### 4.6 Studio Overview

Hardcoded or fallback:

- Live beat count fallback.
- Ready-to-withdraw derivation.
- Activity rows.
- Cart/wishlist/follow activity.
- Draft progress copy.
- Some chart fallback points.

Needed backend support:

- Analytics overview upgrade.
- Earnings summary.
- Activity endpoint.
- Catalog counts.

### 4.7 Studio Analytics

Hardcoded:

- Unique listeners.
- Average watch/listen time.
- Bounce rate.
- Countries.
- Referrers.
- Heatmap.

Needed backend support:

- Analytics event metadata.
- Analytics geo/referrer/heatmap/listeners endpoints.

### 4.8 Studio Tracks

Hardcoded or derived:

- Sold count derived from favorites.
- Earned amount derived from sold guess.
- Release text.
- Draft completeness.
- Sold filter returns false because backend lacks sold filter.
- Revenue/plays sort depends on backend support that is incomplete.

Needed backend support:

- Owner catalog endpoint with revenue, sold, status, draft fields.
- Sort/filter support.

### 4.9 Studio Purchases

Hardcoded or fallback:

- Fallback purchase rows.
- Producer name fallback.
- License metadata display.
- Download limit text.

Needed backend support:

- Expanded `GET /licenses`.
- Fix page size.
- Joined producer/spec/license fields.

### 4.10 Studio Orders

Partially backed:

- Uses `GET /orders/producer`.

Missing:

- Search.
- Status/date/license filters.
- Buyer avatar.
- Spec image.
- Payment method.
- Refund status.
- Export endpoint.

Needed backend support:

- Expanded producer orders API.
- Orders summary/export endpoints.

### 4.11 Studio Earnings

Hardcoded or derived:

- Available balance.
- Pending clearance.
- Last payout.
- Payout history.
- Payout method.
- Tax documents.
- Monthly bars when analytics is missing.

Needed backend support:

- Earnings module.
- Payout tables/endpoints.
- Ledger or balance derivation.

### 4.12 Studio Profile and Settings

Hardcoded:

- Tags.
- Location.
- Profile completeness.
- Producer settings.
- Notification preferences.
- Public preview stats.
- Followers count.

Needed backend support:

- User profile schema expansion.
- Settings endpoint.
- Follow/stats endpoint.

### 4.13 Navbar Notifications

Partially backed:

- Real upload/processing notifications exist.

Hardcoded:

- Sale notifications.
- Message notifications.
- Cart notifications.
- Follow notifications.
- Milestone notifications.
- Wishlist notifications.

Needed backend support:

- Typed notification schema and event generation.

### 4.14 Messages

Hardcoded:

- Studio messages page is static.

Needed backend support:

- Messaging module.

### 4.15 Battles

Fully hardcoded:

- Battles.
- Rooms.
- Leaderboard.
- Activity.
- Battle detail.
- Submissions.
- Voting.

Needed backend support:

- Battles module.

## 5. Catalog and Beat Metadata Upgrades

### 5.1 Existing Support

The `specs` domain already supports:

- title
- category
- type
- bpm
- key
- base price
- image URL
- preview URL
- WAV URL
- stems URL
- tags
- description
- duration
- free MP3 enabled
- processing status
- genres
- license options

### 5.2 New `specs` Columns

| Column | Type | Default | Purpose |
| --- | --- | --- | --- |
| `moods` | `TEXT[]` | `'{}'` | Searchable mood tags from upload |
| `energy` | `VARCHAR(40)` | `NULL` | Optional UI-friendly energy label |
| `usage_tags` | `TEXT[]` | `'{}'` | Sync, film, ads, freestyle, etc. |
| `is_sync_ready` | `BOOLEAN` | `false` | Search extra and beat badge |
| `is_tagless_preview` | `BOOLEAN` | `false` | Search extra and license detail |
| `has_stems` | `BOOLEAN` | `false` | Fast filter; derived from stems file/license support |
| `release_status` | `VARCHAR(40)` | `live` or `draft` | Product status separate from processing |
| `draft_completion` | `INTEGER` | `0` | Studio draft progress |
| `published_at` | `TIMESTAMPTZ` | `NULL` | New-this-week filter |
| `slug` | `VARCHAR(140)` | generated | Short shareable beat identifier |
| `short_code` | `VARCHAR(16)` | generated | Very short unique fallback/share code |
| `waveform` | `JSONB` | `NULL` | Player/detail waveform data |
| `metadata` | `JSONB` | `'{}'` | Future-safe UI metadata |

Suggested `release_status` values:

- `draft`
- `processing`
- `live`
- `hidden`
- `archived`

### 5.3 Beat Duration, Slugs, and Shareable URLs

Duration should be a first-class catalog field, not something the browser discovers only after playback starts.

Current issue:

- The player can read audio duration after loading the preview file.
- `GET /specs` list rows do not reliably include useful beat duration.
- Search cards, list rows, dashboard rows, purchases, and beat detail pages therefore cannot consistently show track time before playback.

Backend requirement:

- Extract duration during upload processing for the preview audio, WAV, or canonical deliverable.
- Store duration in `specs.duration` as integer seconds.
- Return `duration` in all spec list/detail responses.
- Preserve `duration = 0` only for unknown legacy records.
- Add a backfill job or admin utility to compute duration for existing records where possible.

Short beat URLs:

- Add `specs.slug` for readable URLs, generated from the title.
- Add `specs.short_code` for compact stable sharing.
- Keep UUID lookup working for backward compatibility.
- New preferred beat URL patterns:
  - `/beats/{slug}` for readable detail pages.
  - `/b/{short_code}` for compact sharing.
- Example:
  - `/beats/violet-hour`
  - `/b/vh9k2`
- If slugs collide, append a short suffix, e.g. `violet-hour-7x2`.
- Slugs should not change automatically when a title changes unless the producer explicitly regenerates them.
- Store old slugs in an optional `spec_slug_history` table if redirects are desired.

Recommended DB constraints:

- Unique index on `specs.slug`.
- Unique index on `specs.short_code`.
- Optional unique index on `spec_slug_history.slug`.

Optional `spec_slug_history` table:

| Column | Type | Purpose |
| --- | --- | --- |
| `spec_id` | `UUID REFERENCES specs(id)` | Spec that used to own the slug |
| `slug` | `VARCHAR(140)` | Previous slug |
| `changed_at` | `TIMESTAMPTZ DEFAULT NOW()` | Change timestamp |

Use this only if old beat links should redirect after a producer changes a slug/title.

Recommended lookup behavior:

- `GET /specs/{identifier}` should accept UUID, slug, or short code.
- Alternatively add explicit endpoints:
  - `GET /specs/slug/{slug}`
  - `GET /specs/code/{short_code}`
- The single identifier route is simpler for frontend routing, but explicit routes are less ambiguous for backend handlers.

### 5.4 New or Expanded Spec Response

All catalog responses should include:

```json
{
  "id": "uuid",
  "producer_id": "uuid",
  "producer_name": "Producer Name",
  "producer_handle": "producer",
  "producer_avatar_url": "https://...",
  "slug": "violet-hour",
  "short_code": "vh9k2",
  "canonical_url": "/beats/violet-hour",
  "short_url": "/b/vh9k2",
  "title": "Violet Hour",
  "category": "beat",
  "type": "beat",
  "bpm": 92,
  "key": "E MINOR",
  "description": "A slow-burning trap instrumental...",
  "image_url": "https://...",
  "preview_url": "https://...",
  "price": 2499,
  "price_minor": 249900,
  "currency": "INR",
  "duration": 162,
  "free_mp3_enabled": true,
  "moods": ["Moody", "Dark"],
  "energy": "low",
  "usage_tags": ["sync", "film"],
  "is_sync_ready": true,
  "is_tagless_preview": false,
  "has_stems": true,
  "release_status": "live",
  "processing_status": "completed",
  "draft_completion": 100,
  "published_at": "2026-04-23T00:00:00Z",
  "waveform": {
    "peaks": [0.12, 0.32, 0.5],
    "version": 1
  },
  "genres": [],
  "tags": [],
  "licenses": [],
  "analytics": {},
  "created_at": "2026-04-23T00:00:00Z",
  "updated_at": "2026-04-23T00:00:00Z"
}
```

### 5.5 Upload Metadata Contract

`POST /specs` and `PATCH /specs/{id}` should accept the expanded metadata:

```json
{
  "title": "Violet Hour",
  "slug": "violet-hour",
  "category": "beat",
  "type": "beat",
  "bpm": 92,
  "key": "E MINOR",
  "price": 2499,
  "description": "A slow-burning trap instrumental...",
  "tags": ["piano", "dark"],
  "moods": ["Moody", "Dark"],
  "energy": "low",
  "usage_tags": ["sync", "film"],
  "is_sync_ready": true,
  "is_tagless_preview": false,
  "free_mp3_enabled": true,
  "release_status": "draft",
  "rights": {
    "royalty_split": "50/50",
    "contract_type": "Standard",
    "territory": "Worldwide"
  },
  "genres": [
    {
      "name": "Trap",
      "slug": "trap"
    }
  ],
  "licenses": []
}
```

Notes:

- `slug` should be optional on upload. If omitted, backend generates it.
- `duration` should not be trusted from client input. The backend should compute it from audio.
- `short_code` should always be generated by backend.

### 5.6 Indexing

Recommended indexes:

- GIN index on `specs.moods`.
- GIN index on `specs.usage_tags`.
- Partial index on `specs.is_sync_ready`.
- Partial index on `specs.is_tagless_preview`.
- Partial index on `specs.has_stems`.
- B-tree index on `specs.release_status`.
- B-tree index on `specs.published_at DESC`.
- Unique B-tree index on `specs.slug`.
- Unique B-tree index on `specs.short_code`.
- Optional full-text index across title, description, tags, moods, producer name.

## 6. Search API Upgrades

### 6.1 Upgrade `GET /specs`

Add or fix query params:

| Param | Type | Purpose |
| --- | --- | --- |
| `page` | number | Page number |
| `per_page` | number | Page size |
| `limit` | number | Alias for page size |
| `search` | string | Title, description, tags, moods, producer |
| `category` | string | beat/sample |
| `genres` | comma list | Genre filter |
| `tags` | comma list | Tag filter |
| `moods` | comma list | Mood filter |
| `license_type` | string | Basic/Premium/Trackout/Unlimited |
| `min_bpm` | number | Min BPM |
| `max_bpm` | number | Max BPM |
| `min_price` | number | Min price major unit |
| `max_price` | number | Max price major unit |
| `key` | string | Musical key |
| `has_stems` | boolean | Stems available |
| `free_mp3_enabled` | boolean | Free preview download |
| `is_sync_ready` | boolean | Sync-ready catalog |
| `is_tagless_preview` | boolean | Tagless preview |
| `new_this_week` | boolean | `published_at >= now - 7 days` |
| `trending` | boolean | Trending score filter |
| `producer` | string | Producer name/handle |
| `status` | string | Owner/admin status filter |
| `min_duration` | number | Min seconds |
| `max_duration` | number | Max seconds |
| `sort` | string | Sort key |

Sort values:

- `newest`
- `oldest`
- `price_asc`
- `price_desc`
- `bpm_asc`
- `bpm_desc`
- `plays`
- `downloads`
- `revenue`
- `favorites`
- `trending`
- `title`

Response:

```json
{
  "data": [],
  "metadata": {
    "total": 1241,
    "page": 1,
    "per_page": 16,
    "total_pages": 78,
    "has_next": true,
    "has_previous": false
  }
}
```

### 6.2 Add `GET /catalog/facets`

Purpose: Powers search sidebar counts and home tiles.

Query params:

- Same filter params as `GET /specs`, except pagination.
- Facets should be computed after applying active filters where practical.

Response:

```json
{
  "genres": [
    { "name": "Trap", "slug": "trap", "count": 214 }
  ],
  "moods": [
    { "name": "Dark", "count": 82 }
  ],
  "licenses": [
    { "type": "Premium", "count": 74 }
  ],
  "keys": [
    { "key": "E MINOR", "count": 40 }
  ],
  "price": {
    "min": 0,
    "max": 50000
  },
  "bpm": {
    "min": 60,
    "max": 200
  },
  "extras": {
    "has_stems": 118,
    "is_sync_ready": 43,
    "is_tagless_preview": 31,
    "new_this_week": 47,
    "trending": 25
  },
  "trending_searches": [
    {
      "label": "dark piano trap",
      "count": 182,
      "change_pct": 24
    }
  ]
}
```

## 7. Home and Discovery APIs

### 7.1 Add `GET /catalog/home`

Purpose: Replace home hardcoding in hero, ticker, genre tiles, suggestions, and featured catalog.

Auth:

- Public endpoint.
- Flexible auth optional for personalized suggestions/favorite state.

Query params:

- `locale`
- `currency`
- `limit_featured`

Response:

```json
{
  "stats": {
    "total_live_beats": 1241,
    "total_producers": 320,
    "total_artists": 40000,
    "new_drops_this_week": 47
  },
  "ticker_items": [
    "Fresh drops every Friday",
    "Beat battles open for submissions"
  ],
  "hero_suggestions": [
    {
      "kind": "search",
      "label": "dark piano trap",
      "sub": "182 beats - most searched this week",
      "change_pct": 24,
      "query": {
        "search": "dark piano trap"
      }
    },
    {
      "kind": "spec",
      "label": "Violet Hour",
      "sub": "prod. Kita Sol - 92 BPM - E minor",
      "price_minor": 99900,
      "currency": "INR",
      "spec_id": "uuid"
    }
  ],
  "quick_shortcuts": [
    {
      "label": "Beats under INR 999",
      "description": "demos, drafts, loose ideas",
      "query": {
        "max_price": 999
      },
      "count": 99
    }
  ],
  "featured_genres": [
    {
      "name": "Trap & Drill",
      "slug": "trap",
      "count": 214,
      "icon_letter": "T"
    }
  ],
  "featured_specs": []
}
```

### 7.2 Optional Admin Control

If editorial control is needed later, add tables:

- `home_ticker_items`
- `home_featured_slots`
- `home_search_suggestions`

For v1, computed backend results are enough.

## 8. Analytics Upgrades

### 8.1 Analytics Event Metadata

Current `analytics_events.meta JSONB` exists. Use it consistently.

Recommended normalized fields if query performance becomes important:

| Column | Type | Purpose |
| --- | --- | --- |
| `source` | `VARCHAR(80)` | app surface: home/search/player/studio |
| `referrer` | `TEXT` | External referrer/domain |
| `country` | `VARCHAR(2)` | Geo country |
| `device` | `VARCHAR(40)` | mobile/desktop/tablet |
| `session_id` | `VARCHAR(120)` | Anonymous session |
| `listener_id` | `UUID` | User if authenticated |
| `duration_seconds` | `INTEGER` | Listen/play duration |
| `user_agent` | `TEXT` | Optional raw user agent |
| `ip_hash` | `VARCHAR(128)` | Privacy-safe IP hash |

Event types:

- `play`
- `download`
- `favorite`
- `cart_add`
- `wishlist_add`
- `license_view`
- `profile_view`
- `profile_follow`
- `share`
- `message`
- `review`
- `purchase`

### 8.2 Upgrade `POST /specs/{id}/play`

Optional body:

```json
{
  "source": "search",
  "session_id": "anon-session-id",
  "duration_seconds": 32,
  "device": "desktop",
  "referrer": "instagram.com"
}
```

Response:

```json
{
  "ok": true
}
```

### 8.3 Upgrade `POST /specs/{id}/favorite`

Response:

```json
{
  "is_favorited": true,
  "total_count": 128
}
```

### 8.4 Upgrade `GET /analytics/overview`

Query params:

- `days`
- `sortBy`
- `compare`

Response:

```json
{
  "range": {
    "days": 30,
    "start": "2026-03-24",
    "end": "2026-04-23"
  },
  "totals": {
    "plays": 1200,
    "favorites": 80,
    "downloads": 44,
    "revenue_minor": 2499000,
    "currency": "INR",
    "unique_listeners": 420,
    "cart_additions": 31,
    "wishlist_additions": 18,
    "followers": 847,
    "live_beats": 24,
    "draft_beats": 3,
    "processing_beats": 1
  },
  "deltas": {
    "plays_pct": 18,
    "revenue_pct": 12,
    "downloads_pct": 9,
    "followers_pct": 3
  },
  "behavior": {
    "conversion_rate": 4.8,
    "average_listen_seconds": 108,
    "skip_rate": 32
  },
  "series": {
    "plays_by_day": [],
    "downloads_by_day": [],
    "revenue_by_day": []
  },
  "breakdowns": {
    "revenue_by_license": {},
    "countries": [],
    "referrers": [],
    "devices": [],
    "hourly_heatmap": []
  },
  "top_specs": [],
  "recent_activity": []
}
```

Compatibility:

- Keep current top-level fields during transition:
  - `total_plays`
  - `total_favorites`
  - `total_revenue`
  - `total_downloads`
  - `plays_by_day`
  - `downloads_by_day`
  - `revenue_by_day`
  - `top_specs`
  - `revenue_by_license`

### 8.5 Add Analytics Detail Endpoints

#### `GET /analytics/activity`

Query params:

- `scope=producer|listener`
- `limit`
- `page`
- `types`

Response:

```json
{
  "data": [
    {
      "id": "uuid",
      "type": "sale",
      "actor": {
        "id": "uuid",
        "name": "Buyer",
        "avatar_url": "https://..."
      },
      "entity": {
        "type": "spec",
        "id": "uuid",
        "title": "Violet Hour"
      },
      "message": "License sold",
      "amount_minor": 249900,
      "currency": "INR",
      "created_at": "2026-04-23T00:00:00Z"
    }
  ],
  "metadata": {}
}
```

#### `GET /analytics/geo`

Returns country/city breakdowns for maps and lists.

#### `GET /analytics/referrers`

Returns source/referrer breakdowns.

#### `GET /analytics/heatmap`

Returns hourly or day/hour engagement grid.

#### `GET /analytics/listeners`

Returns unique listeners, returning listeners, anonymous/authenticated split.

#### `GET /analytics/specs/{id}/timeline`

Returns per-spec daily plays, downloads, favorites, cart additions, wishlists, and revenue.

## 9. Dashboard APIs

### 9.1 Add `GET /dashboard/summary`

Purpose: Single endpoint for dashboard top-level UI. This avoids each dashboard load making many separate calls.

Auth:

- Required.

Query params:

- `role=producer|listener`
- `days=30`

Producer response:

```json
{
  "role": "producer",
  "user": {
    "id": "uuid",
    "display_name": "Blaze",
    "handle": "saaransh",
    "avatar_url": "https://..."
  },
  "stats": {
    "catalog_count": 24,
    "live_beats": 21,
    "drafts": 3,
    "processing": 0,
    "total_revenue_minor": 247180000,
    "withdrawable_balance_minor": 12475000,
    "plays": 1200,
    "downloads": 44,
    "followers": 847
  },
  "recent_uploads": [],
  "top_beats": [],
  "recent_sales": [],
  "recent_activity": [],
  "upcoming_scheduled_drops": []
}
```

Listener response:

```json
{
  "role": "listener",
  "user": {},
  "stats": {
    "owned_licenses": 47,
    "total_spent_minor": 13240000,
    "downloads_used": 86,
    "wishlist_count": 12,
    "store_credit_minor": 480000,
    "currency": "INR"
  },
  "recent_purchases": [],
  "recently_played": [],
  "favorite_producers": [],
  "activity": []
}
```

## 10. Purchases API Upgrades

### 10.1 Upgrade `GET /licenses`

Query params:

| Param | Type | Purpose |
| --- | --- | --- |
| `page` | number | Page number |
| `limit` | number | Page size |
| `q` | string | Search title, producer, license key |
| `type` | string | License type |
| `producer_id` | uuid | Producer filter |
| `date_from` | date | Purchase date start |
| `date_to` | date | Purchase date end |
| `sort` | string | `newest`, `oldest`, `price_desc`, `price_asc`, `downloads` |

Expanded row:

```json
{
  "id": "uuid",
  "order_id": "uuid",
  "user_id": "uuid",
  "spec_id": "uuid",
  "spec_title": "Violet Hour",
  "spec_image": "https://...",
  "spec_preview_url": "https://...",
  "producer_id": "uuid",
  "producer_name": "Kita Sol",
  "producer_handle": "kitasol",
  "bpm": 92,
  "key": "E MINOR",
  "genres": [
    { "id": "uuid", "name": "Trap", "slug": "trap" }
  ],
  "license_option_id": "uuid",
  "license_type": "Premium",
  "license_name": "Premium WAV",
  "file_types": ["MP3", "WAV"],
  "purchase_price_minor": 249900,
  "purchase_price_major": 2499,
  "currency": "INR",
  "license_key": "LIC-...",
  "is_active": true,
  "is_revoked": false,
  "revoked_reason": null,
  "downloads_count": 2,
  "download_limit": null,
  "downloads_remaining": null,
  "last_downloaded_at": "2026-04-23T00:00:00Z",
  "invoice_url": "https://...",
  "issued_at": "2026-04-23T00:00:00Z",
  "created_at": "2026-04-23T00:00:00Z",
  "updated_at": "2026-04-23T00:00:00Z"
}
```

Response:

```json
{
  "data": [],
  "metadata": {
    "total": 47,
    "page": 1,
    "per_page": 8,
    "total_pages": 6
  },
  "summary": {
    "total_spent_minor": 13240000,
    "currency": "INR",
    "downloads_used": 86,
    "premium_count": 19
  }
}
```

## 11. Orders API Upgrades

### 11.1 Upgrade `GET /orders/producer`

Query params:

| Param | Type | Purpose |
| --- | --- | --- |
| `page` | number | Page number |
| `limit` | number | Page size |
| `q` | string | Buyer, email, spec, order ID |
| `status` | string | paid, processing, refunded, failed |
| `license_type` | string | Basic/Premium/Trackout/Unlimited |
| `date_from` | date | Created at lower bound |
| `date_to` | date | Created at upper bound |
| `sort` | string | newest, oldest, amount_desc, amount_asc |

Expanded row:

```json
{
  "id": "uuid",
  "status": "paid",
  "created_at": "2026-04-23T00:00:00Z",
  "updated_at": "2026-04-23T00:00:00Z",
  "buyer": {
    "id": "uuid",
    "name": "Sam L.",
    "email": "sam@example.com",
    "avatar_url": "https://..."
  },
  "spec": {
    "id": "uuid",
    "title": "Copper Saints",
    "image_url": "https://..."
  },
  "license": {
    "type": "Premium",
    "name": "Premium WAV",
    "option_id": "uuid"
  },
  "money": {
    "amount_minor": 663900,
    "amount_major": 6639,
    "currency": "INR"
  },
  "payment": {
    "method": "upi",
    "razorpay_order_id": "order_x",
    "razorpay_payment_id": "pay_x"
  },
  "invoice_url": "https://...",
  "refund_status": "none"
}
```

Response:

```json
{
  "orders": [],
  "metadata": {
    "total": 30,
    "page": 1,
    "per_page": 10,
    "total_pages": 3
  },
  "summary": {
    "revenue_this_month_minor": 4270000,
    "average_order_value_minor": 249900,
    "paid_count": 25,
    "refunded_count": 1,
    "currency": "INR"
  }
}
```

### 11.2 Add `GET /orders/summary`

Purpose: Stat cards without loading all rows.

### 11.3 Add `GET /orders/export`

Purpose: Server-side CSV export with same filters as order list.

### 11.4 Optional `POST /orders/{id}/refund`

Purpose: Future refund workflow.

This should not be first pass unless refunds are needed operationally.

## 12. Earnings and Payout APIs

### 12.1 New Earnings Module

Recommended module path:

- `internal/modules/earnings`

Suggested boundaries:

- Domain: balances, payouts, payout methods, tax documents, ledger entries.
- Application: summary calculations, payout requests, method updates.
- Infrastructure: PostgreSQL repositories.
- HTTP: earnings handlers.

### 12.2 Database Tables

#### `producer_balances`

Purpose: Current producer financial state.

Columns:

- `producer_id UUID PRIMARY KEY REFERENCES users(id)`
- `currency VARCHAR(3) NOT NULL DEFAULT 'INR'`
- `available_balance_minor INTEGER NOT NULL DEFAULT 0`
- `pending_clearance_minor INTEGER NOT NULL DEFAULT 0`
- `lifetime_revenue_minor INTEGER NOT NULL DEFAULT 0`
- `lifetime_payout_minor INTEGER NOT NULL DEFAULT 0`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

#### `earnings_ledger`

Purpose: Auditable financial movement. Optional but strongly recommended.

Columns:

- `id UUID PRIMARY KEY`
- `producer_id UUID NOT NULL REFERENCES users(id)`
- `order_id UUID REFERENCES orders(id)`
- `payment_id UUID REFERENCES payments(id)`
- `payout_id UUID`
- `type VARCHAR(40) NOT NULL`
- `amount_minor INTEGER NOT NULL`
- `currency VARCHAR(3) NOT NULL DEFAULT 'INR'`
- `status VARCHAR(40) NOT NULL`
- `available_at TIMESTAMPTZ`
- `metadata JSONB DEFAULT '{}'`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

Ledger types:

- `sale_gross`
- `platform_fee`
- `refund`
- `payout`
- `adjustment`

#### `payout_methods`

Columns:

- `id UUID PRIMARY KEY`
- `producer_id UUID NOT NULL REFERENCES users(id)`
- `type VARCHAR(40) NOT NULL`
- `label VARCHAR(120)`
- `upi_id VARCHAR(255)`
- `bank_name VARCHAR(120)`
- `account_last4 VARCHAR(4)`
- `ifsc VARCHAR(20)`
- `is_default BOOLEAN DEFAULT false`
- `status VARCHAR(40) DEFAULT 'active'`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- `updated_at TIMESTAMPTZ DEFAULT NOW()`

#### `payouts`

Columns:

- `id UUID PRIMARY KEY`
- `producer_id UUID NOT NULL REFERENCES users(id)`
- `payout_method_id UUID REFERENCES payout_methods(id)`
- `amount_minor INTEGER NOT NULL`
- `currency VARCHAR(3) NOT NULL DEFAULT 'INR'`
- `status VARCHAR(40) NOT NULL`
- `requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `processed_at TIMESTAMPTZ`
- `reference VARCHAR(120)`
- `failure_reason TEXT`
- `metadata JSONB DEFAULT '{}'`

Payout statuses:

- `requested`
- `processing`
- `paid`
- `failed`
- `cancelled`

#### `tax_documents`

Columns:

- `id UUID PRIMARY KEY`
- `producer_id UUID NOT NULL REFERENCES users(id)`
- `tax_year VARCHAR(20) NOT NULL`
- `document_type VARCHAR(40) NOT NULL`
- `status VARCHAR(40) NOT NULL`
- `file_url TEXT`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- `updated_at TIMESTAMPTZ DEFAULT NOW()`

### 12.3 Endpoints

#### `GET /earnings/summary`

Response:

```json
{
  "currency": "INR",
  "available_balance_minor": 12475000,
  "pending_clearance_minor": 790000,
  "lifetime_revenue_minor": 247180000,
  "this_month_revenue_minor": 21840000,
  "platform_fees_minor": 2400000,
  "refund_deductions_minor": 0,
  "net_earnings_minor": 223180000,
  "total_orders": 86,
  "last_payout": {
    "id": "uuid",
    "amount_minor": 19482000,
    "paid_at": "2026-04-01T00:00:00Z"
  },
  "next_payout_date": "2026-05-01"
}
```

#### `GET /earnings/monthly`

Query params:

- `months=12`

Response:

```json
{
  "data": [
    {
      "month": "2026-04",
      "gross_revenue_minor": 21840000,
      "net_revenue_minor": 19840000,
      "orders": 19,
      "currency": "INR"
    }
  ]
}
```

#### `GET /earnings/license-breakdown`

Response:

```json
{
  "data": [
    {
      "license_type": "Premium",
      "revenue_minor": 44490000,
      "orders": 18,
      "share_pct": 32,
      "currency": "INR"
    }
  ]
}
```

#### `GET /earnings/payouts`

Query params:

- `page`
- `limit`
- `status`

Response:

```json
{
  "data": [],
  "metadata": {}
}
```

#### `POST /earnings/payouts/request`

Body:

```json
{
  "amount_minor": 12475000,
  "currency": "INR",
  "payout_method_id": "uuid"
}
```

Response:

```json
{
  "id": "uuid",
  "status": "requested"
}
```

#### `GET /earnings/payout-method`

Returns default payout method.

#### `PATCH /earnings/payout-method`

Updates or creates payout method.

#### `GET /earnings/tax-documents`

Returns producer tax documents.

## 13. Profile and Social APIs

### 13.1 User Table Additions

Add to `users`:

| Column | Type | Purpose |
| --- | --- | --- |
| `handle` | `VARCHAR(80)` unique | Public profile slug |
| `location` | `VARCHAR(120)` | Display location |
| `country_code` | `VARCHAR(2)` | Localization |
| `locale` | `VARCHAR(20)` | Formatting |
| `preferred_currency` | `VARCHAR(3)` | Currency display |
| `website_url` | `VARCHAR(255)` | Public profile link |
| `banner_url` | `VARCHAR(500)` | Profile banner |
| `profile_tags` | `TEXT[]` | Producer tags |
| `producer_settings` | `JSONB` | Studio producer preferences |
| `notification_preferences` | `JSONB` | User notification settings |

### 13.2 Profile Handles and Public URLs

Profile handles should be first-class and unique. The frontend should be able to route public profiles using a human-readable handle instead of only a UUID.

Preferred route patterns:

- `/@{handle}`
- `/users/{id}` only as a fallback/internal route

Examples:

- `/@kitasol`
- `/@artist.handle`

Handle rules:

- Store lowercase canonical handle in `users.handle`.
- Allow letters, numbers, dots, underscores, and hyphens.
- Recommended regex: `^[a-z0-9][a-z0-9._-]{2,29}$`
- Disallow leading/trailing dot, underscore, or hyphen.
- Prevent consecutive dots if desired.
- Reserve platform routes and words:
  - `admin`
  - `api`
  - `auth`
  - `studio`
  - `search`
  - `beats`
  - `b`
  - `settings`
  - `dashboard`
  - `support`
- Add a unique index on `LOWER(handle)`.
- Generate default handles from name/display name during migration for existing users.
- If a generated handle collides, append a short suffix.

Public lookup options:

- Add `GET /users/handle/{handle}/public`.
- Keep `GET /users/{id}/public` for UUID lookup.
- Frontend profile route `/@{handle}` should call the handle endpoint.

Optional redirect/history:

- Add `user_handle_history` if handle changes should keep old links working.
- Columns:
  - `user_id UUID REFERENCES users(id)`
  - `handle VARCHAR(80)`
  - `changed_at TIMESTAMPTZ DEFAULT NOW()`

### 13.3 `user_follows`

Columns:

- `follower_id UUID NOT NULL REFERENCES users(id)`
- `following_id UUID NOT NULL REFERENCES users(id)`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- Primary key: `(follower_id, following_id)`

### 13.4 `wishlists`

Columns:

- `user_id UUID NOT NULL REFERENCES users(id)`
- `spec_id UUID NOT NULL REFERENCES specs(id)`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- Primary key: `(user_id, spec_id)`

### 13.5 Optional `user_profile_stats`

Use as a cached/materialized stats table if live aggregation becomes expensive.

Columns:

- `user_id UUID PRIMARY KEY`
- `followers_count INTEGER`
- `following_count INTEGER`
- `beats_count INTEGER`
- `total_plays INTEGER`
- `total_sales INTEGER`
- `updated_at TIMESTAMPTZ`

### 13.6 Endpoints

#### Upgrade `GET /users/{id}/public`

Response should include:

```json
{
  "id": "uuid",
  "name": "Producer",
  "display_name": "Producer",
  "handle": "producer",
  "role": "producer",
  "bio": "Bio",
  "avatar_url": "https://...",
  "banner_url": "https://...",
  "location": "India",
  "country_code": "IN",
  "profile_tags": ["Trap", "Cinematic"],
  "website_url": "https://...",
  "instagram_url": "https://...",
  "twitter_url": "https://...",
  "youtube_url": "https://...",
  "spotify_url": "https://...",
  "stats": {
    "followers": 847,
    "beats": 24,
    "plays": 123400
  },
  "viewer": {
    "is_following": false
  },
  "created_at": "2026-04-23T00:00:00Z"
}
```

#### Add `GET /users/handle/{handle}/public`

Purpose: Public profile lookup for `/@artist.handle` style routes.

Response:

- Same shape as `GET /users/{id}/public`.

Errors:

- `404` when handle does not exist.
- `400` when handle format is invalid.

#### `GET /users/{id}/stats`

Returns profile stats only.

#### `POST /users/{id}/follow`

Follows user.

#### `DELETE /users/{id}/follow`

Unfollows user.

#### `GET /me/following`

Returns followed users/producers.

#### `GET /me/followers`

Returns followers.

#### `GET /me/wishlist`

Returns wishlisted specs.

#### `POST /wishlist/{spec_id}`

Adds spec to wishlist.

#### `DELETE /wishlist/{spec_id}`

Removes spec from wishlist.

#### `PATCH /users/settings`

Updates notification preferences, producer settings, locale, currency, and privacy settings.

## 14. Notifications API Upgrades

### 14.1 Table Changes

Add to `notifications`:

| Column | Type | Purpose |
| --- | --- | --- |
| `entity_type` | `VARCHAR(60)` | spec/order/message/payout/user |
| `entity_id` | `UUID` nullable | Entity ID |
| `actor_id` | `UUID` nullable references users | User who caused event |
| `action_url` | `TEXT` | Frontend navigation target |
| `metadata` | `JSONB DEFAULT '{}'` | UI payload |
| `priority` | `VARCHAR(30) DEFAULT 'normal'` | UI ordering/importance |
| `read_at` | `TIMESTAMPTZ` nullable | Read timestamp |

Notification types:

- `sale`
- `message`
- `cart`
- `follow`
- `milestone`
- `wishlist`
- `payout`
- `upload_complete`
- `upload_failed`
- `system`

### 14.2 Upgrade `GET /notifications`

Query params:

- `type`
- `unread`
- `page`
- `limit`
- `offset` retained for compatibility

Response:

```json
{
  "data": [
    {
      "id": "uuid",
      "type": "sale",
      "title": "License sold",
      "message": "Copper Saints sold as Premium WAV",
      "is_read": false,
      "read_at": null,
      "created_at": "2026-04-23T00:00:00Z",
      "actor": {
        "id": "uuid",
        "name": "Sam L.",
        "avatar_url": "https://..."
      },
      "entity": {
        "type": "order",
        "id": "uuid"
      },
      "action_url": "/studio/orders",
      "metadata": {
        "amount_minor": 663900,
        "currency": "INR",
        "license_type": "Premium",
        "spec_title": "Copper Saints",
        "spec_image": "https://..."
      }
    }
  ],
  "counts": {
    "all": 20,
    "unread": 5,
    "sale": 3,
    "message": 2,
    "cart": 1,
    "follow": 1
  },
  "metadata": {
    "total": 20,
    "page": 1,
    "per_page": 10
  }
}
```

### 14.3 Event Sources

Generate notifications from:

- Payment verification success -> sale notification for producer, purchase notification for buyer.
- Message creation -> message notification.
- Wishlist add -> wishlist notification for producer.
- Follow -> follow notification.
- Payout status change -> payout notification.
- Upload processing complete/failed -> upload notification.
- Analytics milestone worker -> milestone notification.

## 15. Messaging APIs

### 15.1 New Messaging Module

Recommended module path:

- `internal/modules/messaging`

### 15.2 Tables

#### `conversations`

Columns:

- `id UUID PRIMARY KEY`
- `subject VARCHAR(255)`
- `created_by UUID REFERENCES users(id)`
- `last_message_at TIMESTAMPTZ`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- `updated_at TIMESTAMPTZ DEFAULT NOW()`

#### `conversation_participants`

Columns:

- `conversation_id UUID REFERENCES conversations(id)`
- `user_id UUID REFERENCES users(id)`
- `role VARCHAR(40)`
- `last_read_at TIMESTAMPTZ`
- `archived_at TIMESTAMPTZ`
- Primary key: `(conversation_id, user_id)`

#### `messages`

Columns:

- `id UUID PRIMARY KEY`
- `conversation_id UUID REFERENCES conversations(id)`
- `sender_id UUID REFERENCES users(id)`
- `body TEXT NOT NULL`
- `attachments JSONB DEFAULT '[]'`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- `edited_at TIMESTAMPTZ`
- `deleted_at TIMESTAMPTZ`

#### `message_reads`

Columns:

- `message_id UUID REFERENCES messages(id)`
- `user_id UUID REFERENCES users(id)`
- `read_at TIMESTAMPTZ DEFAULT NOW()`
- Primary key: `(message_id, user_id)`

### 15.3 Endpoints

#### `GET /conversations`

Query params:

- `page`
- `limit`
- `q`
- `unread`

Returns conversation list for Studio messages.

#### `GET /conversations/{id}`

Returns one conversation and participants.

#### `POST /conversations`

Body:

```json
{
  "participant_ids": ["uuid"],
  "subject": "Collab request",
  "message": "Want to work on the Friday drop?"
}
```

#### `GET /conversations/{id}/messages`

Query params:

- `page`
- `limit`
- `before`

#### `POST /conversations/{id}/messages`

Body:

```json
{
  "body": "Message text",
  "attachments": []
}
```

#### `PATCH /conversations/{id}/read`

Marks conversation read for current user.

## 16. Battles APIs

### 16.1 New Battles Module

Recommended module path:

- `internal/modules/battle`

### 16.2 Tables

#### `battles`

Columns:

- `id UUID PRIMARY KEY`
- `slug VARCHAR(120) UNIQUE`
- `title VARCHAR(200) NOT NULL`
- `type VARCHAR(40) NOT NULL`
- `format VARCHAR(80)`
- `status VARCHAR(40)`
- `brief JSONB DEFAULT '{}'`
- `rules TEXT[] DEFAULT '{}'`
- `reward JSONB DEFAULT '{}'`
- `max_participants INTEGER`
- `starts_at TIMESTAMPTZ`
- `submission_deadline TIMESTAMPTZ`
- `voting_deadline TIMESTAMPTZ`
- `created_by UUID REFERENCES users(id)`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- `updated_at TIMESTAMPTZ DEFAULT NOW()`

Battle types:

- `artist`
- `producer`

Statuses:

- `draft`
- `open`
- `submissions_open`
- `voting`
- `judging`
- `closed`
- `cancelled`

#### `battle_rooms`

Columns:

- `id UUID PRIMARY KEY`
- `battle_id UUID REFERENCES battles(id)`
- `title VARCHAR(200)`
- `type VARCHAR(40)`
- `owner_id UUID REFERENCES users(id)`
- `privacy VARCHAR(40)`
- `invite_code VARCHAR(120)`
- `max_participants INTEGER`
- `deadline TIMESTAMPTZ`
- `created_at TIMESTAMPTZ DEFAULT NOW()`

#### `battle_participants`

Columns:

- `battle_id UUID REFERENCES battles(id)`
- `user_id UUID REFERENCES users(id)`
- `role VARCHAR(40)`
- `status VARCHAR(40)`
- `joined_at TIMESTAMPTZ DEFAULT NOW()`
- Primary key: `(battle_id, user_id)`

#### `battle_submissions`

Columns:

- `id UUID PRIMARY KEY`
- `battle_id UUID REFERENCES battles(id)`
- `user_id UUID REFERENCES users(id)`
- `title VARCHAR(200)`
- `audio_url TEXT`
- `cover_url TEXT`
- `notes TEXT`
- `score INTEGER DEFAULT 0`
- `status VARCHAR(40)`
- `created_at TIMESTAMPTZ DEFAULT NOW()`

#### `battle_votes`

Columns:

- `id UUID PRIMARY KEY`
- `battle_id UUID REFERENCES battles(id)`
- `submission_id UUID REFERENCES battle_submissions(id)`
- `voter_id UUID REFERENCES users(id)`
- `score INTEGER`
- `created_at TIMESTAMPTZ DEFAULT NOW()`
- Unique: `(submission_id, voter_id)`

#### `battle_rewards`

Columns:

- `id UUID PRIMARY KEY`
- `battle_id UUID REFERENCES battles(id)`
- `rank INTEGER`
- `label VARCHAR(120)`
- `metadata JSONB DEFAULT '{}'`

#### `battle_activity`

Columns:

- `id UUID PRIMARY KEY`
- `battle_id UUID REFERENCES battles(id)`
- `type VARCHAR(60)`
- `message TEXT`
- `actor_id UUID REFERENCES users(id)`
- `metadata JSONB DEFAULT '{}'`
- `created_at TIMESTAMPTZ DEFAULT NOW()`

### 16.3 Endpoints

#### `GET /battles`

Query params:

- `type`
- `status`
- `page`
- `limit`

Response includes cards for artist/producer battles.

#### `GET /battles/{id}`

Returns battle detail, rules, settings, rewards, user's participation status, and top submissions.

#### `POST /battles`

Admin or producer-created battle.

#### `POST /battles/{id}/join`

Joins authenticated user.

#### `POST /battles/{id}/submissions`

Creates submission with audio upload or file URL.

#### `GET /battles/{id}/submissions`

Lists submissions.

#### `POST /battles/{id}/votes`

Submits vote.

#### `GET /battles/leaderboard`

Returns global leaderboard.

#### `GET /battle-rooms`

Lists rooms for authenticated user or public rooms.

#### `POST /battle-rooms`

Creates room.

## 17. Recommended Rollout Order

### Phase 1: Response Consistency and Pagination

Fix:

- `GET /specs`
- `GET /users/{id}/specs`
- `GET /licenses`
- `GET /orders/producer`
- `GET /analytics/top-specs`

Acceptance:

- `per_page` works everywhere.
- `limit` is treated as alias where frontend already sends it.
- Response metadata includes `total`, `page`, `per_page`, `total_pages`.
- Money fields are documented and consistent for new responses.

### Phase 2: Beat Metadata and Upload

Add:

- moods
- usage tags
- sync-ready
- tagless preview
- has stems
- release status
- draft completion
- waveform
- rights metadata

Acceptance:

- Upload persists mood and description.
- Search can query mood and extras.
- Beat details no longer needs hardcoded descriptive sections where API data exists.

### Phase 3: Search and Discovery

Add:

- `GET /catalog/facets`
- `GET /catalog/home`
- New filters/sorts in `GET /specs`

Acceptance:

- Search visual-only filters become real.
- Home hero stats, genre counts, suggestions, ticker, and featured sections become API-backed.

### Phase 4: Analytics and Dashboard

Add:

- Analytics event metadata.
- Upgraded overview.
- Activity endpoint.
- Dashboard summary endpoint.

Acceptance:

- Studio analytics countries/referrers/heatmap are API-backed.
- Dashboard listener and producer activity sections are API-backed.

### Phase 5: Purchases and Orders

Upgrade:

- `GET /licenses`
- `GET /orders/producer`
- `GET /orders/summary`
- `GET /orders/export`

Acceptance:

- Studio purchases search/per-page/filter works.
- Studio orders search/per-page/filter works.
- Purchase and order rows have enough joined metadata for UI cards/tables.

### Phase 6: Earnings

Add:

- Earnings module.
- Balance/ledger/payout/tax tables.
- Earnings endpoints.

Acceptance:

- Studio earnings page is fully API-backed.
- Payout history/method/tax docs are no longer hardcoded.

### Phase 7: Profile, Social, Notifications

Add:

- Expanded profile fields.
- Follow/wishlist tables.
- Typed notifications.
- Settings endpoint.

Acceptance:

- Profile/settings preview stats and preferences are API-backed.
- Navbar notifications no longer need reference hardcoded sale/message/cart/follow items.

### Phase 8: Messaging and Battles

Add:

- Messaging module.
- Battles module.

Acceptance:

- Studio messages page is real.
- Battles page/detail/submissions/voting are real.

## 18. Testing Strategy

### 18.1 Backend Tests

Migration tests:

- Every migration has up/down coverage.
- New enum/check constraints accept documented values and reject invalid values.

Repository tests:

- Catalog filters and sorts.
- Pagination metadata.
- License search/filter/limit.
- Producer order search/filter/limit.
- Analytics aggregations.
- Earnings ledger and balance calculations.
- Notification type filters/counts.
- Messaging conversation access.
- Battle join/submission/vote constraints.

Handler tests:

- Query params parsed correctly.
- Auth-required endpoints reject unauthenticated users.
- Flexible-auth endpoints work for anonymous and logged-in users.
- Response shapes include required fields.

Service tests:

- Payment verification emits sale/license/earnings/notification side effects.
- Payout request validates available balance.
- Follow/wishlist idempotency.
- Battle vote uniqueness.

### 18.2 Frontend Tests

Request class specs:

- New params serialize correctly.
- Pagination params use `page` and `per_page`/`limit`.

Adapter specs:

- New `SpecDto` fields map into `Spec`.
- Money fields map safely.

Page/component specs:

- Search filters call API with real params.
- Upload sends moods/description/rights metadata.
- Studio purchases uses pagination component and backend metadata.
- Studio orders uses pagination component and backend metadata.
- Analytics charts render from API arrays.
- Currency formatting service respects user preference/fallbacks.

### 18.3 Acceptance Checks

- No UI section uses fake rows once an API exists.
- Empty states replace fake fallback data.
- Charts render from API arrays without hardcoded fallback series when backend returns data.
- Per-page works on search, tracks, purchases, and orders.
- Search filters are functional, not visual-only.
- Currency displays from API/user preference/fallback locale.
- Checkout remains INR until full multi-currency is intentionally implemented.

## 19. Compatibility Notes

- Do not break current frontend immediately. Add fields before removing legacy fields.
- Keep current top-level analytics fields while adding richer nested fields.
- Keep `amount` on existing responses until frontend moves to `amount_minor`.
- Continue supporting `limit` as an alias where frontend already sends it.
- Continue returning `metadata.per_page`.
- Use null/empty arrays instead of omitting fields where the UI expects arrays.
- Keep UUID beat detail URLs working after adding slugs and short codes.
- Add redirects from old slug history only if handle/slug changes are allowed.
- Keep `GET /users/{id}/public` working after adding `GET /users/handle/{handle}/public`.
- Existing users need generated handles before `/@handle` routes become the primary public profile URL.
- Existing beats need generated slugs, short codes, and duration backfill before `/beats/{slug}` and `/b/{short_code}` become primary share URLs.

## 20. Open Product Decisions

These should be decided before implementation:

1. Are WAV and stems truly required for all beats, or optional by license tier?
2. Is `wishlist` separate from `favorite`, or should favorite become wishlist?
3. Does Cult Beats need full multi-currency checkout, or only localized display?
4. Should battles be public user-generated, admin-created, or both?
5. Should messages support attachments in v1?
6. Should earnings use a true ledger from day one, or derive balances from orders until payout launch?
7. Should home/discovery content be algorithmic only, or admin-curated?
8. Should beat slugs update when titles change, or stay stable forever unless manually edited?
9. Should public profile handles be editable by users, and should old handles redirect?
10. Should compact beat share URLs use `/b/{short_code}` or another prefix?

## 21. Final Recommendation

Start with the small correctness work that unlocks current UI fixes:

1. Honor `per_page`/`limit` in catalog and licenses.
2. Normalize pagination metadata.
3. Expand joined metadata for purchases and orders.
4. Add spec mood/usage/search metadata.
5. Add facets and home summary.

After that, build analytics, earnings, notifications, messages, and battles as formal modules. This sequence turns the redesigned UI from reference-faithful mock surfaces into durable product surfaces without forcing a risky big-bang backend rewrite.
