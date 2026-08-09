# TASK-BE-STORAGE-SYNC evidence

Date: 2026-08-09

## Red

Valid expected-failure command, run before storage, sync, or search production files existed:

```text
go test ./internal/storage ./internal/sync ./internal/search
```

Exit: `1`.

Each target package reported `no non-test Go files`. The compiling test sources define the required behavior and public seams for approved SQLite migrations and permissions, hash-only persistent sessions, 100-item `publishedBefore` pages, idempotent full and five-minute-overlap incremental sync, committed checkpoint recovery, partial NDJSON content retry, seven-field FTS, signed query-bound cursors, and the fixed 100,000-entry performance fixture.

## Green, refactor, and verify

Additional behavior-specific Red runs were captured before their production seams existed:

```text
go test ./internal/sync -run 'TestHTTPSource' -count=1
```

Exit `1`: `NewHTTPSource` and `HTTPSourceConfig` undefined.

```text
go test ./internal/sync -run TestExplicitIncrementalSyncUsesLastSuccessCheckpoint -count=1
```

Exit `1`: explicit incremental request had a nil `publishedAfter` instead of the last-success-minus-five-minutes boundary.

```text
go test ./internal/jobs -run TestRetryValue -count=1
```

Exit `1`: shared `RetryValue` and `RetryPolicy` undefined.

```text
go test ./internal/search -run TestSearchIndexStatusReflectsBuildingAndCorruption -count=1
```

Exit `1`: a count-balanced missing/orphan index returned `ready` instead of `degraded`.

```text
go test ./internal/sync -run TestNDJSONPartialFailureCommitsSuccessesAndQueuesOnlyMissingIDs -count=1
```

Exit `1`: persistent content retry worker `RunOneContentJob` undefined.

Green/refactor results:

- Approved migrations are embedded byte-for-byte. SHA-256: `0001=3ba2b6f9...b15d4ef`, `0002=b0fc9c33...a6a9db5`, `0003=ba89ba40...b97b51`.
- SQLite uses WAL, foreign keys, 5s busy timeout, serialized writers, `0700` data directory, and `0600` database file. Persistent browser sessions contain only the SHA-256 local session ID.
- Full sync uses 100-entry `publishedBefore` pages and 50-ID content streams. Page metadata, content, read/collection state, FTS, checkpoint, and retry enqueue commit atomically.
- Incremental sync uses `last_success_sync_at - 5min`; full and overlapping incremental runs are idempotent.
- Restart after the second committed page resumes at the stored page boundary with no duplicate.
- Partial NDJSON preserves valid rows, counts invalid/unknown/duplicate rows, queues only missing IDs, and a leased content worker retries them at most three times.
- HTTP sync accepts only `https://api.folo.is` or loopback test servers; it calls only SDK 0.3.95 routes `GET /subscriptions`, `POST /entries`, and `POST /entries/stream`. Cookie tokens are validated, redirects are disabled, response bodies are bounded, and upstream bodies are never reflected in errors.
- FTS explicitly indexes original title/body, translated text, Source, Topic, and Tag. Search cursors are HMAC signed and bound to user/query.
- Shared jobs provide bounded exponential retry, leased at-least-once recovery, and terminal failure after the configured attempt count.

Capacity command:

```text
go test ./internal/sync ./internal/search -run 'TestLoad100K|TestHundredThousand' -count=1 -v
```

Exit `0`:

- 100,000-entry sync was killed after page 500, reopened from disk, resumed to exactly 100,000 entries/account states/FTS rows, and matched source ID digest `9b622c11bc1248bc9316912c88400197807947e534e428e32dec782ff9093a8d`.
- Sync duration `36.719s`; heap growth `65,404,928` bytes; integrity `ok`.
- Search P95 `102.560ms`; heap growth `4,096,000` bytes; database size `87,023,616` bytes. Budgets: P95 <=300ms, peak heap growth <=300MiB, DB <=5GiB.
- Capacity/performance assertions use `//go:build !race`, because race instrumentation invalidates latency budgets. All smaller recovery and concurrency tests remain in the race build.

Required race gate:

```text
go test -race ./internal/storage/... ./internal/sync/... ./internal/search/...
```

Exit `0`: storage `ok`, sync `ok` (`4.264s`), search `ok` (`1.679s`).

Full Go regression:

```text
go mod verify && go vet ./... && go test ./... && go build ./cmd/tantan-api
```

Exit `0`; all modules verified, all packages passed, and the local binary built successfully.

Final security/regression check:

```text
go test -race ./...
production-only secret/forbidden-route scan over internal/storage, sync, search, jobs
```

Exit `0`; every Go package passed under the race detector and the production scan reported zero API-key, bearer-token, paid-AI, Wallet, Stripe, Payment, Referral, or Trending references.

## Post-AI dependency audit

The task was reopened after AI enrichment existed. A cross-module Red proved that sync/search refresh still indexed derived data whose `content_hash` no longer matched the Entry:

```text
go test ./internal/search -run TestRefreshInvalidatesDerivedDataWhenContentHashChanges -count=1
```

Exit `1`: the changed Entry's enrichment remained `ready`, so old translation, Topic, and Tag terms could survive a sync content update.

Green now makes every search refresh atomically mark mismatched enrichments `stale`; FTS translation/Tag and Topic joins require the current Entry hash; search result translation and `translated` state also require that hash. Fresh original content remains immediately searchable.

Reopened task gates:

```text
go test ./internal/search ./internal/sync ./internal/storage ./internal/jobs -count=1
go vet ./internal/storage/... ./internal/sync/... ./internal/search/... ./internal/jobs/...
go test -race ./internal/storage/... ./internal/sync/... ./internal/search/... -count=1
```

Exit `0`; race results: storage `1.684s`, sync `4.290s`, search `2.334s`. The 100,000-entry sync/search regression also passed again in the non-race run.

A DTO projection follow-up extended the same Red to assert that a fresh original-content match has `translated=false` and no stale Topic objects. It initially failed with `TopicToken` still present on the card. The result Topic subquery now also requires `entry_topics.content_hash=entries.content_hash`; full search and search-race suites pass (`4.645s` / `1.764s`).
