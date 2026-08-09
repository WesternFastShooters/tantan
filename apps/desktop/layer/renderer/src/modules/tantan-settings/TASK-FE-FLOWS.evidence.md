# TASK-FE-FLOWS evidence

## Red

- `2026-08-09 23:14 CST`: the focused Vitest command failed in three suites because
  `search-model`, `provider-form`, and `subscription-model` did not exist. This locked
  the 250 ms search boundary, provider preset/key payload rules, and preservation of
  the four RSS subscription views before implementation.

## Green / refactor

- Added `/search` with a 250 ms debounce, cursor pagination, first-seen `entryId`
  deduplication, index-building state, and browser return that preserves Home Topic,
  Filter, Queue, and scroll state.
- Added local AI Provider settings backed only by the Go service. Endpoints are
  hard-coded presets; the secret is held only in the password field until submission,
  omitted on blank updates, cleared after save, and never echoed by the UI.
- Added a versioned Topic manager. `recommend` remains immutable and Topic changes
  update the shared Topics/Home cache without replacing the active queue contract.
- Preserved the Folo RSS subscription store and services for add/cancel/list flows,
  including Articles, Social Media, Pictures, and Videos views.
- Added Source history and Favorites views using the existing Folo Entry, Unread, and
  Collection services.
- Added local translation/summary enrichment to Entry detail. Original content always
  remains readable while enrichment is pending or failed.

## Verify

- Full web Vitest: 49 files, 139 tests passed with no type errors, including Home,
  shell, paid-feature policy, and settings metadata regressions.
- Folo Store regression: 4 files, 12 Entry/Unread/Subscription tests passed.
- Flow Playwright: 5/5 passed on PC and Mobile, including search isolation, provider
  canary handling, Topic mutation, RSS add/cancel, collection, and AI failure fallback.
- Combined Tantan Playwright regression: 16/16 passed.
- Forbidden Folo requests in captured browser traffic: 0.
- Provider canary occurrences in URL, localStorage, sessionStorage, and saved form: 0.
- Focused ESLint: passed with no warnings or errors.
- `pnpm --filter @follow/web typecheck`: passed.
- `pnpm build:web`: passed and generated Search, AI settings, Topic settings,
  Favorites, and Source detail routes.
- `bash spec-package/scripts/validate-package.sh`: passed all package checks.
- `/Users/mingrui/Project/Folo` and `apps/mobile/**`: no changes.
