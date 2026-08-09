# TASK-BE-HOME evidence

Date: 2026-08-09

## Red

The ranking and daily-queue tests were written before either Domain existed:

```text
go test ./internal/recommendation ./internal/home -count=1
```

Exit: `1`; both targets reported `no non-test Go files`. The compiling tests lock deterministic scoring/tie-breakers, first-20 diversity, two-card Source continuity, seven-calendar-day candidates, initial 50 and same-day 60 limits, stable persisted pagination, query-bound signed cursors, queue-version invalidation, explicit rebuild, and cross-midnight replacement.

Behavior-specific Reds were also captured before their production seams existed:

```text
go test ./internal/recommendation -run TestFilterSpecStrictlyMatchesApprovedSchemaAndCanonicalizes -count=1
```

Exit `1`: `ValidateFilterSpec` undefined.

```text
go test ./internal/recommendation -run TestFeedbackIsIdempotentUpdatesBlocksAndNeverReaddsCurrentQueue -count=1
```

Exit `1`: Feedback service/types and idempotency conflict did not exist.

```text
go test ./internal/filter -count=1
```

Exit `1`: target reported `no non-test Go files`. The test fixes exactly-one JSON repair, successful atomic Filter/Topic/Queue activation, invalid/provider-failure preservation of the old state, and idempotent reset to the default queue.

Filter request idempotency was then added as a behavior-first extension:

```text
go test ./internal/filter -run TestFilterRepairsOnceSwitchesAtomicallyAndResetRestoresDefault -count=1
```

Exit `1`: `Request.IdempotencyKey` and `ErrIdempotencyConflict` were undefined. Production was then implemented with a deterministic user+key identity, same-request replay, different-request rejection and one in-process mutation critical section.

The first same-day append test exposed a queue-boundary diversity defect:

```text
go test ./internal/home -run TestSameDayAppendHonorsExistingSourceTail -count=1
```

Exit `1`:

```text
same-day append created three consecutive sources: [feed_00 feed_00 feed_00 feed_01 feed_02]
```

Append ranking now includes the last two persisted Sources and never relaxes the consecutive-Source rule.

The calendar-boundary test also exposed RFC3339 lexical ordering admitting a future timestamp and failing to select the exact local-day boundary. Exact parsed-time checks and SQLite time comparison now accompany the indexed coarse range.

The 100k Home load Red measured `P50=63.597209ms`, above the approved 50ms budget. Queue-scoped Entry/account-entry row watermarks now avoid rescanning unchanged candidates while still detecting newly synced rows immediately.

## Green, refactor, and verify

Behavior coverage now proves:

- candidate inclusion is the latest 500 unread, unblocked Entries from the last seven local calendar days; exact lower boundary is included and future/older Entries are excluded;
- deterministic score reasons and diversity order match `testdata/ranking_golden.json`;
- initial Queue is 50, same-day append is capped at 60, cross-midnight creates a new Queue, and append observes the persisted Source tail;
- pages read persisted ranks, contain no overlap, bind cursor to user/topic/filter/timezone/queue/version, reject HMAC tampering and return `ErrQueueVersionChanged` after rebuild;
- Topic tabs are filtered views of the same Queue/version; read completion retains auditable Queue history;
- Filter output gets at most one repair; activation of Filter/Topic/ready Queue is one transaction; injected in-transaction failure, provider failure and invalid output preserve the prior active state;
- Filter replay and Feedback replay are idempotent under eight concurrent callers; conflicting key reuse is rejected; block immediately removes cards and undo never re-adds the current Queue.

Required target gate:

```text
go test -race ./internal/home/... ./internal/filter/... ./internal/recommendation/... -count=1
ok tantan.local/tantan-api/internal/home
ok tantan.local/tantan-api/internal/filter
ok tantan.local/tantan-api/internal/recommendation
```

Capacity gate (100k stored Entries, 500 eligible candidates):

```text
go test ./internal/home/... ./internal/search/... -run 'Load100K|HundredThousand' -count=1 -v
100k Home P50=9.19975ms P95=9.321709ms cold=67.90875ms
100k Search P95=98.72925ms
```

Full backend regression completed with `test -z "$(gofmt -l .)"`, `go vet ./...`, and `go test ./... -count=1`. The approved package validator passed, and the Folo deny-by-default/removed-route tests passed with zero denied upstream calls. `/Users/mingrui/Project/Folo` remains clean at locked HEAD `3846c90b67da351b6017cd4fe9d0992b8077224e`; `apps/mobile/**` remains unchanged.
