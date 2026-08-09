# 后端实施规格：Tantan 本地 Go 服务

> 状态：已批准，可进入实施  
> 类型：greenfield service + Folo 外部系统兼容层  
> 规格负责人：Codex  
> 用户批准记录：Go 中间层、移除 Folo 付费 AI、用户自带 API Key、每日有限推荐队列已确认

## 1. 证据与决策记录

### 1.1 输入资料

| 资料 | 位置 | 用途 | 已验证 |
|---|---|---|---|
| PRD | `/Users/mingrui/Project/tantan/prd(5).md` | 后端能力、状态和验收来源 | 是 |
| Folo 仓库 | `/Users/mingrui/Project/Folo` | 客户端调用与认证证据 | 是，commit `3846c90...` |
| Folo SDK | npm `@follow-app/client-sdk@0.3.95` | 路由、请求/响应 TypeScript 合同 | 是，npm tarball 与 lockfile |
| 前端规格 | `2026-08-09-tantan-frontend-spec.md` | API Consumer 合同 | 是，共享 `API-*` ID |
| 总览 | `2026-08-09-tantan-实施落地方案.md` | 系统边界与阶段 | 是 |
| Agent 规格包 | `spec-package/README.md` | OpenAPI、Folo 路由策略、JSON Schema、DDL 与任务边界 | 是，机器合同优先于叙述性细节 |

### 1.2 代码、Schema 与运行证据

| 事实 | 文件/符号/命令 | 观察结果 |
|---|---|---|
| Folo API 固定生产地址 | `packages/internal/shared/src/env.common.ts` | `https://api.folo.is` |
| SDK 基础客户端 | `apps/desktop/layer/renderer/src/lib/api-client.ts` | credentials include、60s timeout、客户端/会话 Header |
| Folo Auth | `packages/internal/shared/src/auth.ts` | Better Auth，社交登录与 one-time token 插件 |
| Folo loopback 回调先例 | `apps/ssr/client/modules/login/index.ts#parseCliCallbackUrl` | 允许 `http://127.0.0.1`/`localhost` CLI callback |
| Entry 同步参数 | SDK `src/modules/entries/types.ts#EntryListRequest` | `limit`、`publishedAfter`、`publishedBefore`、`withContent`；无普通 cursor |
| Entry 正文批取 | SDK `entriesModule.stream` | `POST /entries/stream`，输入 ids，NDJSON/raw response |
| 读/收藏状态 | SDK EntryWithFeed | List 响应含 `read`、`collections` |
| 核心上游路径 | SDK modules | `/entries`、`/subscriptions`、`/reads`、`/collections`、`/feeds`、`/categories` 等 |
| 被禁路径 | SDK modules | `/ai/**`、`/wallets/**`、`/rsshub/use` |
| 本机 Go/SQLite | `go version`、`sqlite3 --version` | Go 1.26.2，CLI SQLite 3.43.2 |
| Pure Go SQLite | [pkg.go.dev modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | v1.56.0，darwin/linux/windows 含 SQLite 3.53.3 |
| OS Keychain | [pkg.go.dev go-keyring](https://pkg.go.dev/github.com/zalando/go-keyring) | v0.2.8，支持 macOS/Linux/Windows 与测试 Mock |
| 中文子串搜索 | [SQLite FTS5 Trigram](https://www.sqlite.org/fts5.html#the_trigram_tokenizer) | trigram 支持子串匹配 |

### 1.3 用户决策

| 决策 ID | 问题 | 用户确认结果 | 影响范围 |
|---|---|---|---|
| DEC-BE-001 | Folo AI/会员是否保留 | 不保留 | Proxy Policy、AI Module、测试 |
| DEC-BE-002 | AI 凭据来源 | 用户自带各种 API Key | Keychain、Provider API |
| DEC-BE-003 | 首页历史范围 | 最近7天的每日有限队列 | Queue、Ranking、完成态 |
| DEC-BE-004 | 部署 | 本期只在本机运行 | loopback、安全、运维 |

## 2. 背景、目标与边界

### 2.1 参与者与问题

- 浏览器前端：唯一产品 API Consumer。
- 本地用户：拥有 Folo 账号和自己的 AI Provider Key。
- Folo：账号、订阅、Feed/Entry、Read、Collection 的外部事实源。
- AI Provider：用户配置的 OpenAI-compatible 外部依赖。
- Tantan Go：认证桥、上游兼容代理、本地事实源与业务计算所有者。

Folo 客户端原本直接调用上游且 AI 与付费体系耦合。目标服务必须在不复制 Folo OAuth/数据源的前提下，隔离付费路径并增加本地业务能力。

### 2.2 目标结果与成功指标

- `BR-001`：前端所有 Folo 基础请求通过固定上游兼容代理，原状态码和 JSON 结构保持。
- `BR-002`：Folo Google 登录通过 loopback one-time-token 建立本地 session，上游 Token 不进入浏览器。
- `BR-003`：代理永不转发 Folo AI、Wallet、Payment、Stripe Subscription 和未知路由。
- `BR-004`：服务完整同步订阅与 Entry 历史，建立可恢复的 SQLite 缓存和中文全文索引。
- `BR-005`：服务用用户 Key 完成翻译、摘要、质量评分、Topic 和 AI Filter，不依赖 Folo 额度。
- `BR-006`：服务生成稳定的每日推荐队列，执行已读、反馈、屏蔽与多样性规则。
- `BR-007`：外部依赖失败时返回稳定错误和可用降级数据，不破坏已有缓存。
- `BR-008`：API Key、Folo Token、正文与 Prompt 不进入非必要日志或响应。

成功指标：核心 Proxy 合同 fixture 100% 匹配；禁路径上游调用数 0；10万 Entry 数据集上 Home P95≤150ms、Search P95≤300ms；`go test -race ./...` 通过。

### 2.3 范围

认证桥接、路由策略代理、Folo Sync、SQLite/FTS5、AI Provider、Enrichment Job、Topic、Filter、Daily Queue、Feedback、Search、Settings、Health/Ready、Backup/Restore、结构化日志。

### 2.4 非目标

- 自建 Google OAuth、Folo 用户/Feed 抓取后端、RSSHub 服务或远端云服务。
- Folo AI Chat、TTS、AI Task、Wallet、Power、支付、会员升级。
- 多设备 Tantan 偏好同步、多进程集群、消息队列、Redis、Postgres。
- 公开监听、局域网访问和公网部署。

### 2.5 约束、质量优先级和风险

优先级：凭据安全 > Folo 数据不损坏 > 原文可用 > 合同兼容 > 推荐质量 > AI 延迟。Folo 是不可控外部依赖；所有本地缓存必须可从 Folo 重新构建，Tantan 独有偏好必须可备份。

## 3. 当前实现与增量影响

### 3.1 当前入口、模块与依赖

Go 服务为新建；当前 Folo 前端直接通过 `FollowClient` 和 Better Auth 访问上游，并在浏览器/Electron 本地数据库保存镜像。

### 3.2 当前请求、状态和持久化链路

```text
当前：Browser → api.folo.is → Folo Store/IndexedDB
目标：Browser → Tantan Go → allowlisted api.folo.is
                      └→ SQLite/Keychain/AI Provider
```

### 3.3 复用、修改、新增与保持不变

| 类型 | 对象 | 原因 | 影响 |
|---|---|---|---|
| 复用 | Folo SDK 0.3.95 的线协议 | 前端 Store 兼容 | Proxy 不重塑核心响应 |
| 复用 | Folo one-time-token 登录 | 不复制 OAuth | Go 只桥接 session |
| 新增 | `services/tantan-api` | 本地业务后端 | Go 单进程 |
| 新增 | SQLite/FTS5 与 Keychain | 本地事实源和凭据 | 无远端数据库 |
| 保持 | Folo 用户/订阅/Entry/Read/Collection 语义 | 上游事实源 | 本地变更须上游成功后提交 |
| 拒绝 | Folo AI/Wallet/Payment/Unknown | 用户明确范围 | 403/410，零上游调用 |

### 3.4 冲突、兼容和技术债边界

- Proxy 兼容不等于重写全部 SDK 模块。白名单只覆盖产品需要的模块，未知路由 fail closed。
- 本地缓存不是 Folo 的替代事实源。Read/Collection 写入先确认上游成功，再更新缓存。
- Folo SDK 无稳定服务端版本承诺。每次升级先更新脱敏 fixture，再修改 Proxy/Sync Client。

## 4. 系统上下文与架构

### 4.1 信任边界

- Boundary A：Browser↔Loopback Go。只接受 `Host=127.0.0.1:3000|localhost:3000` 与允许 Origin；mutation 检查 Origin 和 session。
- Boundary B：Go↔Folo。目标 host 编译时固定为 `api.folo.is`/`app.folo.is`；Browser 不控制上游 URL。
- Boundary C：Go↔AI Provider。Base URL 由用户配置但经过 URL/IP 校验；Key 由 Keychain 注入。
- Boundary D：Go↔SQLite/Keychain。只对当前 OS 用户开放；文件权限 0600，目录 0700。

### 4.2 模块/服务清单

| MOD ID | 职责 | 公开接口 | 依赖 | 文件 |
|---|---|---|---|---|
| MOD-HTTP | Server、Middleware、错误、CORS、静态资源 | 全部 HTTP | 其余模块 | `internal/httpapi/**` |
| MOD-AUTH | login state、one-time token、local session | API-AUTH-*、API-SESSION | Folo、Keychain、Storage | `internal/auth/**` |
| MOD-PROXY | 路由策略、Reverse Proxy、响应观察 | API-FOLO-COMPAT | Auth、Folo | `internal/folo/proxy/**` |
| MOD-FOLO | 类型化核心 Folo Client 与合同 fixture | 内部接口 | Folo | `internal/folo/client/**` |
| MOD-STORAGE | SQLite、迁移、Repository、事务 | 内部接口 | modernc sqlite | `internal/storage/**` |
| MOD-SYNC | 订阅/Entry/正文/状态同步与索引 | API-SYNC-*、JOB-SYNC | Folo、Storage | `internal/sync/**` |
| MOD-AI | Provider、Keychain、JSON Schema、Enrichment | API-AI-*、API-ENRICHMENT-*、JOB-ENRICH | AI Provider、Storage | `internal/ai/**` |
| MOD-TOPIC | Topic 分类、合并、频道状态 | API-TOPICS/API-TOPIC-PATCH、JOB-TOPIC | AI、Storage | `internal/topic/**` |
| MOD-HOME | Filter、Ranking、Daily Queue、Feedback | API-HOME/API-FILTER-*/API-FEEDBACK、JOB-QUEUE | Storage、Topic | `internal/home/**` |
| MOD-SEARCH | FTS5 写入与查询 | API-SEARCH | Storage | `internal/search/**` |
| MOD-OPS | Health、Ready、Backup、状态、日志 | `/healthz`,`/readyz`、CLI | Storage/Keychain | `internal/ops/**` |

依赖方向：HTTP→Domain Modules→Repository/External Adapters；Domain 不进口 `net/http` 或具体 SQLite driver。

### 4.3 运行时流程

认证：Start→state cookie→Folo login→callback token→apply→Keychain 保存 upstream session→随机 local session→302 Home。任一步失败不创建 session。

同步：触发→锁定用户 sync→拉 subscriptions→按 `publishedBefore` 每页100拉 Entry metadata→每50个 id 调 `/entries/stream`→单页事务 upsert Entry/AccountEntry/FTS→更新进度→循环至不足100→完成。增量同步用 `publishedAfter=last_success_at-5min` 去重回看。

首页：Auth→保证当日 queue 存在→读取 queue items 与 Entry/Enrichment→过滤上游已读/本地 block→cursor page→响应。生成 queue 在单事务中写 Queue 与 Items，旧 queue 在成功提交前保持可用。

AI：Ensure→按 `entryId+contentHash+providerFingerprint+language` 去重→Job Worker 取任务→调用 Provider→校验 JSON→一次修复→事务写 Enrichment/Topic/FTS→完成。失败只更新 Job/Enrichment 状态。

### 4.4 部署拓扑

单机单进程：`tantan-api` 监听 `127.0.0.1:3000`；SQLite 与日志位于 `os.UserConfigDir()/Tantan`；Key 位于 OS Keychain。开发期 Vite 监听5173；本地 release 由 Go 提供构建后的静态 Web 资产。

## 5. 领域模型与数据

### 5.1 实体、值对象和不变量

| Domain ID | 类型 | 生命周期/关系 | 不变量 | 强制执行位置 |
|---|---|---|---|---|
| DOM-SESSION | LocalSession | state→active→expired/revoked | 上游 token 永不进 Browser/DB 明文 | MOD-AUTH+Keychain |
| DOM-ENTRY | FoloEntryCache | Folo 镜像 | Folo ID 全局唯一；缓存可重建 | MOD-SYNC |
| DOM-ACCOUNT-ENTRY | AccountEntry | user↔entry | Read/Collection 以上游成功状态为准 | MOD-PROXY/SYNC |
| DOM-ENRICHMENT | AI 派生数据 | entry+hash+provider+lang | content hash/provider 变化即 stale | MOD-AI |
| DOM-TOPIC | 用户频道 | core/dynamic/filter | 推荐虚拟 Topic 不入表；同用户规范名唯一 | MOD-TOPIC |
| DOM-FILTER | Active Filter | draft 不入库；每用户最多1 active | reset 不影响订阅/已读/收藏 | MOD-HOME |
| DOM-QUEUE | Daily Queue | user+localDate+filterKey | 初始50、上限60；同 key 每日唯一 | MOD-HOME |
| DOM-FEEDBACK | 推荐反馈 | append-only+blocks | idempotency key 同用户唯一 | MOD-HOME |
| DOM-JOB | Local Job | queued→running→succeeded/failed | dedupe key 非终态唯一；running 可超时回收 | Worker |

所有时间存 UTC RFC3339；Daily Queue 同时保存 IANA timezone 与 `local_date`，不得用服务端本地时间计算日期。

### 5.2 数据模型

| Schema ID | 表/对象 | 关键字段 | Key/索引 | 保留/删除 |
|---|---|---|---|---|
| DB-001 | `accounts` | user_id,name,avatar,timezone,last_success_sync_at | PK user_id | logout 清用户数据时删除 |
| DB-002 | `local_sessions` | id_hash,user_id,expires_at,last_seen_at | PK id_hash；idx expires | 到期/登出删除；token 在 Keychain |
| DB-003 | `feeds` | feed_id,title,url,image,view,updated_at | PK feed_id | 可重建 |
| DB-004 | `entries` | entry_id,feed_id,kind,title,description,content,author,url,language,media_json,published_at,content_hash | PK entry_id；idx published/feed | 清本地数据时删除 |
| DB-005 | `account_entries` | user_id,entry_id,read_at,collected_at,last_seen_at | PK(user,entry)；idx user/read/collected | 随账号缓存删除 |
| DB-006 | `entry_enrichments` | entry_id,provider_fp,language,state,translated_*,summary_*,tags_json,quality_score,content_hash,error_code | PK(entry,provider,lang) | provider/hash stale 可清理 |
| DB-007 | `topics` | topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,stable_until | UNIQUE(user,normalized_name) | 用户清数据时删除 |
| DB-008 | `entry_topics` | user_id,entry_id,topic_id,confidence,is_primary,content_hash | PK(user,entry,topic) | Entry/Topic cascade |
| DB-009 | `home_filters` | filter_id,user_id,prompt,normalized_json,status,created_at,updated_at | idx user/status；代码保证1 active | 保留历史 inactive 30条 |
| DB-010 | `daily_queues` | queue_id,user_id,local_date,filter_key,timezone,target_size=50,hard_limit=60,status,version | UNIQUE(user,date,filter_key) | 保留30天 |
| DB-011 | `daily_queue_items` | queue_id,entry_id,rank,score,score_json,state,added_at | PK(queue,entry)；UNIQUE(queue,rank) | Queue cascade |
| DB-012 | `recommendation_events` | event_id,user_id,entry_id,event_type,topic_id,source_id,idempotency_key,created_at | UNIQUE(user,idempotency_key) | 保留365天 |
| DB-013 | `recommendation_blocks` | user_id,target_type,target_id,strength,created_at | PK(user,type,target) | 用户恢复/清数据时删 |
| DB-014 | `ai_provider_configs` | provider_id,base_url,model,fingerprint,enabled,updated_at | PK provider_id；base_url 为内置预设快照；不含 Key | 删除 Provider 时删 |
| DB-015 | `jobs` | job_id,kind,dedupe_key,state,payload_json,attempts,next_run_at,error_code,timestamps | idx state/next_run；active dedupe | 成功30天、失败90天 |
| DB-016 | `sync_state` | user_id,state,cursor_json,total,processed,error_code,timestamps | PK user_id | 随账号删除 |
| DB-017 | `entry_search` FTS5 | entry_id/user_id UNINDEXED,title,translation,content,source,topics,tags | `tokenize='trigram'` | 与 Entry/Enrichment 事务同步 |
| DB-018 | `schema_migrations` | version,checksum,applied_at | PK version | 永久 |
| DB-019 | `core_topic_templates` | slug,name,sort_order | PK slug；UNIQUE name/order | 永久；只通过新迁移变更 |

### 5.3 事务、一致性和并发

- SQLite `foreign_keys=ON`、`busy_timeout=5000`；进程只创建一个 writer，AI 网络调用期间不得持有事务。
- Proxy Read/Collection mutation：上游成功→本地事务更新；上游失败→本地不变。响应观察失败只记录 reconciliation job，不修改上游响应。
- Sync 单页原子提交；崩溃后从最后成功页边界重跑，所有 upsert 幂等。
- Filter 激活、Topic 替换、Filter Queue 创建在同一事务；失败时旧 active filter/queue 不变。
- Feedback 以 `(user_id,idempotency_key)` 去重；重复请求返回第一次结果。
- Queue 生成持有 `(user,date,filterKey)` 应用锁；冲突请求等待现有结果，超时返回409。

### 5.4 迁移、回填与兼容

| MIG ID | 文件 | 变化 | 验证 | 回滚 |
|---|---|---|---|---|
| MIG-001 | `migrations/0001_core.sql` | DB-001..016/018、约束和索引 | `PRAGMA integrity_check='ok'`；schema snapshot | 本期无旧数据；失败删除未提交新 DB |
| MIG-002 | `migrations/0002_search_fts.sql` | DB-017 trigram FTS | 插入中英文 fixture 后 substring 命中 | drop FTS，不影响事实表 |
| MIG-003 | `migrations/0003_seed_core_topics.sql` | 创建 DB-019 并写入核心 Topic 模板；MOD-TOPIC 在账号首次同步时复制为用户 Topic | unique/name/order；每用户幂等复制 | 回滚只删模板表，不删用户已修改 Topic |

迁移启动前对现有 DB 复制 `.bak`；迁移文件一经应用不得修改，只能新增版本。首次历史回填由 JOB-SYNC 完成，可取消和恢复。

## 6. 接口合同

### 6.1 通用合同

所有 `/tantan/v1` JSON 响应含 `X-Request-Id`。错误以 OpenAPI 为准：

```json
{"requestId":"...","error":{"code":"STABLE_CODE","message":"面向用户的简短说明"}}
```

List cursor 是服务端签名 opaque base64url，包含 sort key、id、query hash；篡改或跨 query 使用返回 `400 CURSOR_MISMATCH`。

### 6.2 API

| API ID | 方法/地址 | Auth | 请求/校验 | 响应 | 幂等/限流/兼容 |
|---|---|---|---|---|---|
| API-AUTH-START | GET `/auth/folo/start` | 无 | Host allowlist；生成state | 302 app.folo.is login | 每IP 10/min；SameSite=Lax state；顶层导航可无 Origin |
| API-AUTH-CALLBACK | GET `/auth/folo/callback` | state cookie | query token 必填且一次性；state 只从 HttpOnly cookie 取 | local session Cookie+302 `/` | state 消费后不可复用 |
| API-AUTH-LOGOUT | POST `/auth/logout` | Session+Origin | 无 | 204 | 重复调用仍204 |
| API-SESSION | GET `/tantan/v1/session` | Session | timezone header 可更新 | `{user,timezone}` | 30s客户端缓存 |
| API-FOLO-COMPAT | Folo 原路径 | Session+Origin(mutation) | 路由白名单、大小限制 | 原 status/body/content-type | SDK 0.3.95；禁路径不转发 |
| API-HOME | GET `/tantan/v1/home` | Session | topicId必填,filterId?,cursor?,limit 1..50 | items,nextCursor,queue | Home P95≤150ms；游标绑定queue/version/topic/filter |
| API-TOPICS | GET `/tantan/v1/topics` | Session | 无 | `{topics:[...]}` | 推荐作为虚拟首项 |
| API-TOPIC-PATCH | PATCH `/tantan/v1/topics` | Session+Origin | operations≤100；推荐不可变 | `{topics:[...]}` | version 乐观并发，冲突409 |
| API-FILTER-PUT | PUT `/tantan/v1/filter` | Session+Origin | prompt trim 1..300 | filter,topics,queueId | Idempotency-Key；60s |
| API-FILTER-DELETE | DELETE `/tantan/v1/filter` | Session+Origin | 无 | topics,queueId | 重复 reset 返回当前 default |
| API-FEEDBACK | POST `/tantan/v1/recommendation/feedback` | Session+Origin | entryId/action/optional topic | `{applied:true}` | Idempotency-Key 必填 |
| API-SEARCH | GET `/tantan/v1/search` | Session | q 1..200,cursor,limit1..50 | items,nextCursor,indexStatus | P95≤300ms |
| API-ENRICHMENT-GET | GET `/tantan/v1/entries/{id}/enrichment` | Session | language IETF | state,data,error | ready 24h客户端缓存 |
| API-ENRICHMENT-ENSURE | POST 同上 | Session+Origin | fields enum≤4 | 202 jobId | Idempotency-Key；dedupe |
| API-AI-CONFIG-GET | GET `/tantan/v1/settings/ai-provider` | Session | 无 | 非密钥字段+hasApiKey | Key 永不返回 |
| API-AI-CONFIG-PUT | PUT 同上 | Session+Origin | providerId/model/apiKey?；endpoint 来自内置预设 | 非密钥字段 | 原子保存；Keychain失败不改config |
| API-AI-CONFIG-TEST | POST `.../test` | Session+Origin | 完整临时配置 | ok,latencyMs,model | 3/min；不持久化 |
| API-AI-CONFIG-DELETE | DELETE 同上 | Session+Origin | 无 | 204 | 先删Key，再删config |
| API-SYNC-STATUS | GET `/tantan/v1/sync/status` | Session | 无 | state/counts/error | 无 |
| API-SYNC-TRIGGER | POST `/tantan/v1/sync` | Session+Origin | scope enum | 202 jobId | 同 scope 非终态复用 job |

`HomeEntryCard` 公开字段固定为 `entryId,type,title,excerpt,cover,source{id,name,avatar},publishedAt,topics[{id,name}],translated`；`type` 只允许 `article|post|image|video`。响应不得包含完整正文、内部评分、FilterSpec 或 Provider 信息。

`nextCursor` 是签名的不透明 Base64URL，载荷含 `queueId,queueVersion,topicId,filterId,afterRank`。请求参数与游标不一致返回400 `CURSOR_MISMATCH`；Queue 版本失效返回409 `QUEUE_VERSION_CHANGED` 并要求客户端从第一页重取。同一版本内分页只读取持久化 rank，绝不重新调用 AI 或重新排序。

### 6.3 Proxy 路由策略

允许前缀：`better-auth/get-session|one-time-token/apply|sign-out`、`entries`、`subscriptions`、`reads`、`collections`、`feeds`、`categories`、`discover`、`profiles`、`lists`、`inboxes`、`settings`。每个前缀再按 SDK 0.3.95 的 method+path 表精确匹配。

`/ai/**`、`/wallets/**`、`/better-auth/subscription/**`、`/better-auth/stripe/**`、`/payments/**`、`/referrals/**`、`/trending/**`、`POST /rsshub/use` 返回410 `FOLO_FEATURE_REMOVED`。其他路径返回403 `FOLO_ROUTE_NOT_ALLOWED`。两类响应均不得创建上游请求。

### 6.4 任务与调度

| JOB ID | 触发/调度 | 输入 | 超时/并发 | 结果/失败/重试 | 观测 |
|---|---|---|---|---|---|
| JOB-SYNC | login、手动、每15min | user,scope | 10min/每用户1 | 页级恢复；Folo 429/5xx 最多3次 | sync_state/progress |
| JOB-CONTENT | Sync metadata 后 | entry ids≤50 | 2min/1 | NDJSON逐条 upsert；缺条重试 | processed/missing |
| JOB-ENRICH | ensure/新 Entry | entry,fields,provider fp | 90s/全局2 | 429/5xx重试2；invalid JSON修复1次 | token/latency不含内容 |
| JOB-TOPIC | 新未分类 Entry≥20或10min | entry ids≤20 | 90s/1 | 合并规范名；失败保留未分类 | classified count |
| JOB-QUEUE | 首次Home、日期变化、Filter变化、Sync完成 | user/date/filter | 30s/每key1 | 原子替换；失败保留旧queue | candidate/selected |
| JOB-RECONCILE | 本地观察写失败、启动 | user | 5min/1 | 重拉近7天 read/collection | drift corrected |

本期无 Event Broker/Webhook，`EVT-*` 不适用；Job 表提供至少一次执行，业务写入通过 dedupe key 幂等。

### 6.5 推荐、Topic 与 AI 规则

#### 6.5.1 候选与排序

候选为 `[now-168h,now]` 内未读、未屏蔽、来自当前订阅的 Entry，metadata 最新500条。分数：

```text
recency = 40 * max(0, 1 - ageHours/168)
topic_affinity = clamp(userTopicWeight, 0, 20)
source_affinity = clamp(userSourceWeight, 0, 15)
quality = enrichment ready ? clamp(qualityScore,0,15) : 5
filter_match = active filter ? clamp(matchScore,0,30) : 0
```

明确不感兴趣和 block 直接排除。按总分 desc、publishedAt desc、entryId asc 稳定排序；再执行多样性重排：前20同 Source≤3、同主Topic≤5、同Source不得连续>2。无法填满时放宽“前20数量”限制，但不放宽 block/已读/连续限制。

#### 6.5.2 队列

首次生成选前50；当天 Sync 新 Entry 时从未入队候选中追加，最多60。已读不删除 QueueItem，改 state=`read`，保证历史和完成度可审计；API-HOME 不返回 read items。次日首次请求建立新队列。默认与每个 Filter 使用不同 `filter_key`，Reset 回到当日默认队列。Topic 只是同一 Queue Version 上按稳定 `topicId` 过滤的视图；`queue.total/unread/finished` 针对当前 Topic 计算，切 Tab 不重建队列。

#### 6.5.3 Topic

核心 Topic Seed 来自 PRD：AI、Web3、3D、时事政治、前端、Agent。Dynamic Topic 由 AI 严格 JSON 输出，名称 trim 后1..20字符；Unicode NFKC+lowercase+空白折叠形成 normalized name。模型输出相似候选后再按规范名/别名字典合并。动态 Topic 至少3条未读内容才显示，`stable_until` 7天内不自动消失；pinned 永不自动隐藏。

#### 6.5.4 AI Filter

Prompt 转为 `FilterSpec v1`：`windowDays(1..30), includeTopics[], excludeTopics[], includeSources[], excludeSources[], includeTerms[], negativeTerms[], contentStyles[], languages[], weights{freshness,topicMatch,sourceAffinity,quality,diversity}`；具体长度、枚举和数值上限以 `spec-package/schemas/filter-spec-v1.schema.json` 为准。JSON Schema 校验失败后只进行一次修复；仍失败返回422 `AI_OUTPUT_INVALID`，旧 Active Filter 保持。成功路径为 `Prompt→FilterSpec→Schema校验→生成/合并Dynamic Topic→计算新Queue→单事务激活Filter/Topic/Queue`；前四步失败时旧版本保持可读。FilterSpec 只改变首页，不改变 Folo 订阅、已读、收藏或 Entry。

## 7. 可靠性与失败行为

### 7.1 错误模型

| Error ID | 状态/码 | 可重试 | 调用方结果 | 恢复/观测 |
|---|---|---|---|---|
| ERR-AUTH-001 | 401 `AUTH_REQUIRED` | 否 | 登录页 | session清理 |
| ERR-AUTH-002 | 400 `AUTH_FLOW_INVALID` | 否 | 重新登录 | state失败计数 |
| ERR-SEC-001 | 403 `ORIGIN_REJECTED` | 否 | 拒绝 | 安全日志，不记正文 |
| ERR-PROXY-001 | 403 `FOLO_ROUTE_NOT_ALLOWED` | 否 | 产品错误 | denied metric |
| ERR-PROXY-002 | 410 `FOLO_FEATURE_REMOVED` | 否 | 功能不存在 | removed metric |
| ERR-FOLO-001 | 502 `FOLO_UNAVAILABLE` | 是 | 缓存降级 | upstream latency/status |
| ERR-FOLO-002 | 429 `FOLO_RATE_LIMITED` | 是 | 稍后重试 | Retry-After |
| ERR-AI-001 | 409 `AI_NOT_CONFIGURED` | 否 | 设置入口 | config missing |
| ERR-AI-002 | 502 `AI_PROVIDER_UNAVAILABLE` | 是 | 原文可用 | provider/status/latency |
| ERR-AI-003 | 422 `AI_OUTPUT_INVALID` | 可人工重试 | 旧状态保留 | schema path，不记原文 |
| ERR-DATA-001 | 500 `LOCAL_STORAGE_ERROR` | 否 | 缓存只读/诊断 | sqlite code/requestId |
| ERR-API-001 | 400 `VALIDATION_ERROR` | 否 | 字段错误 | field list |
| ERR-API-002 | 409 `VERSION_CONFLICT` | 是（先刷新） | 刷新 Topic | current version |
| ERR-API-003 | 400 `CURSOR_MISMATCH` | 否 | 从第一页请求 | cursor/request mismatch |
| ERR-API-004 | 409 `QUEUE_VERSION_CHANGED` | 是（从第一页） | 清当前分页缓存 | queue id/version |

### 7.2 依赖与降级

| Dependency | 超时/重试责任 | 不可用行为 | 健康检查 |
|---|---|---|---|
| Folo | Proxy 60s不重试mutation；Sync 30s重试3 | 读缓存；写返回错误 | `readyz`不因Folo临时失败而down |
| AI Provider | 60s，Worker重试2 | 原文、默认quality=5、未分类 | 配置 test，不进readyz |
| SQLite | busy 5s，不自动重试业务写 | ready=false；停止worker | `SELECT 1`+integrity周期检查 |
| Keychain | 5s，不重试 | ready=false；禁止登录/AI保存 | set/get/delete临时探针或平台能力检查 |

### 7.3 部分失败、重复、乱序与对账

- NDJSON Content 缺少单条：提交其他成功条目，缺失 id 进入重试 Job；Sync 不回滚整页 metadata。
- Enrichment 部分字段成功不落 ready；以一次完整 Schema 为原子单位。
- Proxy 响应成功但缓存观察写失败：原响应保持，创建 JOB-RECONCILE。
- Entry 更新以 content_hash 判新旧；较旧 published metadata 不覆盖较新 hash。
- Queue generation version 单调递增；API cursor 包含 version，版本变化后旧 cursor 返回409 `QUEUE_VERSION_CHANGED`，前端从首屏刷新。

## 8. 安全与隐私

### 8.1 身份认证

Local Cookie `tantan_session` 为256-bit随机值，只存其 SHA-256 hash；HttpOnly、SameSite=Lax、Path=/，loopback HTTP 下不设置 Secure，未来 HTTPS 必须设置 Secure。Folo token 保存到 Keychain service `tantan.folo.session`、account=`sessionIdHash`。

### 8.2 授权矩阵

| 主体 | 资源/操作 | 条件 | 强制位置 | 审计 |
|---|---|---|---|---|
| 未登录Browser | start/callback/health | Host/Origin/state | MOD-AUTH/HTTP | code/requestId |
| 登录Browser | 本账号数据读写 | active session；Repository强制user_id | Middleware+Repo | mutation type/id hash |
| Worker | 本账号 Job | job payload user存在 | Worker Claim | job id/state |
| Proxy | Folo 核心接口 | route policy+session | MOD-PROXY | method/path/status |

### 8.3 敏感数据、加密、保留、删除与导出

- API Key 与 Folo Token：Keychain；数据库只存 fingerprint/hasKey。
- Entry 正文、Prompt、翻译：本机 SQLite 0600；不写日志。
- Logout：撤销 session、删除 Folo Token、清该账号 Entry Cache/Queue/Feedback/Topic；设备级 AI Provider Key 默认保留，并在 UI 明示“保存在本机”。“清除本机数据”同时删除 Key。
- Backup：SQLite `VACUUM INTO`；不导出 Keychain secret。Restore 时服务必须停止，校验 schema/checksum 后原子替换。

### 8.4 输入、密钥、服务间信任和滥用防护

- 服务默认只 bind `127.0.0.1`；启动参数要求公开地址时直接拒绝。
- Host allowlist 防 DNS rebinding；mutation Origin 仅允许127.0.0.1:3000/5173和localhost同端口。
- Proxy 去除 hop-by-hop、Forwarded、外部 Cookie/Authorization，重新注入受控 Header。
- AI endpoint 只来自 OpenAI、Anthropic、Google、DeepSeek、OpenRouter 内置 HTTPS 预设，浏览器不能提供 URL；自定义 endpoint 需另立威胁模型，一期禁用。
- 请求体上限2MB；Provider 响应上限4MB；Folo Entry stream上限50MB；超限稳定413/502。
- AI 输出仅作为数据校验，不执行 Markdown HTML、脚本、URL 或工具调用。

## 9. 性能、容量与可观测性

### 9.1 负载和资源预算

| 指标 | 正常/峰值目标 | 测量 | 失败阈值/降级 |
|---|---|---|---|
| 数据量 | 10万 Entry、5GB DB | 固定生成器 | >5GB告警，不自动删用户数据 |
| Home | P50≤50ms/P95≤150ms | 500候选/10万库 | >300ms记录slow query |
| Search | P50≤100ms/P95≤300ms | trigram 10万库 | >500ms限制20条并记录 |
| Proxy开销 | P95≤30ms，不含上游 | httptest/本地 mock | >60ms |
| Memory | 正常≤200MB/峰值≤300MB | runtime metrics | >400MB停止新AI Job |
| AI并发 | 2 | Worker gauge | 队列>100暂停自动enrich，只保留用户显式ensure |

### 9.2 SLI/SLO

本期本地运行采用开发 SLO：服务运行期间 Home/Search 本地请求成功率≥99.5%；Proxy 成功率单独展示且不把 Folo 5xx计为本地逻辑错误；SQLite 写失败率必须为0，否则 ready=false。

### 9.3 日志、指标和追踪

`slog` JSON 日志字段：timestamp,level,requestId,module,method,route,status,durationMs,errorCode,userHash,jobId。禁止 body、query中的q/prompt、Cookie、Authorization、Key、正文。内存 metrics 通过 `/tantan/v1/diagnostics` 返回聚合计数，不开放 pprof。

### 9.4 排障入口

- `/healthz`：进程活跃。
- `/readyz`：SQLite/Keychain/Migration ready。
- `/tantan/v1/sync/status`：同步游标和进度。
- CLI `tantan-api doctor`：端口、权限、Keychain、DB integrity、上游 DNS/TLS，只输出脱敏结果。
- CLI `tantan-api backup --output <explicit-path>`；禁止默认覆盖已有文件。

## 10. 配置、部署、发布与恢复

### 10.1 配置

| Key | 默认/格式 | 来源 | 说明 |
|---|---|---|---|
| `TANTAN_LISTEN_ADDR` | `127.0.0.1:3000` | env/flag | 非loopback拒绝 |
| `TANTAN_DATA_DIR` | `os.UserConfigDir()/Tantan` | env/flag | 测试用临时目录 |
| `TANTAN_FOLO_API_URL` | `https://api.folo.is` | compile default；测试可改为loopback mock | 正式运行只允许该host |
| `TANTAN_FOLO_WEB_URL` | `https://app.folo.is` | 同上 | Auth start |
| `TANTAN_DEV_ORIGINS` | `http://127.0.0.1:5173` | env | 逗号分隔严格URL |
| `TANTAN_LOG_LEVEL` | `info` | env | debug仍不记录秘密 |

优先级：CLI flag > env > default；secret 不接受命令行参数，避免进程列表泄漏。

### 10.2 功能开关

本期只有 `TANTAN_AUTO_ENRICH`，默认 true。false 时不创建自动 Enrichment/Topic Job，用户显式 Entry ensure 仍可用。Folo 禁路径无开关，不能恢复。

### 10.3 发布、兼容与回滚

本期只产出本地二进制。发布顺序：迁移备份→迁移→ready→前端启用。回滚二进制前必须确认其支持当前 schema；不支持则从自动备份恢复。Proxy 路由策略与 SDK 版本一起升级，不能单独放宽。

### 10.4 备份、恢复与灾难恢复

每日首次启动保留最近7份 SQLite backup；Keychain 由 OS 管理且不进入备份。DB 丢失时 Folo 数据可重同步，Tantan Feedback/Topic/Filter 只能从备份恢复。RPO 24h，RTO 30min（10万 Entry 重新索引不计入可读恢复；先恢复基本读取）。

# 依赖清单

## 11. 依赖项与实施启动门禁

### 11.1 新增、升级或重新配置

| DEP ID | 类别 | 名称/用途 | 状态 | 版本/来源 | 安装/初始化 | 影响 |
|---|---|---|---|---|---|---|
| DEP-BE-001 | Runtime | Go | 新增 | 1.26.2，本机验证 | `go version` | `go.mod` |
| DEP-BE-002 | Database Driver | `modernc.org/sqlite` | 新增 | v1.56.0，[pkg.go.dev](https://pkg.go.dev/modernc.org/sqlite) | `go get modernc.org/sqlite@v1.56.0` | go.mod/sum |
| DEP-BE-003 | Secret Store | `github.com/zalando/go-keyring` | 新增 | v0.2.8，[pkg.go.dev](https://pkg.go.dev/github.com/zalando/go-keyring) | `go get github.com/zalando/go-keyring@v0.2.8` | go.mod/sum |
| DEP-BE-004 | External API | Folo API/Web | 复用 | SDK contract 0.3.95；固定hosts | 脱敏fixture | Folo adapters |
| DEP-BE-005 | External API | OpenAI/Anthropic/Google/DeepSeek/OpenRouter | 用户选内置 Provider 并配置 model/key | preset endpoint/model/key | Settings API | AI adapter |
| DEP-BE-006 | Storage Feature | SQLite FTS5 trigram | 新增 | SQLite 3.53.3 from DEP-BE-002 | MIG-002 | Search |

`modernc.org/sqlite` 的 `libc` 使用其 go.mod 声明的精确传递版本，由 `go get`/`go mod tidy` 写入 go.sum；不得手工升级单个 modernc 传递依赖。

### 11.2 配置与启动

```bash
cd /Users/mingrui/Project/tantan/services/tantan-api
go mod download
go run ./cmd/tantan-api
curl --fail http://127.0.0.1:3000/healthz
curl --fail http://127.0.0.1:3000/readyz
```

成功：health `{"status":"ok"}`；ready HTTP200 且 `sqlite/keyring/migrations` 均为 `ok`。Keychain 探针必须创建并删除随机临时条目，不得覆盖产品 secret。

### 11.3 实施启动门禁

- [ ] `go version` 精确确认并写入构建记录。
- [ ] `go mod download` 与 `go mod verify` 通过。
- [ ] MIG-001/002 在临时目录应用且 integrity_check=ok。
- [ ] Keychain Mock 单测与本机 doctor 探针通过。
- [ ] Folo SDK 0.3.95 路由表和脱敏 response fixture 已提交。
- [ ] `/healthz`、`/readyz` 通过后才连接前端。

## 12. 实现要求与执行顺序

### 12.1 可执行任务树

- [ ] TASK-BE-00 `[串行]`：依赖、骨架与合同基线
  - 任务说明：建立 Go module、迁移、测试服务器和 Folo SDK fixture，不实现产品行为。
  - [ ] TASK-BE-00.1 `[Agent:A]` `[DEP-BE-001..006/MIG-001/MIG-002]`：创建服务骨架与门禁
    - **实现什么**：服务能启动、迁移临时DB并通过 health/ready。
    - **怎么实现**：建立目录、go.mod、config、slog、storage、embed migrations、httptest；不连接真实用户账号。
    - **怎么测试**：`go mod verify`、migration test、health/ready test、`go test ./...`。
    - **验收标准**：AC-001。
  - [ ] TASK-BE-00.2 `[Agent:A]` `[DEP-BE-004/API-FOLO-COMPAT]`：固化 Folo 0.3.95 合同
    - **实现什么**：method+path 白名单、禁列表、脱敏 request/response fixture 成为版本事实源。
    - **怎么实现**：从 npm tarball生成路由快照；fixtures 删除token、用户ID、正文和URL秘密。
    - **怎么测试**：snapshot 与SDK包hash一致；敏感扫描无命中。
    - **验收标准**：AC-002。

- [ ] TASK-BE-01 `[串行]`：认证桥与兼容代理（新增或变更行为）
  - 任务说明：按 Red→Green→Refactor→Verify 交付安全登录、核心代理和禁路由。
  - [ ] TASK-BE-01.1 `[Agent:T1]` `[BR-001/BR-002/BR-003/MOD-AUTH/MOD-PROXY/API-AUTH-*/API-FOLO-COMPAT]`：Red—认证/代理合同测试
    - **实现什么**：锁定 state、token不进Browser、响应透传、禁路径零上游调用、Host/Origin拒绝。
    - **怎么实现**：只写 httptest Folo mock、Keychain mock、proxy fixtures 与安全测试。
    - **怎么测试**：因 handler 未实现产生目标404/断言失败，不接受基础设施失败。
    - **验收标准**：AC-003..007 为目标。
  - [ ] TASK-BE-01.2 `[Agent:I1]` `[BR-001/BR-002/BR-003/MOD-AUTH/MOD-PROXY/API-AUTH-*/API-FOLO-COMPAT/DB-002]`：Green—实现认证和代理
    - **实现什么**：完整Auth流程、Keychain token、session middleware、精确路由策略和响应观察完成。
    - **怎么实现**：`net/http`+`httputil.ReverseProxy`；固定upstream；strip headers；method/path matcher；mutation Origin检查；成功写观察任务。
    - **怎么测试**：目标合同测试转绿。
    - **验收标准**：AC-003..007。
  - [ ] TASK-BE-01.3 `[Agent:I1]` `[MOD-AUTH/MOD-PROXY]`：Refactor—分离策略与传输
    - **实现什么**：route policy为纯函数，auth token provider为接口，行为不变。
    - **怎么实现**：不改变公开路径、错误码和Header。
    - **怎么测试**：目标、race与fixture保持绿色。
    - **验收标准**：AC-003..007。
  - [ ] TASK-BE-01.4 `[Agent:T1]` `[BR-001/BR-002/BR-003]`：Verify—独立安全与兼容验证
    - **实现什么**：证明核心Folo响应等价、禁路径零dial、Token不泄漏。
    - **怎么实现**：运行合同、fuzz path、DNS rebinding、header smuggling和日志扫描。
    - **怎么测试**：`go test -race ./...`及专项测试全绿。
    - **验收标准**：AC-003..007、AC-020。

- [ ] TASK-BE-02 `[串行]`：同步、缓存与搜索（新增或变更行为）
  - 任务说明：按 TDD 交付幂等全量/增量同步、FTS5 与恢复。
  - [ ] TASK-BE-02.1 `[Agent:T2]` `[BR-004/MOD-SYNC/MOD-SEARCH/JOB-SYNC/JOB-CONTENT/DB-001/003..005/016/017]`：Red—同步/FTS合同测试
    - **实现什么**：锁定publishedBefore分页、5min重叠增量、NDJSON部分失败、重启恢复和中英文子串搜索。
    - **怎么实现**：生成多页Folo fixture、断流fixture、10万数据生成器。
    - **怎么测试**：Repository/Job未实现导致有效失败。
    - **验收标准**：AC-008..011 为目标。
  - [ ] TASK-BE-02.2 `[Agent:I2]` `[BR-004/MOD-STORAGE/MOD-SYNC/MOD-SEARCH/JOB-SYNC/JOB-CONTENT/MIG-001/MIG-002]`：Green—实现同步与索引
    - **实现什么**：全历史回填、增量、正文stream、read/collection镜像、FTS query和status完成。
    - **怎么实现**：每页100 metadata、每批50正文；页级事务；显式FTS upsert；opaque cursor签名。
    - **怎么测试**：目标测试转绿，10万fixture达到预算。
    - **验收标准**：AC-008..011、AC-018。
  - [ ] TASK-BE-02.3 `[Agent:I2]` `[MOD-SYNC/MOD-SEARCH]`：Refactor—统一checkpoint与重试
    - **实现什么**：Sync/Content Job共用状态转换和退避，不改变数据结果。
    - **怎么实现**：抽取Job runner与Folo page iterator；事务边界保持。
    - **怎么测试**：崩溃注入、恢复、race测试保持绿色。
    - **验收标准**：AC-009..011。
  - [ ] TASK-BE-02.4 `[Agent:T2]` `[BR-004]`：Verify—完整回填和恢复验收
    - **实现什么**：独立执行10万Entry同步、kill/restart、FTS校验和对账。
    - **怎么实现**：固定seed；中途kill；重启；比较source/DB counts/hash。
    - **怎么测试**：无重复、无丢页、integrity ok、搜索预算达标。
    - **验收标准**：AC-008..011、AC-018。

- [ ] TASK-BE-03 `[串行]`：AI Provider、Enrichment 与 Topic（新增或变更行为）
  - 任务说明：按 TDD 交付用户Key、Provider安全调用、结构化输出和失败降级。
  - [ ] TASK-BE-03.1 `[Agent:T3]` `[BR-005/BR-008/MOD-AI/MOD-TOPIC/API-AI-*/API-ENRICHMENT-*/JOB-ENRICH/JOB-TOPIC/DB-006/007/008/014/015]`：Red—AI安全与状态测试
    - **实现什么**：锁定Keychain原子保存、无Key回传、URL/IP限制、JSON修复一次、content hash失效和并发2。
    - **怎么实现**：Keyring Mock、TLS Provider Mock、恶意DNS/响应/日志fixture。
    - **怎么测试**：目标接口未实现导致有效失败。
    - **验收标准**：AC-012..015 为目标。
  - [ ] TASK-BE-03.2 `[Agent:I3]` `[BR-005/BR-008/MOD-AI/MOD-TOPIC/API-AI-*/API-ENRICHMENT-*/JOB-ENRICH/JOB-TOPIC/DB-006..008/014/015/MIG-003]`：Green—实现AI与Topic
    - **实现什么**：Provider设置/测试、Worker、翻译/摘要/质量、Topic seed/dynamic和FTS更新完成。
    - **怎么实现**：stdlib HTTP client自定义Dial校验；Keychain接口；严格Schema；semaphore并发2；网络外不持事务。
    - **怎么测试**：目标单元/集成/安全测试转绿。
    - **验收标准**：AC-012..015。
  - [ ] TASK-BE-03.3 `[Agent:I3]` `[MOD-AI/MOD-TOPIC]`：Refactor—隔离Provider与Prompt版本
    - **实现什么**：Provider adapter、Prompt v1、Schema v1独立版本化，行为不变。
    - **怎么实现**：fingerprint包含 providerId/内置 endpoint/model/promptVersion/schemaVersion。
    - **怎么测试**：旧fingerprint stale、新fingerprint ready测试保持绿色。
    - **验收标准**：AC-013..015。
  - [ ] TASK-BE-03.4 `[Agent:T3]` `[BR-005/BR-008]`：Verify—凭据与降级审计
    - **实现什么**：证明Key/Token/正文不进DB错误字段、日志、response或Folo请求。
    - **怎么实现**：用唯一canary secret运行所有流程并全局扫描输出/DB/HAR。
    - **怎么测试**：canary只存在Keyring Mock与发送给Provider的Authorization。
    - **验收标准**：AC-012、AC-015、AC-020。

- [ ] TASK-BE-04 `[串行]`：Daily Queue、Filter 与 Feedback（新增或变更行为）
  - 任务说明：按 TDD 交付稳定排序、多样性、Filter原子切换和反馈幂等。
  - [ ] TASK-BE-04.1 `[Agent:T4]` `[BR-006/MOD-HOME/API-HOME/API-FILTER-*/API-FEEDBACK/JOB-QUEUE/DB-009..013]`：Red—推荐不变量测试
    - **实现什么**：锁定7天/500候选、50/60队列、稳定排序、多样性、重叠分页、cursor错配、已读/block排除、Filter失败不替换和反馈去重。
    - **怎么实现**：固定时钟/时区、评分与cursor fixture、并发queue/filter/feedback测试。
    - **怎么测试**：目标Domain未实现导致有效失败。
    - **验收标准**：AC-016..019 为目标。
  - [ ] TASK-BE-04.2 `[Agent:I4]` `[BR-006/MOD-HOME/API-HOME/API-FILTER-*/API-FEEDBACK/JOB-QUEUE/DB-009..013]`：Green—实现推荐闭环
    - **实现什么**：Queue生成/追加/读取、评分/重排、FilterSpec、Feedback/Block和Topic视图完成。
    - **怎么实现**：纯函数ranking+diversity；事务切换active filter/queue；cursor绑定queue version；反馈写event/block。
    - **怎么测试**：目标单元/集成/并发测试转绿。
    - **验收标准**：AC-016..019。
  - [ ] TASK-BE-04.3 `[Agent:I4]` `[MOD-HOME]`：Refactor—纯化评分和队列构建
    - **实现什么**：候选查询、评分、重排、持久化四阶段解耦，输出不变。
    - **怎么实现**：Golden fixture锁定entry顺序和score reasons。
    - **怎么测试**：Golden/并发/API测试保持绿色。
    - **验收标准**：AC-016..019。
  - [ ] TASK-BE-04.4 `[Agent:T4]` `[BR-006]`：Verify—每日完整消费验收
    - **实现什么**：独立验证跨午夜、Filter编辑/重置、新内容追加、重叠分页和读完状态。
    - **怎么实现**：Fake clock推进日期；模拟60条、连续同源内容、错配cursor和版本更换。
    - **怎么测试**：队列上限、多样性、分页稳定、历史可搜、默认queue恢复全部符合合同。
    - **验收标准**：AC-016..019。

- [ ] TASK-BE-05 `[串行]`：可靠性、备份与最终验收
  - 任务说明：按 TDD 完成本地运维、安全、故障恢复与全量验证。
  - [ ] TASK-BE-05.1 `[Agent:T5]` `[BR-007/BR-008/MOD-OPS]`：Red—故障、backup/restore与容量测试
    - **实现什么**：锁定DB busy/corrupt、Keychain失败、Folo/AI超时、备份不覆盖和日志脱敏。
    - **怎么实现**：fault injection、临时目录、canary secret、10万数据负载测试。
    - **怎么测试**：目标运维行为缺失产生有效失败。
    - **验收标准**：AC-020..023 为目标。
  - [ ] TASK-BE-05.2 `[Agent:I5]` `[BR-007/BR-008/MOD-OPS]`：Green—实现Doctor/Backup/Recovery
    - **实现什么**：ready降级、doctor、显式backup、停止态restore、日志轮转和资源保护完成。
    - **怎么实现**：VACUUM INTO临时文件后rename；目标存在拒绝覆盖；restore前checksum/schema/integrity检查。
    - **怎么测试**：目标故障与恢复测试转绿。
    - **验收标准**：AC-020..023。
  - [ ] TASK-BE-05.3 `[Agent:I5]` `[MOD-OPS/MOD-HTTP]`：Refactor—统一诊断与错误映射
    - **实现什么**：HTTP/Job/CLI共用错误码和redactor，行为不变。
    - **怎么实现**：禁止把底层error字符串直接放message。
    - **怎么测试**：Golden error/log测试保持绿色。
    - **验收标准**：AC-020..023。
  - [ ] TASK-BE-05.4 `[Agent:T5]` `[BR-001..008]`：Verify—最终独立验收
    - **实现什么**：执行全量unit/integration/contract/security/race/load/recovery验证。
    - **怎么实现**：运行第13.4命令；复核schema/API/任务/日志与前端合同一致。
    - **怎么测试**：全部命令成功，无跳过的关键测试。
    - **验收标准**：AC-001..023。

### 12.2 测试驱动开发执行规则

每个新增行为执行 Red→Green→Refactor→Verify。Red 不得修改生产、Schema或迁移；必须保存“预期失败”的命令、失败用例名和失败原因，且失败只能来自目标行为缺失。Green 只编辑声明的模块/Schema。迁移应用后不得修改。Verify 由未写目标生产代码的验证者运行 race、故障与恢复测试。

### 12.3 串并行和写入范围

TASK-BE-01→02→03→04→05 默认串行。TASK-BE-03 与 TASK-BE-04 共享 Topic/Enrichment 合同，禁止并行。每个 TDD 子树内部串行。Test Agent 只写 `_test.go`、fixture、mock；Implementation Agent 只写声明模块、对应迁移和生成Schema snapshot。

### 12.4 清理、文档和运维资产

最终提交包含：OpenAPI JSON、DB schema snapshot、Folo route policy snapshot、脱敏fixture、doctor说明、backup/restore runbook、错误码表、数据清除说明。不得提交真实 Key、Cookie、Token、Feed私有URL或用户正文。

## 13. 需求、验收与测试

### 13.1 后端需求追踪

| BR ID | MOD/API/JOB/MIG | TASK | AC | TC |
|---|---|---|---|---|
| BR-001 | PROXY/FOLO-COMPAT | BE-01 | AC-005..AC-007 | TC-005..TC-007 |
| BR-002 | AUTH/AUTH-* | BE-01 | AC-003、AC-004 | TC-003、TC-004 |
| BR-003 | PROXY/FOLO-COMPAT | BE-01 | AC-006、AC-007 | TC-006、TC-007 |
| BR-004 | STORAGE/SYNC/SEARCH/JOB-SYNC/MIG-001/002 | BE-02 | AC-008..AC-011、AC-018 | TC-008..TC-011、TC-018 |
| BR-005 | AI/TOPIC/JOB-ENRICH/TOPIC/MIG-003 | BE-03 | AC-012..AC-015 | TC-012..TC-015 |
| BR-006 | HOME/JOB-QUEUE | BE-04 | AC-016..AC-019 | TC-016..TC-019 |
| BR-007 | HTTP/OPS | BE-01..05 | AC-005、AC-011、AC-021..AC-023 | TC-005、TC-011、TC-021..TC-023 |
| BR-008 | AUTH/AI/HTTP/OPS | BE-01,03,05 | AC-004、AC-012、AC-020 | TC-004、TC-012、TC-020 |

### 13.2 验收标准

| AC ID | 精确可观察结果 |
|---|---|
| AC-001 | 新服务在临时数据目录迁移并返回health/ready 200 |
| AC-002 | SDK 0.3.95 method/path快照与包hash一致且fixture无敏感数据 |
| AC-003 | state错误/复用被拒绝，成功callback只创建一个local session |
| AC-004 | Browser/SQLite/log无Folo Token；Keychain中存在且登出删除 |
| AC-005 | 核心Folo status/body/content-type与fixture等价，proxy overhead达标 |
| AC-006 | 禁路径返回410且上游mock调用数0 |
| AC-007 | 未知路径403；Host/Origin/header攻击被拒绝且不dial上游 |
| AC-008 | 全量同步无丢页/重复，metadata/content/read/collection一致 |
| AC-009 | 增量5min重叠幂等，kill/restart从checkpoint恢复 |
| AC-010 | NDJSON部分失败保留成功条目并重试缺失条目 |
| AC-011 | 中文/英文/译文/Source/Topic/Tag搜索正确，indexStatus准确 |
| AC-012 | AI Key只存在Keychain和Provider Authorization，永不回传 |
| AC-013 | Provider timeout/retry/concurrency与URL/IP限制符合合同 |
| AC-014 | JSON一次修复后成功；二次失败旧enrichment/filter保持 |
| AC-015 | content/provider/prompt版本变化使旧enrichment stale并重新生成 |
| AC-016 | 7天/500候选生成50队列，当日最多60；同queue/version稳定分页、cursor错配拒绝、跨日新queue |
| AC-017 | 排序分数和多样性Golden顺序稳定，read/block绝不出现 |
| AC-018 | 10万Entry下Home/Search满足P95预算 |
| AC-019 | Filter原子生成Dynamic Topic并切换/重置，Feedback幂等且queue状态一致 |
| AC-020 | canary secret不出现在DB明文字段、日志、错误、响应和Folo请求 |
| AC-021 | Folo/AI故障降级且不破坏缓存；SQLite故障ready=false |
| AC-022 | backup不覆盖，restore校验后恢复Tantan偏好与队列 |
| AC-023 | doctor输出足以定位端口/DB/Keychain/DNS/TLS且完全脱敏 |

### 13.3 测试用例

`TC-001..023` 与同号 `AC-*` 一一对应。每个测试明确 Setup、Operation 和同号 AC 的唯一结果；合同测试使用 `httptest.Server`，安全测试使用 canary secret，容量测试固定随机 seed，恢复测试使用独立临时目录。真实 Folo 账号只用于人工 smoke，不进入自动测试或 fixture。

### 13.4 验证命令

| 目的 | 工作目录 | 命令 | 成功观察 |
|---|---|---|---|
| 模块校验 | `services/tantan-api` | `go mod verify` | all modules verified |
| 格式/静态 | 同上 | `test -z "$(gofmt -l .)" && go vet ./...` | 无输出/exit0 |
| 单元集成 | 同上 | `go test ./...` | 全部pass |
| Race | 同上 | `go test -race ./...` | 无race、全部pass |
| 合同 | 同上 | `go test ./internal/folo/... -run Contract` | 全部fixture pass |
| 安全 | 同上 | `go test ./internal/auth/... ./internal/folo/proxy/... ./internal/ai/... -run 'Security|Secret|Origin|RoutePolicy'` | 全部pass |
| 负载 | 同上 | `go test ./internal/home/... ./internal/search/... -run Load100K -count=1` | 达到预算 |
| 迁移 | 同上 | `go test ./internal/storage/... -run Migration` | schema/integrity pass |
| 构建 | 同上 | `go build ./cmd/tantan-api` | exit0 |
| Doctor | 同上 | `go run ./cmd/tantan-api doctor --data-dir "$(mktemp -d)"` | sqlite/keyring/dns/tls ok；临时目录仅用于此命令 |
| 规格最终门禁 | Tantan根 | `python3 /Users/mingrui/.codex/skills/backend-spec/scripts/validate_spec.py 2026-08-09-tantan-backend-spec.md --domain backend --stage final` | exit0，0 warnings |

## 14. 覆盖矩阵

| 检查项 | 状态 | 证据 | 结论/不适用原因 |
|---|---|---|---|
| BE-01 | 已确定 | 第2.1/2.2 | 参与者、问题、目标和成功指标已锁定 |
| BE-02 | 已确定 | 第2.3..2.5 | 范围、非目标、约束和优先级已锁定 |
| BE-03 | 代码证实 | 第1.2、3节 | Folo SDK、认证、请求和持久化现状已定位 |
| BE-04 | 已确定 | 第4.1 | Browser、Go、Folo、SQLite 和 Keychain 信任边界明确 |
| BE-05 | 已确定 | 第4.2/4.3 | 模块职责、依赖和运行流程已定义 |
| BE-06 | 已确定 | 第5.1 | Session、Entry、Topic、Filter、Queue 和 Job 不变量已定义 |
| BE-07 | 已确定 | 第6.1..6.3 | REST、Proxy、认证、DTO、游标和错误语义已定义 |
| BE-08 | 不适用 | 第6.4 | 本期无外部Event/Webhook；内部异步只通过持久化Job执行 |
| BE-09 | 已确定 | 第5.3、6.1/6.2 | 写入事务、幂等键、并发锁和版本冲突已定义 |
| BE-10 | 已确定 | 第5.2 | 表、字段、主键、索引和保留期已定义 |
| BE-11 | 已确定 | 第5.3、7.3 | 同步、代理和Job的部分失败、恢复与对账已定义 |
| BE-12 | 已确定 | 第5.4 | 三个迁移、校验、备份和回填路径已定义 |
| BE-13 | 已确定 | 第7.2、11节 | Folo、AI、SQLite、Keychain和库依赖的超时与降级已定义 |
| BE-14 | 已确定 | 第7.1 | 稳定错误码、重试责任和调用方行为已定义 |
| BE-15 | 已确定 | 第8节 | 身份、授权、密钥、SSRF、保留与删除已定义 |
| BE-16 | 已确定 | 第9.1 | 10万Entry容量、内存、延迟和并发预算已定义 |
| BE-17 | 已确定 | 第9.2/9.3 | SLI、日志字段、指标和脱敏规则已定义 |
| BE-18 | 已确定 | 第10.1、11.2 | 配置键、来源、优先级和启动方式已定义 |
| BE-19 | 已确定 | 第10.2..10.4 | 功能开关、升级、回滚、备份和恢复已定义 |
| BE-20 | 已确定 | 第9.4、12.4 | health、ready、doctor、runbook和交付资产已定义 |
| BE-21 | 已确定 | 第12/13节 | TDD任务树与FR/AC/TC验证闭环已定义 |
| BE-22 | 已确定 | 第15节 | 后端和一期端形态均无未决项 |

## 15. 未决问题

无。任何新的原生 App、服务器部署、外部 Webhook 或扩大 Folo 代理路由的需求，必须先修订规格包和威胁模型。
