# TASK-FE-SHELL evidence

## Red

- Baseline component import failed because `TantanAppShell` did not exist; the baseline Mobile route still selected `DownloadPage`, and `/` still redirected to Timeline.
- PWA policy tests failed because no application-shell precache existed, API/Auth exclusions were absent, and install metadata still advertised Folo/POWER.
- The 768px breakpoint test failed before the Tantan-specific responsive hook existed; Folo's shared hook used 1024px.

## Green / verify

- `pnpm --filter @follow/web test --run src/lib/tantan-api/client.test.ts src/modules/tantan-shell/TantanAppShell.test.tsx src/modules/tantan-shell/pwa-policy.test.ts src/modules/tantan-shell/useTantanMobile.test.ts src/modules/tantan-policy/paid-feature-removal.test.ts`: 5 files, 15 tests passed, no type errors.
- `tantan-shell.spec.ts`: 6 Playwright tests passed at 390, 800 and 1440 widths, including browser back, 401 login, service retry and keyboard focus.
- `tantan-no-paid.spec.ts`: 2 regression tests passed; denied Folo route HAR count remained zero.
- `pnpm --filter @follow/web typecheck`: passed.
- `pnpm --dir apps/desktop build:web`: passed; PWA inject-manifest produced an installable service worker with API/Auth deny rules.
- `bash spec-package/scripts/validate-package.sh`: passed with zero errors and zero warnings.
- Protected RSS store, backend, `apps/mobile/**` and `/Users/mingrui/Project/Folo` have no diff.

## Read-only frontend gate

- Result: PASS.
- Every changed Shell/API/PWA behavior maps to focused Vitest or Playwright coverage.
- The required focused tests, typecheck, production build, package validator, PWA scan, policy regression and protected-path checks pass after the final production change.
