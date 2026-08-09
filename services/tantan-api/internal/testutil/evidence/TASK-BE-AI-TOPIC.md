# TASK-BE-AI-TOPIC evidence

Date: 2026-08-09

## Red

The security, schema, fallback, versioning, repair, and concurrency tests were written before production files existed.

```text
go test ./internal/ai ./internal/topic ./internal/enrichment -count=1
```

Exit: `1`.

Each target reported `no non-test Go files`. The compiling test sources define the required public seams and fixtures for:

- API-key canary persistence only in the Keychain abstraction, no response/SQLite/error leakage, Keychain-first delete, and atomic failure behavior;
- five fixed Provider presets, no custom endpoint, private/link-local/loopback dial rejection, credential-header-only injection, bounded response errors, and no response-body reflection;
- byte-level strict AI Enrichment v1 and Topic Classification v1 validation, unknown/missing/range/uniqueness rejection, and exactly one repair attempt;
- Provider failure fallback that preserves original Entry content and schedules a bounded retry;
- core Topic seeding, compatibility normalization, atomic classification, unread counts, immutable Recommend, persistent optimistic versioning, and stale-version rejection;
- content/provider/prompt fingerprint invalidation and new dedupe identities;
- global Provider concurrency no greater than two.

## Green, refactor, and verify

The first implementation pass made the original Red suite green. Refactor then split the fixed Provider presets/adapters, prompt/schema versions, Keychain access, Topic service, and enrichment worker into separate packages without changing the approved database or API contracts.

Additional race-prone contract cases were added before their fixes:

```text
go test ./internal/ai ./internal/topic -count=1
```

Exit: `1`; `ai.TestConnection` and `topic.Service.EnsureDynamic` did not exist.

```text
go test ./internal/ai ./internal/enrichment -count=1
```

Exit: `1`; retry classification did not exist and a permanent Provider error incorrectly left the job queued.

```text
go test ./internal/enrichment -run 'TestEnsureDeduplicates|TestEnsureDoesNotReset|TestReadyEnrichmentWithoutTopics' -count=1
```

Exit: `1`; `fields` was incorrectly part of the dedupe identity, a repeated ensure could reset `processing` to `queued`, and a later Topic request was incorrectly treated as already ready.

Further strictness and stale-write Reds failed because malformed UTF-8 was accepted and a worker could commit after content/provider changed during the Provider call:

```text
go test ./internal/ai -run TestEnrichmentOutputStrictlyFollowsApprovedSchema -count=1
go test ./internal/enrichment -run 'TestWorkerCannotOverwriteNewContent|TestWorkerRevalidatesContentAndProviderBeforeCommit|TestGetNeverServesReadyDataForAChangedContentHash' -count=1
```

Each target exited `1` before the strict UTF-8 check, transactional revalidation, guarded stale transition, and current-content read constraint were implemented.

The Green implementation now provides:

- five exact preset endpoints with provider-specific credential headers, no redirects/proxy/custom URL, public-IP-only dialing, 2 MiB requests, 4 MiB responses, and a 60 second client timeout;
- a non-persisting Provider connection test plus transient-only retry classification;
- Keychain-first save/delete with compensating rollback and a database containing only preset/model/fingerprint;
- byte-strict, valid-UTF-8 AI Enrichment v1 and Topic Classification v1 validation with exactly one repair request;
- four-part enrichment dedupe identity (`entryId`, content hash, provider fingerprint, language), atomic field merging, bounded retry, global concurrency two, late Topic-request reconciliation, pre-commit content/provider revalidation, guarded stale transitions, and one transaction for enrichment/Topic/FTS/job completion;
- per-user idempotent core Topic seed, generated Topic merge, compatibility/case/whitespace normalization, seven-day dynamic stability, visibility rules, unread counts, persistent ordering/versioning, and caller-owned transaction support for the later atomic Filter flow.

Final task gates:

```text
go vet ./internal/ai/... ./internal/topic/... ./internal/enrichment/... ./internal/jobs/...
go test ./... -count=1
go test -race ./internal/ai/... ./internal/topic/... ./internal/enrichment/... -count=1
```

Exit: `0`. Target race results: AI `1.534s`, AI schema `2.315s`, Topic `1.953s`, Enrichment `3.450s`.

Security and regression audit:

```text
go test ./internal/folo -run TestDeniedAndRemovedRoutesNeverReachUpstream -count=1 -v
bash spec-package/scripts/validate-package.sh
```

Exit: `0`.

| Required gate                                               | Evidence                                                                                                 | Result |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- | ------ |
| Red key/schema/provider fallback evidence                   | Initial and supplemental failing commands above                                                          | PASS   |
| API key only through OS Keychain abstraction                | Keychain canary test plus SQLite/WAL byte scan                                                           | PASS   |
| No canary in SQLite, public response, error, or test output | `TestAISettingsPersistKeyOnlyInKeychainAndNeverReturnIt` and Provider error/body tests                   | PASS   |
| Invalid schema never commits                                | invalid-output worker test asserts failed state and zero derived rows after one repair                   | PASS   |
| Folo AI route requests zero                                 | removed-route proxy test asserts zero upstream calls; AI/Topic dependency graph contains no Folo package | PASS   |
| Required race command                                       | exact manifest race command above                                                                        | PASS   |

Forbidden-write audit: `apps/mobile/**`, `spec-package/**`, and `/Users/mingrui/Project/Folo` are unchanged.
