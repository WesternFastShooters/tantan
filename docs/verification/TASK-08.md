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
31/31 PASS; production-only spec excluded
```

Frontend completion gate: PASS. The observable PWA registration/cache behavior maps directly to
the new executable production tests and the focused build/production suite passed after the final
Vite change.

## Remaining external gates

- Rotated-key live Gemini translation: PENDING. The exposed chat credential is not used.
- Physical-phone HTTPS checklist: PENDING until an HTTPS URL reachable from the user's phone is
  available. Deployment itself remains outside phase-one coding scope.

TASK-08 and the Codex Goal must remain active until both observations are completed and final
commands are rerun.
