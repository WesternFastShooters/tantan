# Folo 付费与付费 AI 移除矩阵

## 1. 一期目标

最终前端不展示 Folo Plan、Power、Wallet、Upgrade CTA、付费 AI、AI Chat 或额度；运行时不请求 Folo AI/支付相关路由。翻译、摘要、Topic 分类和 AI Filter 全部改用本地 Go + 用户的 Key。

## 2. 对象分类

| 分类 | 已定位对象 | 处理 | 保护条件 |
|---|---|---|---|
| 付费/会员 UI | `src/modules/plan/**`、`src/modules/power/**`、`src/modules/wallet/**`、`src/queries/wallet.tsx`、`src/pages/**/power/**`、`src/pages/settings/(settings)/plan.tsx` | 删路由与入口，消费方归零后删模块 | 不得影响 RSS 订阅 |
| Folo AI 产品 | `src/modules/ai-chat/**`、`ai-chat-session/**`、`ai-onboarding/**`、`ai-task/**`、`src/modules/app-layout/ai/**`、`ai-enhanced-timeline/**`、`src/pages/**/(ai)/ai/**`、`src/pages/settings/(settings)/ai.tsx` | 移除产品入口和 Folo AI 消费方 | Tantan AI 页面使用新模块名和 `/tantan/v1/**` |
| Folo AI 调用 | `followApi.ai.*`、`followApi.aiTask.*`、`aiAnalytics`、旧 Translation/Summary Sync 消费方 | 先用 Tantan API 替代，再删老消费方 | 生产搜索和 HAR 均为 0 |
| Folo AI 设置同步 | `src/modules/settings/helper/sync-queue.ts` 中 remote tab `ai` 的读写/全量替换 | 移除 `ai` remote tab，仅保留 `appearance|general` | Folo `PATCH /settings/ai` 必须被本地代理拒绝 |
| Stripe 会员 | Better Auth Stripe subscription plugin、`subscription.upgrade` 及 Plan consumer | 从前端 Auth Client 和 UI 中移除 | Better Auth 核心 session/一次性 token 保留 |
| OTA/应用更新 | `src/modules/upgrade/**` 与 `MainDestopLayout` 的消费方 | 作为“一期无 OTA”单独禁用，不归类为付费 | 先确认没有其它本地能力依赖 |
| 外发初始化 | PostHog、Sentry、通知/推送初始化 | 一期移除或不初始化 | 只留本地脱敏诊断 |

## 3. 必须保留

| 对象 | 含义 | 禁止操作 |
|---|---|---|
| `packages/internal/store/src/modules/subscription/**` | RSS/Feed 订阅数据层 | 不得按名称删除、不得替换为付费概念 |
| Entry/Feed/Read/Collection stores | Folo 内容、已读、收藏数据层 | 不得因移除 AI 而删除 |
| Better Auth session + one-time-token | Folo 登录桥接 | 不得与 Stripe plugin 一并删除 |
| Source Detail/Add Subscription/OPML | 一期订阅基础能力 | 不得用“Discover 付费化”作为批量删除理由 |
| `apps/mobile/**` | Folo 原生工程，一期不使用 | 不修改、不删除 |

## 4. 静态与运行时门禁

生产消费方对以下模式必须零命中（测试中的拒绝断言可命中）：

```text
followApi.ai
followApi.aiTask
followApi.wallets
subscription.upgrade
/ai/
/wallets/
/payments/
/better-auth/subscription
```

Playwright 必须拦截全部请求，一旦目标为 Folo AI/Wallet/Payment/Stripe/Referral/Trending 路径立即使测试失败。同时断言 Plan、Power、Wallet、Upgrade CTA、AI Chat 路由不可达且导航中无入口。

## 5. 删除验收顺序

1. Red：基线上的禁路由、禁 UI、禁请求测试出现预期失败。
2. 替换：翻译/摘要/Filter/Topic 调用只走 Tantan API。
3. 断入口：删除路由、菜单、设置项、快捷键和懒加载。
4. 清消费方：全库搜索并逐个处理，不使用目录名 glob 批量删除。
5. 删源文件：只删除已无消费方的明确文件；检查 diff 中 RSS 订阅 Store 为零变更。
6. Verify：typecheck、route generation、Vitest、PC/Mobile E2E、HAR 拒绝断言和生产构建全部通过。
