# Tantan 一期验收矩阵证据

日期：2026-08-10  
基线：`3846c90b67da351b6017cd4fe9d0992b8077224e`，导入提交 `3a34a49`  
范围：Mobile Web/PWA、Go 中间层；PC Web、Electron 和原生 App 明确不在一期范围

下表中的测试均包含在最终门禁中。Playwright 项由 `pnpm --dir apps/desktop e2e:web` 执行；Vitest 项由 `pnpm --filter @follow/web test -- --run` 执行；Go 项由 `go test ./...` 和 `go test -race ./...` 执行。

## 前端 23 项

| ID        | 自动化证据                                                                                                                                             | 结果         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------ |
| FE:TC-001 | 基线导入提交 `3a34a49`、基线门禁提交 `a6b0bf6`；冻结安装 `pnpm install --frozen-lockfile --ignore-scripts` 连续可复现                                  | PASS         |
| FE:TC-002 | `tantan-no-paid.spec.ts`：paid and Folo AI routes expose no product UI；`paid-feature-removal.test.ts` 路由/导航/调用方静态门禁                        | PASS         |
| FE:TC-003 | `tantan-no-paid.spec.ts`：browser HAR contains zero denied Folo route；后端 removed route 无上游拨号测试                                               | PASS，0 次   |
| FE:TC-004 | `tantan-shell.spec.ts` 与生产四项目：390×844/430×932 只有首页、订阅、发现、设置四 Tab，无桌面侧栏或 DownloadPage                                       | PASS         |
| FE:TC-005 | `tantan-shell.spec.ts` 登录跳转与 session 401；`TestAuthCallbackStoresFoloTokenOnlyInSecretStore`；应用集成测试保持同一 userId；订阅/已读/收藏兼容 E2E | PASS         |
| FE:TC-006 | `home-view-store.test.ts`；`tantan-flows.spec.ts` Mobile search preserves Home state；Home 详情返回与 cache 测试                                       | PASS         |
| FE:TC-007 | `tantan-home.spec.ts`：Mobile Home 固定两列，宽屏仍为居中手机壳；四类卡片 fixture                                                                      | PASS         |
| FE:TC-008 | `tantan-home.spec.ts`：图片请求失败后无 `img`、文字卡仍可读                                                                                            | PASS         |
| FE:TC-009 | `home-cache.test.ts`：删除所有 Home cache 中 entryId 且兄弟顺序不变；详情 E2E                                                                          | PASS         |
| FE:TC-010 | `TestTopicIsAStableQueueViewAndFullConsumptionFinishesToday`；Home E2E 显示“今天已经看完”；订阅/搜索 E2E 仍可读历史                                    | PASS         |
| FE:TC-011 | `tantan-acceptance/home-actions.spec.ts`：成功移除、500 回滚、5 秒撤销、屏蔽 Source、设置中恢复                                                        | PASS         |
| FE:TC-012 | `tantan-acceptance/search-detail.spec.ts`：七类字段、两页、边界去重、尾游标为空                                                                        | PASS         |
| FE:TC-013 | `tantan-acceptance/entry-retry.spec.ts`：失败时原文可读并可重试；原流程覆盖 queued 状态                                                                | PASS         |
| FE:TC-014 | `subscription-model.test.ts`；`tantan-flows.spec.ts`：四类视图、RSS 添加/取消、收藏兼容；RSS Subscription Store 回归                                   | PASS         |
| FE:TC-015 | `tantan-acceptance/search-detail.spec.ts`：原/译文、摘要、要点、收藏、安全外链                                                                         | PASS         |
| FE:TC-016 | `tantan-flows.spec.ts`、`tantan-security.spec.ts`：浏览器无 Key 输入，storage/URL/响应/HAR 不回显；发布 canary 扫描                                    | PASS，0 泄漏 |
| FE:TC-017 | `tantan-acceptance/home-actions.spec.ts`：生成、编辑、重置均停留 Home，并原子刷新 Filter/Topic/Queue                                                   | PASS         |
| FE:TC-018 | `tantan-acceptance/performance-accessibility.spec.ts`：60 卡首屏小于 5 秒、DOM ≤30；后端 500 候选容量测试                                              | PASS         |
| FE:TC-019 | `tantan-shell.spec.ts` Go 故障/重试；`entry-retry.spec.ts` AI 故障保留原文；后端 fail-closed 测试                                                      | PASS         |
| FE:TC-020 | `tantan-acceptance/topics.spec.ts`：固定/取消、隐藏/显示、移动、版本递增、推荐不可改                                                                   | PASS         |
| FE:TC-021 | `performance-accessibility.spec.ts`、`tantan-production.spec.ts`：reduced-motion、键盘、Axe critical=0、SW 注册且 `/api` 缓存为 0                      | PASS         |
| FE:TC-022 | `tantan-home.spec.ts`：page2 重复 entryId 只渲染一次；`home-model.test.ts` 保留首次位置                                                                | PASS         |
| FE:TC-023 | `tantan-home.spec.ts` 与 `home-components.test.tsx`：普通搜索进入 `/search`，AI 只开 Sheet，提交前状态不变                                             | PASS         |

## 后端 23 项

| ID        | 自动化证据                                                                                                           | 结果               |
| --------- | -------------------------------------------------------------------------------------------------------------------- | ------------------ |
| BE:TC-001 | `TestApplicationStartsMigratedReadyAndRoutesLocalAPI`、`TestApprovedMigrationsApplyExactlyOnce`、health/ready 测试   | PASS               |
| BE:TC-002 | `TestGeneratedGoTypesMatchApprovedContract`、`TestEmbeddedRoutePolicyMatchesApprovedMachineContract`、合同哈希生成器 | PASS               |
| BE:TC-003 | Auth bridge wrong-flow、受控一次性 token、回滚及正常 callback 测试                                                   | PASS               |
| BE:TC-004 | Auth callback/Logout/SQLite session 测试：Token 仅 Keychain，DB 仅哈希，登出删除                                     | PASS               |
| BE:TC-005 | `TestProxyPreservesEveryEnabledMethodPathAndPerformanceBudget`、响应兼容测试                                         | PASS               |
| BE:TC-006 | `TestDeniedAndRemovedRoutesNeverReachUpstream`                                                                       | PASS，0 次上游拨号 |
| BE:TC-007 | 默认拒绝、Host/Origin、header smuggling、路径双解码及日志脱敏测试                                                    | PASS，0 次上游拨号 |
| BE:TC-008 | `TestFullSyncIsPageBoundedAndIdempotent`                                                                             | PASS               |
| BE:TC-009 | checkpoint 恢复、5 分钟重叠、显式增量 checkpoint 测试                                                                | PASS               |
| BE:TC-010 | NDJSON 部分失败、超长/无效行及只重试缺失 ID 测试                                                                     | PASS               |
| BE:TC-011 | Search 七类字段、签名游标、indexStatus、损坏状态测试                                                                 | PASS               |
| BE:TC-012 | 固定 Gemini 服务端 Secret、fingerprint、只读 settings、浏览器配置拒绝及 Provider body 不回显测试                     | PASS               |
| BE:TC-013 | Provider timeout/429/500 分类、并发≤2、锁定 endpoint、安全拨号与 loopback proxy 测试                                 | PASS               |
| BE:TC-014 | Enrichment/Filter 只修复一次、二次无效不提交测试                                                                     | PASS               |
| BE:TC-015 | content hash/provider fingerprint/prompt version stale 与新 dedupe identity 测试                                     | PASS               |
| BE:TC-016 | 日队列 7 天/500 候选、首批 50、当日 60、签名游标、跨日新队列测试                                                     | PASS               |
| BE:TC-017 | Ranking deterministic/diversity/golden order 测试                                                                    | PASS               |
| BE:TC-018 | 10 万 Entry Home/Search 容量、延迟与内存预算测试                                                                     | PASS               |
| BE:TC-019 | Filter 原子切换/失败保留/reset/并发幂等；Feedback 幂等与不回加当前队列测试                                           | PASS               |
| BE:TC-020 | 应用、AI、Auth、Proxy、日志 canary 测试及最终 SQLite/log/build 扫描                                                  | PASS，0 泄漏       |
| BE:TC-021 | readiness 故障、worker 停止领取、Provider 失败保留原文测试                                                           | PASS               |
| BE:TC-022 | Backup 不覆盖、checksum/integrity/foreign-key/row-count、原子恢复、7 份轮换测试                                      | PASS               |
| BE:TC-023 | Doctor 固定检查名、端口/DB 锁/Keychain/DNS/TLS 故障与脱敏测试                                                        | PASS               |

## 发布门禁结果

- Spec package：PASS，0 errors，0 warnings。
- Frontend spec / Backend spec final validators：PASS，0 warnings。
- Web Vitest：56 files，166 tests，PASS，0 type errors。
- Playwright Mobile Web：33 项开发回归 PASS；生产 Chromium/WebKit × 390/430 共 8 项 PASS；critical Axe violations 0；禁止 Folo 请求 0。
- Go：`go mod verify`、format、vet、普通测试、race 测试、build 全部 PASS。
- `pnpm typecheck`、`pnpm lint`、`pnpm build:web`：PASS。
- Secret canary：Git、SQLite、日志、HAR、浏览器存储、备份及构建产物扫描 0 命中；Keychain 真实 Gemini 翻译与 FilterSpec 调用 PASS。
- 保护路径：`apps/mobile/**` 与 `/Users/mingrui/Project/Folo` 均无修改。

本表 46 个旧 ID 已全部映射。涉及 PC 多列布局、桌面侧栏或浏览器填写 AI Key 的旧观察点，由已批准的 Mobile-Web-only、固定两列 Home 和 Go 服务端 Secret 决策替代；对应自动化证据已更新为新合同。
