# TASK-03 verification

## Red evidence

- `pnpm --filter @follow/web exec vitest run src/modules/tantan-shell/TantanAppShell.test.tsx`
  - Old three-tab/desktop shell failed the exact four-tab assertions.
- `pnpm --filter @follow/web exec vitest run src/lib/tantan-api/client.test.ts`
  - Old absolute loopback/direct request client failed all three same-origin policy tests.
- `pnpm --filter @follow/web exec vitest run src/modules/tantan-shell/LoginRoute.test.tsx`
  - Old single “Folo account” button failed all five provider, Email/TOTP, token and returnTo tests.
- `pnpm --dir apps/desktop exec playwright test -c e2e/playwright.config.ts --project=web e2e/tests/web/tantan-shell.spec.ts`
  - Old web route lost returnTo and emitted browser requests outside `/api`.
- `pnpm --filter @follow/web exec vitest run src/modules/tantan-settings/AIProviderForm.test.tsx`
  - Old browser-owned provider form crashed on the fixed Gemini provider and exposed Key controls.
- `pnpm --filter @follow/web exec vitest run src/modules/tantan-shell/SettingsRoute.test.tsx`
  - Old settings page had no account or logout action.

## Implemented boundary

- Mobile Web always uses the centered four-tab Folo Mobile-style shell at 390, 430 and wide browser widths.
- Login supports the Go-advertised Folo provider order: Google, GitHub, Apple, Email/TOTP and one-time token.
- Browser business calls accept only relative `/api/*`; web startup no longer runs the legacy desktop whoami/settings sync chain.
- Session CSRF remains module-memory only. Password, TOTP and one-time token values are cleared and never persisted.
- AI settings are read-only server status. Provider, endpoint and model are fixed and no browser Key controls exist.
- `/api` is excluded from service-worker navigation and asset caching.

## Verify evidence

- Full renderer Vitest with typecheck: 50 files, 146/146 PASS, zero type errors.
- `pnpm --filter @follow/web typecheck`: PASS.
- `pnpm lint`: PASS (repository baseline warnings remain non-blocking).
- Mobile Playwright shell suite: 6/6 PASS at 390x844, 430x932 and 1440x900.
- `git status --short -- apps/mobile`: empty.
