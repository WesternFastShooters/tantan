# TASK-01 验证证据

日期：2026-08-10

## Red

- 在加入迁移 0004 后先运行 `go test ./migrations ./internal/storage`，旧快照仍要求 3 个 migration，测试按预期失败；失败原因是目标 v2 migration 尚未纳入运行时清单和快照。

## Green / Verify

- `bash spec-package/scripts/validate-package.sh`：exit 0；package validator 通过；Project Spec final 为 0 errors / 0 warnings；SQLite integrity/FK/FTS/invariant 通过。
- `node docs/contracts/generate.mjs --check`：exit 0；合同摘要 `1b3c4c80ce80c5a50504a609d97479c02fb8573551670a00dd875fbf0a756882` 可复现；Folo route policy 同时按字节生成到 Go embed。
- 在 `services/tantan-api` 运行 `go test ./migrations ./internal/storage`：exit 0；空库迁移 1～4 exactly-once、字节快照、integrity 和 foreign key 检查通过。
- `git diff --check`：exit 0。
- `git diff -- apps/mobile`：无输出；原生工程未修改。

## 锁定结果

- 仅 Mobile Web/PWA；PC/Electron/native 不在产品范围。
- Folo 登录方式为 Google、GitHub、Apple、Email、授权令牌。
- 浏览器仅调用同源 `/api`。
- Gemini provider、endpoint、model 固定；API Key 只由 Go 服务端 Secret 配置装载，浏览器请求合同无 Key 字段。
