# TASK-02 验证证据：Go 边界、Folo 登录、密封会话与代理

日期：2026-08-10

## Red

- `go test ./internal/http`：exit 1；旧 Router 仍调用已删除的 redirect callback，并暴露旧 `/auth`、`/tantan/v1` 边界。
- 加入 migration 0004 后，`go test ./cmd/tantan-api`：exit 1；旧 readiness/backup 仍只认识 3 个 migration 和旧 AI 配置表。
- 这些失败发生在实现之前，分别锁定同源 `/api` 路由、v2 session/CSRF、密封 secret 和迁移兼容问题。

## Green / Refactor

- Router 只公开同源 `/api`：精确 public origin、可信反向代理 CIDR、Host/Origin、8KiB header 上限和 mutation CSRF 在 dispatch 前完成。
- 登录支持 Google、GitHub、Apple、Email、授权令牌；社交登录只返回官方 Folo 登录页，Email 支持 TOTP，pending cookie、密码、one-time token 和 Folo session 不写浏览器或日志。
- 本地 cookie 固定为 `__Host-tantan_session; Secure; HttpOnly; SameSite=Lax; Path=/`；SQLite 只保存 session/CSRF hash 和 secret ref，Folo session 使用 AES-256-GCM 密封。
- Folo 代理只挂载 `/api/folo`，按 v2 route policy 精确 method+path 默认拒绝；Folo AI、会员、wallet、payment、referral、trending 在任何网络请求前返回 410/403。
- 生产监听仍固定 `127.0.0.1:3000`；静态 PWA 目录启动时整包校验并载入内存，拒绝 symlink/traversal，支持 client-route fallback。
- 服务端 Secret 配置只接受私有绝对文件路径；master key 必须 32 bytes；Gemini provider/endpoint/model 为只读常量，Key store 不支持 HTTP 风格的 Set/Delete。

## Required Gates

| Gate                                         | 自动证据                                                                                                                | 结果 |
| -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ---- |
| public origin / trusted proxy / CSRF         | `TestRouterValidatesHostOriginAndTrustedProxyBeforeDispatch`、`TestRouterLocalAPIRequiresSessionOriginAndCSRF`          | PASS |
| Folo provider list / social handoff          | `TestProvidersAndSocialStartMatchFoloLoginMethods`                                                                      | PASS |
| token replay / opaque local session          | `TestTokenLoginNormalizesFoloURLCreatesOpaqueSessionAndRejectsReplay`                                                   | PASS |
| Email + TOTP                                 | `TestEmailTwoFactorFlowKeepsPendingCookieServerSide`、`TestAuthClientCompletesEmailAndTOTPWithoutExposingPendingCookie` | PASS |
| sealed Folo session / SQLite plaintext zero  | `TestEncryptedStoreRoundTripsWithoutSQLitePlaintext`、application/session backend tests                                 | PASS |
| deny before upstream                         | `TestRouterFoloPrefixDefaultDenyAndMutationCSRF`、`TestDeniedAndRemovedRoutesNeverReachUpstream`                        | PASS |
| fixed listener / static fallback / readiness | listen, SPA, migrated application and backup tests                                                                      | PASS |
| server-only Gemini Key source                | `TestServerSecretFilesRequireAbsolutePrivateRegularFiles`、`TestMasterKeyAndFixedGeminiServerStore`                     | PASS |

## Verify

- `go test ./cmd/tantan-api ./internal/http ./internal/folo ./internal/auth ./internal/session ./internal/secrets ./internal/ops`：exit 0。
- `go test -race ./internal/auth ./internal/session ./internal/secrets ./internal/folo ./internal/http`：exit 0。
- `go test ./internal/api/gen`：exit 0。
- `node docs/contracts/generate.mjs --check`：exit 0。

`go test ./...` 当前仅剩 TASK-05 范围的旧 per-user AI settings/enrichment/filter 测试失败：migration 0004 已按合同把 `ai_provider_configs` 迁移为 legacy 表，后续固定服务端 Gemini 实现负责替换；TASK-02 包和 cmd gates 均通过。
