# TASK-05 verification

## Red evidence

- AI settings tests initially failed because the inherited implementation persisted browser-selected providers/models and expected the removed `ai_provider_configs` table.
- Provider tests initially failed because legacy OpenAI/Anthropic/Google/DeepSeek/OpenRouter adapters, arbitrary models and deprecated sampling fields were still accepted.
- Filter contract tests initially failed because successful and replayed mutations omitted `topicsRevision` and `queueGeneration`.
- Search component coverage initially lacked an executable assertion that a stale debounced request is aborted without changing Home Topic, Filter or scroll state.
- Mobile E2E exposed two integration gaps: `/settings/ai` was not registered and direct Entry detail did not load through the same-origin Folo proxy.
- The live translation harness initially accepted only a Secret file even though local Go-service configuration also permits OS Keychain; the Keychain-only acceptance path failed before any provider call.

## Implemented boundary

- The only AI preset is `google-gemini-openai`, the official OpenAI-compatible endpoint and `gemini-3.5-flash-lite`.
- The Go service loads the Key from a private absolute file or local OS Keychain. Settings are read-only; browser bodies containing Key, model or endpoint are rejected and the response exposes only an eight-character uppercase fingerprint.
- Gemini requests use `Authorization: Bearer`, an approved embedded JSON Schema and no `temperature`, `top_p` or `top_k`. Redirects and private-address dialing remain denied.
- Invalid/repair-failed AI output cannot write enrichment, Filter, Topic or Queue state. Filter success, replay and reset return the committed Topic revision and Queue generation.
- Search waits 250ms, passes the React Query abort signal, removes stale requests, de-duplicates cursor pages and leaves Home state untouched.
- Mobile `/settings/ai` and direct Entry detail use same-origin `/api`; Entry detail keeps original content readable while enrichment fails or retries.
- All Playwright mocks now target browser-visible same-origin `/api` paths instead of bypassing the Go boundary.

## Verify evidence

- `go test ./... -count=1`: PASS, including 100k sync/search capacity coverage.
- `go vet ./...`, focused Go race (`ai`, `enrichment`, `filter`, `cmd/tantan-api`) and Go build: PASS.
- Renderer Vitest/typecheck: 52 files, 152/152 PASS, zero type errors after the Entry fallback fix.
- Mobile Playwright: search/debounce/state preservation, AI Filter atomic apply/edit/reset, fixed server AI settings, search cursor fields and enrichment retry/original content: PASS at 390x844.
- Spec package validation and `git diff --check`: PASS.
- `TestLiveKeyLoaderFallsBackToServerKeychain`: PASS; the opt-in live test can use the local Go-service Keychain without exporting or printing the credential.
- Keychain-backed live translation and FilterSpec generation both pass against the fixed Gemini endpoint/model. The credential value is never exported or printed by the harness and remains absent from commands, SQLite, browser storage, logs, HAR, fixtures, Git and build output.
