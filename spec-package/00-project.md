# 项目规格：Tantan 一期 Mobile Web/PWA

## 1. 包信息与领域

| 项目       | 值                                                | 证据                                                                             |
| ---------- | ------------------------------------------------- | -------------------------------------------------------------------------------- |
| 生命周期   | incremental                                       | Folo 基线已在提交 `3a34a49` 导入，Tantan 已有一版 Go 与 Web 实现                 |
| 已选领域   | frontend、backend、cross-domain                   | 首页与移动 Web 消费 Go/Folo/AI 合同                                              |
| 规格负责人 | Codex Goal `019fe5a3-95e2-7d61-8cb9-03a4123979ac` | 用户要求持续执行至编码、测试和手机验收完成                                       |
| 当前状态   | approved                                          | 2026-08-10 用户明确否决 PC/Electron，要求建立长任务完成本规格所述 Mobile Web/PWA |

### 1.1 文档清单

| 文档                         | 领域或职责                                         | 是否必需 | 状态             |
| ---------------------------- | -------------------------------------------------- | -------- | ---------------- |
| `00-project.md`              | 项目边界、证据、架构                               | 是       | 已确定           |
| `10-frontend.md`             | Mobile Web/PWA、Folo Mobile 视觉和交互、原型首页   | 是       | 已确定           |
| `20-backend.md`              | 可部署 Go 同源 API、Folo 代理、队列与服务端 Gemini | 是       | 已确定           |
| `80-cross-domain.md`         | HTTP、身份、错误、版本和安全边界                   | 是       | 已确定           |
| `90-delivery.md`             | TDD 任务、验收、命令和覆盖                         | 是       | 已确定           |
| `api/openapi.json`           | 公共 HTTP 机器合同                                 | 是       | v2 修订后锁定    |
| `api/folo-route-policy.json` | Folo 上游精确白名单                                | 是       | 保留默认拒绝     |
| `schemas/*.schema.json`      | AI 输出和首页 DTO                                  | 是       | 保留并按 v2 校验 |
| `db/*.sql`                   | SQLite 迁移                                        | 是       | 保留向前迁移     |

### 1.2 领域边界与所有权

| Domain/Boundary ID | 范围                                        | 所有者                                          | 输入                                        | 输出                                      | 不包含                                             |
| ------------------ | ------------------------------------------- | ----------------------------------------------- | ------------------------------------------- | ----------------------------------------- | -------------------------------------------------- |
| DOM-FE             | 可安装的移动 Web UI、路由、缓存和 PWA       | `apps/desktop/layer/renderer` 的 WEB_BUILD 产物 | 同源 `/api`、Folo Mobile 只读基线、首页原型 | 手机浏览器 UI                             | Electron 主进程、PC 专用布局、原生 App             |
| DOM-BE             | Go 单体、SQLite、Folo/AI 出站和静态文件服务 | `services/tantan-api`                           | 浏览器同源请求、Folo API、AI Provider       | `/api`、PWA 静态资源                      | 公网 TLS 终止、原生推送                            |
| DOM-CONTRACT       | OpenAPI、JSON Schema、迁移、路由策略        | `spec-package`                                  | 已批准产品决策                              | 生成 DTO、合同测试                        | 临时实现偏好                                       |
| DOM-UPSTREAM       | Folo 官方服务                               | 外部 Folo                                       | Go 精确白名单请求                           | 账号、RSS、Feed/Entry、已读、收藏、Source | Folo AI、会员、Wallet、Payment、Referral、Trending |
| DOM-AI             | 项目所有者自有模型调用                      | Go AI 模块                                      | Go 进程 Secret、内置 Provider 预设          | 翻译、摘要、分类、Filter JSON             | 浏览器提交/保存密钥、自定义 endpoint、Folo 付费 AI |

## 2. 证据与决策

### 2.1 输入资料

| Evidence ID | 资料             | 位置                                                                 | 支持的事实                                                                     | 已验证                                 |
| ----------- | ---------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------ | -------------------------------------- |
| EV-01       | Folo Mobile 源码 | `/Users/mingrui/Project/Folo/apps/mobile` 与本仓库只读 `apps/mobile` | 当前底栏为首页、订阅、发现、设置四项；使用安全区、模糊底栏、栈式详情和分组设置 | 是                                     |
| EV-02       | 首页原型         | `tantan前端原型.zip`                                                 | 首页标题、顶部 Topic、搜索/AI 图标、两列瀑布流、AI Sheet 和筛选状态条          | 是                                     |
| EV-03       | PRD              | `prd(5).md`                                                          | 首页业务、订阅和设置意图                                                       | 是；非首页端形态被最新用户决定覆盖     |
| EV-04       | 旧实施规格       | `2026-08-09-tantan-实施落地方案.md` 等                               | 队列、AI Filter、搜索和 Go 代理的早期合同                                      | 是；PC/loopback/全删 AI 部分被 v2 覆盖 |
| EV-05       | 旧机器合同       | `spec-package/api`、`schemas`、`db`                                  | 已存在 DTO、错误、队列和数据模型                                               | 是；按 v2 做兼容修订                   |

### 2.2 代码、设计、Schema 与运行证据

| Evidence ID | 事实                                              | 文件/符号/Schema/设计节点/命令                                                                          | 观察结果                                                                             |
| ----------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| EV-06       | Folo Mobile 有四个主 Tab                          | `apps/mobile/src/main.tsx`                                                                              | `IndexTabScreen`、`SubscriptionsTabScreen`、`DiscoverTabScreen`、`SettingsTabScreen` |
| EV-07       | 旧 Tantan 壳偏离目标                              | `modules/tantan-shell/TantanAppShell.tsx`                                                               | 含桌面侧栏、三 Tab、`md` 断点和自定义黑色桌面容器                                    |
| EV-08       | 旧认证无法用于服务器手机访问                      | `services/tantan-api/internal/auth/bridge.go`                                                           | 回调强制 `http://127.0.0.1:3000/auth/folo/callback`                                  |
| EV-09       | Folo CLI 回调限制 loopback                        | `/Users/mingrui/Project/Folo/apps/ssr/client/modules/login/index.tsx`                                   | `cli_callback` 仅允许 localhost/127.0.0.1                                            |
| EV-10       | Folo 支持 Google、GitHub、Apple、Email 和授权令牌 | `/Users/mingrui/Project/Folo/apps/ssr/client/modules/login/index.tsx`、Mobile email、Desktop TokenModal | provider 列表、email signIn 和 one-time-token apply 均有代码                         |
| EV-11       | 当前前端会直连 Folo                               | 用户提供的浏览器错误与 `lib/api-client.ts`                                                              | `api.folo.is/better-auth/get-session` 被 CORS 拦截                                   |
| EV-12       | 队列和 AI 基础实现已存在                          | `services/tantan-api/internal/home`、`internal/ai`                                                      | 可复用，但监听、密钥和路由需适配部署                                                 |

### 2.3 用户决策

| Decision ID | 问题            | 用户确认结果                                                                                          | 影响的文档/合同                     |
| ----------- | --------------- | ----------------------------------------------------------------------------------------------------- | ----------------------------------- |
| DEC-01      | 交付端          | 只做部署后手机访问的 Mobile Web/PWA；不做 PC Web、Electron 和原生 App                                 | 全包                                |
| DEC-02      | 非首页 UI       | Folo Mobile 前端怎样，Tantan Mobile Web 就怎样                                                        | `10-frontend.md`                    |
| DEC-03      | 首页            | 只按原型替换为类小红书瀑布流、AI Topic、搜索和 AI Filter                                              | `10-frontend.md`、OpenAPI           |
| DEC-04      | 导航            | Folo Mobile 当前四 Tab 是权威，原型旧三 Tab 不覆盖“发现”                                              | `10-frontend.md`                    |
| DEC-05      | AI 和会员       | 用项目所有者自己的 Key；不显示或调用 Folo 充值、会员升级和付费 AI                                     | 全包、route policy                  |
| DEC-06      | 推荐队列        | 最近 7 天未读；初始最多 50；当天最多 60；消费完显示“今天已经看完”                                     | 前后端、Schema、DDL                 |
| DEC-07      | 搜索语义        | 普通搜索进入独立页，不改变 Home Topic、Filter 和 Queue                                                | 前后端                              |
| DEC-08      | AI 图标语义     | 只打开 Filter Sheet；提交成功才原子更新 Filter、Topics 和 Queue                                       | 前后端                              |
| DEC-09      | 执行授权        | 用户要求建立并持续执行长任务满足以上目标                                                              | `90-delivery.md`                    |
| DEC-10      | 登录能力        | Folo 怎么登录，Tantan 就怎么登录；不得只支持 Email                                                    | 前后端 Auth、OpenAPI、验收          |
| DEC-11      | Gemini Key 位置 | 固定 `gemini-3.5-flash-lite`；API Key 由 Go 服务端配置注入，浏览器不得提交、保存或读取                | 后端配置、OpenAPI、设置页、安全验收 |
| DEC-12      | Email 2FA       | Folo Email 登录若要求 TOTP，Tantan 必须在同一登录页完成；pending upstream cookie 只在 Go 内存短时保存 | Auth、OpenAPI、登录 E2E             |

## 3. 背景、目标与边界

### 3.1 用户、参与者与问题

主要用户在手机 Safari/Chrome 中访问自己部署的 Tantan。当前版本首屏是自造桌面壳，而且浏览器直接访问 Folo 导致 CORS；部署认证仍依赖访问设备自身的 loopback。实施者需要在不修改 Folo 源仓库和原生 `apps/mobile` 的前提下，得到可运行、可测试、可部署的移动 Web。

### 3.2 目标结果与成功指标

| Goal ID | 目标结果                           | 成功指标                                                               | Guardrail                                     | Owner |
| ------- | ---------------------------------- | ---------------------------------------------------------------------- | --------------------------------------------- | ----- |
| GOAL-01 | 手机端观感和导航忠实于 Folo Mobile | 390×844 与 430×932 核心截图/交互验收通过，四 Tab 和栈式详情一致        | 不以桌面侧栏降级                              | FE    |
| GOAL-02 | 首页成为稳定 AI 推荐瀑布流         | 两列、稳定分页、无重复/跳位、Topic/搜索/Filter 行为通过                | 分页不触发重排                                | FE+BE |
| GOAL-03 | 所有浏览器业务流量同源             | 网络测试中直连 Folo/AI 为 0，CORS 错误为 0                             | Go 默认拒绝未授权上游                         | BE    |
| GOAL-04 | 服务端自有 Key 取代 Folo 付费 AI   | 翻译、摘要、分类、Filter 使用 Go 的固定 Gemini 配置；付费入口/请求为 0 | 密钥不进入浏览器请求/存储、SQLite、日志和 Git | BE+FE |
| GOAL-05 | 可部署并可恢复                     | 生产构建、同源启动、迁移、备份恢复和 readiness 通过                    | TLS 由反向代理终止                            | BE    |

### 3.3 范围

- 响应式 Mobile Web/PWA，主要支持宽 360～430 CSS px 的竖屏手机。
- Folo Mobile 四 Tab、移动顶栏、底部安全区、列表/详情栈、订阅、发现、设置的 Web 等价实现。
- 原型首页、每日队列、Topic、普通搜索、AI Filter、翻译、摘要、已读、收藏、订阅管理。
- Go 同源 API、Folo Google/GitHub/Apple/Email/授权令牌登录桥、精确代理、会话、SQLite、任务、健康、备份恢复和生产静态文件服务。
- 本地开发与可部署服务器运行手册；真实手机在局域网或 HTTPS 测试域名验收。

### 3.4 非目标

- PC 专用布局、桌面侧栏、多列 PC 验收、Electron 安装包。
- 修改或发布 React Native `apps/mobile`、iOS/Android 原生 App。
- 修改 Folo 官方 OAuth 服务端或其 trusted callback 配置；Tantan 通过 Folo 官方登录页和一次性授权令牌完成社交登录，不伪造 OAuth client。
- 多租户运营后台、计费、团队、服务器自动购买、域名和公网部署执行。
- 用户自定义 AI Provider URL；只允许内置 HTTPS 预设。
- Folo AI Chat、Wallet、Power、Plan、Stripe Subscription、Referral、Trending。

### 3.5 约束、质量优先级与风险

优先级依次为：密钥和会话安全、数据正确性、手机交互一致性、可恢复性、性能。`/Users/mingrui/Project/Folo`、`apps/mobile/**` 和原型 ZIP 只读。不得用名称批量删除 `subscription`，因为 RSS 订阅数据层必须保留。用户曾在对话中粘贴 API Key；该值视为已暴露，不能写入任何命令、代码或证据，真实验收前必须轮换，并通过 root-readable Secret 文件或本机 Keychain 注入 Go。

## 4. 当前实现与增量影响

### 4.1 当前入口与端到端链路

当前 Web 由 `apps/desktop/layer/renderer` 在 `WEB_BUILD=1` 下构建，经 React Router 进入 `TantanAppShell`；业务使用 `@follow/store` 和 `FollowClient`。Go 监听 127.0.0.1:3000，已有 Home/AI/Search/Sync 模块，但认证桥固定 CLI loopback。目标链路为：手机 HTTPS → 反向代理 → Go（静态 PWA 与 `/api`）→ SQLite/Folo/AI。

### 4.2 复用、修改、新增、删除与保持不变

| 类型     | 领域 | 文件/符号/Schema/配置                                                 | 原因                                | 影响                       |
| -------- | ---- | --------------------------------------------------------------------- | ----------------------------------- | -------------------------- |
| 复用     | FE   | React 19、React Query、React Router、Tailwind/UIKit token、Folo store | 避免重造数据与基础组件              | Web 构建仍来自 renderer    |
| 复用     | BE   | home/search/sync/storage/recommendation/enrichment                    | 已有测试覆盖                        | 改外层路径、身份和密钥实现 |
| 修改     | FE   | `tantan-shell`、subscriptions/settings/detail/search/home             | 恢复 Folo Mobile 信息架构和手机交互 | 删除桌面侧栏和 PC 分支     |
| 新增     | FE   | Discover Tab、移动栈导航、移动登录、PWA 离线壳                        | 当前三 Tab 缺失                     | 对齐 Folo Mobile           |
| 修改     | BE   | auth/router/application/keyring                                       | 支持服务器同源和多会话              | 取代 CLI callback          |
| 修改     | 合同 | OpenAPI、manifest、task manifest、验收矩阵                            | 清除 PC/loopback 冲突               | v2 为唯一可实施合同        |
| 删除     | 产品 | Folo 会员/升级/支付入口和调用                                         | Go 服务端自有 Key                   | RSS subscription 保持不变  |
| 保持不变 | 外部 | `/Users/mingrui/Project/Folo`、`apps/mobile/**`、原型 ZIP             | 只读参考和用户资产                  | diff 必须为 0              |

### 4.3 冲突、兼容与技术债边界

旧规格的 PC 2/3/4 列规则改为手机固定 2 列；PC 仅显示居中调试画布，不作为产品或验收。旧的“删除全部 Folo AI UI”改为“删除 Folo 付费调用和门槛，保留有价值的摘要/翻译入口并改接 Go”。旧 `/auth/folo/start` CLI 流程不再作为产品入口，保留一版兼容返回 `410 AUTH_FLOW_REMOVED` 后删除。旧 API 路径仅在一个发布周期内内部重定向到 `/api/tantan/v1`，新前端只使用新路径。

## 5. 项目上下文与总体架构

### 5.1 系统、用户与外部依赖

唯一浏览器客户端是移动 Safari/Chrome/PWA。Go 是浏览器唯一业务 Provider；Folo 和 AI Provider 只接受 Go 出站。SQLite 是 Tantan Topic、队列、Filter、索引、任务和会话元数据的真实来源；Folo 是账号/RSS/Feed/Entry/已读/收藏的真实来源。

### 5.2 领域关系与数据流

登录凭据通过同源 HTTPS 只在单次请求中到达 Go，Go 调 Folo Better Auth 并捕获上游会话，密封后与不透明 Tantan 会话关联。同步任务经白名单拉取内容入 SQLite/FTS；AI 任务只从 Go 启动配置引用的 Secret 来源读取 Key，经内置 endpoint 调 Provider，Schema 校验后写翻译、摘要、Topic。Home 从最近 7 天未读生成版本化队列，浏览器只按游标读取。已读成功先写 Folo，再更新本地和失效所有 Home cache。

### 5.3 构建、运行与部署边界

本地：Vite 手机预览通过代理调用 127.0.0.1:3000；生产：`pnpm build:web` 生成静态资源，Go 从显式目录提供资源和 SPA fallback，同时处理 `/api`。反向代理在同机终止 HTTPS 并只把请求转发给 Go 的 127.0.0.1 监听；`X-Forwarded-*` 仅在配置可信代理地址后采信。数据库、加密主密钥、备份目录和日志位于服务器持久卷，静态资源不可包含运行时密钥。

## 6. 风险与未支持领域

| Risk ID | 风险或未覆盖领域                             | 影响                                                    | 缓解或所需决定                                                                                                                                    | Owner    |
| ------- | -------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| RISK-01 | Folo 官方 one-time callback 只允许 localhost | 部署到服务器后，手机社交登录不能从 Folo 自动跳回 Tantan | 保留 Google/GitHub/Apple 按钮；在 Folo 官方页登录后复制一次性令牌回 Tantan；令牌单次兑换且不落浏览器存储                                          | FE+BE    |
| RISK-02 | Folo 私有 API 变化                           | 同步或代理失败                                          | 精确路由、合同测试、版本标识、失败关闭                                                                                                            | BE       |
| RISK-03 | 服务器无 OS Keychain                         | Folo 会话和 Gemini Key 需要不同的服务端 Secret 来源     | Folo 会话用主密钥 AES-GCM 密封；Gemini Key 从 `gemini_api_key_file`（部署）或 Keychain（本机）装载，不写 SQLite；配置来源不可用时 AI 状态为未配置 | BE       |
| RISK-04 | AI 模型名或配额变化                          | 真实 AI 测试失败                                        | 模型属于内置可版本化预设，健康测试报告 provider/model 错误但不泄密                                                                                | BE       |
| RISK-05 | 手机浏览器内存与瀑布流高度变化               | 卡片跳动或崩溃                                          | 两列、尺寸占位、窗口化、entryId 去重、图片失败文字卡、性能门禁                                                                                    | FE       |
| RISK-06 | 旧桌面代码仍在仓库                           | 维护者误以为是交付物                                    | 根脚本和文档只暴露 Web/PWA；Electron/PC 测试不作为 Goal 门禁                                                                                      | Delivery |
