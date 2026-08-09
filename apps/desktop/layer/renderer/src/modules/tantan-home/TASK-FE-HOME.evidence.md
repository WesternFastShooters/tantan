# TASK-FE-HOME evidence

## Red

- `2026-08-09 22:53 CST`: the focused Vitest command failed in three suites because
  `card-presentation`, `home-model`, `home-cache`, and `home-view-store` did not exist.
  This locked overlapping-page deduplication, 2/3/4 column boundaries, card fallback,
  all-Home-cache read removal, independent Topic scroll, and atomic filter state.
- `2026-08-09 23:00 CST`: the AI Filter Sheet interaction test failed because Tab on
  the last control did not return focus to the first dialog control (`expected 取消,
received null`).

## Green / refactor

- Added a stable `useInfiniteQuery` Home pipeline. `nextCursor` is the only pagination
  input and pages are merged by first-seen `entryId`.
- Added the five approved Home components: `HomeHeader`, `TopicTabs`,
  `ActiveAIFilterBar`, `MasonryFeed`, and `AIFilterSheet`.
- Reused Folo's internal masonic adapter, with exact 2/3/4 viewport boundaries and
  deterministic article/post/image/video fallback ratios.
- Added a Zustand in-memory Home view store for independent Topic scroll positions.
- Kept normal search as route navigation. The AI icon only opens the modal Sheet;
  successful submission updates Topics, Filter, and Queue together.
- Added optimistic recommendation feedback with snapshot rollback and a unique
  idempotency key.
- Added `/entries/:entryId`. It uses the existing Folo Entry/Unread services and only
  removes a card from all Home query caches after read success. Mobile bottom
  navigation is hidden on the detail route.

## Verify

- Focused Vitest: 5 files, 14 tests passed, including the shell detail regression.
- Home Playwright: 3/3 passed at 390, 800, 1024, and 1440 widths; includes overlapping
  cursor pages, broken-image text fallback, completion, search/AI callback separation,
  filter activation, detail read, cache removal, and browser return.
- Shell and no-paid Playwright regression: 8/8 passed; denied Folo HAR requests remain 0.
- Focused ESLint: passed with no warnings or errors.
- `pnpm --filter @follow/web typecheck`: passed.
- `pnpm build:web`: passed and generated `/entries/:entryId` in the route table.
- `bash spec-package/scripts/validate-package.sh`: passed all package, contract, task,
  acceptance, schema, and SQLite checks.
- `/Users/mingrui/Project/Folo` and `apps/mobile/**`: no changes.
