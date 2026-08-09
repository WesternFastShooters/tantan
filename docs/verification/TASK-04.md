# TASK-04 verification

## Red evidence

- Go Home/Topic/API focused tests initially did not compile because `QueueState.Generation`, `Page.QueueGeneration` and `ListResponse.TopicsRevision` were absent.
- `home-model.test.ts` proved the inherited responsive 3/4-column behavior violated the fixed Mobile Web two-column contract.
- Home pagination model/store/query-key tests failed before generation-bound page params, per-scope generations and generation query keys existed.
- API application test accepted `limit=21` although the OpenAPI maximum is 20.
- Playwright `TC-33` reproduced a delayed page from a different generation; the first recovery implementation exposed that clearing only the active generation could reuse the old null-generation cache.
- Repeated cursor tamper coverage exposed non-canonical Base64 spellings that could decode to the same signed bytes.

## Implemented boundary

- New queues persist a stable `generation`, `topic_id` and `filter_id`; API queue metadata and top-level `queueGeneration` are identical.
- Version-2 HMAC cursors bind query hash, queue ID, queue version, generation and position; non-canonical Base64 is rejected.
- Topic mutations advance the account-owned monotonic `topicsRevision` in the same transaction.
- Home Query keys include topic, filter and generation. Every next-page request carries its expected generation; mismatches discard all cached generations for that scope and reload page one.
- Home remains exactly two columns at every browser width, de-duplicates by `entryId`, preserves Topic scroll positions and falls back to text when images fail.
- Public Home/Search `limit` is constrained to 1–20.

## Verify evidence

- Relevant Go packages: storage, sync, search, jobs, topic, home and recommendation PASS.
- Go race for the same packages PASS; concurrent first Home coverage uses 20 workers and one generation.
- 100k sync: 36.824s, 65,372,160-byte heap growth, recovery digest verified.
- 100k Search: P95 102.050ms, 0 heap growth, 87,060,480-byte database.
- 100k Home: cold 68.545ms, P50 9.497ms, P95 9.678ms, 0 heap growth.
- Full renderer Vitest/typecheck: 51 files, 150/150 PASS, zero type errors.
- Mobile Playwright Home gates: AC-03 long scroll and TC-33 delayed-generation recovery 2/2 PASS at 390x844; full Home generation/search/filter tests 3/3 PASS. Detail/read integration remains assigned to TASK-06.
- `pnpm lint`: no new TASK-04 errors; repository baseline warnings remain.
- `git status --short -- apps/mobile` and `/Users/mingrui/Project/Folo` status are empty.
