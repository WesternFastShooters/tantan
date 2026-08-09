# Tantan 一期 Mobile Web/PWA Spec Package v2

这是当前唯一可实施规格。2026-08-09 的 PC/loopback 方案已被用户在 2026-08-10 的 Mobile-only 决策取代。

## 读取顺序

1. `spec.yaml`
2. `00-project.md`
3. `10-frontend.md`
4. `20-backend.md`
5. `80-cross-domain.md`
6. `90-delivery.md`
7. `agent/EXECUTION.md`
8. `agent/task-manifest.json`
9. `agent/paid-feature-removal.md`
10. `api/openapi.json`、`api/folo-route-policy.json`、`schemas`、`db`

## 锁定范围

- 只交付部署后手机 Safari/Chrome/PWA 访问的 Mobile Web。
- 首页之外忠实采用 Folo Mobile 当前四 Tab 与移动交互；首页按原型实现两列瀑布流、Topic、搜索和 AI Filter。
- Go 提供同源 `/api`、Folo Google/GitHub/Apple/Email/授权令牌登录、精确代理、每日队列、搜索和服务端 Gemini 翻译摘要分类/Filter。
- 禁止 PC 专用 UI、Electron 交付、原生 App 修改、Folo 付费/会员/AI Chat、浏览器直连 Folo/Gemini。
- `/Users/mingrui/Project/Folo`、`apps/mobile/**`、PRD 和原型 ZIP 只读。

## 校验

```bash
bash spec-package/scripts/validate-package.sh
```

该入口同时验证标准 Project Spec、manifest/hash、OpenAPI/JSON Schema、DDL、route policy 和任务 DAG。任务顺序与写入范围以 `agent/task-manifest.json` 为准。
