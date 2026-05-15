# Cult Beats API Implementation Tracker

Use this file as the working checklist while upgrading the backend to match the redesigned UI and newer frontend data requirements.

## Specs / Beats API
- [x] DB columns added
- [ ] duration populated from backend processing
- [ ] `/specs` accepts `page` and `limit`
- [ ] `/specs` accepts `per_page` alias
- [ ] response metadata returns correct pagination
- [ ] moods persisted
- [ ] instruments persisted
- [ ] producer handle returned
- [ ] slug returned
- [ ] short code returned
- [ ] detail lookup by slug
- [ ] detail lookup by short code
- [ ] upload accepts new metadata
- [ ] update endpoint accepts new metadata
- [ ] tests updated
- [ ] legacy specs backfill planned/run

## Search & Facets
- [ ] search filters audited against current UI
- [ ] visual-only filters identified
- [ ] `/specs` search filters expanded
- [ ] sort options aligned with frontend
- [ ] facet/count API planned
- [ ] tests updated

## Home / Discovery
- [ ] hero stats inventory completed
- [ ] ticker/shortcut content inventory completed
- [ ] hardcoded home cards mapped to data requirements
- [ ] discovery/home summary API planned
- [ ] tests updated

## Dashboard
- [ ] producer dashboard hardcoded sections audited
- [ ] listener dashboard hardcoded sections audited
- [ ] dashboard summary contract planned
- [ ] recent activity requirements documented
- [ ] tests updated

## Studio Analytics
- [ ] analytics overview hardcoded values audited
- [ ] heatmap/countries/referrers requirements documented
- [ ] follower and conversion metrics planned
- [ ] analytics activity/feed requirements planned
- [ ] tests updated

## Purchases
- [ ] purchases pagination aligned with UI
- [ ] purchases search/filter requirements documented
- [ ] missing license/spec metadata identified
- [ ] `/licenses` upgrade planned
- [ ] tests updated

## Orders
- [ ] orders pagination aligned with UI
- [ ] orders search/filter requirements documented
- [ ] buyer/spec metadata additions planned
- [ ] export/refund follow-up documented
- [ ] tests updated

## Earnings
- [ ] earnings hardcoded sections audited
- [ ] payout summary requirements documented
- [ ] payout history requirements documented
- [ ] payout method requirements documented
- [ ] tax document requirements documented
- [ ] earnings module/API planned
- [ ] tests updated

## Profile / Social
- [ ] profile handle requirements documented
- [ ] public profile route requirements documented
- [ ] follow/wishlist/social stats requirements documented
- [ ] profile/settings backend fields planned
- [ ] tests updated

## Notifications
- [ ] notification popup hardcoded cards audited
- [ ] notification types expanded
- [ ] typed notification payload contract planned
- [ ] unread/filter pagination requirements documented
- [ ] tests updated

## Messages
- [ ] studio/dashboard messaging requirements documented
- [ ] conversation/message entities planned
- [ ] messaging API contract planned
- [ ] tests updated

## Battles
- [ ] battles UI hardcoded sections audited
- [ ] battle entities and flows documented
- [ ] leaderboard/submission/voting APIs planned
- [ ] tests updated
