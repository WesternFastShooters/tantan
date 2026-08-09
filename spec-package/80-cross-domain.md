# 跨领域合同：Tantan 一期 Mobile Web/PWA

## 1. 共享术语、标识与所有权

| Shared ID | 概念/数据     | Canonical 名称/类型                               | Source of Truth           | 写入 Owner               | 读取方                    |
| --------- | ------------- | ------------------------------------------------- | ------------------------- | ------------------------ | ------------------------- |
| SH-01     | 当前用户会话  | `TantanSession`、HttpOnly cookie、SessionDTO      | Go session store          | Auth                     | FE、所有 Go handler       |
| SH-02     | Folo 内容标识 | `entryId`、`sourceId`、`feedId` string            | Folo                      | Folo/Sync mirror         | FE、Home、Search          |
| SH-03     | 首页分类      | `topicId` stable string + `topicsRevision` uint64 | Go Topic repository       | Topic/Filter transaction | FE TopicTabs/Home         |
| SH-04     | 推荐快照      | `queueGeneration` opaque string + `nextCursor`    | Go Home repository        | Home/Filter              | FE React Query            |
| SH-05     | AI 配置       | `providerId`、`modelId`、`configured`；无 Key     | Go 启动配置/Secret source | operator config          | FE settings/detail 只读   |
| SH-06     | 请求关联      | `X-Request-Id` 128-bit hex                        | Go edge                   | HTTP middleware          | FE error UI、Go logs/jobs |
| SH-07     | 错误          | `ErrorEnvelope v1`                                | OpenAPI                   | Go modules               | FE mapping、tests         |

## 2. Provider-Consumer 接口

| API/EVT ID                     | Provider        | Consumer              | Schema/版本                        | Auth                  | 成功                              | 错误                             | 超时/重试/取消                 | 兼容                    |
| ------------------------------ | --------------- | --------------------- | ---------------------------------- | --------------------- | --------------------------------- | -------------------------------- | ------------------------------ | ----------------------- |
| API-01、API-02                 | Go Auth         | SessionBoundary/Login | OpenAPI v2 SessionDTO              | cookie/CSRF           | 建立/读取/清除 session            | AUTH/SESSION envelope            | 15s；登录不自动重发            | 一期 v2                 |
| API-03、API-04                 | Go Topic/Home   | Home UI               | Home JSON Schema + OpenAPI         | cookie                | revision/generation 一致的 pages  | QUEUE_VERSION_CHANGED 触发第一页 | 15s；Abort 分页                | DTO 字段只增不删        |
| API-05、API-08、API-09、API-11 | Go Folo Adapter | stores/pages          | folo-route-policy + normalized DTO | cookie/CSRF           | 只返回业务 DTO                    | FOLO_ROUTE_DENIED 在出站前       | GET 有界；mutation idempotency | policy hash 版本化      |
| API-06                         | Go Filter/AI    | AIFilterSheet/Home    | Filter snapshot v2                 | cookie/CSRF           | filter/topics/queue 原子 revision | 失败保持旧 snapshot              | 60s；不自动重提                | revision mandatory      |
| API-07                         | Go Search       | SearchPage            | SearchResponse v1                  | cookie                | cursor pages                      | stable envelope                  | 15s；输入取消旧请求            | v1                      |
| API-10、API-12                 | Go AI           | Settings/EntryDetail  | ProviderMetadata、Enrichment v1    | cookie/CSRF           | metadata/ready/queued             | AI stable codes                  | test 30s、job 60s              | schemaVersion mandatory |
| API-13、API-14                 | Go Sync/Ops     | UI/operator           | SyncStatus/Health v1               | session/public health | 状态                              | fail-closed 503                  | 有界手动重试                   | v1                      |

## 3. 状态与错误映射

| Shared Case ID | Provider 结果             | Consumer 状态/调用方结果                     | 文案/播报                             | 恢复                                                     | 观测                                   |
| -------------- | ------------------------- | -------------------------------------------- | ------------------------------------- | -------------------------------------------------------- | -------------------------------------- |
| CASE-01        | 401 SESSION_REQUIRED      | 清 user query；push login with returnTo      | “登录后继续”                          | 任一 Folo provider 或授权令牌登录成功后 replace returnTo | requestId + route                      |
| CASE-02        | 403 ORIGIN/CSRF           | 不重试 mutation；保留输入                    | “安全校验失败，请刷新后重试”          | 刷新 session/CSRF                                        | security counter                       |
| CASE-03        | 403 FOLO_ROUTE_DENIED     | 显示功能不可用；绝不直连 Folo fallback       | “此操作未被 Tantan 服务允许”          | 修订 route policy 后发布                                 | deny counter 必须有且 upstream count 0 |
| CASE-04        | 409 QUEUE_VERSION_CHANGED | 丢弃该 key pages，取第一页                   | 不弹全局错误，polite“推荐已更新”      | 自动一次                                                 | old/new generation hash                |
| CASE-05        | 409 AI_NOT_CONFIGURED     | 显示服务端 AI 状态                           | “服务器尚未配置 AI，请联系站点管理员” | operator 配置并重启/重载后用户重提                       | provider config state                  |
| CASE-06        | 422 AI_OUTPUT_INVALID     | Sheet 保留 prompt；详情显示失败              | “AI 返回格式无效，请重试”             | 有界重试/换 preset 发布                                  | schemaVersion/path                     |
| CASE-07        | 503 UPSTREAM_UNAVAILABLE  | cache 可读，write 不乐观提交                 | “Folo 暂时不可用”                     | 手动或 worker backoff                                    | dependency/status/duration             |
| CASE-08        | 200 read mutation         | 所有 Home query 移除 entryId；详情 read=true | 无 toast 或简短成功                   | sync 对账                                                | mutation requestId                     |

## 4. 安全、隐私与信任传播

| Boundary ID             | 身份/Auth                   | 敏感字段                           | 存储/保留                                        | Redaction/Audit                 | 强制执行方           |
| ----------------------- | --------------------------- | ---------------------------------- | ------------------------------------------------ | ------------------------------- | -------------------- |
| BND-01 Browser→Go       | Tantan cookie、CSRF、Origin | password、prompt；禁止 AI Key 字段 | password 请求结束即清引用；prompt 按 DB policy   | body/header/query secret 不日志 | FE policy + Go HTTP  |
| BND-02 Go→Folo          | 服务端密封 Folo token       | token、email、内容                 | token secret store；内容 SQLite                  | 固定 route、剥离浏览器 headers  | Go Folo proxy        |
| BND-03 Go→Gemini        | Go 启动时装载的 AI Key      | Key、文章、prompt                  | Key 仅在进程内存/权限 Secret 来源；结果过 Schema | provider/model/status only      | Go AI client         |
| BND-04 Go→SQLite/Backup | userId ownership            | personal data、ciphertext          | 0600、备份 retention                             | SQL 参数化；doctor secret-free  | repository/ops       |
| BND-05 PWA cache        | 无认证能力                  | 业务 API 响应                      | 不缓存 API，只缓存 hash 静态资源                 | build/canary scan               | Service Worker tests |

身份传播规则：浏览器身份只由 session context 得到，handler 不接受请求 body/query 中的 userId；所有 repository 方法强制传 context user。Folo token只在最小出站函数内解封；Gemini Key 只由 AI client 持有，二者都不进入通用 request context。

## 5. 交付顺序、版本与恢复

| Delivery ID | 合同/生成                                        | 迁移                 | Provider       | Consumer               | 兼容窗口                 | Rollback/Recovery                  |
| ----------- | ------------------------------------------------ | -------------------- | -------------- | ---------------------- | ------------------------ | ---------------------------------- |
| DEL-01      | 先锁 OpenAPI/Schema/route policy并生成 Go/TS DTO | 无                   | 合同测试       | FE/BE                  | 同提交                   | 生成 diff 必须可复现               |
| DEL-02      | Session/Auth v2                                  | MIG-01               | Go Auth/Folo   | FE Login/Boundary      | 旧 CLI path 一个周期 410 | 回滚需用户重新登录，不恢复旧 token |
| DEL-03      | Queue/revision v2                                | MIG-02               | Go Home        | FE Home                | 老 cursor 明确 409       | 清 v2 queue 后可重建               |
| DEL-04      | FTS v2                                           | MIG-03               | Go Search/Sync | FE Search              | shadow table 到验证完成  | 切旧 FTS 或恢复备份                |
| DEL-05      | Mobile UI/PWA                                    | 无                   | Go API ready   | Web static             | API 兼容旧前端一版       | 原子切静态目录                     |
| DEL-06      | 服务端 Gemini AI                                 | operator Secret 配置 | Go AI          | Settings/Detail/Filter | preset version           | 关 flag，规则 Home 正常            |

## 6. 端到端可观测性、测试与验收

| Flow ID             | Correlation                        | Logs/Metrics/Traces                  | Contract Test                       | E2E/Failure Test                                               | Owner    | AC ID |
| ------------------- | ---------------------------------- | ------------------------------------ | ----------------------------------- | -------------------------------------------------------------- | -------- | ----- |
| FLOW-01 Login       | requestId→session idHash（不输出） | auth code/status/duration            | OpenAPI auth + fake Folo Set-Cookie | 成功、错密、2FA、Folo down、secret scan                        | FE+BE    | AC-20 |
| FLOW-02 Home        | requestId→generation               | page count/duration/version conflict | Home Schema/cursor                  | stable pagination、read removal、图片失败、finished            | FE+BE    | AC-21 |
| FLOW-03 Filter      | requestId→jobId→revision           | AI/filter/transaction status         | AI Schema + snapshot                | success/invalid/timeout/crash 保持旧首页                       | FE+BE    | AC-22 |
| FLOW-04 Search/Sync | requestId→sync job/checkpoint      | freshness/result count               | Search DTO                          | 原文/译文/Source/Topic/Tag、cancel/resume                      | FE+BE    | AC-23 |
| FLOW-05 Server AI   | requestId→provider preset          | provider/model/status                | preset allowlist/schema             | canary 扫 browser request/storage、DB、log、HAR、build、backup | Security | AC-24 |
| FLOW-06 Deploy/PWA  | requestId→release version          | ready/static/version                 | health/config                       | 真实手机 HTTPS、offline shell、restore                         | Ops      | AC-25 |

## 7. 跨领域需求追踪

| XR ID | 端到端需求                                       | Provider/Consumer Requirement     | API/EVT ID             | 实现位置                         | TASK ID | AC ID | TC ID               |
| ----- | ------------------------------------------------ | --------------------------------- | ---------------------- | -------------------------------- | ------- | ----- | ------------------- |
| XR-01 | 浏览器所有业务请求同源且禁止直连 Folo/AI         | BR-01、BR-03、FR-08、FR-10        | API-01～API-14         | HTTP client、CSP、Go router      | TASK-07 | AC-20 | TC-31               |
| XR-02 | 登录成功后安全恢复目标页且 secret 不下发         | BR-02、FR-08                      | API-01、API-02         | Auth + SessionBoundary           | TASK-03 | AC-20 | TC-18、TC-32        |
| XR-03 | Home/Filter/Topic revision 原子一致              | BR-06、BR-07、FR-02、FR-03、FR-05 | API-03、API-04、API-06 | Home controller + Go transaction | TASK-05 | AC-22 | TC-27、TC-33        |
| XR-04 | 已读成功同步 Folo、本地 mirror 和全部 Home cache | BR-08、FR-06                      | API-05、API-11         | Go adapter/sync + FE cache       | TASK-06 | AC-21 | TC-28、TC-34        |
| XR-05 | 服务端 Gemini 可用且任何层 Secret 泄漏为 0       | BR-05、BR-09、FR-09、FR-10        | API-10、API-12         | config/AI/settings/scanners      | TASK-07 | AC-24 | TC-24、TC-35        |
| XR-06 | 生产构建经同源 Go 在真实手机可安装、交互、恢复   | BR-01、BR-09、FR-01、FR-10        | API-14                 | build/static/ready/PWA/E2E       | TASK-08 | AC-25 | TC-16、TC-30、TC-36 |
