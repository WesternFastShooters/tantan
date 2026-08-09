# Codex Goal 执行协议 v2

## 唯一目标

持续交付 Tantan 一期 Mobile Web/PWA。Folo Mobile 是首页之外的 UI/交互基线；首页使用原型瀑布流；Go 是浏览器唯一业务 API 和 Folo/Gemini 出站方。PC、Electron、原生 App、Folo 付费和浏览器直连均不属于目标。

## 每轮开始

1. 运行 `bash spec-package/scripts/validate-package.sh`。
2. 读取 `agent/task-manifest.json`，选择第一个 `pending` 且依赖全部完成的 TASK；一次只激活一个。
3. 运行 `git status --short`，保护用户修改；确认 `git diff -- apps/mobile` 为空，且不写 `/Users/mingrui/Project/Folo`。
4. 读取该 TASK 关联的 `00/10/20/80/90` 规格、OpenAPI、Schema、DDL 和 route policy。

## 每个行为

1. Red：只改测试/fixture，运行最小命令，保存由目标行为缺失导致的预期失败。
2. Green：只在 allowedWrites 做最小生产实现，使相同测试通过。
3. Refactor：行为保持整理；相同测试持续为绿。
4. Verify：复核 diff，运行目标、受影响、合同、集成、race、安全和手机 E2E；保存命令、exit code、关键观察。

机器合同冲突时停止当前 TASK，报告精确字段、调用方和风险；不得改合同迁就实现。用户文件重叠、真实账号需人工输入或必须轮换后的 Secret 时才请求用户动作。普通实现困难、上下文压缩和部分页面可运行都不是停止原因。

## 安全边界

- 浏览器只调用相对 `/api`；发现 `api.folo.is` 或 Gemini direct request 立即视为失败。
- 密码、Folo token、AI Key、master key 不进入日志、URL、SQLite 明文、浏览器请求/存储、HAR、fixture、构建或 Git。
- Gemini Key 只由 Go 从 `gemini_api_key_file` 或本机 Keychain 装载；前端只有只读配置状态和“测试连接”，不得提交 Key、model 或 endpoint。不得把对话中粘贴的旧 Key 写入 shell 或文件。
- Folo proxy 精确 method+path 默认拒绝；被拒绝路由的 upstream request 必须为 0。
- RSS subscription store 保留；不做名称批量删除。

## 完成 TASK

只有 requiredGates 全部通过并有 Red/Verify 证据，才把状态改为 `completed`。然后提交一个范围清晰的 commit，再进入下一 TASK。合同/迁移与功能实现必须分开提交。

## 完成 Goal

TASK-01～TASK-08、AC-01～AC-25、适用的旧 46 项映射、全部最终命令、真实手机核心流程、安全扫描和只读边界全部通过后才调用 Goal complete；交付启动方式、测试结果、变更摘要、提交列表和非阻塞风险。
