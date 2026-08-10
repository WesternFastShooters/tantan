# TASK-E2E-RELEASE evidence

Task ID: `TASK-E2E-RELEASE`  
Baseline commit: `3846c90b67da351b6017cd4fe9d0992b8077224e`  
Date: 2026-08-10

## Red

- Entry enrichment acceptance initially failed because failed jobs had no visible retry action.
- Source block acceptance exposed a public-contract gap: blocks could be created but not listed/restored. The user had pre-authorized remaining in-scope contract work; the contract was minimally extended with exact GET/DELETE routes, regenerated types, and a default-deny implementation.
- The real Google translation initially failed behind the machine's fake-IP DNS because the strict dialer correctly rejected `198.18.0.0/15`. A locked, explicit loopback HTTP(S) proxy policy was added; remote, credentialed, path/query, SOCKS and hostname proxies remain rejected.
- Axe initially found invalid Masonry grid parentage and an unnamed programmatic popover trigger. Both failures were preserved in Playwright output before the semantic fixes.
- Full lint initially failed on one unused test import and one unsorted production import.

## Green / refactor

- Added retryable enrichment UI while preserving original content.
- Added optimistic feedback rollback, five-second undo, Source/Topic blocks, blocked Source settings and idempotent restore without re-adding the current queue.
- Added Filter edit/reset, reduced-motion handling, list semantics for Masonry and an accessible non-focusable popover anchor.
- Added locked local-proxy transport for Provider requests without weakening exact Provider host, TLS, redirect or custom-endpoint restrictions.
- Added acceptance Playwright coverage for feedback, Filter, search, detail, retry, Topic management, capacity, PWA cache policy, keyboard and Axe.

## Verify

- Package validator: PASS, 0 errors, 0 warnings.
- Web Vitest: 56 files / 166 tests PASS, 0 type errors.
- Tantan Playwright: 33/33 Mobile Web regression tests PASS; production Chromium/WebKit 390/430 is 8/8 PASS.
- Go ordinary and race suites: all packages PASS.
- Typecheck, lint, web build, Go format/vet/build and module verification: PASS.
- Frozen pnpm lockfile installation: PASS.
- Contract generation/hash and generated Go/TypeScript contract tests: PASS.
- Acceptance evidence: all 46 IDs mapped in `acceptance-matrix-evidence.md`.

## Security checks

- Denied Folo route upstream calls: 0.
- Critical Axe violations: 0.
- Sensitive API/Auth Cache Storage entries: 0.
- Unique API Key/Token canary in Git, SQLite, logs, HAR, browser storage, backup and build outputs: 0. Keychain-backed live Gemini translation and FilterSpec calls PASS.
- AI Provider endpoint remains preset-only; explicit proxy is limited to a literal loopback IP and contains no credentials.
- `apps/mobile/**` and `/Users/mingrui/Project/Folo`: no modifications.

The local macOS milestone is complete. Physical-phone trust setup belongs to the later server-deployment milestone. External Folo/Google availability and account credentials remain runtime dependencies.
