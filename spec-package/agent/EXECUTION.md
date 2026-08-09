# Agent 安全执行协议

## 1. 开工前

1. 验证当前目录是 Tantan 实现仓库，不是 `/Users/mingrui/Project/Folo`。
2. 记录 `git status --short`；现有更改归用户所有，不得覆盖、还原或夹带修改。
3. 核对 Folo 导入基线 commit 与 SDK 锁定版本。不同时停止并报告规格偏移。
4. 运行整包校验；领取一个无未完成依赖的任务。
5. 列出该任务的预计文件清单。任何不在 `allowedWrites` 的路径都必须先修订任务合同。

## 2. TDD 和独立验证

- Red：只写测试、fixture 或 mock；保存命令、失败用例名、exit code 和“目标行为尚未存在”的预期失败原因。
- Green：只实现当前测试要求的最小生产变更，不顺手重构或扩展功能。
- Refactor：在测试持续通过时清理结构，不改变 OpenAPI、Schema、DDL 与可观察行为。
- Verify：运行任务门禁和相关回归；高风险安全项必须由未写该生产模块的人或 Agent 复核。

不允许先写生产代码再补“Red”，不允许用 snapshot 替代安全或状态机断言。

## 3. 写入与并行边界

- 一次只能处理一个任务 ID；不得同时展开多个 Green 实现。
- 共享文件只允许拥有者修改：OpenAPI/Schema/DDL 的拥有者是 `TASK-CONTRACT`，根路由/全局导航是 `TASK-FE-SHELL`。
- 后续 Agent 只消费契约；如果实现无法满足，不得私自改契约迁就代码。
- 不得修改 PRD、原型 ZIP、Folo 源仓库、`apps/mobile/**` 或本包外的用户文件。
- 不得使用 `git reset --hard`、`git checkout --`、`git clean`、宽泛递归删除或任何覆盖用户更改的命令。

## 4. 删除代码的附加门禁

移除 Folo 付费/AI 能力是授权范围，但仍必须逐项满足：

1. 目标存在于 `agent/paid-feature-removal.md` 的明确分类，不能只因目录名相似就删除。
2. 先用静态测试证明当前路由/入口/请求存在，再断开消费方。
3. 全库搜索证明生产消费方已为零，才能删除源文件。
4. 使用版本控制中的精确文件删除；删除后立即检查 `git diff --stat` 和 `git status --short`。
5. RSS 订阅 Store、Entry/Feed/Read/Collection 不可作为付费代码删除。

## 5. 安全不变量

- HTTP Server 显式 bind `127.0.0.1:3000`，禁止 `0.0.0.0`、`::` 或 LAN IP。
- 所有受保护变更请求要求有效本地 Session、允许的 `Origin` 和幂等键（按 OpenAPI 要求）。
- Auth callback 只接受 Folo 签发的一次性 token；不接受任意 redirect，不把 Folo session token 发给浏览器。
- 上游代理在代码中使用精确方法+正则路径匹配，默认 deny；不接受用户提供的 upstream URL。
- AI Provider URL 仅能选择内置 provider；自定义 endpoint 需独立后续威胁模型，一期禁用。
- 日志在序列化前删除 `Authorization`、Cookie、API Key、OAuth token、AI 完整 Prompt/Response 与文章全文。
- 对 AI 结构化输出执行 Schema 校验、字段长度上限、枚举和数组上限；校验失败不得入库。

## 6. 必须停工并修订规格的情况

- 需要新增或改变公共 API、错误码、游标绑定或事务语义。
- 需要修改已应用的 SQL 迁移，或在 Keychain 之外存储密钥。
- 需要启用 Folo 白名单外路由，特别是 AI、Wallet、Payment、Stripe、Referral 和 Trending。
- 需要引入原生 App、服务器部署、外部事件/Webhook、自定义 AI endpoint 或远程遥测。
- 基线与锁定版本不符，或实现需要越过任务写入边界。

## 7. 交付记录模板

```text
Task ID:
Baseline commit:
Files changed:
Red command / expected failure:
Green command / result:
Refactor command / result:
Verify commands / results:
Security checks:
Remaining risks: none | <spec amendment reference>
```
