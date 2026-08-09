#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
SERVICE_ROOT="${REPO_ROOT}/services/tantan-api"
DESKTOP_ROOT="${REPO_ROOT}/apps/desktop"
BUILD_ROOT="${DESKTOP_ROOT}/out/web"
TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/tantan-security.XXXXXX")"
CANARY_FILE="${TEMP_ROOT}/gemini.key"

cleanup() {
  rm -rf "${TEMP_ROOT}"
}
trap cleanup EXIT

fail() {
  printf 'security verification failed: %s\n' "$1" >&2
  exit 1
}

for command in git go pnpm rg; do
  command -v "${command}" >/dev/null 2>&1 || fail "missing command ${command}"
done

test -d "${BUILD_ROOT}" || fail "production web build is missing; run pnpm build:web first"
test -f "${BUILD_ROOT}/sw.js" || fail "production Service Worker is missing"

umask 077
CANARY="AQ.TANTAN_RELEASE_$(LC_ALL=C od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
printf '%s' "${CANARY}" >"${CANARY_FILE}"
chmod 600 "${CANARY_FILE}"

(
  cd "${SERVICE_ROOT}"
  TANTAN_SECURITY_CANARY_FILE="${CANARY_FILE}" go test \
    ./internal/ops ./internal/observability ./internal/http ./cmd/tantan-api \
    -run 'Readiness|Backup|Doctor|Rotating|HealthHandler|ForeignKeyViolations|ExplicitlyRestrictBrowserConnections|ReleaseSecretCanary' \
    -count=1
)

(
  cd "${DESKTOP_ROOT}"
  TANTAN_SECURITY_CANARY_FILE="${CANARY_FILE}" pnpm exec playwright test \
    -c e2e/playwright.config.ts \
    --project=web \
    --grep 'TASK-07 security|browser HAR contains zero denied Folo route|SEC-03 every browser API request'
)

if rg -q --fixed-strings --hidden --text \
  --glob '!.git/**' \
  --glob '!**/node_modules/**' \
  -- "${CANARY}" "${REPO_ROOT}"; then
  fail "release canary found in repository, fixture, build, log, HAR, SQLite or backup artifacts"
fi

if git -C "${REPO_ROOT}" grep -q --fixed-strings -- "${CANARY}"; then
  fail "release canary found in Git-tracked content"
fi

if rg -q --hidden \
  --glob '!**/*_test.go' \
  --glob '!**/*.test.ts' \
  --glob '!**/*.test.tsx' \
  --glob '!apps/desktop/e2e/**' \
  --glob '!apps/mobile/**' \
  --glob '!**/node_modules/**' \
  -- 'AQ\.[A-Za-z0-9_-]{30,}' \
  "${SERVICE_ROOT}" \
  "${DESKTOP_ROOT}/layer/renderer/src" \
  "${BUILD_ROOT}" \
  "${REPO_ROOT}/docs" \
  "${REPO_ROOT}/spec-package"; then
  fail "Gemini-style credential found outside an approved server secret source"
fi

printf '%s\n' 'security verification passed'
printf '%s\n' 'secret matches=0; forbidden Folo calls=0; direct browser egress=0'
