# TASK-08 production and release-readiness evidence

Date: 2026-08-10

## Red

The first production-bundle run reached the restart checkpoint in all four projects, then showed
that `navigator.serviceWorker.ready` never resolved. Inspection proved that `sw.js` existed but the
Mobile-only router bypassed Folo `RootProviders`, so the old registration component was absent from
the built JavaScript and no registration script was injected.

The first test also expected the original Filter prompt after restart. The API contract only
guarantees the active Filter identity; the test oracle was corrected to the contracted visible
state `AI 智能筛选已启用` without changing product behavior.

The first full development-suite rerun also proved the new production-only spec was being collected
by the broad development matcher and attempted to open port 4173. The regular Web project now
explicitly ignores that file; the production configuration remains its only owner.

The first real LAN HTTPS render produced 64 browser warnings because the bundled SN Pro CSS pointed
at `assets/files/*.woff2`, but those files were absent and the SPA fallback returned HTML. The
production test then failed on `Content-Type: text/html` instead of `font/woff2`.

## Green and refactor

- Vite PWA now injects `registerSW.js` with a deferred production registration. The Mobile router
  stays lightweight and does not regain Folo desktop providers.
- Added a production-only Playwright configuration with Chromium and WebKit at 390×844 and
  430×932.
- Core production tests cover login methods, Home, Topic, normal search, AI Filter, detail/read,
  subscriptions, Discover, settings, reload/restart state and same-origin requests.
- PWA tests separately allow Service Workers and prove the generated worker becomes active while
  authenticated `/api` responses remain absent from Cache Storage. Core API mocks run with Service
  Workers blocked because Playwright cannot route requests intercepted by a worker.
- The Web build now emits every referenced SN Pro WOFF/WOFF2 asset at the exact same-origin path.
  The production test verifies the response MIME type and WOFF2 magic bytes instead of accepting a
  200 SPA fallback.

## Verify

```text
pnpm build:web
PASS; registerSW.js, manifest.webmanifest and sw.js generated

pnpm --dir apps/desktop e2e:web:production
8/8 PASS:
  chromium-390x844 core + PWA
  chromium-430x932 core + PWA
  webkit-390x844 core + PWA
  webkit-430x932 core + PWA

pnpm --dir apps/desktop e2e:web
33/33 PASS; production-only spec excluded

real LAN HTTPS production render
/, manifest.webmanifest, sw.js, /api/healthz, /api/readyz: 200
five Folo login methods visible; browser font warnings: 0

live Keychain-backed Gemini checks
translation PASS; FilterSpec PASS
```

Frontend completion gate: PASS. The observable PWA registration/cache behavior maps directly to
the executable production tests; font loading additionally maps to MIME and WOFF2-byte assertions.
The focused build and Chromium/WebKit production suite passed after the final Vite change.

## Local milestone disposition

The user explicitly selected macOS acceptance for this local milestone. Real-account login, Home,
dynamic Topic lazy loading, AI Filter, reset, re-filter, Chinese detail and service restart were
exercised through the LAN HTTPS origin. Physical-phone trust setup is therefore deferred to the
later server-deployment milestone and is not a blocker for this local Goal.
