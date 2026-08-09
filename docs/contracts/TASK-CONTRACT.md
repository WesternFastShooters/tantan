# TASK-CONTRACT evidence

Date: 2026-08-09 Asia/Shanghai

## Test basis and obligations

- Basis: `TASK-CONTRACT` required gates; OpenAPI 3.1, four approved JSON Schemas, and SQLite DDL 0001..0003.
- Contract seam: generated Go/TypeScript DTO files carry one aggregate source digest and required public DTO names.
- Schema seam: generated AI Schema files are byte-for-byte equal to the approved package files.
- Migration seam: generated SQL is byte-for-byte equal, applies to an empty modernc SQLite database, records versions 1..3 exactly once, and passes foreign-key and integrity checks.
- Negative assertion: no generated output may diverge from the approved inputs or write into `spec-package/**`.

## Red evidence

### Go contracts

Command:

```bash
cd services/tantan-api
go test ./internal/api/gen ./internal/ai/schema ./migrations
```

Result: `VALID_RED`, exit 1.

- `TestGeneratedGoTypesMatchApprovedContract`: `types.go` did not exist.
- `TestGeneratedAISchemasMatchApprovedSnapshots`: all four generated Schema files did not exist.
- `TestApprovedMigrationsApplyExactlyOnce`: `0001_core.sql` did not exist.

The failures reached the intended public artifact checks; the Go runner and SQLite dependency were working.

### Frontend contract

Command:

```bash
fnm exec --using=22 pnpm --filter @follow/web test -- --run src/lib/tantan-api/gen/contract-generation.test.ts
```

Result: `VALID_RED`, exit 1. `CONTRACT:Tantan generated frontend API` failed because `src/lib/tantan-api/gen/types.ts` did not exist; the other 37 files/109 baseline tests passed and typecheck reported no errors.

An earlier Go attempt stopped before the tests because the baseline module build list was incomplete. It was classified `INVALID`, fixed only in the `TASK-BASELINE`-owned `go.mod/go.sum`, and is not counted as Red evidence.

## Green evidence

Implementation:

- `docs/contracts/generate.mjs` reads the eight approved machine-contract inputs and writes one aggregate SHA-256 into both language outputs.
- Go and TypeScript DTOs expose the public OpenAPI response/request shapes and enums.
- Four AI Schema files and three migration files are copied byte-for-byte; the generator never writes `spec-package/**`.
- Go outputs are normalized by `gofmt`; TypeScript output uses the repository Prettier configuration.

Focused results:

```text
go test ./internal/api/gen ./internal/ai/schema ./migrations
exit 0; all three packages passed

fnm exec --using=22 pnpm --filter @follow/web test -- --run src/lib/tantan-api/gen/contract-generation.test.ts
exit 0; 38 files / 110 tests passed; no type errors
```

## Refactor evidence

Generation, source hashing, byte-copy behavior, Go formatting, and TypeScript formatting were centralized in one deterministic script. `--check` compares every output without writing. Focused tests remained green after formatting and import-order cleanup.

## Verify evidence

Commands and results:

```text
bash spec-package/scripts/validate-package.sh
exit 0; package, frontend spec, backend spec, SQL and invariant gates passed

fnm exec --using=22 node docs/contracts/generate.mjs --check
exit 0; verified 903356ee67d1365af86e53d2fe4e250415b75c5dc2c0767e6823e854a5a1016c

fnm exec --using=22 pnpm --filter @follow/web typecheck
exit 0

fnm exec --using=22 pnpm --filter @follow/web test -- --run src/lib/tantan-api/gen/contract-generation.test.ts
exit 0; 38 files / 110 tests passed

go vet ./internal/api/gen ./internal/ai/schema ./migrations
exit 0

go test -race ./internal/api/gen ./internal/ai/schema ./migrations
exit 0; all three packages passed

test -z "$(gofmt -l services/tantan-api/internal/api/gen services/tantan-api/internal/ai/schema services/tantan-api/migrations)"
exit 0
```

Migration verification applied 0001, 0002, and 0003 to an empty modernc SQLite database, skipped all three on the second pass, observed exactly three migration rows, no `foreign_key_check` rows, and `integrity_check=ok`.

## Test quality handoff

- Status: `VALID_GREEN` after the recorded `VALID_RED`.
- Layer/seam: build/static and contract tests through generated artifacts and an actual temporary SQLite database.
- Sensitivity: deleting or changing any generated DTO, Schema, migration, or aggregate digest makes the focused tests fail.
- External dependencies: none; tests use approved local files and a temporary local database.
