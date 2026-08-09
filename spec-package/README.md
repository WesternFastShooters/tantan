# Tantan 一期 Agent 可编码规格包

> 状态：已批准，仅适用于一期本地交付  
> 包版本：`1.0.0`  
> 基线：Folo `3846c90b67da351b6017cd4fe9d0992b8077224e` + `@follow-app/client-sdk@0.3.95`  
> 产品端：PC Web + 响应式 Mobile Web/PWA；不包含 iOS/Android 原生 App

## 1. 使用方式

Agent 在修改业务代码前必须按顺序完成：

1. 读取本文、`agent/EXECUTION.md`、`agent/task-manifest.json` 和自己领取任务引用的详细规格。
2. 运行 `bash spec-package/scripts/validate-package.sh`，只有全部通过才可开工。
3. 一次只领取一个 `TASK-*`，严格遵守 `allowedWrites` 与 `forbiddenWrites`。
4. 对新行为执行 Red→Green→Refactor→Verify；Red 必须留下预期失败证据。
5. 交付时运行该任务 `requiredGates` 及整包校验，并记录命令、exit code 和关键结果。

## 2. 权威顺序

上层资料冲突时按以下顺序执行：

1. 本包的 OpenAPI、JSON Schema、DDL、Folo 路由策略、任务清单。
2. `../2026-08-09-tantan-frontend-spec.md` 和 `../2026-08-09-tantan-backend-spec.md`。
3. `../2026-08-09-tantan-实施落地方案.md`。
4. `../prd(5).md` 与 `../tantan前端原型.zip`，仅作产品和视觉证据。
5. Folo 基线实现，仅作可复用代码证据。

公共 API、安全边界、数据库不变量或一期端形态出现冲突时，Agent 必须停止实现并修订规格，不得自行猜测。

## 3. 包内容

| 路径 | 用途 |
|---|---|
| `manifest.json` | 包版本、基线、决策、文件清单与校验入口 |
| `agent/EXECUTION.md` | Agent 执行、写入、删除、安全和变更协议 |
| `agent/BASELINE_IMPORT.md` | 从锁定 commit 安全导入 Folo 而不覆盖规格/用户文件 |
| `agent/task-manifest.json` | 可领取任务、依赖、写入边界与门禁 |
| `agent/paid-feature-removal.md` | Folo 付费/AI 移除矩阵与 RSS 订阅保护线 |
| `api/openapi.json` | `/tantan/v1/**` 公共 HTTP 合同 |
| `api/folo-route-policy.json` | Folo 上游的精确方法+路径白名单及拒绝规则 |
| `schemas/*.schema.json` | Home DTO 和 AI 结构化输出合同 |
| `db/*.sql` | SQLite 只追加迁移基线 |
| `examples/*.json` | 与 Schema 配套的无密钥固定样例 |
| `tests/acceptance-matrix.md` | 可执行验收用例、故障注入与成功观察 |
| `scripts/validate-package.sh` | 无网络的整包机器校验 |

## 4. 不可越过的边界

- 浏览器仅连接 loopback Go；不直连 Folo 或 AI Provider。
- API Key 仅存 OS Keychain；SQLite、浏览器存储、日志、fixture 和 Git 中都不得出现。
- Folo 代理默认拒绝，只允许 `api/folo-route-policy.json` 中启用的精确方法+路径。
- `/ai/**`、`/wallets/**`、Stripe/Plan/Power/Referral/Trending 相关 Folo 路由始终拒绝。
- `packages/internal/store/src/modules/subscription/**` 是 RSS 订阅，必须保留；不允许按 `subscription` 名称批量删除。
- `apps/mobile/**`、`/Users/mingrui/Project/Folo/**`、PRD 和原型都是只读输入。
- 一期不进行服务器部署、外部端口暴露、遥测上传、自动上游升级或 OTA。

## 5. 完成定义

一期只在以下全部成立时视为完成：

- `task-manifest.json` 中所有任务门禁通过，没有越界写入。
- 前后端 FR→AC→TC 均有自动或明确人工证据，关键安全项必须自动化。
- PC 1440×900 与 Mobile 390×844 核心 E2E 通过。
- Folo 被禁请求为 0，API Key 泄漏扫描为 0，Go 只监听 `127.0.0.1:3000`。
- 迁移可从空库一次应用，`PRAGMA integrity_check` 为 `ok`。
- 无法通过本包校验、typecheck、lint、Vitest、Playwright、`go test -race ./...` 或构建时，不得宣布完成。
