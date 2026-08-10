# Translated content pool verification

- Raw sync remains the durable source pool in `entries` + `account_entries`.
- `entry_enrichments` is the display gate: original Chinese or a ready translation for the current content hash is required.
- Home and Topic pages already enforce the gate; Source history now reads `GET /api/tantan/v1/content-pool` and never reads raw Folo Entry rows.
- The worker fills a bounded, deduplicated all-history translation backlog and also runs missing-content retry jobs.

Evidence on 2026-08-10:

- Focused Playwright test first failed because Source history never requested the Go pool, then passed after implementation.
- The focused scenario proves translated list title/excerpt, pending progress, full translated detail, and absence of the raw English body.
- Frontend: 56 Vitest files / 166 tests passed; 34 Web Playwright tests passed; typecheck, lint, and production PWA build passed.
- Backend: `go vet`, full tests, race tests, build, contract generator check, package validator, and security verifier passed.
- Security verifier: secret matches 0, forbidden Folo calls 0, direct browser egress 0.

Local runtime status:

- 100 synced entries were present; 60 were Chinese-display-ready and 40 were queued/retrying.
- The configured Keychain credential was rejected by Google as an invalid API key during the live smoke test. Code and mock-provider contracts pass, but remaining live translations require replacing that Keychain value with a valid rotated Gemini API key.
