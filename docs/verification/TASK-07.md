# TASK-07 Security, operations, capacity, and recovery evidence

Date: 2026-08-10

## Red

The task began with focused failing gates before production changes:

```text
go test ./internal/ops ./internal/http \
  -run 'ForeignKeyViolations|ExplicitlyRestrictBrowserConnections' -count=1

FAIL TestInspectDatabaseRejectsForeignKeyViolations
  database inspection accepted a foreign-key violation
FAIL TestSecurityHeadersExplicitlyRestrictBrowserConnectionsToSelf
  CSP did not contain connect-src 'self'

bash spec-package/scripts/verify-security.sh
exit 127: security verifier did not exist
```

The failures were limited to the missing TASK-07 behavior.

A real second start later exposed an upgrade/restart failure not covered by the original fixture:
the service had created the day's backup before migration 0004, then treated that older snapshot as
if it had to satisfy the new migration set. Startup failed closed with `SERVICE_NOT_READY`.
The filename also used the UTC date even though the runbook promised the local date. Focused Red
tests reproduced both failures.

## Green and refactor

- Database inspection now runs `PRAGMA foreign_key_check` in addition to checksum, integrity,
  migration, and audited row-count checks. Restore still validates the isolated copy before the
  atomic replacement.
- Automatic backups use the local calendar date and include a zero-padded schema version in the
  filename. An upgrade on the same day therefore preserves the pre-migration snapshot and creates
  a new verified snapshot without overwriting either file; retention still keeps exactly seven.
- The HTTP CSP now explicitly contains `connect-src 'self'`.
- The production Service Worker remains static-asset-only: navigation denies `/api`, image caching
  rejects `/api`, and no `NetworkFirst` or `StaleWhileRevalidate` authenticated-response strategy
  exists.
- `spec-package/scripts/verify-security.sh` creates a unique private-file canary, exercises Go and
  browser security tests, scans repository/Git/build/artifacts, and fails on any match. It never
  prints the canary.
- The Go canary test loads the value through the same private server file path and proves it is
  absent from public settings JSON, SQLite, logs, and a verified backup.
- The browser canary test proves it is absent from requests, response metadata, URL, cookies,
  local/session storage, Cache Storage, IndexedDB metadata, rendered UI, and HAR. It also records
  zero direct Folo or Gemini browser origins.

## Verify

```text
bash spec-package/scripts/validate-package.sh
PASS: 0 errors, 0 warnings

bash spec-package/scripts/verify-security.sh
4 Playwright security tests passed
security verification passed
secret matches=0; forbidden Folo calls=0; direct browser egress=0

pnpm --dir apps/desktop e2e:web
31 passed

pnpm build:web
PASS; production PWA and sw.js generated

go mod verify
PASS
test -z "$(gofmt -l .)"
PASS
go vet ./...
PASS
go test ./... -count=1
PASS
go test -race ./... -count=1
PASS
go build ./cmd/tantan-api
PASS
```

Capacity evidence:

```text
100k Home: P50 9.05ms, P95 9.18ms, cold 65.26ms, heap growth 0
100k Search: P95 101.29ms, DB 87,060,480 bytes, heap growth 0
100k Sync: 36.97s, recovered/reconciled, heap growth 69,468,160 bytes
```

Real local doctor evidence:

````text
ok=true
port=ok; data_directory=0700; sqlite=ok; migrations=ok;
keychain set/get/delete=ok; Folo DNS=ok; Folo TLS=ok

Real upgrade/restart evidence:

```text
legacy snapshot retained: tantan-2026-08-09.sqlite
current snapshot created: tantan-2026-08-10-v000004.sqlite
Go restart: PASS on 127.0.0.1:3000
HTTPS /api/healthz: 200
HTTPS /api/readyz: 200
wrong direct Host: 403
````

```

## Required-gate mapping

- Doctor/readiness fail closed and secret-free: PASS (fault matrix, canary tests, real doctor).
- Backup/isolated restore checksum, integrity, foreign keys, row counts: PASS.
- Service Worker caches no authenticated API response: PASS (source policy plus browser Cache
  Storage inspection).
- CSP self-only connection policy and zero browser direct Folo/AI: PASS.
- Browser/URL/HTTP/SQLite/log/HAR/fixture/Git/build/backup canary matches: 0.
- 100k capacity, `go vet`, all Go tests, and race: PASS.

The operator-rotated real Gemini credential/live translation call remains TASK-05 evidence and was
not replaced with the credential previously exposed in chat.
```
