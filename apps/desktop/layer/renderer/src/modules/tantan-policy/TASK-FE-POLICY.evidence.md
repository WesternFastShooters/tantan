# TASK-FE-POLICY evidence

## Red

- Baseline route/UI scan exposed Plan, Power, Wallet, Upgrade and Folo AI products.
- `paid-feature-removal.test.ts` failed after adding the forbidden Trending assertion, reporting `modules/trending/index.tsx` as a caller of `followClient.api.trending`.

## Green / verify

- `pnpm --filter @follow/web test --run src/modules/app-layout/timeline/TimelineLayout.test.tsx src/modules/tantan-policy/paid-feature-removal.test.ts src/modules/settings/helper/sync-queue.test.ts`: 3 files, 14 tests passed, no type errors.
- `pnpm --filter @follow/web typecheck`: passed.
- `pnpm exec playwright test -c e2e/playwright.config.ts --project=web e2e/tests/web/tantan-no-paid.spec.ts --reporter=line`: 2 tests passed; denied Folo route HAR count is zero.
- `pnpm --dir apps/desktop build:web`: passed, including PWA service-worker build.
- `bash spec-package/scripts/validate-package.sh`: passed with zero errors and zero warnings.
- RSS subscription, Entry, Feed, Unread and Collection stores have no diff; `apps/mobile/**` and `/Users/mingrui/Project/Folo` have no diff.

## Read-only frontend gate

- Result: PASS.
- Changed behavior maps to executable Vitest and Playwright tests.
- Required focused tests, typecheck, production build, package validator, route scan, HAR scan and protected-path checks all pass after the final production change.
