# Mobile Web/PWA v2 acceptance evidence

Date: 2026-08-10

| AC    | Evidence                                                                    | Result                    |
| ----- | --------------------------------------------------------------------------- | ------------------------- |
| AC-01 | `tantan-shell.spec.ts`; production Chromium/WebKit at 390×844 and 430×932   | PASS                      |
| AC-02 | `tantan-home.spec.ts` 60-card, duplicate-boundary, stable two-column scroll | PASS                      |
| AC-03 | Home Topic tests and independent query/scroll store tests                   | PASS                      |
| AC-04 | Search debounce/cursor tests and Home state-preservation E2E                | PASS                      |
| AC-05 | Filter cancel/success/failure/edit/reset and revision/generation tests      | PASS                      |
| AC-06 | Detail/read success removes all Home caches; failure preserves card         | PASS                      |
| AC-07 | Subscription, Discover, Source and grouped settings component/E2E tests     | PASS                      |
| AC-08 | Google/GitHub/Apple/Email/TOTP/token auth bridge and returnTo tests         | PASS                      |
| AC-09 | Paid-feature policy tests, source scan and HAR; product entries are zero    | PASS                      |
| AC-10 | Production manifest/SW registration, no-API-cache and explicit CSP tests    | PASS                      |
| AC-11 | Application start, static SPA, listener, public-origin and readiness tests  | PASS                      |
| AC-12 | Folo auth bridge, one-time-token replay and sealed session canary tests     | PASS                      |
| AC-13 | Exact Folo method/path route-policy matrix; denied upstream count zero      | PASS                      |
| AC-14 | Sync crash/resume/idempotency, FTS seven-field and 100k tests               | PASS                      |
| AC-15 | Fixed Gemini endpoint/model/schema, concurrency and failure tests           | PASS                      |
| AC-16 | Seven-day, 50/60, generation/cursor and 20-concurrent Home tests            | PASS                      |
| AC-17 | Filter transaction crash points, replay and atomic snapshot tests           | PASS                      |
| AC-18 | Read/favorite/subscribe idempotency, rollback and repair tests              | PASS                      |
| AC-19 | Doctor/readiness faults, race, backup/restore checksum/integrity/FK/count   | PASS                      |
| AC-20 | Browser same-origin assertions, HAR, CSP and proxy deny counter             | PASS                      |
| AC-21 | Read success/failure across Folo, SQLite mirror and Home cache tests        | PASS                      |
| AC-22 | Filter revision conflict and stale generation discard tests                 | PASS                      |
| AC-23 | 250ms debounce/abort, cursor fields and Home non-mutation tests             | PASS                      |
| AC-24 | Unique canary scans plus live Gemini translation and FilterSpec calls       | PASS; zero secret matches |
| AC-25 | Production Chromium/WebKit 390/430 plus real-account macOS LAN acceptance   | PASS                      |

The user narrowed this local milestone to macOS acceptance before server deployment. The real
account flow was exercised through the LAN HTTPS origin, and the fixed Gemini provider was tested
through the Go service using the Keychain-backed credential. No credential value entered browser
storage, SQLite, logs, HAR, fixtures, Git or build output.
