# TASK-06 verification

Date: 2026-08-10

## Scope

- Folo-Mobile-style four-tab secondary-page information architecture.
- Discover Source search through same-origin `POST /api/folo/discover`.
- RSS subscription add/remove through the preserved Folo Subscription Store.
- Grouped mobile settings with only local preferences, server Gemini status, Topics, and About.
- Entry read and collection state coordination for both store-backed and direct-detail entries.
- No Plan, Power, Wallet, Upgrade, paid quota CTA, or Folo AI Chat entry.

## Red evidence

Command:

```text
pnpm --filter @follow/web test -- --run ...DiscoverPage.test.tsx ...MobileSettingsPages.test.tsx ...SettingsRoute.test.tsx
```

Expected failure, exit 1:

- `DiscoverPage` did not exist.
- `MobileSettingsPages` did not exist.
- Settings had zero `[data-settings-group]` groups.

The first read-failure E2E also failed because a direct-detail entry was absent from the Folo
frontend store. The old code treated the store's no-op as success and removed the Home card without
an upstream read mutation. This was classified as a valid product Red.

## Green evidence

```text
pnpm --filter @follow/web test -- --run
PASS: 54 files, 156 tests, type errors 0

pnpm typecheck
PASS: 20/20 tasks

pnpm lint
PASS: exit 0 (repository baseline warnings only)

go test ./internal/folo ./internal/sync
PASS

pnpm exec playwright test -c e2e/playwright.config.ts --project=web \
  e2e/tests/web/tantan-acceptance/home-actions.spec.ts \
  e2e/tests/web/tantan-acceptance/performance-accessibility.spec.ts \
  e2e/tests/web/tantan-acceptance/search-detail.spec.ts \
  e2e/tests/web/tantan-flows.spec.ts
PASS: 11/11

pnpm --dir apps/desktop e2e:web
PASS: 29/29

pnpm exec playwright test -c e2e/playwright.config.ts --project=web \
  e2e/tests/web/tantan-home.spec.ts --grep 'read (success|failure)'
PASS: 2/2

bash spec-package/scripts/validate-package.sh
PASS: 0 errors, 0 warnings

pnpm build:web
PASS: Mobile Web/PWA production bundle and service worker generated
```

## Gate mapping

- Discover, subscriptions, grouped settings, Source/Entry detail, and Favorites retain the Folo
  Mobile navigation model while Home remains the approved Tantan replacement.
- Browser-side Folo SDK calls are rewritten to same-origin `/api/folo/**`; the Go route policy still
  performs exact allow/deny enforcement.
- Subscription, collection, and read mutations have single-flight UI locks. Failure restores the
  source-of-truth UI and exposes an error.
- Direct-detail read success removes the entry from every Home query; read failure keeps it.
- `packages/internal/store/src/modules/subscription/**` was not changed.

Formal TASK-06 manifest completion remains deferred while its TASK-05 rotated-key live gate is open.

## Frontend test gate

Verdict: PASS

- Discover search and subscription failure -> `DiscoverPage.test.tsx` FR-15/FR-16 and
  `tantan-mobile-pages.spec.ts` Discover/failed-unsubscribe cases.
- Mobile settings and working destinations -> `MobileSettingsPages.test.tsx`,
  `SettingsRoute.test.tsx`, and the grouped-settings mobile E2E.
- Folo SDK same-origin rewrite and RSS Store preservation -> `tantan-flows.spec.ts` RSS case.
- Direct read success/failure cache coordination -> `tantan-home.spec.ts` read cases.
- Collection single-flight, failure reconciliation, retry, and remove ->
  `search-detail.spec.ts`, `tantan-flows.spec.ts`, and `FavoritesPage.test.tsx`.
- Feedback overlay/nav stacking -> `home-actions.spec.ts` explicit z-index oracle.
- Focused tests and the full 55-file/157-test frontend regression pass; full Mobile Web E2E is
  29/29.
