# TASK-BE-AUTH-PROXY evidence

Date: 2026-08-09

## Red

Valid expected-failure command, run before production handlers existed:

```text
go test ./internal/session ./internal/folo ./internal/auth ./internal/http
```

Exit: `1`.

The four target packages failed because they contained contract tests but no non-test Go files. The Red tests already named and exercised the required seams: local session hashing/expiry, auth callback state and token secrecy, strict Origin/Host checks, exact Folo policy decisions, response compatibility, and zero upstream calls for denied/removed routes. An earlier syntax-error attempt was discarded and is not counted as Red evidence.

## Green and refactor

- Implemented the loopback router, Folo login bridge, account-scoped Keychain token store, hashed local session store, fixed-host Folo auth client, exact policy matcher, and bounded proxy transport.
- Kept route policy, proxy transport, auth provider, session backend, local API mount, and health handler as separate seams.
- Added a session backend interface so the later SQLite task can persist `local_sessions` without changing this task's auth contract.
- Added a generic authenticated local handler mount so later Home/Search/AI tasks do not need to edit the security middleware.
- Added method+path-aware CORS preflight authorization; unknown routes and sensitive requested headers are rejected before dispatch.
- Production Folo URLs accept only the built-in hosts; loopback URLs are accepted only as server-side test configuration. Browser input cannot set an upstream URL.

Focused Green command:

```text
go test ./internal/session ./internal/folo ./internal/auth ./internal/http ./cmd/tantan-api
```

Exit: `0`.

## Required gate evidence

| Gate | Automated evidence | Result |
|---|---|---|
| Listener is fixed to `127.0.0.1:3000` | `TestListenAddressRejectsNonLoopback` plus `TestResolveServerURLAllowsOnlyOfficialOrLoopback` | PASS |
| Wrong flow and replay do not create sessions | `TestAuthCallbackRejectsWrongFlowBeforeUpstream`, `TestAuthCallbackStoresFoloTokenOnlyInSecretStore` | PASS |
| Token remains Keychain-only and logout deletes it | auth callback, rollback, unsafe-cookie and logout tests; local session stores only SHA-256 hash | PASS |
| Unknown and disabled routes fail before dial | deny/removal proxy tests and full-router policy-order test | PASS |
| All enabled SDK routes preserve status/body/content-type | `TestProxyPreservesEveryEnabledMethodPathAndPerformanceBudget` covers all 54 enabled method+path fixtures; observed P95 was 111.084µs | PASS |
| Host/Origin, DNS rebinding and header smuggling | router hostile Host/Origin test; hop-by-hop and CRLF proxy tests | PASS |
| Logs and browser response contain no canary token | auth/proxy log scans plus production-binary canary scan | PASS |
| Route policy equals the approved contract byte-for-byte | `TestEmbeddedRoutePolicyMatchesApprovedMachineContract` and `cmp` | PASS |
| Path matcher is robust | seed corpus and 3-second fuzz run | PASS |

The machine contract's public default-deny error is `FOLO_ROUTE_NOT_ALLOWED`; this is used instead of the task-manifest prose shorthand `FOLO_ROUTE_DENIED`.

## Verify commands

All exited `0`:

```text
bash spec-package/scripts/validate-package.sh
go vet ./internal/session/... ./internal/auth/... ./internal/folo/... ./internal/http/... ./cmd/tantan-api
go test ./...
go test -race ./...
go test ./internal/folo -run=^$ -fuzz=FuzzRoutePolicyNeverBypassesDecisionClasses -fuzztime=3s
test -z "$(gofmt -l cmd internal)"
cmp ../../spec-package/api/folo-route-policy.json internal/folo/route-policy.json
go build -o <temporary-directory>/tantan-api ./cmd/tantan-api
if rg -a 'CANARY|one-time-token-1234567890|folo-session-CANARY' <temporary-directory>/tantan-api; then exit 1; fi
```

The last scan returned no matches. The final fuzz run executed 101,183 inputs after loading its 102-input corpus, without failure. `go.mod` and `go.sum` were unchanged.
