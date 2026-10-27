# U11 — Operator read-only library search

Owner /find keeps its existing Google-authorized whole-Drive search. Operator
/find searches collection names in their own library via the existing scoped
CollectionPage API, including retired collections, 10 per cursor page. It never
calls Drive search or offers a copy action. Read-only collection details recheck
principal scope and revision. The app search helper permits owners to search all
principals, rejects empty/unknown identities. No new route/schema/migration;
GET collections query/cursor already supplies the HTTP equivalent. Existing
allowlist/private-chat/one-shot/TTL checks remain at Telegram dispatch.
