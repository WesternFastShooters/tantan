# 后端领域规格：Tantan 一期 Go 同源中间层

## 1. 系统上下文与架构

### 1.1 系统边界、参与者、外部系统与信任边界

手机浏览器只信任 Tantan HTTPS origin，并仅调用相对 `/api`。Go 是唯一公开应用服务，负责静态 PWA、身份、CSRF、业务 API、任务和出站；SQLite 与密封 secret store 位于服务器持久卷。Folo 和 Gemini 是不受信外部依赖：输入输出均做长度、状态、Content-Type 和 Schema 校验。反向代理只负责 TLS、请求大小和转发，不拥有业务身份。

### 1.2 模块与服务

| MOD ID | 职责                                                     | Owner    | 公开接口                  | 依赖                                     | 文件/部署单元                                        |
| ------ | -------------------------------------------------------- | -------- | ------------------------- | ---------------------------------------- | ---------------------------------------------------- |
| MOD-01 | 配置、启动、静态 PWA、graceful shutdown                  | Go app   | process、static、health   | MOD-02、SQLite                           | `cmd/tantan-api`                                     |
| MOD-02 | HTTP 路由、安全中间件、error envelope                    | HTTP     | `/api/*`                  | session、CSRF、limits                    | `internal/http`                                      |
| MOD-03 | Folo Provider/Email/授权令牌登录、Tantan session、logout | Auth     | API-01、API-02            | Folo Better Auth、one-time-token、MOD-04 | `internal/auth`、`internal/session`                  |
| MOD-04 | 密钥密封与版本化                                         | Security | internal secret interface | Keychain 或 AES-GCM master key           | `internal/secrets`                                   |
| MOD-05 | Folo 精确 method+path 代理                               | Upstream | `/api/folo/*`             | route policy、session secret             | `internal/folo`                                      |
| MOD-06 | SQLite、迁移、Repository                                 | Data     | internal repositories     | modernc SQLite                           | `internal/storage`、`migrations`                     |
| MOD-07 | Folo 内容同步、checkpoint、任务                          | Sync     | API-13、JOB-01            | MOD-05、MOD-06                           | `internal/sync`、`internal/jobs`                     |
| MOD-08 | FTS 搜索                                                 | Search   | API-07                    | MOD-06                                   | `internal/search`                                    |
| MOD-09 | Topic、推荐、版本化每日队列                              | Home     | API-03、API-04、API-06    | MOD-06、MOD-10                           | `internal/topic`、`home`、`recommendation`、`filter` |
| MOD-10 | 服务端 Gemini、Schema、翻译摘要分类                      | AI       | API-10、API-12            | Go Secret 配置、Gemini preset            | `internal/ai`、`enrichment`                          |
| MOD-11 | readiness、doctor、backup/restore、redaction             | Ops      | API-14、CLI               | all modules                              | `internal/ops`、`observability`                      |

### 1.3 运行时流程与部署拓扑

生产拓扑：`Mobile HTTPS → Caddy/Nginx → 127.0.0.1:3000 Go → SQLite/secret store/Folo/Gemini`。Go 启动依次解析配置、锁数据目录、装载会话主密钥、从 `gemini_api_key_file` 或本机 Keychain 装载可选 Gemini Key、应用迁移、验证 route policy、启动 worker、最后置 ready；被显式配置但不可读的密钥或迁移错误保持 `/api/readyz` 503，未配置 Gemini 则核心服务 ready 但 AI 状态为未配置。退出先拒绝新 mutation，再等待最长 20s 的 HTTP/worker，提交 checkpoint 后关闭 DB。

登录入口从固定 allowlist 返回 Google、GitHub、Apple、Email 和 authorization-token。社交开始接口只生成 `https://app.folo.is/login?provider=` 加 allowlist provider 的 URL，不接受回调 URL；用户在官方页完成登录并复制 Folo 生成的 one-time token。Email 流为 Origin/限流 → 校验邮箱密码 → Go 调 Folo `/better-auth/sign-in/email`；若返回 `twoFactorRedirect`，Go 把 pending upstream cookie 仅在内存中以随机 flowId 保存 5 分钟，再经 `/better-auth/two-factor/verify-totp` 完成。Token 流为 Origin/限流 → 规范化 raw、`auth?token=` 或 `folo://auth?token=` → Go 调 `/better-auth/one-time-token/apply`，404 兼容时才调 verify。成功流只从受限 `Set-Cookie`/响应提取 Folo session token，再用 get-session 验证 user、密封 token、建立随机 Tantan cookie并返回无 secret user DTO。密码、TOTP 和 one-time token 只存在当前请求缓冲；token 必须单次使用并做短时 hash replay 防护。

Home：session → 参数/游标 HMAC 校验 → 找 timezone 当日 generation → 无队列时从最近 7 个日历日未读候选生成最多 50 条并持久化 → 分页只读固定 position → 返回 queue metadata。同步发现新内容时可稳定追加到末尾，但总数不超过 60。

## 2. 领域模型与数据

### 2.1 实体、值对象、标识与不变量

| Domain ID | 类型           | 字段/关系                                                         | 生命周期               | 不变量                                                 | 强制执行位置          |
| --------- | -------------- | ----------------------------------------------------------------- | ---------------------- | ------------------------------------------------------ | --------------------- |
| DM-01     | TantanSession  | idHash、userId、sealedFoloTokenRef、csrfHash、expiresAt           | 登录到退出/过期        | cookie 随机 256-bit；DB 不存原 token；用户隔离         | MOD-03/MOD-04         |
| DM-02     | EntryMirror    | entryId、sourceId、内容、译文、状态                               | Folo sync 到保留期结束 | entryId 唯一；Folo 状态为权威，本地写幂等              | MOD-06/MOD-07         |
| DM-03     | Topic          | topicId、name、fixed、order、revision                             | 固定或 Filter 生成     | ID 稳定；name 不作查询主键；recommend 固定             | MOD-09                |
| DM-04     | Filter         | filterId、promptHash、specJSON、active、revision                  | 提交成功到重置/替换    | 每用户至多一个 active；spec 必须过 Schema              | MOD-09                |
| DM-05     | DailyQueue     | userId、localDate、timezone、topicId、filterId、generation        | 当地日历日             | 初始≤50、当日≤60、position 不变、entryId 唯一          | MOD-09/DB constraints |
| DM-06     | QueueCursor    | version、userHash、topic/filter/generation、position、expiry、MAC | 单分页序列             | 不能跨用户/队列/版本重放                               | MOD-09                |
| DM-07     | ServerAIConfig | providerId、modelId、keySource、keyFingerprint                    | Go 启动/安全重载       | endpoint/model 来自二进制 preset；Key 不进 HTTP/SQLite | MOD-01/MOD-10         |
| DM-08     | AIResult       | kind、entryId、locale、schemaVersion、content、status             | queued 到 ready/failed | 只有 Schema 通过才写 ready                             | MOD-10                |

### 2.2 数据模型

| Schema ID | 表/集合/对象                     | 字段/类型/默认值                                           | Key/约束                                          | 索引                | 读写方          | 保留/删除                                     |
| --------- | -------------------------------- | ---------------------------------------------------------- | ------------------------------------------------- | ------------------- | --------------- | --------------------------------------------- |
| DB-01     | `sessions`                       | id_hash、user_id、secret_ref、csrf_hash、expires_at        | PK id_hash                                        | expires_at          | MOD-03          | 过期 7 天内清理                               |
| DB-02     | `entries`/`sources`              | 现有迁移字段                                               | entry_id/source_id 唯一、FK                       | published_at/source | MOD-06/07/08/09 | 用户数据删除时级联                            |
| DB-03     | FTS5                             | title/content/translation/source/topic/tag                 | content row link                                  | FTS                 | MOD-08          | entry 同步删除                                |
| DB-04     | `topics`/`entry_topics`          | user、id、name、fixed、revision                            | user+topic_id 唯一                                | order/revision      | MOD-09          | fixed 保留；dynamic 可删                      |
| DB-05     | `filters`                        | id、user、spec、active、revision                           | partial unique active/user                        | user/active         | MOD-09          | 替换后保留 30 天                              |
| DB-06     | `home_queues`/`home_queue_items` | local_date/timezone/topic/filter/generation/position/entry | user+scope+generation 唯一；position/entry unique | scope+position      | MOD-09          | 保留 14 天                                    |
| DB-07     | `secret_records`                 | owner、kind=`folo_session`、nonce、ciphertext、key_version | owner+kind unique                                 | key_version         | MOD-04          | logout 删除 Folo token；Gemini Key 不进入该表 |
| DB-08     | `ai_results`/`jobs`              | kind/schema/status/attempt                                 | entry+kind+locale unique                          | status/next_run     | MOD-10/JOB-02   | 结果随 entry；失败任务 30 天                  |
| DB-09     | `sync_checkpoints`               | user/cursor/updated                                        | user+stream unique                                | updated             | MOD-07          | 账号删除时删除                                |

`db/*.sql` 是 canonical 迁移；运行时 embed 副本必须字节一致。v2 新增的 session/secret/queue字段使用新 migration，不编辑已应用 migration。

### 2.3 事务、一致性与并发

Filter 提交在一个 SQLite `BEGIN IMMEDIATE` 中写新 Filter、Topics、ready Queue 和 revision，提交后才返回；AI 失败或事务冲突不改变旧 active snapshot。每用户/日期/Topic generation 使用 DB unique + singleflight，两个首次 Home 请求只生成一次。同步每页内容和 checkpoint 同事务；Folo mutation 使用 idempotency key 并在成功后更新 mirror。SQLite 开 WAL、foreign_keys、busy_timeout；写竞争超时返回可重试 `DB_BUSY`，不做无限重试。

### 2.4 迁移、回填与兼容

| MIG ID | 前置                       | Schema/Data 变化                      | 锁/事务/在线行为           | 回填/双写                                                 | 验证                            | 回滚/版本偏差                  |
| ------ | -------------------------- | ------------------------------------- | -------------------------- | --------------------------------------------------------- | ------------------------------- | ------------------------------ |
| MIG-01 | 旧 0001～0003 已应用或空库 | 新增密封 secret/session 字段与版本    | 启动前单事务；数据库文件锁 | 旧 loopback session 作废，不回填 token                    | integrity、FK、schema snapshot  | 备份恢复旧二进制；新列向前保留 |
| MIG-02 | MIG-01                     | 增加 queue scope/revision constraints | 单事务                     | 从旧 ready queue 生成 v2 generation；不复制 position 冲突 | unique/容量/游标测试            | 回滚恢复迁移前备份             |
| MIG-03 | MIG-01                     | FTS trigger/version 修订              | shadow FTS 后事务切换      | 全量批次回填，可恢复 checkpoint                           | 100k fixture count/query parity | 旧 FTS 表保留到发布验证后清理  |

二进制发现数据库 schema 高于自身支持版本时拒绝 ready，不自动降级。

## 3. 接口合同

### 3.1 API

所有业务路径以 `/api` 开头，JSON 错误统一为顶层 `error` 对象，其字段为 code、message、requestId、retryable 和可选 details。Cookie 为 `__Host-tantan_session; Secure; HttpOnly; SameSite=Lax; Path=/`；mutation 同时要求可信 Origin 和 `X-CSRF-Token`。OpenAPI 是字段、状态和错误的 canonical 机器合同。

| API ID | Provider/Consumer | 协议/版本/地址                                                                                        | Auth                                     | 请求与校验                                                                                  | 响应/状态                                                        | 错误                                                                                                                         | 幂等/并发                            | 分页/限流              | 兼容                    |
| ------ | ----------------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | ---------------------- | ----------------------- |
| API-01 | Go/Login          | GET `/api/auth/folo/providers`；POST `/api/auth/folo/social-start`、`/email`、`/two-factor`、`/token` | anonymous+exact Origin on POST           | provider allowlist；email≤254/password 8～128；TOTP 6～10 digits；token 20～4096；body≤8KiB | providers、fixed authorizeUrl、2FA challenge 或 200 session user | AUTH_PROVIDER_INVALID 400、AUTH_INVALID 401、AUTH_TOKEN_USED 409、AUTH_2FA_REQUIRED 409、AUTH_FLOW_INVALID 410、UPSTREAM 503 | token replay hash；flowId single-use | IP+identity hash 5/min | 新 v2                   |
| API-02 | Go/App            | GET `/api/tantan/v1/session`；POST `/api/auth/logout`                                                 | cookie；logout CSRF                      | timezone IANA 可选                                                                          | 200 session/204                                                  | SESSION_REQUIRED 401                                                                                                         | logout 幂等                          | 60/min                 | 旧 logout 410 后移除    |
| API-03 | Go/Home           | GET `/api/tantan/v1/topics`                                                                           | cookie                                   | revision 可选                                                                               | 200 topics+revision                                              | SESSION_REQUIRED                                                                                                             | read                                 | 120/min                | v1 DTO 加 revision      |
| API-04 | Go/Home           | GET `/api/tantan/v1/home`                                                                             | cookie                                   | topicId/filterId/cursor/limit 1～20/timezone                                                | 200 HomeResponse                                                 | CURSOR_INVALID、QUEUE_VERSION_CHANGED 409                                                                                    | generation singleflight              | cursor；120/min        | v1 字段只增不删         |
| API-05 | Go/Folo           | PUT `/api/folo/reads`                                                                                 | cookie+CSRF                              | 1～100 entryIds                                                                             | 204                                                              | FOLO_ROUTE_DENIED、UPSTREAM errors                                                                                           | idempotent                           | 30/min                 | exact adapter           |
| API-06 | Go/Filter         | PUT、DELETE `/api/tantan/v1/filter`                                                                   | cookie+CSRF                              | prompt 1～1000；Idempotency-Key                                                             | 200 snapshot/204 reset                                           | AI_NOT_CONFIGURED、AI_OUTPUT_INVALID、FILTER_CONFLICT                                                                        | key 24h；transaction                 | 10/min                 | v1 snapshot 增 revision |
| API-07 | Go/Search         | GET `/api/tantan/v1/search`                                                                           | cookie                                   | q 1～200、cursor、limit≤20                                                                  | 200 results                                                      | CURSOR_INVALID                                                                                                               | read                                 | cursor；60/min         | v1                      |
| API-08 | Go/Folo           | GET/POST/DELETE `/api/folo/subscriptions`                                                             | cookie；mutation CSRF                    | route-policy exact DTO                                                                      | upstream-normalized result                                       | FOLO_ROUTE_DENIED/UPSTREAM                                                                                                   | mutation Idempotency-Key             | 30/min                 | adapter version header  |
| API-09 | Go/Folo           | GET `/api/folo/discover`                                                                              | optional session                         | q≤200、cursor                                                                               | sources                                                          | UPSTREAM/429                                                                                                                 | read                                 | cursor；30/min         | 禁止 Trending fallback  |
| API-10 | Go/Settings       | GET `/api/tantan/v1/settings/ai-provider`；POST `/test`                                               | cookie；test CSRF                        | test 无 body；不接受 provider/model/key/URL                                                 | 只读固定 metadata；test result                                   | AI_NOT_CONFIGURED、AI_AUTH_FAILED、AI_RATE_LIMITED                                                                           | read/test                            | test 3/min/session     | Key 永不进入 HTTP       |
| API-11 | Go/Folo           | entry/feed/source/collection 精确代理                                                                 | cookie+CSRF mutation                     | exact route DTO                                                                             | normalized Folo DTO                                              | FOLO_ROUTE_DENIED/UPSTREAM                                                                                                   | collection idempotent                | route budget           | policy versioned        |
| API-12 | Go/AI             | GET、POST `/api/tantan/v1/entries/:entryId/enrichment`                                                | cookie+CSRF POST                         | kind translation/summary、locale                                                            | 200 ready/202 queued                                             | AI_NOT_CONFIGURED/AI_PROVIDER_UNAVAILABLE                                                                                    | entry+kind+locale unique             | poll Retry-After       | schemaVersion           |
| API-13 | Go/Sync           | POST `/api/tantan/v1/sync`、GET `/status`                                                             | cookie+CSRF POST                         | force=false 默认                                                                            | 202 job/status                                                   | SYNC_ALREADY_RUNNING/UPSTREAM                                                                                                | per-user singleflight                | 2/min                  | v1                      |
| API-14 | Go/Ops            | GET `/api/healthz`、`/api/readyz`、`/api/tantan/v1/diagnostics`                                       | health public；diagnostics authenticated | 无                                                                                          | status/diagnostics                                               | 503 fail-closed                                                                                                              | read                                 | 60/min                 | secret-free             |

### 3.2 事件与 Webhook

一期没有外部事件或 Webhook。内部 worker 通过持久 `jobs` 表轮询，不把内存 channel 当作真实来源；因此不定义公网事件接口。未来 push/telemetry 需要独立规格与信任边界。

### 3.3 任务与调度

| JOB ID | 触发/调度                | 输入                                   | Owner  | 超时/取消             | 幂等/并发                          | 结果/失败/重试                             | 状态/观测                      |
| ------ | ------------------------ | -------------------------------------- | ------ | --------------------- | ---------------------------------- | ------------------------------------------ | ------------------------------ |
| JOB-01 | 登录后、手动、每 15min   | user、checkpoint                       | MOD-07 | 2min；shutdown cancel | 每用户单实例；page+checkpoint 事务 | 指数 5s～15min，最多 8；401 暂停           | sync status、counts、requestId |
| JOB-02 | 新 Entry 或用户触发      | entry、kind、locale、provider revision | MOD-10 | 单次 60s              | unique key；最多 2 worker/用户     | 429 respect Retry-After；schema 失败最多 2 | queued/running/ready/failed    |
| JOB-03 | local midnight/Home 首访 | user/date/timezone/topic/filter        | MOD-09 | 30s                   | generation unique                  | 规则排序总能降级；AI 失败不阻塞规则队列    | generation/count/duration      |
| JOB-04 | 每日 03:30               | expiry thresholds                      | MOD-11 | 5min                  | 全局锁                             | 清 session/job/queue；失败次日重试         | deleted counts only            |

## 4. 可靠性与失败行为

### 4.1 错误模型

| Error ID | 原因                | 稳定错误码/状态                     | 调用方结果                                | 可重试性      | 恢复/补偿                      | 日志/指标                              |
| -------- | ------------------- | ----------------------------------- | ----------------------------------------- | ------------- | ------------------------------ | -------------------------------------- |
| ERR-01   | 无/过期 session     | SESSION_REQUIRED/401                | 登录页并保存 returnTo                     | 用户动作      | 清 cookie/cache                | route+requestId，不含 token            |
| ERR-02   | Origin/CSRF 失败    | ORIGIN_REJECTED 或 CSRF_INVALID/403 | 阻止 mutation                             | 否            | 刷新 session 获取新 CSRF       | 安全计数                               |
| ERR-03   | 路由不在白名单      | FOLO_ROUTE_DENIED/403               | 功能错误，无上游请求                      | 否            | 改合同而非实现绕过             | method+route template                  |
| ERR-04   | Folo 超时/5xx       | UPSTREAM_UNAVAILABLE/503            | 保留 cache，可重试                        | GET 有界      | worker backoff                 | status/duration                        |
| ERR-05   | 队列游标过期/错版本 | QUEUE_VERSION_CHANGED/409           | 清页面并取第一页                          | 是            | 无状态重取                     | generation only                        |
| ERR-06   | Go 未装载 AI Key    | AI_NOT_CONFIGURED/409               | 提示联系站点管理员                        | operator 动作 | 安全配置 Key 并重启/重载后重提 | provider ID only                       |
| ERR-07   | AI 输出不合 Schema  | AI_OUTPUT_INVALID/422               | Filter 保持旧 snapshot；enrichment failed | 有界          | 修复 prompt/provider 后重提    | schemaVersion/violation path，不记正文 |
| ERR-08   | DB busy/corrupt     | DB_BUSY/503 或 NOT_READY/503        | 保留 UI cache                             | busy 可重试   | integrity/restore runbook      | code/duration                          |
| ERR-09   | 图片源失败          | 不进入 Go error envelope            | 前端文字卡                                | 否            | 无                             | 不记录完整 URL query                   |

### 4.2 依赖与降级

| Dependency ID | 版本/Owner                           | 用途                            | 超时               | 重试责任                                       | 配额/成本           | 不可用行为                     | 健康检查                     |
| ------------- | ------------------------------------ | ------------------------------- | ------------------ | ---------------------------------------------- | ------------------- | ------------------------------ | ---------------------------- |
| DEP-01        | Folo API / Folo                      | auth、RSS/content mutation/sync | auth 15s、read 20s | Go GET/worker 有界；浏览器 mutation 不自动重发 | 外部策略            | 已缓存内容可读；写功能明确失败 | diagnostics metadata request |
| DEP-02        | Gemini OpenAI compatibility / Google | 翻译、摘要、分类、Filter        | 60s                | worker 有界，respect Retry-After               | 站点所有者 Key 配额 | 规则队列继续；AI 功能提示失败  | 用户触发只读配置 `/test`     |
| DEP-03        | SQLite modernc v1.56.0               | 持久状态                        | busy 5s            | repository 有界                                | 本机资源            | ready 失败                     | quick_check + migration      |
| DEP-04        | Keychain 或 master-key vault         | 密封 secret                     | 5s                 | 不自动                                         | 系统能力            | ready 失败，禁止弱化明文       | startup seal/unseal canary   |

Gemini 内置 preset：`providerId=google-gemini-openai`，endpoint 固定 `https://generativelanguage.googleapis.com/v1beta/openai`，model 固定 `gemini-3.5-flash-lite`，不发送已弃用的 `temperature`、`top_p`、`top_k`，结构化任务使用该兼容接口支持的 JSON Schema。任何用户输入 URL 都在解析前拒绝。

### 4.3 部分失败、重复、乱序与对账

同步先写页面内容再 checkpoint，同 entry upsert；重复任务不产生重复行。Folo 已读成功而本地更新失败时记录无 secret repair job，下次 sync 对账；Folo 失败则不乐观永久删除 Home card。AI 完成携带 provider revision，旧 revision 结果可保存历史但不覆盖新请求状态。Filter 事务失败不暴露半写 Topic/Queue。每天 doctor 比较 Folo mirror 计数、孤儿 FK、active Filter 和 ready Queue 引用。

## 5. 安全与隐私

### 5.1 身份认证与服务信任

Go 不把 Folo cookie/token发到浏览器。Tantan cookie 随机、HttpOnly、Secure、SameSite=Lax；登录和 mutation 校验 Origin/Host/CSRF。登录调用只允许 `https://api.folo.is` 固定 IP 解析后 HTTPS，禁止 redirect 到非同 host；不接受浏览器提供的 upstream headers。反向代理 IP allowlist 配置后才采信转发头，默认忽略。

本地 macOS 可使用 OS Keychain。服务器用 `TANTAN_MASTER_KEY_FILE` 指向 root-readable 32-byte会话主密钥，AES-256-GCM 每条 Folo session 随机 nonce、AAD 绑定 owner/kind/version；Gemini 使用独立 `gemini_api_key_file` 指向 root-readable Secret，启动时读入进程内存且不复制到 SQLite。禁止把任何 key 值放 CLI 参数、普通环境、日志或 HTTP。轮换以原子替换 Secret 文件并安全重启/重载完成。

### 5.2 授权矩阵

| 主体/角色          | 资源/操作                                      | 条件                                              | 强制执行位置             | 审计                           |
| ------------------ | ---------------------------------------------- | ------------------------------------------------- | ------------------------ | ------------------------------ |
| anonymous          | health、登录、有限 discover                    | Origin/limit；discover 不暴露私人数据             | MOD-02/03/05             | code/route/requestId           |
| authenticated user | 自己的 Entry/Subscription/Home/Search/Settings | session userId 与资源 owner 相等                  | handler+repository query | mutation result code           |
| worker             | 用户同步/AI                                    | 只使用 job owner 的 sealed ref；context 绑定 user | MOD-07/10                | jobId/providerId               |
| operator CLI       | doctor/backup/restore                          | 本机文件权限；不可通过 HTTP 调 restore            | MOD-11                   | 操作时间/结果，不含路径 secret |

### 5.3 敏感数据、加密、保留、删除与导出

高敏：Folo session token、Folo one-time token、邮箱密码、Gemini Key、master key；session token 只密封存储，one-time token 和密码不存储，master key 与 Gemini Key 位于相互独立的权限文件/Keychain 项。个人数据：email/user/profile、prompt、阅读/收藏/订阅和正文；SQLite 文件权限 0600，备份加密/权限继承，日志不记录内容。退出删除会话和 Folo token ref，不默认删除用户内容；账号数据删除命令删除 Tantan mirror/AI results/filters并执行 vacuum policy。导出只含用户内容和设置 metadata，不含 secret。

### 5.4 输入、密钥与滥用防护

全局 body≤1MiB，登录 4KiB，Filter 16KiB；JSON unknown field 默认拒绝；字符串 UTF-8、长度和控制字符校验。Folo proxy 不是通用 URL proxy：匹配 canonical method/path template 后由服务端构造固定 origin URL，剥离 Authorization/Cookie/Forwarded/Referer。AI preset 编译进代码；DNS 解析和 TLS 验证防 SSRF。日志结构化 redaction 覆盖 cookie、authorization、key、password、token、prompt、正文和 URL query。测试使用 secret canary 扫 SQLite、日志、HTTP、备份、HAR、fixtures、Git diff、build output。

## 6. 性能、容量与可观测性

### 6.1 负载与资源预算

| 指标           | 正常/峰值目标                       | 测量环境/方法              | 失败阈值              | 扩展/降级                |
| -------------- | ----------------------------------- | -------------------------- | --------------------- | ------------------------ |
| 用户规模       | 1～20 / 50 concurrent               | 单 2 vCPU/2GB 实例         | 50 时 p95 预算失败    | worker per-user fairness |
| Home API p95   | ≤200ms cached、≤2s first generation | 100k entries fixture       | >300ms/>3s            | 规则排名、后台 AI        |
| Search p95     | ≤300ms                              | 100k FTS、20 results       | >500ms                | query limit/cancel       |
| DB size        | 100k entry ≤2GB 含正文              | fixture                    | >2.5GB                | retention/vacuum         |
| Sync           | 1k items/min 不重复                 | fake Folo paginated server | <600/min 或 duplicate | checkpoint/backoff       |
| AI concurrency | 每用户 2、全局 8                    | test provider              | 超限仍发请求          | queue + 429              |

### 6.2 SLI/SLO

| SLI                 | SLO                            | 数据源               | 告警阈值     | Owner/响应         |
| ------------------- | ------------------------------ | -------------------- | ------------ | ------------------ |
| HTTP availability   | 月 99.5%（排除 operator 维护） | status class metrics | 5m 内 5xx>5% | operator/runbook   |
| Home success        | 99% 非 4xx                     | route metric         | 10m <97%     | BE                 |
| Sync freshness      | 95% 用户 ≤20min                | checkpoint age       | >45min       | worker diagnostics |
| Secret leakage      | 0                              | canary scans         | 任意 1       | 立即停发布、轮换   |
| Forbidden Folo call | 0                              | proxy policy counter | 任意 1       | fail release       |

### 6.3 日志、指标、追踪、Dashboard 与告警

每请求生成/验证 128-bit requestId，响应回 `X-Request-Id`，传给内部 job 但不传用户 secret。日志字段限 timestamp、level、requestId/jobId、module、route template、status/errorCode、duration、count。指标为 HTTP、Folo route allow/deny、queue generation、sync checkpoint age、AI status/provider preset、DB busy；禁止 label 使用 userId、entryId、query/prompt。默认本地 JSON 日志轮换，无第三方 telemetry。

### 6.4 排障入口与运行手册

`tantan-api doctor` 检查配置权限、master key seal roundtrip、迁移、quick_check、DNS/TLS、route policy hash和静态资源；`backup` 用 SQLite backup API；`restore --verify` 写临时库并检查 integrity/FK/row counts 后才要求 operator 切换。HTTP diagnostics 只返回布尔状态、版本和年龄区间，不返回路径、host IP、secret 或上游 body。

## 7. 配置、部署、发布与恢复

### 7.1 环境、配置优先级、密钥来源与健康检查

优先级：安全 CLI 显式配置路径（仅 operator）→ 配置文件 `/etc/tantan/config.yaml` → 非敏感环境变量 → 默认。核心配置：`listen=127.0.0.1:3000`、`public_origin=https://...`、`data_dir`、`static_dir`、`trusted_proxy_cidrs`、`master_key_file`、`gemini_api_key_file`、Folo 固定 base、worker limits。配置文件只保存 Secret 路径，不保存值；敏感值只可由权限文件/Keychain取得。Gemini provider/model/endpoint 不可配置，固定为批准预设。public_origin 必须 HTTPS（测试 loopback 可 HTTP），Host/Origin 精确匹配。`healthz` 仅进程存活；`readyz` 要迁移、DB、secret store、route policy、static manifest 全部就绪。

### 7.2 功能开关

| Flag            | 默认 | 关闭行为                              | 开启行为                          | Rollout        | 监控                | 删除条件                 |
| --------------- | ---- | ------------------------------------- | --------------------------------- | -------------- | ------------------- | ------------------------ |
| `ai_enrichment` | true | 隐藏翻译/摘要提交，规则 Home 正常     | 使用服务端 Gemini 配置            | 单实例配置     | AI status           | 稳定两个发布后改普通设置 |
| `ai_filter`     | true | 隐藏 Home AI 图标，不改变 Topic/Queue | 原子 Filter                       | 单实例配置     | filter success/fail | 产品永久需要则删除 flag  |
| `legacy_paths`  | true | 旧路径 404                            | 旧内部路径 307/410，不代理 secret | 仅一个发布周期 | legacy hit count    | 连续 30 天 0 hit         |

### 7.3 发布顺序、兼容窗口与回滚

备份 → doctor → 应用向前兼容 migration → 启动新 Go 并 ready → 切反向代理 → 发布 PWA 静态资源（API 向后兼容旧一版）→ mobile smoke。回滚先切旧二进制/静态资源；若旧二进制不能识别新 schema，则从发布前备份恢复到独立目录后切换，绝不在原库执行 destructive downgrade。

### 7.4 备份、恢复与灾难恢复

每日自动 SQLite consistent backup，保留 7 日 + 4 周；secret master key 单独离线备份，不与 DB 同一位置。RPO 24h、RTO 2h。季度或最终验收执行一次隔离 restore：checksum → SQLite integrity → FK → migration version → row counts → 用测试 key 解封 canary → 启动 ready；恢复演练不得使用生产 AI Key 发请求。

## 8. 后端需求追踪

| BR ID | 需求                                                                  | MOD/API/EVT/JOB/MIG ID                                 | 实现文件/符号/Schema                  | TASK ID | AC ID | TC ID        |
| ----- | --------------------------------------------------------------------- | ------------------------------------------------------ | ------------------------------------- | ------- | ----- | ------------ |
| BR-01 | Go 同源 `/api`、静态 PWA、可部署监听与可信代理                        | MOD-01、MOD-02、API-14                                 | `cmd/tantan-api`、`internal/http`     | TASK-02 | AC-11 | TC-17        |
| BR-02 | Folo Google/GitHub/Apple/Email/授权令牌登录与不透明密封会话           | MOD-03、MOD-04、API-01、API-02、MIG-01                 | `internal/auth`、`session`、`secrets` | TASK-02 | AC-12 | TC-18、TC-19 |
| BR-03 | Folo 默认拒绝精确代理且浏览器无 token                                 | MOD-05、API-05、API-08、API-09、API-11                 | `internal/folo`、route policy         | TASK-02 | AC-13 | TC-20        |
| BR-04 | 幂等同步、checkpoint、SQLite 与 FTS 搜索                              | MOD-06、MOD-07、MOD-08、API-07、API-13、MIG-03、JOB-01 | storage/sync/search/jobs              | TASK-04 | AC-14 | TC-21、TC-22 |
| BR-05 | Gemini 3.5 Flash-Lite 服务端 Secret 配置、浏览器零 Key 和 Schema 输出 | MOD-01、MOD-10、API-10、API-12、JOB-02                 | ai/enrichment/config/schemas          | TASK-05 | AC-15 | TC-23、TC-24 |
| BR-06 | 最近 7 天、50/60 上限、版本化稳定每日队列                             | MOD-09、API-03、API-04、MIG-02、JOB-03                 | home/topic/recommendation             | TASK-04 | AC-16 | TC-25、TC-26 |
| BR-07 | AI Filter 原子更新与失败保留旧 snapshot                               | MOD-09、MOD-10、API-06                                 | filter/recommendation                 | TASK-05 | AC-17 | TC-27        |
| BR-08 | 已读/收藏/订阅 mutation 与本地对账                                    | MOD-05、MOD-07、API-05、API-08、API-11                 | folo/sync                             | TASK-06 | AC-18 | TC-28        |
| BR-09 | readiness、日志脱敏、容量、备份恢复和 race                            | MOD-11、API-14、JOB-04                                 | ops/observability/runbooks            | TASK-07 | AC-19 | TC-29、TC-30 |
