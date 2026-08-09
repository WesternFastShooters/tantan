# TASK-BE-OPS evidence

Date: 2026-08-09

## Red

- `go test ./internal/ops ./internal/observability ./internal/http -run 'Readiness|Backup|Doctor|Rotating|HealthHandler' -count=1` initially failed because the ops and observability packages, `NewHealthHandler`, and readiness/backup/doctor implementations did not exist.
- `go test ./internal/http -run Diagnostics -count=1` initially failed with undefined `NewDiagnosticsHandler` and `DiagnosticsConfig`.
- `go test ./cmd/tantan-api -run 'BackupCommand|SQLiteSession|Application' -count=1` initially failed with undefined management commands, persistent session backend, and application composition root.
- The authenticated application contract test initially exposed an incorrect test table name (`ai_provider_settings`); the test was corrected to the approved `ai_provider_configs` contract before Green.

## Green, refactor, and verify

- Added `/healthz` and fail-closed `/readyz` checks for SQLite read/integrity, exact migration checksums, and a random temporary OS Keychain set/get/delete probe.
- Added a fixed-order, redacted `doctor` report for loopback port, data-directory permissions, SQLite read/write/integrity, migrations, Keychain, fixed Folo DNS, and verified TLS. Fault tests cover port, SQLite writer lock, Keychain, DNS and TLS without echoing injected canaries.
- Added consistent `VACUUM INTO` backups that never overwrite, integrity/schema/checksum/row-count inspection, atomic verified restore with a recovery hardlink, and idempotent daily backups retaining exactly seven files.
- Added 10 MiB/5-file JSON log rotation with `0600` files and symlink rejection, aggregate-only diagnostics, persistent hashed sessions, the local HTTP application composition root, and readiness-gated workers.
- Added authenticated integration coverage for Home, Topics and Search; the search request leaves the daily queue ID unchanged. Strict AI settings JSON rejects a caller-provided endpoint and never reflects or stores the API-key canary.
- Added `docs/runbooks/local-operations.md` covering local startup, doctor, backup/restore, data clearing, and public error codes.

Verification on 2026-08-09:

```text
go test ./...                                           PASS (all packages)
go test -race ./...                                     PASS (all packages)
test -z "$(gofmt -l .)"                                 PASS
go vet ./...                                            PASS
go build ./cmd/tantan-api                               PASS
bash spec-package/scripts/validate-package.sh           PASS (0 errors, 0 warnings)
go run ./cmd/tantan-api doctor --data-dir <temp-dir>    PASS (7/7 checks: port, data_directory, sqlite, migrations, keychain, dns, tls)
```

Acceptance linkage:

- `BE:TC-020`: strict AI endpoint rejection and canary absence in API/SQLite complement the cross-module secret tests.
- `BE:TC-021`: readiness faults return 503, and `TestWorkerDoesNotClaimJobsWhileReadinessFailsClosed` proves queued work remains unclaimed.
- `BE:TC-022`: existing targets remain byte-identical; verified backup/restore preserves Filter and Queue rows and rejects corrupt input before replacement.
- `BE:TC-023`: deterministic doctor fault injection and a real local doctor run both pass; output contains only fixed messages and recovery guidance.
