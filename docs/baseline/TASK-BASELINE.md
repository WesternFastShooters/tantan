# TASK-BASELINE evidence

Date: 2026-08-09 Asia/Shanghai

## Locked inputs

- Approved specification commit: `497d7b1` (`docs: lock approved tantan specification package`)
- Imported baseline commit: `3a34a49` (`chore: import locked Folo baseline 3846c90b`)
- Folo source repository: `/Users/mingrui/Project/Folo`
- Folo source HEAD before and after import: `3846c90b67da351b6017cd4fe9d0992b8077224e`
- Folo source status before and after import: clean
- Imported tracked files: 3,418
- Missing or mismatched Folo tree records after import: 0
- Existing Tantan path collisions: 0
- Approved input hashes: unchanged; `bash spec-package/scripts/validate-package.sh` passed after import

The import used `git archive` into a dedicated `mktemp -d` directory, rejected any existing file collision, and copied with `rsync --ignore-existing --checksum`. It did not modify the Folo source repository or overwrite any Tantan input.

## Runtime and dependency locks

- Node contract: `.nvmrc` = `22`
- Node used for all baseline frontend commands: `v22.23.2` via `fnm exec --using=22`
- Package manager: `pnpm@10.17.0`
- Installed `@follow-app/client-sdk`: `0.3.95`
- Registry `dist.shasum`: `e4d0de60206f4a66b3cb6c29053f51377a165143`
- Go: `go1.26.2 darwin/arm64`

## Baseline command evidence

### Install

Command:

```bash
fnm exec --using=22 pnpm install --frozen-lockfile
```

Result: exit 0. Lockfile was current; 25 workspace projects installed with pnpm 10.17.0. The existing dependency policy reported ignored install scripts for `@parcel/watcher`, `@swc/core`, `better-sqlite3`, and `workerd`; package postinstall completed successfully.

### Typecheck

Command:

```bash
fnm exec --using=22 pnpm typecheck
```

Result: exit 0; 20/20 Turbo tasks passed in 40.614s.

### Renderer Vitest

Command:

```bash
fnm exec --using=22 pnpm --filter @follow/web test -- --run
```

Result: exit 0; 37 test files and 109 tests passed; no type errors.

### Folo baseline Web E2E

Command:

```bash
fnm exec --using=22 pnpm --dir apps/desktop e2e:web
```

Result: exit 1; both existing Folo tests reached registration, then timed out after 90 seconds waiting for authenticated UI readiness.

- `e2e/tests/web/core.spec.ts`: `waitForAuthenticated` expected `true`, received `false`.
- `e2e/tests/web/settings-sync.spec.ts`: the same authenticated UI readiness timeout.
- Failure location: `apps/desktop/e2e/support/app.ts:193`.

This is recorded as an external-auth-dependent baseline failure. No product implementation existed and no failure was hidden or converted to a pass.

## Safety checks

- `git diff -- apps/mobile`: empty after the import commit.
- `/Users/mingrui/Project/Folo` remained clean and at the locked commit.
- PRD, prototype ZIP, three approved specifications, and `spec-package/**` hashes remained unchanged.
- Working tree was clean before adding this evidence file.

## TDD applicability

`TASK-BASELINE` introduces no product behavior, so a Red phase is not applicable. Its required evidence is the unmodified upstream baseline and the honest pass/fail command record above.
