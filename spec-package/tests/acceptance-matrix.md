# 一期验收矩阵

## 1. 执行规则

- 用例 ID 的 `FE`/`BE` 命名空间分别对应前后端规格的同号 `AC-*`。
- `Auto` 用例必须在 CI 或本地总门禁自动执行；`Manual+Auto` 先自动断言可观察结果，再做视觉/读屏复核。
- 所有 fixture 使用虚构 ID、`.invalid` 域名和 canary secret；不使用真实 Token、Key、邮箱或私人内容。
- 性能数据使用固定 seed；安全用例失败时不得以人工 smoke 替代。

## 2. 前端用例

| TC        | Mode        | Setup                                                                         | Operation                                                                        | 必须观察到的唯一成功结果                                                              |
| --------- | ----------- | ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| FE:TC-001 | Auto        | 导入锁定 Folo commit，安装锁定 Node/pnpm/SDK                                  | 执行基线记录脚本两次                                                             | commit/版本/命令/exit code 一致，第二次不改文件                                       |
| FE:TC-002 | Auto        | 已登录，生成路由表                                                            | 直访 `/ai`、`/power`、`/settings/plan`，扫描一级/设置导航                        | 路由不存在，导航无 Plan/Power/Wallet/会员/升级/AI Chat                                |
| FE:TC-003 | Auto        | Playwright 捕获全部 request，启用本地 AI fixture                              | 执行首页、详情、设置核心流程                                                     | Folo AI/Wallet/Payment/Stripe Subscription 请求数为 0                                 |
| FE:TC-004 | Auto        | viewport `390x844` 和 `430x932`，已登录                                       | 打开四个主路由                                                                   | 显示首页/订阅/发现/设置四项 Folo Mobile 风格底栏，无桌面侧栏或 Electron 下载入口      |
| FE:TC-005 | Auto        | Folo auth mock 有 providers、Email/TOTP、用户、订阅、read、collection fixture | 分别完成 Google/GitHub/Apple 官方页→授权令牌、Email（含 TOTP）、直接授权令牌登录 | 五类入口均建立同一 userId 的 Tantan session，订阅/已读/收藏数与 fixture 相等          |
| FE:TC-006 | Auto        | 为 Home Topic 设置独立滚动位置                                                | 切 Topic、进搜索、开详情后返回                                                   | route、activeTopicId、activeFilterId 和该 Topic scroll 全部恢复，queueId/version 未变 |
| FE:TC-007 | Auto        | 固定四类 HomeCard fixture                                                     | 分别以 390x844、430x932 打开 Home                                                | 两个手机视口都固定 2 列，article/post/image/video 均使用对应卡片降级规则且无 PC 分支  |
| FE:TC-008 | Auto        | cover 返回 404 与超时                                                         | 等待 image error/timeout                                                         | 图片容器收起并显示文字卡，无破图图标且布局不填塞                                      |
| FE:TC-009 | Auto        | 同 entryId 存在于推荐、AI 两个 Home Query Cache                               | 详情页标记已读并返回成功                                                         | 该 entryId 从所有 `home` cache 消失，其它卡片顺序不变                                 |
| FE:TC-010 | Auto        | ready queue 有50条且历史订阅内容存在                                          | 顺序将队列全部标读                                                               | Home 显示“今天已经看完”，历史 Entry 仍可在订阅/搜索打开                               |
| FE:TC-011 | Auto        | feedback API 依次配置成功和 500                                               | 对两张卡执行不感兴趣，再屏蔽/恢复 Source                                         | 成功项移出，失败项回滚，Source 恢复后可进入下次队列                                   |
| FE:TC-012 | Auto        | FTS fixture 分别仅在已读、未读、原文、译文、Source、Topic、Tag 命中           | 输入查询并翻两页                                                                 | 七类命中均出现，无重复且 nextCursor 结束为 null                                       |
| FE:TC-013 | Auto        | enrichment 分别为 queued/failed                                               | 打开详情并切原文                                                                 | 原文全程可读；queued 显示进度，failed 显示重试且不覆盖原文                            |
| FE:TC-014 | Auto        | 有 article/social/picture/video 订阅及可添加 Source                           | 切四类、开 Source 历史、添加/取消订阅、收藏/取消                                 | UI 和 Folo 数据最终一致，RSS subscription Store 行为未改                              |
| FE:TC-015 | Auto        | 详情有原文、译文、摘要和外链                                                  | 切换原/译文、收藏、开原文链接                                                    | 显示正确文本与收藏态，外链使用 `noopener,noreferrer`                                  |
| FE:TC-016 | Auto        | Go 从测试 Secret 来源装载虚构 API Key canary                                  | 打开 AI 设置并测试连接，再导出 HAR/日志并扫全部浏览器请求与存储                  | 页面无 Key 输入；canary 在 request/response/URL/storage/log/HAR/Folo request 中均为 0 |
| FE:TC-017 | Auto        | 默认 Home 与可成功生成的 AI Filter                                            | 打开 Sheet，提交，编辑，重置                                                     | 路由始终为 `/`；Topic/顺序/卡片/Active Bar 原子改变，reset 回到默认 queue             |
| FE:TC-018 | Auto        | 500 候选、60 队列卡、固定机器配置                                             | 记录 Home 加载/滚动和 DOM 卡片数                                                 | 达到前端性能预算，DOM 同时卡片数不超过可视+过扫描预算                                 |
| FE:TC-019 | Auto        | 先填充缓存，再分别中断 Folo/AI/Go                                             | 访问 Home/详情并触发重试                                                         | 缓存原文仍可读；仅依赖相关操作禁用，重试后可恢复                                      |
| FE:TC-020 | Auto        | 核心、动态和虚拟推荐 Topic                                                    | 固定/取消、隐藏/显示、排序，尝试删推荐                                           | 版本递增且顺序持久化，推荐仍在首位且不可改                                            |
| FE:TC-021 | Manual+Auto | 安装态 Mobile PWA，390/430，reduced-motion 开/关                              | 断网、检查 Cache Storage，键盘/触控走完主流程并运行 axe                          | manifest/install 有效，API/Auth 无缓存，焦点可见，无 critical axe 违规，动效遵守偏好  |
| FE:TC-022 | Auto        | page2 重复 page1 末尾 entryId，cursor 携带同 queue/version                    | 连续触发两次翻页                                                                 | 重复 entryId 只渲染一次，旧卡片位置不变，不发生 AI 重排                               |
| FE:TC-023 | Auto        | 记录 route/Sheet/Topic/Filter/query cache                                     | 先点普通搜索图标，返回后点 AI 图标                                               | 普通搜索仅进 `/search`；AI 仅打开 Sheet，未提交时所有 Home 状态不变                   |

## 3. 后端用例

| TC        | Mode | Setup                                                       | Operation                                                                             | 必须观察到的唯一成功结果                                                                                                   |
| --------- | ---- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| BE:TC-001 | Auto | 空临时数据目录                                              | 启动服务并请求 health/ready                                                           | 迁移 1,2,3,4 各一次，health=200，ready=200，integrity=ok                                                                   |
| BE:TC-002 | Auto | SDK 0.3.95 打包产物和脱敏 fixture                           | 校验 package hash 与 method/path snapshot                                             | hash 等于锁定值，snapshot 与 route policy 一致，fixture 密钥扫描为 0                                                       |
| BE:TC-003 | Auto | auth mock 支持 provider URL、Email/TOTP 和一次性 token      | 请求非法 provider、过期/错误 TOTP、重放 token、再完成 Email 和三个社交 provider token | 非法/过期/重放拒绝且无 session；五类入口各只创建一个 Tantan session                                                        |
| BE:TC-004 | Auto | Folo token canary 与仿真 Keychain/AES-GCM vault             | 登录、请求 session、查 DB/log，再登出                                                 | canary 只存在安全 secret store，Browser/DB 明文/log 无；登出后 token 删除                                                  |
| BE:TC-005 | Auto | httptest Folo 对白名单路由返回固定 status/body/content-type | 经 Go proxy 请求全部 enabled fixture                                                  | 客户端观察值与上游等价，proxy P95 开销不超 30ms                                                                            |
| BE:TC-006 | Auto | 上游 mock 记录 dial 数                                      | 请求 AI/Wallet/Payment/Stripe/Referral/Trending/RSSHub-use 路由                       | 每次返回 410 `FOLO_FEATURE_REMOVED`，上游调用数为 0                                                                        |
| BE:TC-007 | Auto | 上游 mock 记录 dial 数                                      | 发送未知路由、恶意 Host/Origin、Forwarded/Cookie/Authorization 夹带                   | 请求在本地 403，上游未 dial，日志不含夹带值                                                                                |
| BE:TC-008 | Auto | 250 条、3页 metadata，content/read/collection fixture       | 运行全量同步两次                                                                      | 条目/关系计数与 fixture 相等，第二次不增加重复数据                                                                         |
| BE:TC-009 | Auto | 增量窗口内有重叠 Entry                                      | 在第2页事务后 kill，重启并再同步                                                      | 从已提交 checkpoint 继续，5min 重叠不生成重复且最终数据完整                                                                |
| BE:TC-010 | Auto | NDJSON 50 项中缺 3 项并有2条无效                            | 运行 content job                                                                      | 45 条成功单项提交，5个 ID 进可观测重试，整批不回滚                                                                         |
| BE:TC-011 | Auto | FTS 七类字段 fixture，索引构建中/完成状态                   | 搜索中英文和子串                                                                      | 标题/正文/译文/Source/Topic/Tag 结果正确，indexStatus 与实际状态一致                                                       |
| BE:TC-012 | Auto | API Key canary 权限文件/仿真 Keychain 与 Provider           | 启动、读只读配置、调 Provider，扫 HTTP/SQLite/log/backup                              | canary 只存在 Secret 来源、Go 进程和发往 Gemini 的 Authorization；配置 GET 只返回固定 provider/model/hasApiKey/fingerprint |
| BE:TC-013 | Auto | Provider 依次 timeout/429/500，同时提交3任务                | 运行 worker，并向所有浏览器 AI API 夹带 URL/内网 IP/Key 字段                          | 超时/重试次数符合合同，并发最大2，未知字段被拒且任意 URL 不 dial                                                           |
| BE:TC-014 | Auto | Provider 先返无效 JSON，然后分别修复成功/再失败             | 生成 enrichment 和 filter                                                             | 只修复一次；成功时入库，再失败时返回 `AI_OUTPUT_INVALID` 且旧状态不变                                                      |
| BE:TC-015 | Auto | 已 ready enrichment                                         | 依次变更 content hash、provider fingerprint、prompt version                           | 每次旧记录成 stale 且新 dedupe key 只创建一个 job                                                                          |
| BE:TC-016 | Auto | 7天内500条未读、跨日时钟、签名cursor                        | 生成/追加/翻页，改 query 重放cursor，跨日请求                                         | 首队列50、当日最多60；同 version 顺序稳定；错配 `CURSOR_MISMATCH`；跨日新 queue                                            |
| BE:TC-017 | Auto | 固定分数/Source/Topic/read/block Golden fixture             | 两次建队并计算前20分布                                                                | 两次顺序字节等价，同 Source/Topic 约束成立，read/block 条目为 0                                                            |
| BE:TC-018 | Auto | 固定 seed 生成10万 Entry 和500候选                          | 多轮 Home/Search 预热后测 P50/P95                                                     | Home P95≤150ms，Search P95≤300ms，内存在后端预算内                                                                         |
| BE:TC-019 | Auto | 旧 active filter/queue，新 filter fixture，重复幂等键       | 执行成功/失败 filter、reset 和重复 feedback                                           | 成功为单事务切换，失败保留旧版，reset 回默认，重放 feedback 只有一条事件                                                   |
| BE:TC-020 | Auto | 同一 canary 放入 Key/Token/Prompt/Content 各输入            | 运行登录、AI、错误、同步和代理后扫描                                                  | canary 不出现于 DB 明文字段、log、error response 和 Folo request                                                           |
| BE:TC-021 | Auto | 先填充缓存，再注入 Folo/AI/SQLite 故障                      | 请求 Home/详情/ready                                                                  | Folo/AI 失败时缓存原文可读且不损坏状态；SQLite 失败时 ready=503、worker停止                                                |
| BE:TC-022 | Auto | 有 Topic/Filter/Queue/偏好的 DB，目标备份路径已有文件       | 先尝试覆盖，再备份到新路径并恢复到新数据目录                                          | 已有文件不被覆盖；新备份 integrity=ok，恢复后行数与偏好/队列一致                                                           |
| BE:TC-023 | Auto | 依次注入端口占用、DB锁、secret store失败、DNS/TLS失败       | 执行 `tantan-api doctor`                                                              | 每个故障都有稳定检查名/结果/恢复建议，输出不含 Token/Key/Prompt/正文                                                       |

## 4. 总门禁

```bash
bash spec-package/scripts/validate-package.sh
python3 /Users/mingrui/.agents/skills/development-scenarios/project-spec/scripts/validate_spec_package.py spec-package --stage final
pnpm typecheck
pnpm lint
pnpm --filter @follow/web test -- --run
pnpm --dir apps/desktop e2e:web
pnpm build:web
(cd services/tantan-api && go vet ./... && go test -race ./... && go build ./cmd/tantan-api)
```

业务实现开始后，所有适用命令都是完成必要条件；PC/Electron 和原生 App 命令不属于本期门禁。
