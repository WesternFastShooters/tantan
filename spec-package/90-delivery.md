# 项目交付规格：Tantan 一期 Mobile Web/PWA

## 1. 依赖清单

### 1.1 新增、升级或重新配置

| DEP ID | 领域/类别       | 名称/用途/Owner/消费方                 | 状态     | 版本/镜像/来源                                      | 安装/初始化/迁移/生成命令                                         | 影响文件/Schema           |
| ------ | --------------- | -------------------------------------- | -------- | --------------------------------------------------- | ----------------------------------------------------------------- | ------------------------- |
| DEP-01 | Runtime         | Node/pnpm；FE build/test               | 已锁定   | Node 22、pnpm 10.17.0                               | `corepack enable && pnpm install --frozen-lockfile`               | root lockfile             |
| DEP-02 | Runtime         | Go；API build/test                     | 已锁定   | Go 1.26.2，`go.mod`                                 | `cd services/tantan-api && go mod verify`                         | `go.mod`、`go.sum`        |
| DEP-03 | Data            | modernc SQLite；storage                | 已锁定   | v1.56.0                                             | `tantan-api migrate` 由启动执行                                   | `db`、`migrations`        |
| DEP-04 | PWA             | vite-plugin-pwa；manifest/SW           | 已锁定   | 1.3.0                                               | `pnpm build:web`                                                  | Vite config、SW、manifest |
| DEP-05 | E2E             | Playwright Chromium/WebKit；手机验收   | 已锁定   | 1.61.1                                              | `pnpm --dir apps/desktop exec playwright install chromium webkit` | e2e config/tests          |
| DEP-06 | Secret          | Keychain 或 AES-GCM master-key file    | 重新配置 | OS Keychain；Go standard crypto                     | `tantan-api doctor` seal roundtrip                                | secret migration/config   |
| DEP-07 | External        | Folo API；账号/RSS/content             | 外部     | 固定 HTTPS origin + route policy hash               | fake contract first；real smoke user-triggered                    | route policy/adapters     |
| DEP-08 | External/Secret | Gemini OpenAI compatibility；服务端 AI | 外部     | endpoint/model 固定；站点 Key 由 Go Secret 配置装载 | AI status `/test`；Key 不进 HTTP/shell history/Git                | AI preset/config/schema   |

### 1.2 配置与启动

| DEP ID | 配置文件/环境键/凭据名                | 值格式与本地安全来源                                           | 端口/卷/网络         | 启动顺序与命令                    | 健康检查             | 失败信号与本地诊断/修复                                 | 精确成功结果                         |
| ------ | ------------------------------------- | -------------------------------------------------------------- | -------------------- | --------------------------------- | -------------------- | ------------------------------------------------------- | ------------------------------------ |
| DEP-03 | `TANTAN_DATA_DIR`                     | 绝对目录、0700                                                 | SQLite/backup 持久卷 | Go 启动自动迁移                   | readyz + quick_check | `tantan-api doctor`                                     | migration version 与 binary 相符     |
| DEP-06 | `TANTAN_MASTER_KEY_FILE`              | root-readable 32-byte 文件；本地可 Keychain                    | 不挂到静态目录       | secret store 在 DB 前验证         | seal/unseal canary   | 权限或长度错误使 readyz 503                             | canary 一致且日志无值                |
| DEP-07 | `TANTAN_FOLO_BASE_URL`                | 固定 `https://api.folo.is`；测试仅 loopback fixture            | 仅 Go 出站 443       | route policy 后启用 client        | diagnostics          | DNS/TLS/policy hash                                     | exact allow call 成功、deny 出站为 0 |
| DEP-08 | `gemini_api_key_file` / Keychain item | root-readable Secret 文件路径或本机 Keychain；配置文件只有引用 | 仅 Go 出站 443       | Go 启动装载，用户只能触发只读测试 | API-10 status/test   | 显式路径不可读则 readyz 503；未配置则 AI_NOT_CONFIGURED | provider/model 固定且测试返回译文    |
| DEP-01 | Vite dev proxy                        | `/api` → `http://127.0.0.1:3000`                               | Web 2233、Go 3000    | Go ready 后 `pnpm dev:web`        | 浏览器 `/api/readyz` | Vite proxy/network panel                                | 200 且无跨源请求                     |

### 1.3 本次复用

| DEP ID | 依赖                         | 已验证版本/位置                                                  | 充分性证据                                           | 验证命令与结果                   |
| ------ | ---------------------------- | ---------------------------------------------------------------- | ---------------------------------------------------- | -------------------------------- |
| DEP-09 | Folo Mobile 只读基线         | commit `3846c90b67da351b6017cd4fe9d0992b8077224e`、`apps/mobile` | 四 Tab、栈页、设置分组和安全区代码已审查             | `git diff -- apps/mobile` 必须空 |
| DEP-10 | Folo internal packages/store | workspace packages                                               | RSS subscription/feed/entry/read/favorite 模型可复用 | typecheck + targeted tests       |
| DEP-11 | 已有 Go Home/AI/Search 模块  | `services/tantan-api/internal`                                   | 旧任务已有 unit/race 测试                            | 修改前基线 `go test ./...` 记录  |
| DEP-12 | 原型视觉                     | `tantan前端原型.zip` SHA256 由 manifest 记录                     | 两列 Home、Topic、搜索/AI、Sheet 已检查              | ZIP hash 与只读 diff             |

## 2. 实施启动门禁

- [ ] 用当前 v2 `spec.yaml`、OpenAPI、Schema、DDL 和 route policy 生成并验证合同。
- [ ] 记录 `git status`、Folo 基线、`apps/mobile` 和原型 ZIP hash；保护用户现有修改。
- [ ] 运行前端/Go 当前基线，区分既有失败与目标 Red 失败。
- [ ] 建立安全本地测试配置、临时数据库和 fake Folo/Gemini；fixture 不含真实 secret。
- [ ] Go `/api/readyz` 通过后才开始浏览器集成；浏览器 network 必须无直接 Folo/AI。

## 3. 可执行任务树

- [ ] TASK-01 [串行]：修订并锁定 v2 机器合同和生成代码
  - 关联：FR-02、FR-04、FR-05、BR-01、BR-02、BR-03、BR-05、BR-06、BR-07、XR-01、XR-03。
  - 写入：`spec-package`、Go/TS generated DTO、合同生成与测试；不改业务生产逻辑。
  - Red：先让现有 validator/contract tests 对 Mobile-only、`/api`、Folo 全登录桥、revision 和密封 secret 合同产生预期失败证据。
  - Green：更新 OpenAPI、Schema、DDL、route policy、manifest、task manifest、生成器和 DTO，使 canonical 文件与生成结果一致。
  - Refactor：合并重复枚举和路径常量，保持合同字节/规范化快照稳定。
  - Verify：运行两个规格 validator、生成幂等 diff、空库迁移、OpenAPI/Schema 解析和 policy hash 测试。
  - 验收：AC-02、AC-04、AC-05、AC-11、AC-12、AC-13、AC-15、AC-16、AC-17、AC-20、AC-22；TC-17、TC-20、TC-23、TC-25、TC-27、TC-31。

- [ ] TASK-02 [串行，依赖 TASK-01]：实现可部署 Go 边界、Folo 全登录桥、密封会话和精确代理
  - 关联：BR-01、BR-02、BR-03。
  - 写入：`cmd/tantan-api`、`internal/http`、`auth`、`session`、`secrets`、`folo`、MIG-01 和对应测试/证据。
  - Red：添加服务器 public-origin、trusted-proxy、provider allowlist、social one-time-token、email/TOTP Set-Cookie capture、password/token/TOTP redaction、CSRF 和 deny-before-upstream 的失败测试。
  - Green：实现同源 `/api`、静态 SPA、Google/GitHub/Apple 官方登录跳转、Email/TOTP/授权令牌兑换、登录/退出、Keychain/vault secret store、固定 Folo client 和 exact adapter。
  - Refactor：把边界校验集中到 middleware/policy，保持 error envelope 和 handler 行为不变。
  - Verify：auth/folo/http/secrets race、fake upstream、canary、listener/static/ready integration 全通过。
  - 验收：AC-11、AC-12、AC-13；TC-17～TC-20。

- [ ] TASK-03 [串行，依赖 TASK-02]：重建 Folo Mobile Web 壳、四 Tab、栈导航与登录边界
  - 关联：FR-01、FR-08、XR-02。
  - 写入：renderer shell/router/auth/service-status/styles、PWA manifest、对应 Vitest/E2E；`apps/mobile` 只读。
  - Red：手机 viewport 测试先证明旧三 Tab、桌面侧栏、直连 Folo 和 loopback login 失败。
  - Green：实现四 Tab blur/safe-area、移动顶栏/栈路由、与 Folo 同入口的 provider/Email/授权令牌登录面板、同源 API client、service boundary 和 scroll state。
  - Refactor：提取 MobileSurface、Header、TabBar、StackPage primitives，保持截图和 a11y 行为。
  - Verify：Vitest、typecheck、390/430 Playwright、network origin assertions、`apps/mobile` diff 空。
  - 验收：AC-01、AC-08、AC-20；TC-01、TC-02、TC-13、TC-14、TC-32。

- [ ] TASK-04 [串行，依赖 TASK-02 和 TASK-03]：交付同步、搜索索引、Topic 和稳定每日 Home 队列
  - 关联：FR-02、FR-03、BR-04、BR-06。
  - 写入：Go storage/sync/search/topic/home/recommendation/jobs、MIG-02/MIG-03；FE Home query/model/components 测试与实现。
  - Red：添加 7 日边界、50/60、并发首次生成、cursor 绑定、分页稳定、entryId 去重、图片失败和 Topic scroll 的失败测试。
  - Green：实现 checkpoint/FTS、generation queue 和两列 Home/Topic。
  - Refactor：分离候选、排名、持久队列和 view mapping，行为保持。
  - Verify：100k fixture、race、Home contract、Vitest、Playwright 长滚动/重复/CLS。
  - 验收：AC-02、AC-03、AC-14、AC-16、AC-21、AC-23；TC-04～TC-06、TC-21、TC-22、TC-25、TC-26、TC-33。

- [ ] TASK-05 [串行，依赖 TASK-04]：交付服务端 Gemini、翻译摘要、普通搜索和 AI Filter
  - 关联：FR-04、FR-05、BR-05、BR-07、XR-03。
  - 写入：Go ai/enrichment/filter/config/settings；FE search/filter/settings/entry AI；Schema 和测试按 TASK-01 合同。
  - Red：添加 preset SSRF、服务端 Secret canary、浏览器 Key 字段拒绝、Gemini 参数、Schema invalid、Filter crash atomicity、搜索不改 Home 的失败测试。
  - Green：实现固定 Gemini endpoint/model、服务端 Secret 装载、只读 metadata settings、AI jobs、SearchPage 和 Filter Sheet 原子 snapshot。
  - Refactor：共享 AI structured-output runner/error mapping，保持 Provider/DTO 行为。
  - Verify：fake Gemini 全分支、operator 注入轮换后 Key 并由设置页触发只读 test、Vitest/E2E、所有 secret scan。
  - 验收：AC-04、AC-05、AC-15、AC-17、AC-22、AC-24；TC-07、TC-08、TC-23、TC-24、TC-27、TC-35。

- [ ] TASK-06 [串行，依赖 TASK-04 和 TASK-05]：补齐订阅、发现、设置、详情与 Folo 状态协同
  - 关联：FR-06、FR-07、FR-09、BR-08、XR-04。
  - 写入：FE subscriptions/discover/settings/entry/source/favorites，Go Folo adapters/sync repair，route policy 只经 TASK-01 合同修改。
  - Red：用 Folo Mobile 行为特征测试证明四域缺项、付费 CTA、read cache 不一致和 mutation 失败乐观状态。
  - Green：实现移动 pager/list/grouped settings/reader，保留服务端 AI 状态和摘要/翻译，移除 Plan/Power/Wallet/Upgrade/AI Chat 入口，已读/收藏/订阅走 Go。
  - Refactor：复用 Mobile list/card/header primitives 和 mutation invalidation helper，保持视觉/状态。
  - Verify：组件测试、Folo fake integration、核心 E2E、禁止路由计数为 0；RSS subscription store 存在性测试通过。
  - 验收：AC-06、AC-07、AC-09、AC-18、AC-21；TC-09～TC-12、TC-28、TC-34。

- [ ] TASK-07 [串行，依赖 TASK-02 至 TASK-06]：安全、运维、容量和恢复硬化
  - 关联：BR-09、XR-01、XR-05。
  - 写入：ops/observability/security tests/runbooks、CSP/SW、扫描脚本；不扩大产品功能。
  - Red：注入 secret canary、恶意 Origin/URL/path、corrupt DB、缺 master key、forbidden Folo route、API cache，确认门禁能失败。
  - Green：实现 doctor、redaction、backup/restore、readiness、CSP、no-API-cache 和资源预算检查。
  - Refactor：统一诊断和测试 fixture，保持安全失败语义。
  - Verify：race、100k 容量、restore drill、日志/HAR/DB/backup/build/Git scan、forbidden outbound=0。
  - 验收：AC-10、AC-19、AC-24；TC-15、TC-24、TC-29～TC-31、TC-35。

- [ ] TASK-08 [串行，依赖 TASK-07]：生产构建、真实手机验收和发布就绪
  - 关联：FR-10、XR-06。
  - 写入：E2E、verification evidence、runbook、启动器；仅修复验收发现的范围内缺陷。
  - Red：在生产构建上执行完整验收，任何失败先保存可复现证据。
  - Green：逐项修复目标缺陷并让相同测试通过。
  - Refactor：只做行为保持的启动/证据整理。
  - Verify：全量命令、390/430 Chromium+WebKit、真实手机 HTTPS/PWA、46 项旧矩阵映射和 v2 AC-01～AC-25 全有证据。
  - 验收：AC-01～AC-25；TC-01～TC-36。

## 4. 测试驱动开发执行规则

- 所有新增或变更行为按同一行为串行执行 Red → Green → Refactor → Verify；行为保持改动在编辑前后跑同一特征测试。
- Red 只编辑测试、fixture 和证据，预期失败必须由目标行为缺失或错误导致；基础设施、语法、导入或环境失败无效。
- Green 只做让目标合同通过的最小生产实现；不得修改 OpenAPI、Schema、迁移或 route policy 来迁就实现。
- Refactor 不改变用户可见行为、DTO、错误、数据或运维合同。
- Verify 复查实际 diff 并运行目标、受影响、合同、集成、race、安全、构建和手机验收；保存 Red 失败与最终通过证据。
- 一次只推进任务树中的一个 TASK；写入边界以更新后的 `agent/task-manifest.json` 为机器门禁。

## 5. 验收标准

| AC ID | 覆盖 FR/BR/XR | 前置条件                                  | 操作                                                                           | 精确可观察结果                                                                                                                               |
| ----- | ------------- | ----------------------------------------- | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| AC-01 | FR-01         | 390×844/430×932、已登录                   | 浏览四主页面和详情返回                                                         | 只有首页/订阅/发现/设置四 Tab；无桌面侧栏；safe-area 不遮挡；详情栈恢复位置                                                                  |
| AC-02 | FR-02         | 60 条混合卡 fixture                       | 连续滚动全部分页                                                               | 固定 2 列；entryId 无重复；已渲染卡不跳位；尾部显示“今天已经看完”                                                                            |
| AC-03 | FR-03         | 至少 3 Topic                              | 切换并回到旧 Topic                                                             | query/scroll 各自恢复，Topic 用 id 请求而非名称                                                                                              |
| AC-04 | FR-04         | Home 已滚动并有 filter                    | 搜索、打开结果、返回                                                           | URL 为 `/search`；Home topic/filter/generation/scroll 不变                                                                                   |
| AC-05 | FR-05         | 已配置 AI                                 | 打开/取消/成功/失败/重置 Filter                                                | 打开不请求；取消无副作用；成功三 revision 一致；失败旧首页不变；重置回 recommend                                                             |
| AC-06 | FR-06         | Home 有未读 entry                         | 打开详情并完成已读                                                             | 详情正确；成功后所有 Home cache 无该 entry；失败则仍在                                                                                       |
| AC-07 | FR-07         | fake Folo 有订阅和发现结果                | 浏览订阅/发现/设置                                                             | 信息架构和移动组件符合 Folo Mobile；订阅可增删；发现可搜索                                                                                   |
| AC-08 | FR-08         | 未登录、fake Folo                         | Google/GitHub/Apple handoff、Email/TOTP、授权令牌、错误、退出、Go down/recover | 五类 Folo 入口都可用；authorize URL 固定为 Folo；密码/TOTP/令牌不残留；session 恢复 returnTo；服务异常可原地重试                             |
| AC-09 | FR-09         | 全路由                                    | 扫 UI、生成路由和网络                                                          | Plan/Power/Wallet/Upgrade/付费 CTA/额度门槛/Folo AI Chat 为 0；服务端 AI 状态入口存在且无 Key 输入                                           |
| AC-10 | FR-10         | production PWA                            | mobile a11y/perf/offline 测试                                                  | axe serious/critical=0；预算内；离线只显示壳/cache；API 不在 SW cache                                                                        |
| AC-11 | BR-01         | production config                         | 启动 Go/静态资源/代理                                                          | 只监听配置 loopback；`/api/readyz` 200；SPA reload 成功；错误 origin 被拒绝                                                                  |
| AC-12 | BR-02         | fake Better Auth/one-time-token           | 走 provider start、Email、授权令牌并扫描响应/DB/log                            | provider 仅 allowlist；Email/token 都得到密封 Folo session；浏览器只得 Tantan cookie/user DTO；password/one-time/session token 明文出现 0 次 |
| AC-13 | BR-03         | allow/deny 路由集合                       | 调用每条路由                                                                   | allow 精确转发；未知/禁用 method/path 在出站前 FOLO_ROUTE_DENIED；禁用上游计数 0                                                             |
| AC-14 | BR-04         | 100k entry fixture                        | 中断/恢复 sync 并搜索各字段                                                    | 无重复；从已提交 checkpoint 恢复；原文/译文/Source/Topic/Tag 均命中；p95 达标                                                                |
| AC-15 | BR-05         | operator 通过服务端 Secret 配置轮换后 Key | 测只读连接、Gemini translation/schema failure                                  | 浏览器不提交 Key；使用固定 endpoint 和 `gemini-3.5-flash-lite`；不发弃用 sampling 参数；Schema 失败不提交                                    |
| AC-16 | BR-06         | timezone 边界和 7 日 fixture              | 并发首次 Home、追加、翻页                                                      | 单 generation；候选准确；初始≤50、当日≤60；position 稳定；旧 cursor 409                                                                      |
| AC-17 | BR-07         | active old filter                         | 注入 AI/DB crash 后再成功                                                      | 失败后 old filter/topics/queue 完整；成功时一次性切新 revision                                                                               |
| AC-18 | BR-08         | fake Folo mutation                        | read/favorite/subscribe 重复与失败                                             | 幂等；失败不永久乐观写；repair/sync 最终一致                                                                                                 |
| AC-19 | BR-09         | 独立 restore dir                          | doctor、backup、corrupt/restore、race                                          | secret-free doctor；缺密钥/坏迁移 ready 503；restore integrity/FK/row count；race 通过                                                       |
| AC-20 | XR-01、XR-02  | E2E 网络记录                              | 登录并走全核心路径                                                             | 非静态业务请求只到当前 origin `/api`；Folo/AI direct=0；requestId 可关联错误                                                                 |
| AC-21 | XR-04         | Home/详情/fake Folo                       | mark read 成功与失败                                                           | 三层状态按 CASE-08 一致，刷新后不反弹                                                                                                        |
| AC-22 | XR-03         | Filter E2E                                | success/version conflict/failure                                               | FE 只显示一个一致 snapshot；409 自动取第一页且不混页                                                                                         |
| AC-23 | BR-04、FR-04  | sync/search fixture                       | 搜索并快速改词                                                                 | 250ms 防抖，旧请求 abort，结果字段完整，Home 无 mutation                                                                                     |
| AC-24 | XR-05         | unique secret canary                      | 从 Go Secret 来源装载并运行全扫描和真实 AI test                                | browser request/storage、URL、HTTP response、SQLite 明文、log、HAR、fixture、Git、build、backup 命中均为 0                                   |
| AC-25 | XR-06         | production build + HTTPS 手机             | Safari/Chrome 安装 PWA并走核心路径                                             | 启动、登录、Home、Topic、搜索、Filter、详情、订阅、发现、设置和重启恢复全部通过                                                              |

## 6. 测试用例

| TC ID | 层级               | 覆盖 Requirement/AC       | Setup/Input                                 | Operation                                                 | 精确期望结果                                                    |
| ----- | ------------------ | ------------------------- | ------------------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------- |
| TC-01 | FE unit/E2E        | FR-01/AC-01               | 390/430 routes                              | 点四 Tab                                                  | active icon/label/route 正确，无第五项                          |
| TC-02 | FE visual/a11y     | FR-01/AC-01               | dark/light、safe areas                      | 截图+axe                                                  | 与基准阈值一致，无遮挡/严重问题                                 |
| TC-03 | FE unit            | FR-01/AC-01               | HomeHeader                                  | 触发图标                                                  | search 导航；AI 只开 Sheet                                      |
| TC-04 | FE integration     | FR-03/AC-03               | 3 topics/pages                              | 切换/返回                                                 | 独立 query 和 scroll 恢复                                       |
| TC-05 | FE integration     | FR-02/AC-02               | duplicate boundary/images                   | 分页/图片失败                                             | 去重、稳定、文字卡                                              |
| TC-06 | FE performance     | FR-02/AC-02               | 500 cards                                   | 滚动到底                                                  | 请求不重复、内存/CLS 预算通过                                   |
| TC-07 | FE/BE E2E          | FR-05/AC-05               | old filter + fake AI                        | cancel/fail/success/reset                                 | 原子状态合同通过                                                |
| TC-08 | FE/BE E2E          | FR-04/AC-04               | Home state + FTS                            | search/back                                               | 结果正确，Home 完整恢复                                         |
| TC-09 | FE integration     | FR-07/AC-07               | subscriptions fixture                       | view/add/remove/source                                    | Folo Mobile 风格和 mutation 正确                                |
| TC-10 | FE integration     | FR-07/AC-07               | discover fixture                            | search/subscribe                                          | 结果/空/错误和返回状态正确                                      |
| TC-11 | FE policy          | FR-07、FR-09/AC-07、AC-09 | all settings/routes                         | render/scan                                               | 非付费分组存在，付费入口为 0                                    |
| TC-12 | FE/BE integration  | FR-06、FR-09/AC-06、AC-09 | entry fixture                               | read/favorite/translate/summary                           | 状态和服务端 AI 行为正确                                        |
| TC-13 | FE/BE E2E          | FR-08/AC-08               | fake provider/email/TOTP/token auth         | provider handoff、wrong/correct token、email/TOTP、logout | 所有 Folo 入口与 form/session/cache 安全转换                    |
| TC-14 | FE E2E             | FR-08/AC-08               | Go unavailable then ready                   | retry                                                     | shell 保留并恢复，不展示静态假内容                              |
| TC-15 | FE security        | FR-10/AC-10               | built PWA                                   | inspect CSP/SW/storage                                    | connect-src self，API/secret 不缓存                             |
| TC-16 | FE production E2E  | FR-10、XR-06/AC-10、AC-25 | HTTPS 390/430 Chromium/WebKit               | install/smoke                                             | 所有核心路径通过                                                |
| TC-17 | BE integration     | BR-01/AC-11               | production config                           | start/reload/bad origin                                   | static/API/ready/host 行为精确                                  |
| TC-18 | BE auth            | BR-02、XR-02/AC-12、AC-20 | fake providers/Better Auth/one-time cookies | social start、email、token、session、logout               | fixed Folo URL；only Tantan cookie browser-visible              |
| TC-19 | BE security        | BR-02/AC-12               | password/one-time/session token canary      | replay token并扫 response/DB/log                          | replay 被拒；明文 0 match，ciphertext 非明文                    |
| TC-20 | BE policy          | BR-03/AC-13               | allow/deny matrix                           | all method/path variants                                  | exact allow；deny upstream count 0                              |
| TC-21 | BE sync            | BR-04/AC-14               | paged duplicate fixture                     | crash/resume/repeat                                       | checkpoint/row idempotency                                      |
| TC-22 | BE search/capacity | BR-04/AC-14               | 100k multi-field data                       | queries                                                   | fields/p95/limit 达标                                           |
| TC-23 | BE AI              | BR-05/AC-15               | fake Gemini recorder                        | translate/classify/filter                                 | endpoint/model/params/schema 正确                               |
| TC-24 | BE security        | BR-05、BR-09/AC-15、AC-24 | Secret source canary                        | startup/test/backup/scan                                  | 浏览器和 SQLite plaintext 0；显式 Secret 路径不可读 fail-closed |
| TC-25 | BE Home            | BR-06/AC-16               | 8-day timezone fixture                      | first/append/page                                         | 7-day、50/60、stable                                            |
| TC-26 | BE concurrency     | BR-06/AC-16               | 20 parallel first requests                  | race/test                                                 | one generation，no duplicate position                           |
| TC-27 | BE/FE Filter       | BR-07、XR-03/AC-17、AC-22 | crash points/version                        | submit                                                    | old snapshot or complete new snapshot only                      |
| TC-28 | BE mutation        | BR-08/AC-18               | fake Folo faults                            | repeat/fail/repair                                        | idempotent/final consistency                                    |
| TC-29 | BE ops             | BR-09/AC-19               | health failure matrix                       | doctor/ready                                              | redacted status and fail-closed                                 |
| TC-30 | BE recovery        | BR-09、XR-06/AC-19、AC-25 | backup copy                                 | restore verify/start                                      | checksum/integrity/FK/count/ready                               |
| TC-31 | Cross security     | XR-01/AC-20               | full HAR + egress recorder                  | core flows                                                | browser cross-origin=0、forbidden egress=0                      |
| TC-32 | Cross auth         | XR-02/AC-20               | protected returnTo                          | login/reload/back                                         | identity and navigation consistent                              |
| TC-33 | Cross queue        | XR-03/AC-22               | delayed old page                            | filter switch then response                               | stale page discarded，不混 generation                           |
| TC-34 | Cross read         | XR-04/AC-21               | multiple Home caches                        | read success/fail                                         | all cache and server status consistent                          |
| TC-35 | Cross secret       | XR-05/AC-24               | rotated real Key + canary                   | live test + scans                                         | translation success；all scan 0                                 |
| TC-36 | Manual mobile      | XR-06/AC-25               | real iOS Safari/Android Chrome equivalent   | acceptance checklist                                      | touch/keyboard/safe-area/PWA/restart 全通过                     |

## 7. 验证命令

| 目的            | 工作目录              | 命令                                                                                                                                   | 精确成功观察                                                      |
| --------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| v2 规格最终门禁 | repo root             | `python3 /Users/mingrui/.agents/skills/development-scenarios/project-spec/scripts/validate_spec_package.py spec-package --stage final` | PASS，0 errors                                                    |
| 机器合同        | repo root             | `bash spec-package/scripts/validate-package.sh`                                                                                        | package validation passed                                         |
| 前端类型        | repo root             | `pnpm typecheck`                                                                                                                       | exit 0                                                            |
| 前端 lint       | repo root             | `pnpm lint`                                                                                                                            | exit 0                                                            |
| 前端 unit       | repo root             | `pnpm --filter @follow/web test -- --run`                                                                                              | exit 0，无失败                                                    |
| Mobile Web E2E  | repo root             | `pnpm --dir apps/desktop e2e:web -- --grep Tantan`                                                                                     | Chromium/WebKit mobile projects 通过                              |
| PWA 构建        | repo root             | `pnpm build:web`                                                                                                                       | exit 0，生成 production assets                                    |
| Go 格式/依赖    | `services/tantan-api` | `go mod verify && test -z "$(gofmt -l .)"`                                                                                             | modules verified；无未格式化文件                                  |
| Go 静态/单测    | `services/tantan-api` | `go vet ./... && go test ./...`                                                                                                        | exit 0                                                            |
| Go race         | `services/tantan-api` | `go test -race ./...`                                                                                                                  | exit 0，无 race                                                   |
| Go build        | `services/tantan-api` | `go build ./cmd/tantan-api`                                                                                                            | exit 0                                                            |
| Secret/egress   | repo root             | `bash spec-package/scripts/verify-security.sh`                                                                                         | secret matches=0；forbidden Folo calls=0；direct browser egress=0 |
| 只读边界        | repo root             | `git diff --exit-code -- apps/mobile && git -C /Users/mingrui/Project/Folo status --short`                                             | 第一命令 exit 0；Folo 无本任务修改                                |

## 8. 覆盖矩阵

### 8.1 项目公共覆盖

| 检查项 | 状态     | 证据                          | 结论或不适用原因                           |
| ------ | -------- | ----------------------------- | ------------------------------------------ |
| PJ-01  | 已确定   | `00-project.md` 1.2、3.3、3.4 | 前后端、合同、外部和非目标边界明确         |
| PJ-02  | 已确定   | `00-project.md` 2.1～2.3      | PRD、原型、代码和用户决策可追溯            |
| PJ-03  | 代码证实 | EV-06～EV-12                  | 当前入口、偏差、登录冲突和可复用模块已定位 |
| PJ-04  | 已确定   | `00-project.md` 3.2           | 成功指标和 guardrail 可量化                |
| PJ-05  | 已确定   | `00-project.md` 4.2           | 复用、修改、新增、删除和不变项明确         |
| PJ-06  | 已确定   | `00-project.md` 5.1～5.3      | 系统关系、数据流、构建和部署明确           |
| PJ-07  | 已确定   | 本文 1、2                     | 依赖、配置、启动和前置门禁完整             |
| PJ-08  | 已确定   | 本文 3、4                     | 任务依赖和每项 TDD 阶段明确                |
| PJ-09  | 已确定   | 本文 5～7                     | AC、TC 和验证命令均为精确结果              |
| PJ-10  | 已确定   | `00-project.md` 6             | 风险、Owner 和缓解明确                     |

### 8.2 前端覆盖

| 检查项 | 状态     | 证据                               | 结论或不适用原因                         |
| ------ | -------- | ---------------------------------- | ---------------------------------------- |
| FE-01  | 已确定   | `10-frontend.md` 1                 | 8 条用户旅程含恢复和 URL                 |
| FE-02  | 已确定   | `10-frontend.md` 2.1               | 10 个操作合同覆盖 pending/失败/取消/输入 |
| FE-03  | 已确定   | `10-frontend.md` 2.2～2.3          | UI 状态和 Home/Filter 状态机明确         |
| FE-04  | 资料证实 | `10-frontend.md` 3.1、EV-01、EV-02 | Folo Mobile 和原型视觉来源明确           |
| FE-05  | 已确定   | `10-frontend.md` 3.2               | 字体、主题、图标、图片和动效明确         |
| FE-06  | 已确定   | `10-frontend.md` 3.3               | 精确文案和 i18n 行为明确                 |
| FE-07  | 已确定   | `10-frontend.md` 3.4               | 手机视口、PWA、键盘和大屏非目标明确      |
| FE-08  | 已确定   | `10-frontend.md` 4.1               | 路由和组件树完整                         |
| FE-09  | 已确定   | `10-frontend.md` 4.2               | 12 个组件合同含文件和测试                |
| FE-10  | 已确定   | `10-frontend.md` 5.1～5.2          | 状态唯一来源、更新、失效和数据流明确     |
| FE-11  | 已确定   | `10-frontend.md` 5.3               | 12 个 API 消费合同含重试、缓存、Auth     |
| FE-12  | 已确定   | `10-frontend.md` 5.4               | 登录、Filter、搜索和 Key 表单完整        |
| FE-13  | 已确定   | `10-frontend.md` 6.1               | WCAG、焦点、语义和 axe 门禁明确          |
| FE-14  | 已确定   | `10-frontend.md` 6.2               | CSP、存储、SW、HTML 和 secret 边界明确   |
| FE-15  | 已确定   | `10-frontend.md` 6.3               | JS/LCP/CLS/input/memory/request 预算明确 |
| FE-16  | 已确定   | `10-frontend.md` 6.4、7            | 监控限制和 FR 追踪完整                   |

### 8.3 后端覆盖

| 检查项 | 状态   | 证据                     | 结论或不适用原因                            |
| ------ | ------ | ------------------------ | ------------------------------------------- |
| BE-01  | 已确定 | `20-backend.md` 1.1      | 系统和信任边界明确                          |
| BE-02  | 已确定 | `20-backend.md` 1.2      | 11 个模块职责/依赖/位置明确                 |
| BE-03  | 已确定 | `20-backend.md` 1.3      | 启动、登录、Home 和部署流程明确             |
| BE-04  | 已确定 | `20-backend.md` 2.1      | 8 个领域对象和不变量明确                    |
| BE-05  | 已确定 | `20-backend.md` 2.2      | 9 个 Schema 的 key/index/保留明确           |
| BE-06  | 已确定 | `20-backend.md` 2.3      | 事务、并发、幂等和 checkpoint 明确          |
| BE-07  | 已确定 | `20-backend.md` 2.4      | 3 个向前迁移和恢复策略明确                  |
| BE-08  | 已确定 | `20-backend.md` 3.1      | 14 个 API 合同含错误/分页/限流              |
| BE-09  | 不适用 | `20-backend.md` 3.2      | 一期无外部事件/Webhook，持久 job 取代事件   |
| BE-10  | 已确定 | `20-backend.md` 3.3      | 4 个任务含调度、幂等、超时和重试            |
| BE-11  | 已确定 | `20-backend.md` 4.1      | 9 类稳定错误和恢复明确                      |
| BE-12  | 已确定 | `20-backend.md` 4.2～4.3 | 依赖降级、部分失败和对账明确                |
| BE-13  | 已确定 | `20-backend.md` 5.1～5.2 | session、proxy、secret trust 和授权矩阵明确 |
| BE-14  | 已确定 | `20-backend.md` 5.3～5.4 | 敏感数据、加密、保留、SSRF/滥用明确         |
| BE-15  | 已确定 | `20-backend.md` 6.1～6.2 | 容量预算与 SLO 明确                         |
| BE-16  | 已确定 | `20-backend.md` 6.3～6.4 | 低基数日志、指标、doctor/runbook 明确       |
| BE-17  | 已确定 | `20-backend.md` 7        | 配置、flag、发布、回滚和 DR 明确            |

### 8.4 跨领域覆盖

| 检查项 | 状态   | 证据                      | 结论或不适用原因                             |
| ------ | ------ | ------------------------- | -------------------------------------------- |
| XD-01  | 已确定 | `80-cross-domain.md` 1    | 共享标识、类型和 Owner 明确                  |
| XD-02  | 已确定 | `80-cross-domain.md` 2    | Provider/Consumer 成功、错误、重试、兼容明确 |
| XD-03  | 已确定 | `80-cross-domain.md` 3    | 8 类状态/错误映射明确                        |
| XD-04  | 已确定 | `80-cross-domain.md` 4    | 五条信任边界和身份传播明确                   |
| XD-05  | 已确定 | `80-cross-domain.md` 5    | 合同→迁移→Provider→Consumer→恢复顺序明确     |
| XD-06  | 已确定 | `80-cross-domain.md` 6～7 | 六条端到端 flow 和 XR 追踪明确               |

## 9. 未决问题

| Question ID | 影响 | 需要谁决定 | 决定记录 |
| ----------- | ---- | ---------- | -------- |

## 10. 批准记录

| 项目                    | 结果/证据                                                                                                                                                      |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Package validator final | 本规格落盘后必须运行 final validator；结果记录进任务证据                                                                                                       |
| 所有覆盖项无阻塞        | PJ、FE、BE、XD 均为已确定、代码证实、资料证实或有原因的不适用                                                                                                  |
| 所有需求完成追踪        | FR-01～FR-10、BR-01～BR-09、XR-01～XR-06 均关联 TASK、AC、TC                                                                                                   |
| 用户确认可实施          | 2026-08-10 用户明确要求仅 Mobile Web/PWA、Folo Mobile 为非首页基线、原型只替换首页、Go 使用服务端配置的 Gemini Key 取代 Folo 付费 AI，并要求建立长任务持续完成 |
