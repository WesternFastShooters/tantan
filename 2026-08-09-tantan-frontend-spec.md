# 前端实施规格：Tantan 响应式 Web/PWA

> 状态：已批准，可进入实施  
> 类型：incremental，基于 Folo Web Renderer 修改  
> 规格负责人：Codex  
> 用户批准记录：已确认移除 Folo 付费/付费 AI；已确认每日推荐队列；2026-08-09 要求继续直至生成 Agent 可编码 spec package，一期执行边界锁定为 PC Web + Mobile Web/PWA

## 1. 证据与决策记录

### 1.1 输入资料

| 资料 | 位置 | 用途 | 已验证 |
|---|---|---|---|
| 产品 PRD | `/Users/mingrui/Project/tantan/prd(5).md` | 页面、交互、文案、范围 | 是，完整读取 |
| 前端原型 | `/Users/mingrui/Project/tantan/tantan前端原型.zip` | 视觉与交互参考 | 是；核心为 56KB 单文件 `src/app/App.tsx`，使用静态卡片和本地状态 |
| Folo 前端 | `/Users/mingrui/Project/Folo` | 增量改造基线 | 是，提交 `3846c90b67da351b6017cd4fe9d0992b8077224e` |
| 全栈总览 | `2026-08-09-tantan-实施落地方案.md` | 跨端边界、阶段与验收总线 | 是 |
| 后端规格 | `2026-08-09-tantan-backend-spec.md` | API Provider 合同 | 与本文共享 `API-*` ID |
| Agent 规格包 | `spec-package/README.md` | OpenAPI、Schema、DDL、任务与写入边界 | 是，机器合同优先于叙述性细节 |

### 1.2 代码与运行证据

| 事实 | 文件/符号/命令 | 观察结果 |
|---|---|---|
| Web 与 Electron 共用 Renderer | `apps/desktop/layer/renderer/src/main.tsx` | 单一 React Root，注入 `followApi`、Auth Client、React Query |
| 当前 Mobile Web 不进入应用 | `apps/desktop/layer/renderer/src/pages/(main)/layout.tsx` | `<768px` 选择 `DownloadPage` |
| 当前首页跳转传统 Timeline | `apps/desktop/layer/renderer/src/pages/(main)/index.sync.tsx#loader` | `/` 重定向到 `/timeline/...` |
| 当前请求直连 `VITE_API_URL` | `apps/desktop/layer/renderer/src/lib/api-client.ts#followClient` | `@follow-app/client-sdk` 直接请求配置的 API |
| 当前认证使用 Better Auth | `packages/internal/shared/src/auth.ts#Auth` | 包含 Stripe subscription 插件和 Folo 社交登录处理 |
| 当前本地状态可复用 | `packages/internal/store/src/modules/*` | Entry、Subscription、Read、Collection 已有 Store 与本地 DB 同步 |
| 搜索不覆盖翻译与 Topic | `apps/desktop/layer/renderer/src/store/search/index.ts#createLocalDbSearch` | Fuse 只索引 Entry/Feed/Subscription 原字段 |
| 已读与收藏行为可复用 | `packages/internal/store/src/modules/unread/store.ts`、`collection/store.ts` | 已有上游写入、回滚和本地失效逻辑 |
| PWA 基础已存在 | `apps/desktop/vite.config.ts#VitePWA` | 已包含 `vite-plugin-pwa@1.3.0` 与图标资产 |
| Masonry 依赖已存在 | `apps/desktop/layer/renderer/package.json` | `masonic@4.1.0` |
| 测试基础已存在 | `apps/desktop/layer/renderer/package.json`、`apps/desktop/e2e/playwright.config.ts` | Vitest、Playwright Web/Electron 项目已配置 |
| Folo Web 工具链 | `.nvmrc`、根 `package.json` | Node 22、pnpm 10.17.0、React 19.2.7、Vite 7.3.1 |

### 1.3 用户决策

| 决策 ID | 问题 | 用户确认结果 | 影响范围 |
|---|---|---|---|
| DEC-001 | 是否保留 Folo 付费 AI | 不保留；使用用户自己的 API Key | 设置、详情、AI Filter、请求隔离 |
| DEC-002 | 首页是否展示历史全部未读 | 否；采用最近 7 天每日有限队列 | 首页、完成态、后端排序 |
| DEC-003 | 交付平台 | PC Web 与 Mobile 端 | 响应式 Shell、测试矩阵 |
| DEC-004 | Mobile 是否为 Web/PWA | 是；一期为响应式 Mobile Web/PWA | 复用 Web Renderer；原生 App 需独立规格 |

## 2. 背景、目标与边界

### 2.1 用户与问题

目标用户使用 Folo 账号订阅大量信息源，但传统单列 Timeline 难以快速筛选重点内容；Folo 原 AI 功能绑定会员额度，不能满足用户自带 API Key 的使用方式。

### 2.2 目标结果与成功指标

- `FR-001`：同一 Web 应用在 PC 与 Mobile 浏览器提供完整功能，不再向 Mobile 展示下载页。
- `FR-002`：用户通过 Folo Google 账号登录并恢复订阅、已读、收藏和账号资料。
- `FR-003`：首页用小红书式 Masonry 分页展示稳定每日推荐队列，分页按 `entryId` 去重，已读后从全部首页 Topic Cache 退出。
- `FR-004`：Topic 使用稳定 `topicId`；自然语言 AI Filter 在首页 Sheet 内重构 Topic、权重和队列，筛选保持到用户编辑或重置。
- `FR-005`：普通搜索进入独立路由，不改变首页 Topic、Filter 或 Queue；搜索覆盖已读/未读、原文、译文、Source、Topic 与 Tag。
- `FR-006`：翻译、摘要和分类只调用本地 Go AI Provider，不调用 Folo AI。
- `FR-007`：最终 UI、路由、构建产物和运行时请求不存在 Folo 会员、支付、钱包和升级入口。
- `FR-008`：模型或上游不可用时，已有原文仍可阅读，已有缓存仍可浏览。

成功指标：390×844 和 1440×900 两个视口完成登录后首页→详情→已读退出、AI Filter、搜索、订阅、收藏、设置核心 E2E；运行时被禁止的 Folo 请求为 0。

### 2.3 范围

Login、Default Home、AI Filter Sheet、Filtered Home、Search、Article/Post Detail、Subscriptions、Favorites、Source Detail、Add Subscription、Topic Management、Settings、Empty/Loading/Error、PWA 安装与本地服务断开状态。

### 2.4 非目标

- iOS/Android 原生 App。
- 社区、评论、点赞、UGC、开放推荐池、广告、AI Chat 一级入口。
- 本期服务器部署、多用户共享服务、跨设备同步 Tantan 本地偏好。
- 重写 Folo 正文解析、订阅发现和 OPML 基础能力。

### 2.5 约束

- Folo 源码与 SDK 固定版本，不追随上游 `dev` 自动升级。
- 原型只提供视觉证据，不作为生产组件或状态架构。
- 浏览器不得存储 API Key、Folo 上游 Token 或 AI 完整 Prompt/Response 日志。
- 所有业务请求必须先到 `127.0.0.1:3000`。

## 3. 当前实现与增量影响

### 3.1 当前页面、组件与入口

当前 `/` 经 `index.sync.tsx#loader` 进入传统 Timeline；`MainDestopLayout` 渲染 Subscription Column、Entry Column 与 Entry Content；窄屏由 `withResponsiveComponent` 替换为 `DownloadPage`。设置、Discover、Power、AI、Plan 均由文件路由生成器从 `src/pages` 建立路由。

### 3.2 当前事件、状态和请求链路

```text
页面事件
→ packages/internal/store SyncService
→ @follow-app/client-sdk FollowClient
→ VITE_API_URL（当前为 Folo API）
→ API Morph
→ Zustand + 本地 DB
→ React 组件
```

目标链路把 `VITE_API_URL` 指向本地 Go；Folo SDK 响应结构不变。新增首页、Topic、Search 与 AI 设置使用 `/tantan/v1/**` 和 React Query，不写入 Folo Store 的内部不变量。

### 3.3 复用、修改、新增、删除与保持不变

| 类型 | 文件/符号 | 原因 | 影响 |
|---|---|---|---|
| 复用 | `packages/internal/store/src/modules/{entry,subscription,feed,unread,collection}` | Folo 数据与本地缓存 | 保持上游数据语义 |
| 复用 | `modules/entry-content` 正文 Renderer 与媒体组件 | Article/Post Detail | AI 区域替换数据源 |
| 复用 | `modules/discover` 的 Source Preview/订阅动作 | Add Subscription | 移除陌生推荐与 RSSHub 高级配置入口 |
| 修改 | `src/pages/(main)/layout.tsx` | Mobile Web 进入真实 Shell | 删除 DownloadPage 分支 |
| 修改 | `src/pages/(main)/index.sync.tsx` | `/` 成为首页 | 不再重定向 Timeline |
| 修改 | `src/lib/api-client.ts`、`src/lib/auth.ts` | 只连接本地 Go | Folo 数据仍走兼容代理 |
| 新增 | `src/modules/tantan-shell/**` | PC/Mobile 一级导航 | 单一 App Root |
| 新增 | `src/modules/tantan-home/**` | Masonry、Topic、AI Filter | 首页状态由 React Query + URL 拥有 |
| 新增 | `src/modules/tantan-search/**` | 统一搜索 | 替代产品入口中的 Fuse 搜索 |
| 新增 | `src/modules/tantan-settings/**` | AI Provider、频道、偏好 | Key 不进入客户端状态 |
| 删除 | `src/modules/{plan,power,wallet}`、`src/queries/wallet.tsx` 与关联页面 | 付费/钱包产品范围外 | 先断开消费方和路由，再按明确文件清单删除 |
| 删除 | `src/modules/{ai-chat,ai-chat-session,ai-task,ai-onboarding}` 与 AI 页面 | 禁止 Folo 付费 AI/Chat | 本地 AI 组件独立实现 |
| 删除 | `src/pages/.../power`、`src/pages/settings/(settings)/plan.tsx` | 不得访问 | 生成路由中不出现 |
| 删除 | Folo PostHog/Sentry/通知初始化 | 本期本地优先与隐私边界 | 使用本地诊断日志 |
| 禁用 | `src/modules/upgrade/**` 的 OTA/版本更新消费方 | 本期无发布与 OTA，它不属于付费 Upgrade | 与付费功能分开审计；消费方归零后再删除 |
| 保持不动 | `apps/mobile/**` | 一期 Mobile 由 Web/PWA 交付 | 未来原生端需独立规格，本期 Agent 禁止写入 |

### 3.4 冲突与兼容边界

- `subscription` 同时表示 RSS 订阅和 Better Auth Stripe 会员订阅。只删除 Stripe 插件与付费消费方，`packages/internal/store/src/modules/subscription` 必须保留。
- 不得根据 `subscription` 或 `upgrade` 目录名批量删除；每个删除项必须先证明路由、入口、消费方与运行时请求已为零。
- Folo `TranslationSyncService` 与 `SummarySyncService` 调用 `/ai/**`。所有消费者切换到 Tantan API 后才能删除旧实现。
- PC 沿用 Folo 信息密度并使用左侧一级导航；Mobile 使用 PRD 底栏。业务路由与数据源相同。
- `推荐`是虚拟 Topic，不进入 Topic 删除/隐藏合同。

## 4. 用户旅程与导航

| Journey ID | 用户/前置条件 | 入口 | 步骤与分支 | 退出/恢复 | 最终结果 |
|---|---|---|---|---|---|
| J-001 登录 | 未登录、Go ready | `/login` | 点 Google→`API-AUTH-START`→Folo→回调 | 失败回登录并显示稳定错误 | `/` 恢复账号数据 |
| J-002 首页消费 | 已登录、有订阅 | `/` | 加载队列→切 Topic→开详情→标已读→返回 | 保留 Topic/滚动；失败显示缓存 | 已读卡从所有首页视图退出 |
| J-003 AI Filter | 已登录、AI 配置有效 | 首页 ✨ | 输入 1..300→生成→Topic/卡片变化 | 失败保留旧首页；编辑/重置 | Active Filter 持久化 |
| J-004 搜索 | 已登录 | `/search?q=` | 输入→等待 250ms→结果分页→开详情 | 返回恢复 q/滚动 | 已读与历史结果均可见 |
| J-005 订阅管理 | 已登录 | `/subscriptions` | 媒体类型→分组→Source→详情/订阅 | 返回恢复分组展开状态 | 查看/添加/取消订阅 |
| J-006 收藏 | 已登录 | `/favorites` | 浏览→详情→取消收藏 | 与 read 状态无关 | 收藏集合正确更新 |
| J-007 AI 设置 | 已登录 | `/settings/ai` | 填 Provider→测试→保存 | Key 错误原位提示；可删除 | Go Keychain 保存成功 |
| J-008 频道管理 | 已登录 | `/settings/topics` | 排序/固定/隐藏/恢复 | 离开后持久化 | 首页 Topic 顺序同步 |

PC 路由用左侧一级导航；Mobile 用底栏导航。详情路由不显示底栏，返回后恢复上一页状态。浏览器 Back/Forward 必须可用。

## 5. 交互与状态设计

### 5.1 操作合同

| Action ID | 可用条件 | 触发 | 即时反馈 | Pending | 成功 | 失败/恢复 | 撤销/取消 |
|---|---|---|---|---|---|---|---|
| ACT-LOGIN | 未登录 | Google 按钮 | 按钮禁用 | 跳转中 | 回首页 | 显示 `AUTH_*` 文案 | 浏览器返回 |
| ACT-TOPIC | 首页 populated | 点击/键盘选择 | underline 移动 | 新数据 Skeleton 只覆盖内容区 | 新 Topic 卡片 | 保留旧卡并显示重试 | 再选原 Topic |
| ACT-READ | Detail 打开且设置开启 | 路由完成后触发 | 不阻塞正文 | 单次写入，去重 entryId | 返回时卡片移除 | 保留卡片并显示同步失败 | 设置中可关闭自动已读 |
| ACT-FILTER | Prompt 有效 | 生成 | CTA 禁用、Spinner | 60s 超时 | 状态栏与 Topic 更新 | 旧首页不变、保留输入 | Sheet 取消/重置 |
| ACT-FEEDBACK | 卡片 `...` 或长按 | 选择动作 | 菜单关闭、卡片动画 | 乐观隐藏 | 队列更新 | 回滚卡片并 Toast | `不感兴趣` 5 秒撤销；屏蔽 Source 只能在设置恢复 |
| ACT-SEARCH | q 长度≥1 | 输入 250ms | 清除旧错误 | 请求可取消 | 分页结果 | 保留 q、显示重试 | 清空 q |
| ACT-SEARCH-OPEN | 首页可交互 | 点搜索图标 | 保存 topicId/scrollY 到 history.state | 路由跳转 | `/search`，首页状态不变 | 留在首页并 Toast | 浏览器返回恢复首页 |
| ACT-AI-FILTER-OPEN | 首页可交互 | 点 AI 图标 | Sheet 打开 | 无 | 焦点进入 Prompt | 无 | Esc/下滑/关闭按钮恢复图标焦点 |
| ACT-AI-SAVE | 字段通过校验 | 测试并保存 | 表单禁用 | 60s | 显示“连接成功” | Key 不清空，错误关联字段 | 取消恢复已保存非密钥字段 |

### 5.2 UI 状态清单

| State ID | 进入条件 | 展示/文案 | 可用操作 | 离开事件 | 无障碍反馈 |
|---|---|---|---|---|---|
| UI-LOADING | 首次加载 | 卡片 Skeleton | 导航 | 成功/失败 | `aria-busy=true` |
| UI-POPULATED | 有队列项 | Masonry | 全部操作 | 切 Topic/读完 | 无额外播报 |
| UI-DONE-DAY | 推荐队列为空 | `今天值得看的内容已经看完 ✓` | 查看最近已读 | 新内容/明日 | polite live region |
| UI-DONE-TOPIC | Topic 队列为空 | `今天的 {Topic} 内容已经看完 ✓` | 查看最近已读 | 新内容/切 Topic | polite live region |
| UI-NO-SUB | 无订阅 | PRD 无订阅文案 | 添加订阅/导入 OPML | 建立订阅 | 标题聚焦 |
| UI-STALE | 同步失败且有缓存 | `暂时无法同步新内容，正在展示已有内容` | 重试 | 同步成功 | status role |
| UI-OFFLINE-GO | Go 不可达 | `本地服务未启动` | 查看诊断/重试 | health 恢复 | alert role |
| UI-AI-PENDING | enrichment 未完成 | `AI 处理中…`，原文正常显示 | 原文/离开 | 成功/失败 | 不抢焦点 |
| UI-AI-FAILED | AI 失败 | `AI 处理失败，已显示原文` | 重试 | 成功 | status role |
| UI-UNAUTH | Session 失效 | 登录页 | 登录 | 成功 | 标题聚焦 |

### 5.3 状态机

首页：`idle → loading → populated | empty | stale | error`；Topic/Filter 变化取消旧请求并回到 `loading`，旧数据保留到新请求成功。Query Key 固定为 `['home',activeTopicId,activeFilterId]`；分页合并按 `entryId` 去重。`readSucceeded(entryId)` 从所有已缓存首页查询移除 Entry；`readFailed` 不移除。

AI Filter：`closed → editing → submitting → active | submitError`。`cancel` 从 editing 返回 active/closed，不修改服务端；`reset` 成功后进入 closed；重复 submit 被守卫拒绝。

## 6. 视觉与 UED 合同

### 6.1 布局与层级

- 页面背景 `#08090B`，内容卡 `#17181B`，浮层 `#1E1F23`。
- Mobile 页面左右 8px、列间 8px；PC 内容区左右 24px、列间 12px，最大宽度 1280px。
- Header 56px；Topic 40px；Mobile Bottom Nav 含 safe-area 后最小 56px。
- 卡片圆角 12px，图片按媒体原始比例，比例缺失时 Article 4:3、Video 16:9、Image 1:1。
- Title 最多 3 行、摘要最多 2 行；完整内容只在 Detail 展示。

### 6.2 设计 Token

| Token/样式 | 复用或新增 | 精确值/来源 | 使用位置 |
|---|---|---|---|
| `--tan-bg` | 新增 | `#08090B`，原型 | 页面 |
| `--tan-surface` | 新增 | `#17181B` | 卡片 |
| `--tan-surface-2` | 新增 | `#1E1F23` | Sheet/Menu |
| `--tan-accent` | 新增 | `#FF5A00` | Active/CTA/Unread |
| `--tan-text` | 新增 | `#F0F0F2` | 主文字 |
| `--tan-muted` | 新增 | `#9898A6` | 次文字 |
| 字体 | 复用 | `@fontsource/sn-pro@5.2.6` | 全局 |
| Motion | 复用 | `motion@12.42.2`；150–220ms | Sheet、卡片退出、Tab underline |

### 6.3 字体、图标、图片、动效与主题

仅交付暗色主题；设置中的“随系统”在本期保持暗色并标注产品只支持暗色，不出现无效开关。图标复用已获许可的组件；`icons/mgc` 在公开分发前必须替换。`prefers-reduced-motion` 下取消位移动效，只保留透明度变化。

### 6.4 产品文案

文案以 PRD 第 11、13、34 节为准；错误码不得直接显示。相对时间使用用户 locale；超过 7 天显示本地日期。

### 6.5 响应式、触控和平台差异

- `<768px`：Mobile Shell、双列、Bottom Sheet、底栏。
- `768–1023px`：PC Shell、双列、左侧导航 72px collapsed。
- `1024–1439px`：三列、左侧导航 240px。
- `>=1440px`：四列、左侧导航 240px。
- 触控目标最小 44×44px；长按 500ms 打开菜单，移动超过 10px 取消长按。
- Hover 只增强视觉，不承载唯一操作；卡片始终有 `...` 按钮。
- 支持 Chrome/Edge 最新两个稳定版本、Safari 17+；Firefox 完成功能回归但不承诺 PWA 安装。

## 7. 页面与组件架构

### 7.1 路由和页面树

```text
/
├── login
├── home (index)
├── search?q=
├── subscriptions?view=articles|social|pictures|videos
├── favorites
├── sources/:sourceId
├── entries/:entryId
└── settings
    ├── ai
    ├── topics
    ├── reading
    ├── recommendation
    ├── subscriptions
    └── account
```

删除 `/ai`、`/power`、`/settings/plan` 及所有 Upgrade Modal 路由。

### 7.2 组件树

```text
TantanAppShell
├── DesktopPrimaryNav | MobileBottomNav
└── Outlet
    ├── HomePage
    │   ├── HomeHeader → SearchIconButton + AIFilterIconButton
    │   ├── TopicTabs
    │   ├── ActiveAIFilterBar
    │   ├── MasonryFeed → FeedCard
    │   └── AIFilterSheet
    ├── SearchPage → SearchResultList
    ├── SubscriptionsPage / FavoritesPage / SourceDetailPage
    ├── EntryDetailPage → LocalTranslationToggle + LocalSummaryCard
    └── SettingsPage → AIProviderForm + TopicManager
```

### 7.3 组件合同

| CMP ID | 职责 | 输入/输出 | 状态/API | A11y/响应式 | 文件 |
|---|---|---|---|---|---|
| CMP-SHELL | 一级导航与服务状态 | location；navigate | API-SESSION | landmark；PC/Mobile 变体 | `src/modules/tantan-shell/TantanAppShell.tsx` |
| CMP-HOME | 首页查询与恢复 | topicId/filter/cursor | API-HOME | main、busy/live | `src/modules/tantan-home/HomePage.tsx` |
| CMP-HOME-HEADER | 首页标题与两个搜索入口 | activeTopicId,activeFilterId,scrollY；navigate/openSheet | 无 | 两个44px按钮具备独立aria-label | `tantan-home/HomeHeader.tsx` |
| CMP-TOPICS | Topic 选择 | topics, activeId；onSelect | API-TOPICS | tablist/arrow keys | `tantan-home/TopicTabs.tsx` |
| CMP-MASONRY | 虚拟瀑布流 | items；onVisibleRange | 无本地写 | DOM/焦点顺序稳定 | `tantan-home/MasonryFeed.tsx` |
| CMP-CARD | 四种卡片与降级 | `HomeEntryCard`；open/menu | API-FEEDBACK | article/link/menu button | `tantan-home/FeedCard.tsx` |
| CMP-FILTER | AI Filter 表单 | activeFilter；submit/reset | API-FILTER-PUT/DELETE | dialog、focus trap/restore | `tantan-home/AIFilterSheet.tsx` |
| CMP-SEARCH | 统一搜索 | URL q/cursor | API-SEARCH | search landmark/result count | `tantan-search/SearchPage.tsx` |
| CMP-DETAIL | 正文、已读、AI | entryId | API-ENRICHMENT-*、API-FOLO-COMPAT | article heading order | `tantan-entry/EntryDetailPage.tsx` |
| CMP-AI-FORM | Provider 设置 | 非密钥配置；save/test/delete | API-AI-CONFIG-* | 字段错误关联 | `tantan-settings/AIProviderForm.tsx` |
| CMP-TOPIC-MGR | 排序/固定/隐藏 | topics；patch/reorder | API-TOPIC-PATCH | 键盘排序按钮 | `tantan-settings/TopicManager.tsx` |
| CMP-SERVICE | Go 断开提示 | health state；retry | `/healthz` | alert + diagnostic | `tantan-shell/LocalServiceGuard.tsx` |

### 7.4 复用和拆分理由

Tantan 模块不修改 Folo Store 的内部 Schema；它通过公开 getter/sync service 复用 Folo 数据，通过独立 Query Keys 消费 Go 业务接口。这样能锁住 Folo SDK 兼容层并删除原型的第二套应用根。

## 8. 状态、数据流与接口

### 8.1 状态所有权

| State ID | 类型 | 唯一数据源 | 读取方 | 写入方 | 更新/失效/重置 |
|---|---|---|---|---|---|
| ST-SESSION | Server | Go/Folo | Shell/Login | Auth flow | 401 清空全部用户 Query |
| ST-HOME | Server cache | Go Daily Queue | Home/Masonry | read/feedback/filter | Query invalidation + 精确移除 |
| ST-TOPICS | Server cache | Go SQLite | Tabs/Manager | Topic Manager/Filter | patch 成功失效 |
| ST-FILTER-DRAFT | Form local | Sheet | Sheet | 用户 | cancel 丢弃；success 以服务端为准 |
| ST-FILTER-ACTIVE | Server | Go SQLite | Home/Bar | Filter API | reset 删除 |
| ST-ENTRY | Folo Store | Folo + local DB | Card/Detail | Folo SyncService | 保持现有同步语义 |
| ST-ENRICHMENT | Server cache | Go SQLite | Card/Detail/Search | AI Job | 24h stale；provider 变更失效 |
| ST-SEARCH | URL + Server | `q`/Go FTS | Search | URL input | q 变化取消旧请求 |
| ST-NAV-RESTORE | Session UI | history.state | Shell/pages | 路由离开 | logout 清空 |
| ST-HOME-VIEW | Session UI | Zustand memory store | Header/Tabs/Home | Topic选择/Filter成功/scroll | 每Topic保存scrollY；刷新页回recommend顶部；logout清空 |

### 8.2 数据流

首页：`Go DailyQueue → API-HOME → React Query → MasonryFeed → FeedCard → 打开 Folo Entry → mark read → API-FOLO-COMPAT → Go 代理更新缓存 → 所有 API-HOME Query 移除 entryId`。

AI：`AIProviderForm → Go Keychain → Entry ensure → in-process Job → enrichment SQLite → API-ENRICHMENT-GET → Detail/Card`。前端永不接触已保存 Key。

首页四条回调锁定如下；生产实现放在 `tantan-home/useHomeController.ts`，页面组件不自行拼接请求：

```ts
const homeQueryKey = (topicId: string, filterId: string | null) =>
  ["home", topicId, filterId] as const

function handleTopicChange(nextTopicId: string) {
  const previous = homeViewStore.getState().activeTopicId
  homeViewStore.getState().saveScroll(previous, window.scrollY)
  homeViewStore.getState().setActiveTopic(nextTopicId)
  requestAnimationFrame(() =>
    window.scrollTo({ top: homeViewStore.getState().scrollY[nextTopicId] ?? 0 }),
  )
}

function handleSearchClick() {
  navigate("/search", {
    state: { returnTopicId: activeTopicId, returnScrollY: window.scrollY },
  })
}

function handleAIFilterClick() {
  setAIFilterSheetOpen(true)
}

async function handleGenerateAIFilter(prompt: string) {
  const result = await saveFilterMutation.mutateAsync({ prompt })
  homeViewStore.getState().activateFilter(result.filter.id, "recommend")
  setAIFilterSheetOpen(false)
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["topics"] }),
    queryClient.invalidateQueries({ queryKey: ["home"] }),
  ])
  window.scrollTo({ top: 0 })
}

async function handleResetAIFilter() {
  await api.deleteActiveFilter()
  homeViewStore.getState().activateFilter(null, "recommend")
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["topics"] }),
    queryClient.invalidateQueries({ queryKey: ["home"] }),
  ])
  window.scrollTo({ top: 0 })
}
```

Masonry 使用 `useInfiniteQuery`，`getNextPageParam` 只读 `nextCursor`。合并页时用 `Set<entryId>` 保留第一次出现的卡片；IntersectionObserver 只触发 `fetchNextPage`，不触发 AI、Topic 或 Queue 重建。`readSucceeded` 使用 `queryClient.setQueriesData({queryKey:["home"]}, updater)` 遍历全部页删除目标卡片。

`HomeResponse` 与 `HomeCard` 的公开 DTO 固定为：

```ts
interface HomeResponse {
  items: HomeCard[]
  nextCursor: string | null
  queue: { total: number; unread: number; finished: boolean }
}

interface HomeCard {
  entryId: string
  type: "article" | "post" | "image" | "video"
  title: string
  excerpt: string | null
  cover: string | null
  source: { id: string; name: string; avatar: string | null }
  publishedAt: string
  topics: Array<{ id: string; name: string }>
  translated: boolean
}
```

### 8.3 API 消费合同

| API ID | 方法/地址 | 请求 | 成功响应 | 前端映射 | 超时/重试/缓存 |
|---|---|---|---|---|---|
| API-AUTH-START | GET `/auth/folo/start` | navigation | 302 | 离开登录页 | 浏览器导航，不重试 |
| API-SESSION | GET `/tantan/v1/session` | 无 | `{user,timezone}` | ST-SESSION | 10s；GET 重试1；30s |
| API-HOME | GET `/tantan/v1/home` | topicId,filterId?,cursor,limit=20 | `{items,nextCursor,queue}` | UI 状态 | 10s；重试1；30s |
| API-TOPICS | GET `/tantan/v1/topics` | 无 | `{topics}` | Tabs/Manager | 10s；重试1；5min |
| API-TOPIC-PATCH | PATCH `/tantan/v1/topics` | operations[] | `{topics}` | 替换缓存 | 10s；不自动重试 |
| API-FILTER-PUT | PUT `/tantan/v1/filter` | `{prompt}` | `{filter,topics,queueId}` | 关闭 Sheet、切recommend、失效Home | 60s；不自动重试 |
| API-FILTER-DELETE | DELETE `/tantan/v1/filter` | 无 | `{topics,queueId}` | 切recommend、恢复默认 | 10s；不自动重试 |
| API-FEEDBACK | POST `/tantan/v1/recommendation/feedback` | entryId,action,topicId? | `{applied}` | 乐观更新/失败回滚 | 10s；Idempotency-Key |
| API-SEARCH | GET `/tantan/v1/search` | q,cursor,limit=20 | `{items,nextCursor,indexStatus}` | Results | 10s；重试1；按 q 缓存5min |
| API-ENRICHMENT-GET | GET `/tantan/v1/entries/:id/enrichment` | language=zh-CN | `{state,data,error}` | 原文/AI 状态 | 10s；30s pending、24h ready |
| API-ENRICHMENT-ENSURE | POST `/tantan/v1/entries/:id/enrichment` | fields[] | `202 {jobId}` | 开始轮询 | 10s；Idempotency-Key |
| API-AI-CONFIG-GET | GET `/tantan/v1/settings/ai-provider` | 无 | 非密钥字段+`hasApiKey` | 表单初值 | 10s；不持久缓存 |
| API-AI-CONFIG-PUT | PUT 同上 | providerId/model/apiKey?；endpoint 不可编辑 | 非密钥字段 | 保存成功 | 60s；不重试 |
| API-AI-CONFIG-TEST | POST `.../test` | 表单完整值 | `{ok,latencyMs}` | 测试状态 | 60s；不重试 |
| API-AI-CONFIG-DELETE | DELETE 同上 | 无 | 204 | 清空表单 | 10s；不重试 |
| API-SYNC-STATUS | GET `/tantan/v1/sync/status` | 无 | counts/state | 设置/搜索提示 | 10s；5s polling 仅 syncing |
| API-SYNC-TRIGGER | POST `/tantan/v1/sync` | scope | `202 {jobId}` | 进入 syncing | 10s；防重复 |
| API-FOLO-COMPAT | 原 Folo 路径 | SDK 原请求 | 原响应 | Folo Store | 保持现有策略；禁路由映射产品错误 |

所有 fetch 使用 `credentials:"include"`、`X-Request-Id` 和 `X-Tantan-Timezone`。组件卸载或参数变化时用 AbortSignal 取消。401 统一进入 Login；403/410 不重试；429 显示依赖限流；5xx GET 仅重试一次。

### 8.4 表单与校验

| Form/Field ID | 类型/默认值 | 约束 | 校验与错误 | 提交 |
|---|---|---|---|---|
| FILTER.prompt | string/active prompt | trim 后 1..300 | blur+submit；输入框下方 | 单次 PUT，Pending 禁用 |
| SEARCH.q | string/URL | trim 后 1..200 | 小于1显示空提示 | 250ms debounce |
| AI.providerId | 枚举 | OpenAI/Anthropic/Google/DeepSeek/OpenRouter；显示只读内置 endpoint | 字段下方 | Test/Save |
| AI.model | string | trim 后 1..100 | 字段下方 | Test/Save |
| AI.apiKey | password | 新建必填 8..4096；更新可空表示保留 | 不回显、不写浏览器存储 | Test/Save 后清空 |

## 9. 横切要求

### 9.1 无障碍

- 页面只含一个 `main`；Header、Nav、Search、Article 使用语义 landmark。
- Topic 使用 tablist/tab/tabpanel，支持左右键、Home、End。
- Sheet 打开后焦点进入标题后的输入框，关闭后恢复 ✨ 按钮。
- Card 标题链接可键盘访问；菜单支持 Enter/Space/Escape 和方向键。
- 错误与完成态使用 `role=status/alert`，不重复播报滚动加载。
- 文本与背景对比达到 WCAG 2.2 AA；200% zoom 不截断核心操作。

### 9.2 性能与资源预算

| 指标 | 目标 | 测量方法 | 失败阈值 |
|---|---|---|---|
| LCP | 本地缓存 P75 ≤2.0s | Playwright/Lighthouse，Fast 3G 关闭 | >2.5s |
| INP | P75 ≤200ms | Chrome Performance | >300ms |
| 首页 JS 增量 | gzip ≤120KB | Vite analyzer，对比锁定基线 | >150KB |
| Masonry | 500 条候选只渲染可视窗口；滚动≥50fps | Performance trace | 连续2s低于45fps |
| 图片 | 首屏外 lazy；最大解码宽度按容器×DPR | Network/Performance | 下载原图且宽度>需求2倍 |

### 9.3 安全与隐私

- 不使用 `dangerouslySetInnerHTML` 渲染未清洗 Feed；继续复用 Folo HTML 清洗链路。
- 外链加 `rel="noopener noreferrer"`；非 http/https URL 不可点击。
- API Key 输入设置 `autocomplete="off"`，提交后清空内存字符串。
- Query 持久化排除 AI Provider 表单、Filter Prompt 原文和错误响应体。
- 禁用 Folo Tracker、PostHog、Sentry、Web Push 和 OTA 初始化；本期无外发分析事件。

### 9.4 内容与国际化

一期 UI 默认简体中文，保留 i18next 架构；新增文案写入 `locales/zh-CN/tantan.json`，英文 fallback 写入 `locales/en/tantan.json`。不交付 RTL 专门布局，但 flex/grid 不使用硬编码 left/right 表达逻辑方向。

### 9.5 日志和诊断

前端错误只记录 `requestId`、route、errorCode 和组件边界，不记录正文、Prompt、Token 或 Key。设置页提供复制诊断信息，输出版本、Go health、索引状态和最近稳定错误码。

# 依赖清单

## 10. 依赖项与实施启动门禁

### 10.1 依赖

| DEP ID | 类别 | 名称/用途 | 状态 | 精确版本/来源 | 命令/影响文件 |
|---|---|---|---|---|---|
| DEP-FE-001 | Runtime | Node.js | 复用 | 22，`.nvmrc` | `nvm use` |
| DEP-FE-002 | Package Manager | pnpm | 复用 | 10.17.0，根 packageManager | `corepack prepare pnpm@10.17.0 --activate` |
| DEP-FE-003 | 上游代码 | Folo | 锁定 | commit `3846c90...` | 导入 Tantan 仓库 |
| DEP-FE-004 | SDK | `@follow-app/client-sdk` | 复用 | 0.3.95 | `pnpm install --frozen-lockfile` |
| DEP-FE-005 | UI Runtime | React/React DOM | 复用 | 19.2.7 | 原 lockfile |
| DEP-FE-006 | Masonry | masonic | 复用 | 4.1.0 | 原 renderer package |
| DEP-FE-007 | Server State | React Query | 复用 | 5.101.2 | 原 renderer package |
| DEP-FE-008 | Router | React Router | 复用 | 8.2.0 | 原 renderer package |
| DEP-FE-009 | PWA | vite-plugin-pwa | 复用/重配 | 1.3.0 | `apps/desktop/vite.config.ts` |
| DEP-FE-010 | Test | Vitest/Playwright | 复用 | 4.1.10 / 1.61.1 | renderer/desktop package |
| DEP-BE | Local API | 后端规格全部 `DEP-BE-*` | 新增 | 见后端规格 | 必须先通过 `/readyz` |

### 10.2 配置与启动

| 配置 | 值 | 启动/验证 | 成功结果 |
|---|---|---|---|
| `VITE_API_URL` | `http://127.0.0.1:3000` | `pnpm --dir apps/desktop dev:web` | 浏览器请求只到 loopback |
| `VITE_WEB_URL` | `http://127.0.0.1:5173` | 同上 | Auth Start 回到本地 |
| Go readiness | `/readyz` | `curl --fail .../readyz` | sqlite/keyring ok |

### 10.3 实施启动门禁

- [ ] Folo 基线按 `spec-package/agent/BASELINE_IMPORT.md` 导入；PRD/原型作为只读输入保留原路径并已提交。
- [ ] `pnpm install --frozen-lockfile` 成功。
- [ ] `pnpm --filter @follow/web test -- --run`、`pnpm typecheck` 基线结果已记录。
- [ ] Go `/healthz` 与 `/readyz` 通过。
- [ ] 一期端形态严格限定为 PC Web + Mobile Web/PWA，`apps/mobile/**` 保持不动。

## 11. 实现要求与执行顺序

### 11.1 可执行任务树

- [ ] TASK-FE-00 `[串行]`：锁定基线与依赖
  - 任务说明：完成仓库导入、现有行为特征测试和本地服务门禁。
  - [ ] TASK-FE-00.1 `[Agent:A]` `[DEP-FE-001..010/DEP-BE]`：建立仓库与验证基线
    - **实现什么**：生成可重复的前端安装、启动和测试基线，不改产品行为。
    - **怎么实现**：按总览导入 Folo；记录 commit、SDK、Node/pnpm；保存现有 typecheck/Vitest/Web E2E 结果。
    - **怎么测试**：运行依赖安装、`pnpm typecheck`、Renderer Vitest、Web core E2E。
    - **验收标准**：AC-001；所有命令结果可复现，失败项有与目标改动无关的基线记录。

- [ ] TASK-FE-01 `[串行]`：移除付费、Folo AI 与外发初始化（新增或变更行为）
  - 任务说明：按 Red→Green→Refactor→Verify 证明最终产品不存在被禁止功能与请求。
  - [ ] TASK-FE-01.1 `[Agent:T1]` `[FR-007/API-FOLO-COMPAT]`：Red—建立禁路由与禁 UI 测试
    - **实现什么**：测试在当前 Folo 基线因仍有 Plan/Power/AI/Upgrade 而失败。
    - **怎么实现**：新增 `src/modules/tantan-policy/removed-features.test.ts` 与 Playwright `tests/web/tantan-no-paid.spec.ts`；静态断言生成路由和关键进口，运行时拦截禁路径。
    - **怎么测试**：Vitest/Playwright 必须分别因现有付费路由和请求入口存在而失败。
    - **验收标准**：AC-002 为目标，本阶段仅取得有效 Red。
  - [ ] TASK-FE-01.2 `[Agent:I1]` `[FR-006/FR-007]`：Green—删除产品入口和消费方
    - **实现什么**：UI、路由、懒加载和初始化不再引用 Folo 付费与 AI 产品。
    - **怎么实现**：删除第 3.3 节声明目录；从 Settings/Shell/User/Entry 中移除消费方；`initializeApp` 不启动 Tracker、AI Chat Session、Web Push、OTA；移除 Better Auth Stripe client plugin；保留 RSS Subscription Store。
    - **怎么测试**：目标 Vitest 与 Playwright 转绿；TypeScript 无悬空进口。
    - **验收标准**：AC-002、AC-003。
  - [ ] TASK-FE-01.3 `[Agent:I1]` `[FR-007]`：Refactor—收敛移除策略
    - **实现什么**：把禁功能列表集中到 `tantan-policy/removed-features.ts`，行为不变。
    - **怎么实现**：只整理路由与能力常量，不恢复任一禁功能。
    - **怎么测试**：目标和受影响测试保持绿色。
    - **验收标准**：AC-002、AC-003。
  - [ ] TASK-FE-01.4 `[Agent:T1]` `[FR-007/API-FOLO-COMPAT]`：Verify—独立证明零入口零请求
    - **实现什么**：验证构建产物与核心旅程无付费/AI 上游调用。
    - **怎么实现**：运行静态扫描、bundle analyzer、Playwright network audit。
    - **怎么测试**：禁路径请求计数 0；禁页面返回 404；设置无 Plan/Upgrade。
    - **验收标准**：AC-002、AC-003。

- [ ] TASK-FE-02 `[串行]`：登录与响应式 Shell（新增或变更行为）
  - 任务说明：按 TDD 交付 PC/Mobile 统一路由、Folo 登录和导航恢复。
  - [ ] TASK-FE-02.1 `[Agent:T2]` `[FR-001/FR-002/CMP-SHELL/API-AUTH-START/API-SESSION]`：Red—编写 Shell/Auth 交互与 E2E
    - **实现什么**：锁定 `<768px` 不显示 DownloadPage、三入口和登录回跳。
    - **怎么实现**：组件测试覆盖断点；Playwright mock Go Auth/Session。
    - **怎么测试**：当前 Mobile 仍下载页导致预期失败。
    - **验收标准**：AC-004、AC-005 为目标。
  - [ ] TASK-FE-02.2 `[Agent:I2]` `[FR-001/FR-002/CMP-SHELL/API-AUTH-START/API-SESSION]`：Green—实现 TantanAppShell
    - **实现什么**：PC 左侧导航、Mobile 底栏、Login、服务断开 Guard 完成。
    - **怎么实现**：修改 layout/index loader；新增 `tantan-shell`；session 401 统一清缓存；导航状态写 history.state。
    - **怎么测试**：组件与两个视口 E2E 转绿。
    - **验收标准**：AC-004、AC-005、AC-006。
  - [ ] TASK-FE-02.3 `[Agent:I2]` `[FR-001/CMP-SHELL]`：Refactor—统一导航模型
    - **实现什么**：PC/Mobile 共享一份 `primaryRoutes`，只替换表现组件。
    - **怎么实现**：导航 label/icon/path/activeMatch 集中定义。
    - **怎么测试**：路由、焦点和回退测试保持绿色。
    - **验收标准**：AC-004、AC-006。
  - [ ] TASK-FE-02.4 `[Agent:T2]` `[FR-001/FR-002]`：Verify—跨视口登录导航验收
    - **实现什么**：独立运行真实 Go 测试环境核心导航。
    - **怎么实现**：390×844 与1440×900 完成 Login→Home→Subscriptions→Settings。
    - **怎么测试**：Playwright trace 无下载页、无丢失 history、无直接 Folo 请求。
    - **验收标准**：AC-004、AC-005、AC-006。

- [ ] TASK-FE-03 `[串行]`：首页、Topic、已读与反馈（新增或变更行为）
  - 任务说明：按 TDD 交付 Masonry、每日队列、完成态和推荐反馈闭环。
  - [ ] TASK-FE-03.1 `[Agent:T3]` `[FR-003/CMP-HOME/CMP-HOME-HEADER/CMP-TOPICS/CMP-MASONRY/CMP-CARD/API-HOME/API-TOPICS/API-FEEDBACK]`：Red—首页状态与交互测试
    - **实现什么**：锁定卡片降级、Topic 滚动恢复、重叠分页去重、已读退出、队列完成和反馈回滚。
    - **怎么实现**：建立四类卡片 fixture、重叠 cursor fixture、React Query mock、Masonry 可见区测试和 E2E。
    - **怎么测试**：目标模块不存在导致有效失败。
    - **验收标准**：AC-007..AC-011、AC-022 为目标。
  - [ ] TASK-FE-03.2 `[Agent:I3]` `[FR-003/CMP-HOME/CMP-HOME-HEADER/CMP-TOPICS/CMP-MASONRY/CMP-CARD/API-HOME/API-TOPICS/API-FEEDBACK]`：Green—实现首页
    - **实现什么**：响应式 Masonry、HomeHeader、Topic、状态、卡片、读后移除和菜单完成。
    - **怎么实现**：新增 `tantan-home/**`；复用 `masonic`；Query Key 含 topic/filter；分页按entryId去重；read success 精确更新所有 home query；feedback 使用 Idempotency-Key 和乐观回滚。
    - **怎么测试**：单元、组件和 E2E 转绿。
    - **验收标准**：AC-007..AC-011、AC-022。
  - [ ] TASK-FE-03.3 `[Agent:I3]` `[FR-003/CMP-MASONRY/CMP-CARD]`：Refactor—拆分卡片策略
    - **实现什么**：Article/Post/Image/Video 使用共享 Card Shell 和确定降级函数。
    - **怎么实现**：抽取 `resolveCardPresentation` 纯函数；不改变 DOM 顺序和可见结果。
    - **怎么测试**：fixture 快照、a11y 与 E2E 保持绿色。
    - **验收标准**：AC-007、AC-008。
  - [ ] TASK-FE-03.4 `[Agent:T3]` `[FR-003]`：Verify—性能与队列验收
    - **实现什么**：验证 500 条候选性能、稳定分页与完整消费。
    - **怎么实现**：Performance trace；连续读取含重叠边界的三页；逐条标记队列为已读直到空。
    - **怎么测试**：达到第 9.2 节预算，顺序不跳动、DOM无重复、完成态出现且历史内容可访问。
    - **验收标准**：AC-009、AC-010、AC-018、AC-022。

- [ ] TASK-FE-04 `[串行]`：搜索、订阅与详情（新增或变更行为）
  - 任务说明：按 TDD 连接 Go 搜索与 Folo 基础消费能力。
  - [ ] TASK-FE-04.1 `[Agent:T4]` `[FR-005/FR-008/CMP-HOME-HEADER/CMP-SEARCH/CMP-DETAIL/API-SEARCH/API-ENRICHMENT-*]`：Red—核心内容旅程测试
    - **实现什么**：锁定搜索图标独立路由、首页状态恢复、已读历史搜索、Source Detail、收藏和 AI 失败显示原文。
    - **怎么实现**：mock Folo Store/Go API；建立 search index building/ready 两组 fixture。
    - **怎么测试**：旧 Fuse 不含译文/Topic，目标断言失败。
    - **验收标准**：AC-006、AC-012..AC-015、AC-023 为目标。
  - [ ] TASK-FE-04.2 `[Agent:I4]` `[FR-005/FR-006/FR-008/CMP-HOME-HEADER/CMP-SEARCH/CMP-DETAIL/API-SEARCH/API-ENRICHMENT-*]`：Green—实现内容页适配
    - **实现什么**：搜索、订阅四类筛选、收藏、Source Detail、Add Subscription、详情 AI 区域完成。
    - **怎么实现**：HomeHeader 搜索按钮保存 returnState 后 navigate(`/search`)；产品搜索入口切换 API-SEARCH；复用 Subscription/Collection/Entry Renderer；新增本地 enrichment hooks；去除原 Folo AI 组件。
    - **怎么测试**：目标组件和 E2E 转绿。
    - **验收标准**：AC-006、AC-012..AC-015、AC-023。
  - [ ] TASK-FE-04.3 `[Agent:I4]` `[FR-005/FR-008]`：Refactor—统一 Entry 导航合同
    - **实现什么**：Home/Search/Favorites/Source 共用 `openEntry(entryId, returnState)`。
    - **怎么实现**：抽取导航 Hook，保持各列表来源状态。
    - **怎么测试**：四个入口返回恢复测试保持绿色。
    - **验收标准**：AC-006、AC-012、AC-014。
  - [ ] TASK-FE-04.4 `[Agent:T4]` `[FR-005/FR-006/FR-008]`：Verify—真实账号回归
    - **实现什么**：独立验证 Folo 数据恢复与 Go 搜索/enrichment。
    - **怎么实现**：测试账号执行添加订阅→同步→搜索→详情→收藏→已读。
    - **怎么测试**：状态重启后保持；模型断开仍读原文。
    - **验收标准**：AC-012..AC-015、AC-019。

- [ ] TASK-FE-05 `[串行]`：AI Filter、Provider 与频道管理（新增或变更行为）
  - 任务说明：按 TDD 交付用户自带 Key 与首页重构能力。
  - [ ] TASK-FE-05.1 `[Agent:T5]` `[FR-004/FR-006/CMP-HOME-HEADER/CMP-FILTER/CMP-AI-FORM/CMP-TOPIC-MGR/API-FILTER-*/API-AI-CONFIG-*/API-TOPIC-PATCH]`：Red—表单、安全与状态测试
    - **实现什么**：锁定 AI 图标只开 Sheet、Filter 生命周期、Key 不回显/不持久、频道排序与错误恢复。
    - **怎么实现**：组件测试、Query 持久化检查、Playwright storage/network audit。
    - **怎么测试**：目标组件不存在导致有效失败。
    - **验收标准**：AC-016、AC-017、AC-020、AC-023 为目标。
  - [ ] TASK-FE-05.2 `[Agent:I5]` `[FR-004/FR-006/CMP-HOME-HEADER/CMP-FILTER/CMP-AI-FORM/CMP-TOPIC-MGR/API-FILTER-*/API-AI-CONFIG-*/API-TOPIC-PATCH]`：Green—实现设置与筛选
    - **实现什么**：AI Provider 测试/保存/删除、AI图标回调、Filter编辑/重置、Topic固定/隐藏/排序完成。
    - **怎么实现**：HomeHeader AI按钮仅打开 AIFilterSheet；新增 settings/home 组件；API Key 只保留在表单内存直到请求完成；成功后清空。
    - **怎么测试**：目标测试转绿。
    - **验收标准**：AC-016、AC-017、AC-020、AC-023。
  - [ ] TASK-FE-05.3 `[Agent:I5]` `[FR-004/FR-006]`：Refactor—统一异步表单状态
    - **实现什么**：Filter 与 Provider 共用不含秘密持久化的 mutation 状态工具。
    - **怎么实现**：抽取错误映射和重复提交守卫，不共享字段数据。
    - **怎么测试**：安全与交互测试保持绿色。
    - **验收标准**：AC-016、AC-020。
  - [ ] TASK-FE-05.4 `[Agent:T5]` `[FR-004/FR-006]`：Verify—Key 与禁上游审计
    - **实现什么**：证明 Key 只到本地 Go，Filter 改变首页且 Folo AI 调用为0。
    - **怎么实现**：检查 browser storage、HAR、Go 测试日志脱敏输出。
    - **怎么测试**：Key 字符串全局搜索无命中；Home topics/items 变化；禁路径0。
    - **验收标准**：AC-003、AC-016、AC-017、AC-020。

- [ ] TASK-FE-06 `[串行]`：PWA、无障碍和最终验证
  - 任务说明：在所有行为通过后完成安装、故障降级、预算和全量回归。
  - [ ] TASK-FE-06.1 `[Agent:T6]` `[FR-001/FR-008/CMP-SERVICE/DEP-FE-009]`：Red—PWA/Go 断开/a11y 测试
    - **实现什么**：锁定本地服务断开、缓存壳、键盘与 reduced-motion 行为。
    - **怎么实现**：Playwright 离线/断服务场景与 axe 扫描。
    - **怎么测试**：缺失目标行为导致有效失败。
    - **验收标准**：AC-018、AC-019、AC-021 为目标。
  - [ ] TASK-FE-06.2 `[Agent:I6]` `[FR-001/FR-008/CMP-SERVICE/DEP-FE-009]`：Green—完成 PWA 和降级
    - **实现什么**：manifest、应用壳缓存、本地服务 Guard、故障文案和重试完成。
    - **怎么实现**：重配 VitePWA；API 不进 Cache Storage；已有图片缓存遵守原策略。
    - **怎么测试**：目标测试转绿。
    - **验收标准**：AC-018、AC-019、AC-021。
  - [ ] TASK-FE-06.3 `[Agent:I6]` `[FR-001/FR-008]`：Refactor—清理遗留 Folo 产品资源
    - **实现什么**：删除未引用文案、路由、资产和依赖，保留许可证文件。
    - **怎么实现**：depcheck、route snapshot、bundle analyzer；不修改已锁定行为。
    - **怎么测试**：全量测试与构建保持绿色。
    - **验收标准**：AC-002、AC-021。
  - [ ] TASK-FE-06.4 `[Agent:T6]` `[FR-001..008]`：Verify—最终独立验收
    - **实现什么**：执行全部前端验收总线。
    - **怎么实现**：运行 typecheck、lint、Vitest、两个视口 Playwright、a11y、构建、性能与人工文案检查。
    - **怎么测试**：第 12.4 节全部命令成功。
    - **验收标准**：AC-001..AC-023 全部通过。

### 11.2 测试驱动开发执行规则

每个行为严格执行 Red→Green→Refactor→Verify；Red 只能改测试与 fixture，失败原因必须是目标行为缺失。Green 只做最小生产修改。Verify 由未修改该行为生产代码的验证者执行。任务父节点仅在目标、受影响、基线和验收全部通过后勾选。

### 11.3 串并行与写入范围

本任务树默认串行，因为多个阶段同时修改路由、初始化和共享 Query Keys。合同锁定后，TASK-FE-03 与 TASK-FE-04 的完整 TDD 子树才允许并行；同一子树内部禁止并行。Test Agent 只写 `*.test.*`、Playwright 与 fixture；Implementation Agent 只写该叶声明的生产目录。

### 11.4 发布、兼容和回滚

本期不部署。每阶段以 Git commit 作为回滚点；SQLite/API Schema 兼容由后端控制。Folo 上游升级必须单独执行 SDK 合同回归，不得与产品功能提交混合。

## 12. 需求、验收与测试

### 12.1 功能需求追踪

| FR ID | 需求 | CMP/API | TASK | AC | TC |
|---|---|---|---|---|---|
| FR-001 | PC/Mobile Web/PWA | CMP-SHELL/CMP-SERVICE | FE-02,06 | AC-004、AC-006、AC-021 | TC-004、TC-006、TC-021 |
| FR-002 | Folo 登录恢复 | CMP-SHELL/API-AUTH-START/SESSION | FE-02 | AC-005 | TC-005 |
| FR-003 | 每日 Masonry/已读/反馈 | CMP-HOME/TOPICS/MASONRY/CARD | FE-03 | AC-007..AC-011、AC-022 | TC-007..TC-011、TC-022 |
| FR-004 | Topic 与 AI Filter | CMP-HOME-HEADER/FILTER/TOPIC-MGR | FE-05 | AC-016、AC-017、AC-023 | TC-016、TC-017、TC-023 |
| FR-005 | 独立统一搜索 | CMP-HOME-HEADER/SEARCH/API-SEARCH | FE-04 | AC-006、AC-012、AC-023 | TC-006、TC-012、TC-023 |
| FR-006 | 本地 AI | CMP-DETAIL/AI-FORM | FE-01,04,05 | AC-003、AC-013、AC-016 | TC-003、TC-013、TC-016 |
| FR-007 | 移除付费 | API-FOLO-COMPAT | FE-01,06 | AC-002、AC-003 | TC-002、TC-003 |
| FR-008 | 故障降级 | CMP-SERVICE/DETAIL | FE-04,06 | AC-019 | TC-019 |

### 12.2 验收标准

| AC ID | 精确可观察结果 |
|---|---|
| AC-001 | 锁定提交、依赖与基线命令结果已记录且可复现 |
| AC-002 | `/ai`、`/power`、`/settings/plan` 不存在，UI 无会员/升级/钱包 |
| AC-003 | Folo AI、Wallet、Payment、Stripe Subscription 请求计数为0 |
| AC-004 | 390px 进入应用并显示三项底栏，不显示 DownloadPage |
| AC-005 | Google 登录完成后显示同一 Folo 用户及其订阅/已读/收藏 |
| AC-006 | PC/Mobile 导航、普通搜索和详情返回恢复 route、Topic、Filter、scroll，搜索不改首页状态 |
| AC-007 | 首页按视口显示2/3/4列且四类卡片降级正确 |
| AC-008 | broken image 自动退化为文字卡，无破图图标 |
| AC-009 | Entry 标记已读成功后从全部 Home Topic 缓存消失 |
| AC-010 | 50条队列消费完显示今日完成态，历史仍可访问 |
| AC-011 | 反馈成功更新队列；失败回滚；屏蔽 Source 在设置可恢复 |
| AC-012 | 搜索命中已读、未读、原文、译文、Source、Topic、Tag，并支持游标分页 |
| AC-013 | AI pending/failed 时原文始终可读 |
| AC-014 | 订阅四类筛选、收藏、Source 历史与 Add Subscription 可用 |
| AC-015 | Detail 支持译文/原文、摘要、收藏、原文链接 |
| AC-016 | AI Key 不出现在 response、storage、URL、日志、Folo 请求 |
| AC-017 | Filter 生效后仍在首页，Topic/顺序/内容变化，编辑和重置可用 |
| AC-018 | 500候选达到性能预算，虚拟列表不一次渲染全部卡片 |
| AC-019 | Folo/模型/Go 部分失败按合同降级，有缓存时可阅读 |
| AC-020 | Topic 可固定、隐藏、排序；推荐不可删除 |
| AC-021 | PWA 可安装；API 不缓存；键盘、焦点、对比和 reduced-motion 通过 |
| AC-022 | 连续翻页时队列版本不变、卡片不跳位，分页边界相同entryId只渲染一次 |
| AC-023 | 搜索图标进入/search；AI图标只开Sheet；两个回调的路由与首页副作用完全隔离 |

### 12.3 测试用例

`TC-001..TC-023` 与同号 `AC-*` 一一对应。每个 TC 使用上表前置状态，执行 AC 描述的用户动作或网络故障注入，并断言唯一可观察结果。涉及安全的 TC-003/016 同时检查 HAR、localStorage、sessionStorage、IndexedDB、Cache Storage 和前端日志；涉及性能的 TC-018 使用固定 500 条 fixture；TC-022 注入重叠分页；TC-023 分别点击两个 Header 图标并记录 route、Sheet、Topic、Filter 与 Query Cache 变化。

### 12.4 验证命令

| 目的 | 工作目录 | 命令 | 成功观察 |
|---|---|---|---|
| 类型 | Tantan 根 | `pnpm typecheck` | exit 0 |
| Lint | Tantan 根 | `pnpm lint` | exit 0 |
| 前端单测 | Tantan 根 | `pnpm --filter @follow/web test -- --run` | 目标与基线全部 pass |
| Web E2E | `apps/desktop` | `pnpm e2e:web` | Tantan PC/Mobile specs 全 pass |
| 构建 | Tantan 根 | `pnpm build:web` | `apps/desktop/out/web` 生成且 exit 0 |
| 禁请求 | Tantan 根 | `rg -n 'followApi\.ai|followApi\.wallets|subscription\.upgrade' apps/desktop/layer/renderer/src` | 生产消费方无命中 |
| 规格最终门禁 | Tantan 根 | `python3 /Users/mingrui/.codex/skills/frontend-spec/scripts/validate_spec.py 2026-08-09-tantan-frontend-spec.md --domain frontend --stage final` | exit 0，0 warnings |

## 13. 覆盖矩阵

| 检查项 | 状态 | 证据 | 结论/不适用原因 |
|---|---|---|---|
| FE-01 | 已确定 | 第2节、FR-001..008 | 目标、范围、成功指标和禁区已锁定 |
| FE-02 | 已确定 | DEC-003/004、第14节 | 一期锁定 PC Web + Mobile Web/PWA，原生 App 不在范围 |
| FE-03 | 代码证实 | 第1.2、3节 | Folo 当前入口、Store、请求链路和移动下载页已定位 |
| FE-04 | 已确定 | 第4节 | 八条主旅程包含入口、分支、恢复和完成态 |
| FE-05 | 已确定 | 第5.1 | 首页、搜索、Filter、反馈和登录操作合同已定义 |
| FE-06 | 已确定 | 第5.2 | 加载、空、完成、离线和 AI 故障状态均有退出事件 |
| FE-07 | 已确定 | 第5.3 | Home 与 Filter 状态机含取消、失败和恢复 |
| FE-08 | 资料证实 | 第6节、PRD、原型 | Token、布局、图片、动效和文案来源已锁定 |
| FE-09 | 已确定 | 第6.5、DEC-004 | 响应式与触控合同覆盖一期 Mobile Web/PWA |
| FE-10 | 已确定 | 第9.1 | 键盘、焦点、语义和读屏反馈已定义 |
| FE-11 | 已确定 | 第6.4、9.4 | 文案、locale、日期和当前暗色主题已定义 |
| FE-12 | 已确定 | 第7.1/7.2 | 路由、五个首页组件及其它页面树已定义 |
| FE-13 | 已确定 | 第7.3 | 每个关键组件有职责、输入、API、A11y和落点 |
| FE-14 | 已确定 | 第8.1/8.2 | Session、Home、Topic、Filter、Search 和恢复状态所有权唯一 |
| FE-15 | 已确定 | 第8.3、后端第6节 | 共享 API ID、DTO、错误和缓存合同一致 |
| FE-16 | 已确定 | 第8.4 | Prompt、搜索和 Provider 字段校验已定义 |
| FE-17 | 已确定 | 第9.3 | Token、Key、缓存、CSP 和禁上游请求边界已定义 |
| FE-18 | 已确定 | 第9.2 | Masonry、API、图片、Bundle 和长任务预算可测量 |
| FE-19 | 已确定 | 第9.5 | 只保留本地脱敏诊断，本期无外发分析 |
| FE-20 | 已确定 | 第10节 | 依赖版本、配置命令和启动门禁已列出 |
| FE-21 | 已确定 | 第11节 | TDD任务树、写入范围、串并行和回滚已定义 |
| FE-22 | 已确定 | 第12节 | FR、AC、TC和验证命令形成追踪闭环 |

## 14. 未决问题

无。如果新需求改变公共接口、安全边界、持久化模型或一期端形态，必须先修订规格包并重跑最终校验。
