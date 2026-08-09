# 前端领域规格：Tantan 一期 Mobile Web/PWA

## 1. 用户旅程与导航

| Journey ID | 用户/前置条件            | 入口           | 步骤与分支                                                                                                                                                                       | 退出/恢复                                             | URL/历史                                | 最终结果                                       |
| ---------- | ------------------------ | -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- | --------------------------------------- | ---------------------------------------------- |
| J-01       | 已登录、Go ready         | 打开站点/PWA   | 恢复上次 Home Topic/Filter/滚动；读取 Topic 与队列；下拉/滚动分页                                                                                                                | 切 Tab 保留各 Tab scroll；进详情返回原卡片位置        | `/`，详情 `/entries/:entryId` 入栈      | 两列稳定推荐流                                 |
| J-02       | 已登录                   | Home 搜索图标  | 进入独立搜索；250ms 防抖；查标题、正文、译文、Source、Topic、Tag；点击结果进详情                                                                                                 | 返回恢复 Home 原 Topic/Filter/Queue/scroll            | `/search?q=`；浏览器返回有效            | 找到历史与当前内容且不改变首页                 |
| J-03       | 已登录、服务端 AI 已配置 | Home AI 图标   | Bottom Sheet 输入自然语言；提交；服务端校验并原子切换 Filter/Topics/Queue                                                                                                        | 取消无副作用；失败保留旧首页；重置回推荐              | URL 不变，Sheet 属于页面 overlay        | 生成新的推荐流                                 |
| J-04       | 已登录                   | 底栏“订阅”     | 查看 Folo Mobile 同类顶栏/内容类型 pager/分组 Source；进入 Source；添加或取消订阅                                                                                                | 返回保持类型和滚动                                    | `/subscriptions`、`/sources/:sourceId`  | 管理 RSS 订阅                                  |
| J-05       | 匿名或已登录             | 底栏“发现”     | 搜索网站/RSS/Source；查看结果；已登录可订阅                                                                                                                                      | 返回保留关键词和结果                                  | `/discover`                             | 发现并添加 Source                              |
| J-06       | 已登录                   | 底栏“设置”     | 用户头图；分组进入通用、外观、数据、账号、AI、隐私、关于；AI 页只读显示服务端配置状态并可测试连接；退出需确认                                                                    | 子页返回设置原滚动                                    | `/settings`、`/settings/:section`       | 管理非付费设置并查看 AI 状态                   |
| J-07       | 未登录                   | 任意需登录路由 | 显示与 Folo 相同的 Google、GitHub、Apple、Email 和授权令牌入口；Email 由同源 Go 登录；社交按钮打开 Folo 官方登录页，完成后把单次授权令牌粘贴回 Tantan 由 Go 兑换；成功进入原目标 | 错误留在登录页；provider 流可取消；刷新可恢复 session | `/login?returnTo=`；Folo 官方页在新 tab | 建立不透明 Tantan 会话且所有 Folo 登录方式可用 |
| J-08       | Go 暂时不可用            | 任意页         | 保留 App Shell；内容区显示连接失败、重试和诊断 ID                                                                                                                                | Go 恢复后原地重试                                     | URL 不变                                | 不展示伪数据、不跳 Folo                        |

主导航固定为 Folo Mobile 当前四 Tab：`首页`、`订阅`、`发现`、`设置`。详情、搜索和设置子页为栈式页面并隐藏底栏或按 Folo Mobile 对应页面行为处理。原型中的三 Tab 只作为首页视觉输入，不覆盖该导航合同。

## 2. 交互与 UI 状态

### 2.1 操作合同

| Action ID | 可用条件                | 触发                               | 即时反馈                                                             | Pending                         | 成功                                            | 失败/恢复                                     | 撤销/取消                     | 键盘/触控                                                            |
| --------- | ----------------------- | ---------------------------------- | -------------------------------------------------------------------- | ------------------------------- | ----------------------------------------------- | --------------------------------------------- | ----------------------------- | -------------------------------------------------------------------- |
| ACT-01    | Home ready              | 点 Topic                           | 激活指示条立即切换，旧 scroll 记录                                   | 新 Topic skeleton，旧内容不混入 | 展示对应版本队列并恢复其 scroll                 | 内联错误与重试；旧 Topic cache 保留           | 再点旧 Topic                  | 44px 触控；左右可滚；Tab/Enter 可用                                  |
| ACT-02    | Home                    | 点搜索                             | 记录 Home view state                                                 | 路由过渡                        | 搜索输入自动聚焦                                | 路由失败回 Home                               | 浏览器返回/左上返回           | 图标有“搜索”名称                                                     |
| ACT-03    | Home                    | 点 AI 筛选                         | Sheet 从底部进入，焦点进标题/输入                                    | 无网络请求                      | Sheet ready                                     | 不适用                                        | 下滑、遮罩、取消、Escape 关闭 | 焦点陷阱；关闭回触发按钮                                             |
| ACT-04    | Sheet prompt 合法       | 点“生成信息流”                     | 按钮禁用并显示“生成中…”                                              | 请求不可重复；关闭受保护        | 原子替换 active filter、topics、home；scroll=0  | Sheet 保留 prompt，显示稳定错误；旧首页不变   | Abort 只在请求未提交时有效    | Enter 不在 textarea 单独提交；按钮 44px                              |
| ACT-05    | Active Filter           | 点“重置”                           | 按钮 pending                                                         | 禁止再次提交                    | filter=null、topic=recommend、队列刷新          | 保留当前 filter 并提示重试                    | 无                            | 可键盘触发                                                           |
| ACT-06    | Home page 有 nextCursor | sentinel 进入视口                  | 不改变现有卡片                                                       | 页尾 skeleton                   | 追加并按 entryId 去重                           | 页尾错误和“重试”                              | 离页取消 fetch                | 不依赖 hover                                                         |
| ACT-07    | 卡片可读                | 点卡片                             | 卡片保持尺寸                                                         | 详情 skeleton                   | 详情入栈；已读成功后从全部 Home cache 移除      | 详情错误可重试；已读失败不移卡                | 返回恢复 scroll               | 卡片整体链接，无嵌套交互冲突                                         |
| ACT-08    | 搜索页                  | 输入关键词                         | 清除按钮显隐                                                         | 250ms 后取消旧请求并发新请求    | 结果按 cursor 追加                              | 错误保留关键词与旧结果                        | 清除回空状态                  | `type=search`、输入法 search 动作                                    |
| ACT-09    | 登录页                  | 点 provider、提交 Email 或授权令牌 | provider 显示“在 Folo 完成登录后粘贴授权令牌”；密码/令牌只在受控内存 | 按钮 pending、防重复            | Go 兑换 Folo session，清敏感字段并恢复 returnTo | 通用错误，不泄露账号存在性；密码/令牌立即清空 | 关闭 Folo tab 或返回公开页    | provider 44px；autocomplete email/current-password；token 禁自动完成 |
| ACT-10    | AI 设置且已登录         | 点“测试连接”                       | 按钮禁用并显示测试中                                                 | Go 使用启动时装载的服务端配置   | 显示固定 provider、model、已配置状态和延迟      | 稳定错误不泄露 Key                            | 无                            | 不存在 Key/model/endpoint 输入框                                     |

### 2.2 UI 状态清单

| State ID         | 进入条件                     | 展示/精确文案                          | 可用操作           | 离开事件                  | Focus/Announcement  | 恢复                    |
| ---------------- | ---------------------------- | -------------------------------------- | ------------------ | ------------------------- | ------------------- | ----------------------- |
| UI-HOME-LOADING  | 首次无 cache                 | 保留顶栏/Topic skeleton；卡片骨架      | 搜索、AI、底栏可用 | Home 成功/失败            | `aria-busy=true`    | 成功数据缓存            |
| UI-HOME-EMPTY    | ready 且 total=0             | “最近 7 天还没有可推荐的未读内容”      | 去订阅、刷新       | 新内容入队                | 标题获得公告        | 队列更新                |
| UI-HOME-FINISHED | finished=true                | “今天已经看完”与“去订阅看看”           | 切 Topic、订阅     | 新内容追加或日期变化      | polite announcement | 次日新 generation       |
| UI-HOME-ERROR    | Home 请求失败                | “推荐加载失败”与“重试”；显示 requestId | 重试、切 Tab       | 成功                      | alert 仅一次        | 保持先前 cache          |
| UI-IMAGE-ERROR   | cover 加载失败               | 隐藏破图，卡片改为文字布局             | 打开卡片           | 无                        | 不重复读 alt        | 本次 session 记失败 URL |
| UI-AI-MISSING    | Go 未装载 AI Key             | “服务器尚未配置 AI，请联系站点管理员”  | 取消、重试状态     | 管理员配置并重启/重载成功 | dialog title 聚焦   | 返回重新提交            |
| UI-AI-ERROR      | AI/Schema/限流失败           | 对应稳定文案与重试                     | 重试、编辑 prompt  | 成功/取消                 | alert               | 旧首页保持              |
| UI-OFFLINE       | navigator offline 或网络失败 | “当前离线，正在显示已缓存内容”         | 浏览 cache、重试   | online                    | polite              | 自动失效 health/session |
| UI-UNAUTH        | session=anonymous            | Folo Mobile 风格登录 CTA               | 登录               | session ready             | 页面标题聚焦        | returnTo                |

### 2.3 状态机

Home 组合状态包含 session、service、topicId、filterId、queueGeneration、pageStatus 和 detailTransition。`TOPIC_SELECT` 只改变 topicId/query key 并保存旧 scroll；`FILTER_SUBMIT` 进入 `filterSubmitting`，只有服务端事务返回新 `filterId + topicsRevision + queueGeneration` 时一次性写入三者，任一失败回 `ready(old snapshot)`；`PAGE_APPEND` 必须绑定 topic/filter/generation，响应不匹配则丢弃并重新取第一页；`ENTRY_READ_SUCCEEDED` 按 entryId 遍历所有 `home` query cache 删除卡片。每个 request 使用 AbortController；路由离开取消搜索和分页，但已被服务端接受的 Filter 任务等待结果并通过 revision 对账。

## 3. 视觉、内容与平台行为

### 3.1 布局、层级与设计 Token

| Token/样式                  | 复用或新增               | 精确值/来源                                            | 使用位置              |
| --------------------------- | ------------------------ | ------------------------------------------------------ | --------------------- |
| `--app-width`               | 新增                     | `min(100vw, 430px)`；大屏仅居中调试                    | App root              |
| `--safe-top/bottom`         | Web 平台                 | `env(safe-area-inset-top/bottom)`                      | 固定顶栏、底栏、Sheet |
| `--accent`                  | 复用 Folo Mobile         | `#FF5C00`；来自 `accentColor` 语义                     | 激活 Tab、主要按钮    |
| `systemBackground`          | 复用 Folo Mobile 语义    | light `#FFFFFF`，dark `#000000`                        | 页面底色              |
| `secondarySystemBackground` | 复用语义                 | light `#F2F2F7`，dark `#1C1C1E`                        | 卡片、分组设置        |
| `separator`                 | 复用语义                 | light `rgba(60,60,67,.18)`，dark `rgba(84,84,88,.65)`  | 顶栏/底栏/列表分隔    |
| 移动顶栏                    | 复用形态                 | 内容高 44px + safe top；半透明 blur                    | 主页面/栈页           |
| 移动底栏                    | 复用形态                 | 图标 25px、label 10px、内容高 56px + safe bottom、blur | 四主 Tab              |
| 触控最小区                  | 强制                     | 44×44 CSS px                                           | 所有按钮/图标         |
| Home gutter                 | 原型                     | 外侧 12px、列间 8px、固定 2 列                         | MasonryFeed           |
| Sheet                       | 原型 + Folo Mobile modal | 圆角 20px 20px 0 0，最大高 85dvh                       | AI Filter             |

### 3.2 字体、颜色、图标、图片、动效与主题

字体使用系统栈 `-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`，支持 Dynamic Type 到 200% 而不截断主要操作。图标继续使用仓库 MingCute 资产，对应 Folo Mobile 的 active filled/inactive regular 语义；不使用 emoji 作为导航图标。Home cover 使用服务端尺寸元数据预留 aspect ratio，`object-fit: cover`，失败进入文字卡。主题跟随系统并允许设置覆盖；`prefers-reduced-motion` 下关闭 spring/位移动画，仅保留不超过 120ms 的透明度过渡。

### 3.3 产品文案与本地化

| 场景           | 精确文案                                  | 本地化键                  | RTL/扩展/换行行为    |
| -------------- | ----------------------------------------- | ------------------------- | -------------------- |
| Home 标题      | 发现                                      | `tantan.home.title`       | 单行；超长语言仍居中 |
| Home 搜索      | 搜索                                      | `tantan.home.search`      | 仅无障碍名称         |
| Home AI        | AI 智能筛选                               | `tantan.home.ai_filter`   | 仅无障碍名称         |
| Filter CTA     | 生成信息流                                | `tantan.filter.generate`  | 允许两行，不截断     |
| Filter Reset   | 重置筛选                                  | `tantan.filter.reset`     | 单行                 |
| Queue Finished | 今天已经看完                              | `tantan.home.finished`    | 居中换行             |
| Login CTA      | 登录 Folo 账号                            | `tantan.auth.sign_in`     | 可换行               |
| AI 说明        | AI 由本站 Go 服务提供，不需要 Folo 会员。 | `tantan.ai.server_notice` | 允许三行             |

一期至少提供简体中文和英文键；新文案不得直接散落在页面组件。用户生成标题/摘要按 Unicode 安全换行，Topic 单项最多 12 个中文字符的视觉宽度，超出省略但 `title`/无障碍名称保留完整内容。

### 3.4 响应式、输入与平台差异

验收视口为 390×844（iPhone Safari 等价）和 430×932（Android Chrome 等价），另以 360×800 检查最小宽度。Home 始终 2 列，不实现 3/4 列 PC 模式。宽度大于 480px 时 App 只在灰色开发背景中居中为 430px，不出现桌面侧栏，不属于产品交互。使用 `100dvh` 并提供 `100vh` fallback；软键盘打开时 Sheet 和搜索框保持可见；横屏能操作但不作为截图一致性门禁。PWA standalone 与浏览器模式共享路由和状态。

## 4. 页面与组件架构

### 4.1 路由、页面树与组件树

```text
MobileWebApp
├─ SessionBoundary
├─ MobileStackRouter
│  ├─ MainTabs
│  │  ├─ HomePage
│  │  │  ├─ HomeHeader(SearchAction, AIFilterAction)
│  │  │  ├─ TopicTabs
│  │  │  ├─ ActiveAIFilterBar
│  │  │  └─ MasonryFeed(FeedCard, QueueEndState)
│  │  ├─ SubscriptionsPage(TimelineHeader, ViewPager, SubscriptionGroups)
│  │  ├─ DiscoverPage(DiscoverHeader, SourceSearchResults)
│  │  └─ SettingsPage(UserHeader, GroupedSettingsCards)
│  ├─ SearchPage
│  ├─ EntryDetailPage(Reader, TranslateAction, SummaryAction, Read/Favorite)
│  ├─ SourceDetailPage
│  ├─ SettingsSectionPage
│  └─ LoginPage
├─ MobileTabBar(Home, Subscriptions, Discover, Settings)
└─ ModalLayer(AIFilterSheet, ConfirmDialog, Toast)
```

主路由：`/`、`/subscriptions`、`/discover`、`/settings`。栈路由：`/search`、`/entries/:entryId`、`/sources/:sourceId`、`/settings/general`、`/settings/appearance`、`/settings/data`、`/settings/account`、`/settings/ai`、`/settings/privacy`、`/settings/about`、`/login`。

### 4.2 组件合同

| CMP ID | 职责                  | 父/子组件             | 输入                    | 输出/消费者                                           | 状态所有权       | API/数据       | UI 状态                             | A11y/响应式                                  | 文件                                         | 测试         |
| ------ | --------------------- | --------------------- | ----------------------- | ----------------------------------------------------- | ---------------- | -------------- | ----------------------------------- | -------------------------------------------- | -------------------------------------------- | ------------ |
| CMP-01 | 四 Tab 移动壳与安全区 | MobileWebApp/MainTabs | route、theme            | navigate                                              | Router           | 无             | ready/offline                       | tablist、44px、无桌面分支                    | `tantan-shell/MobileWebAppShell.tsx`         | TC-01、TC-02 |
| CMP-02 | Home 顶栏             | HomePage              | title、actions          | search/filter callbacks                               | Home page        | 无             | fixed/scroll                        | 两图标有名称                                 | `tantan-home/HomeHeader.tsx`                 | TC-03        |
| CMP-03 | Topic 列表            | HomePage              | topics、activeId        | topicId                                               | Home controller  | API-03         | loading/ready/error                 | tablist、横向滚动                            | `tantan-home/TopicTabs.tsx`                  | TC-04        |
| CMP-04 | 瀑布流                | HomePage/FeedCard     | pages、queue            | loadNext/open/read                                    | React Query      | API-04         | loading/empty/error/end             | 两列、稳定尺寸                               | `tantan-home/MasonryFeed.tsx`                | TC-05、TC-06 |
| CMP-05 | AI Filter Sheet       | ModalLayer            | active filter           | submit/reset/close                                    | Sheet form       | API-06         | edit/pending/error                  | dialog/focus trap/safe area                  | `tantan-home/AIFilterSheet.tsx`              | TC-07        |
| CMP-06 | 普通搜索              | stack                 | q                       | open result                                           | URL + query      | API-07         | empty/loading/results/error         | search semantics                             | `tantan-search/SearchPage.tsx`               | TC-08        |
| CMP-07 | 订阅 Tab              | MainTabs              | view                    | add/remove/open                                       | URL/search state | API-08         | anonymous/loading/groups/error      | Folo Mobile pager/list                       | `tantan-subscriptions/SubscriptionsPage.tsx` | TC-09        |
| CMP-08 | 发现 Tab              | MainTabs              | q                       | subscribe/open                                        | page             | API-09         | discover/search/error               | Folo Mobile header/cards                     | `tantan-discover/DiscoverPage.tsx`           | TC-10        |
| CMP-09 | 设置列表              | MainTabs              | user/settings           | push section/logout                                   | Router           | API-10         | anonymous/ready/error               | grouped cards                                | `tantan-settings/SettingsHomePage.tsx`       | TC-11        |
| CMP-10 | 详情/AI 操作          | stack                 | entryId                 | read/favorite/translate/summary                       | query + server   | API-11、API-12 | loading/reader/error                | heading、reader typography                   | `tantan-entry/EntryDetailPage.tsx`           | TC-12        |
| CMP-11 | Folo 登录面板         | stack                 | returnTo、provider list | open provider/submit email/submit token/session ready | form memory only | API-01         | idle/provider-handoff/pending/error | Folo 同顺序入口、autocomplete、error summary | `tantan-auth/LoginPage.tsx`                  | TC-13        |
| CMP-12 | Service Boundary      | root                  | health/session          | retry                                                 | query            | API-02         | ready/degraded/unavailable          | alert/requestId                              | `tantan-service-status/ServiceBoundary.tsx`  | TC-14        |

### 4.3 复用、拆分与设计理由

不直接编译 React Native 工程；它是只读的视觉、信息架构和交互证据。Web 继续复用 Folo 内部 store/model/UIKit token，但所有新页面用 Web 语义元素实现。Home 与 Folo 默认 EntryList 分离，因为它拥有不同队列、分页稳定性和缓存失效规则；EntryDetail、Source、Subscription 域模型继续复用。

## 5. 状态、数据流与接口消费

### 5.1 状态所有权

| State ID | 类型              | 唯一数据源                          | 读取方                    | 写入方              | 更新/失效/同步/重置                                     |
| -------- | ----------------- | ----------------------------------- | ------------------------- | ------------------- | ------------------------------------------------------- |
| ST-01    | server            | Tantan session cookie/`GET session` | SessionBoundary、设置     | Go auth             | 登录/退出清全部 user-scoped cache                       |
| ST-02    | server            | topics revision                     | TopicTabs                 | Go filter/topic     | Filter 成功或 sync 后失效                               |
| ST-03    | server            | queue generation + pages            | MasonryFeed               | Go home/read/filter | key=`home,topic,filter,generation`; entry read 遍历删除 |
| ST-04    | URL               | `q`                                 | SearchPage                | search input        | replace history 防抖；离页 cancel                       |
| ST-05    | client ephemeral  | Sheet form                          | AIFilterSheet             | user                | close 默认保留本次草稿；成功清空                        |
| ST-06    | client session    | 每 Tab scroll 和 Home topic         | shell/pages               | scroll listeners    | 内存 + `history.state`；不含秘密                        |
| ST-07    | server            | Provider/model/configured           | AI Settings/entry actions | Go settings status  | Key 永不进入浏览器；配置变化由服务端重启或安全重载生效  |
| ST-08    | upstream mirrored | subscriptions/entries/read/favorite | subscription/detail/store | Go proxy/sync       | mutation success invalidates relevant queries           |

### 5.2 数据流

Home：同源 API → 运行时 DTO 校验 → React Query page cache → `entryId` 去重和尺寸映射 → MasonryFeed → 用户打开详情 → Go 标记已读成功 → 全部 Home cache 删除。AI Filter：prompt → Go → AI Schema → DB 事务写 Filter/Topics/Queue → revision response → React batch 更新 → 新 query key。浏览器不持有也不提交 Folo token、AI Key 或 Provider endpoint。

### 5.3 API、订阅与回调消费

| API ID | 触发方 | Provider | 协议/版本/方法/地址                                                                    | 请求                                                | 响应                                               | 状态/错误映射                                 | Auth                     | 取消/超时/重试            | 缓存/失效                       |
| ------ | ------ | -------- | -------------------------------------------------------------------------------------- | --------------------------------------------------- | -------------------------------------------------- | --------------------------------------------- | ------------------------ | ------------------------- | ------------------------------- |
| API-01 | CMP-11 | Go       | GET `/api/auth/folo/providers`；POST `/api/auth/folo/social-start`、`/email`、`/token` | provider 或 email/password 或一次性 token、returnTo | providers、固定 Folo authorize URL 或 session user | provider 非法 400；登录/令牌无效 401；2FA 409 | anonymous + exact Origin | 15s，不自动重试           | 成功清 user cache；token 不缓存 |
| API-02 | CMP-12 | Go       | GET `/api/healthz`、`/api/readyz`、`/api/tantan/v1/session`                            | 无                                                  | health/session                                     | unavailable/unauth                            | cookie for session       | 5s，手动+online 重试      | 30s stale                       |
| API-03 | CMP-03 | Go       | GET `/api/tantan/v1/topics`                                                            | revision 可选                                       | topics、revision                                   | 保留旧 tabs + banner                          | cookie                   | 10s，一次                 | Filter/sync 失效                |
| API-04 | CMP-04 | Go       | GET `/api/tantan/v1/home`                                                              | topicId、filterId、cursor、limit=20、timezone       | items、nextCursor、queue、generation               | version changed 重新第一页                    | cookie                   | 15s，页尾手动重试         | 不在分页中重排                  |
| API-05 | CMP-10 | Go       | PUT `/api/folo/reads`                                                                  | entryIds                                            | success                                            | 失败保留 Home card                            | cookie                   | 10s，幂等重试一次         | home/entry/unread               |
| API-06 | CMP-05 | Go       | PUT/DELETE `/api/tantan/v1/filter`                                                     | prompt 或 active filter                             | filter/topicsRevision/generation                   | AI/schema/limit 映射 Sheet                    | cookie+CSRF              | 60s，不自动重提           | topics/home 原子切换            |
| API-07 | CMP-06 | Go       | GET `/api/tantan/v1/search`                                                            | q、cursor、limit=20                                 | items、nextCursor                                  | 空/错误                                       | cookie                   | abort + 15s               | query scoped 5m                 |
| API-08 | CMP-07 | Go/Folo  | GET/POST/DELETE `/api/folo/subscriptions`                                              | 精确 Folo DTO                                       | subscriptions                                      | upstream 错误                                 | cookie+CSRF mutation     | GET 一次；mutation 幂等键 | subscription/home               |
| API-09 | CMP-08 | Go/Folo  | GET `/api/folo/discover`                                                               | q、cursor                                           | Source results                                     | 429/empty/error                               | session optional         | abort + 15s               | q scoped                        |
| API-10 | CMP-09 | Go       | GET `/api/tantan/v1/settings/ai-provider`；POST `/test`                                | test 无 body                                        | 固定 provider/model/configured metadata 或测试延迟 | server-secret/provider errors                 | cookie；test 带 CSRF     | 30s，无自动重试           | status 短缓存                   |
| API-11 | CMP-10 | Go/Folo  | GET `/api/folo/entries/:id`、collections mutation                                      | id                                                  | entry/read/favorite                                | 404/error                                     | cookie                   | 15s                       | entry                           |
| API-12 | CMP-10 | Go       | GET/POST `/api/tantan/v1/entries/:id/enrichment`                                       | kind/locale                                         | translation/summary/status                         | missing-key/queued/failed                     | cookie+CSRF POST         | GET poll 有界             | entry enrichment                |

### 5.4 表单与校验

| Form/Field ID                | 类型/默认值 | 约束                                                                   | 校验时机    | 错误展示/播报              | 提交与防重复                                    | 成功/重置         |
| ---------------------------- | ----------- | ---------------------------------------------------------------------- | ----------- | -------------------------- | ----------------------------------------------- | ----------------- |
| FORM-01 email                | email/空    | RFC 浏览器 email；最大 254                                             | blur+submit | 字段下方 + summary         | pending 禁用                                    | 登录后清空        |
| FORM-02 password             | password/空 | 8～128；不 trim                                                        | submit      | 通用错误，不区分账号       | pending 禁用；不记录                            | 每次结果后清空    |
| FORM-02B authorization token | password/空 | 接受原始 token、`auth?token=` 或 `folo://auth?token=`；解析后 20～4096 | submit      | 通用“授权令牌无效或已使用” | pending 禁用；不记录/缓存/trim 后仅当前 request | 每次结果后清空    |
| FORM-03 filter prompt        | textarea/空 | trim 后 1～1000 Unicode 字符                                           | submit      | Sheet 内 alert             | 单 mutation；idempotency key                    | 成功清空          |
| FORM-04 search q             | search/URL  | trim 后 1～200                                                         | 250ms       | 结果区 alert               | abort previous                                  | clear 时 URL 无 q |

## 6. 前端横切合同

### 6.1 无障碍

页面只有一个 `h1`；顶栏、Tab、Sheet、错误和列表使用正确 landmark/role。触控目标至少 44px，颜色对比 WCAG AA，焦点可见，Sheet 焦点陷阱，路由变化将焦点移到页面标题并更新 `document.title`。图片装饰 alt 为空，内容图片 alt 由标题生成；瀑布流 DOM 顺序必须与视觉按列阅读顺序一致。axe 核心路径 serious/critical 为 0。

### 6.2 安全与隐私

请求只用相对 `/api`；CSP `default-src 'self'`，图片允许明确 HTTPS 域，`connect-src 'self'`。前端合同不存在 AI Key 输入或请求字段；禁止在 localStorage、sessionStorage、IndexedDB、Cache API、URL、错误上报、HAR、fixture 和 source map 中存 AI Key、Folo token、邮箱密码。Mutation 带 CSRF；外链 `noopener noreferrer`；文章 HTML 经现有 sanitizer。Service Worker 不缓存认证/业务 API 响应，只缓存带 hash 的静态资源和离线壳。

### 6.3 性能与资源预算

| 指标               | 目标                    | 测量环境/方法                          | 失败阈值            |
| ------------------ | ----------------------- | -------------------------------------- | ------------------- |
| 首屏 JS gzip       | ≤ 320 KiB               | production build bundle report         | > 380 KiB           |
| Home LCP           | ≤ 2.5s                  | 4G Fast、4×CPU、缓存冷启动、390×844    | > 3.0s              |
| Home CLS           | ≤ 0.10                  | 20 张混合图片卡 + 图片失败             | > 0.15              |
| Topic/Tab 点击反馈 | ≤ 100ms                 | PerformanceObserver/input timing       | > 200ms             |
| 长列表内存         | 500 卡后 ≤ 180MB        | Playwright Chromium mobile + windowing | > 230MB             |
| 页面请求           | 分页每次 1 个 Home 请求 | 网络断言                               | 重复或并行同 cursor |

### 6.4 分析、错误与性能监控

一期默认关闭第三方分析。客户端错误只记录稳定 errorCode、route label、requestId、版本和时长；不记录 query 原文、文章正文、邮箱、prompt、token 或 Key。开发时可用 PerformanceObserver；生产远程上报需独立批准。

## 7. 前端需求追踪

| FR ID | 需求                                                            | Journey/CMP/API ID                       | 实现文件/符号                                                | TASK ID | AC ID | TC ID               |
| ----- | --------------------------------------------------------------- | ---------------------------------------- | ------------------------------------------------------------ | ------- | ----- | ------------------- |
| FR-01 | 四 Tab、安全区、栈导航与 Folo Mobile 外观                       | J-01、J-04、J-05、J-06、CMP-01           | `tantan-shell`                                               | TASK-03 | AC-01 | TC-01、TC-02        |
| FR-02 | Home 固定两列稳定瀑布流和队列终态                               | J-01、CMP-04、API-04                     | `tantan-home`                                                | TASK-04 | AC-02 | TC-05、TC-06        |
| FR-03 | Topic Tab 缓存与滚动恢复                                        | J-01、CMP-03、API-03                     | `TopicTabs`、`useHomeController`                             | TASK-04 | AC-03 | TC-04               |
| FR-04 | 普通搜索独立路由且不改变 Home                                   | J-02、CMP-06、API-07                     | `tantan-search`                                              | TASK-05 | AC-04 | TC-08               |
| FR-05 | AI Filter Sheet 原子提交和重置                                  | J-03、CMP-05、API-06                     | `AIFilterSheet`                                              | TASK-05 | AC-05 | TC-07               |
| FR-06 | 详情、已读、收藏、翻译、摘要和 Home cache 协同                  | J-01、CMP-10、API-05、API-11、API-12     | `tantan-entry`                                               | TASK-06 | AC-06 | TC-12               |
| FR-07 | 订阅、发现、设置保持 Folo Mobile 信息架构                       | J-04、J-05、J-06、CMP-07、CMP-08、CMP-09 | `tantan-subscriptions`、`tantan-discover`、`tantan-settings` | TASK-06 | AC-07 | TC-09、TC-10、TC-11 |
| FR-08 | Folo Google/GitHub/Apple/Email/授权令牌登录、服务状态和失败恢复 | J-07、J-08、CMP-11、CMP-12               | `tantan-auth`、`tantan-service-status`                       | TASK-03 | AC-08 | TC-13、TC-14        |
| FR-09 | 服务端 AI 状态 UI 无 Folo 会员/付费/额度门槛                    | J-06、CMP-09、CMP-10、API-10             | AI settings/detail actions                                   | TASK-06 | AC-09 | TC-11、TC-12        |
| FR-10 | Mobile PWA、离线壳、性能、无障碍和秘密扫描                      | CMP-01、CMP-04                           | manifest、service worker、E2E                                | TASK-08 | AC-10 | TC-15、TC-16        |
