# Tantan 本地运维手册

以下命令都从仓库根目录 `/Users/mingrui/Project/tantan` 执行。服务只允许监听 `127.0.0.1:3000`，数据库和日志默认位于系统配置目录的 `Tantan` 子目录；可用 `TANTAN_DATA_DIR` 指向另一个本地目录。

## 本机启动与就绪

```bash
cd services/tantan-api
go run ./cmd/tantan-api serve
curl --fail http://127.0.0.1:3000/api/healthz
curl --fail http://127.0.0.1:3000/api/readyz
```

`healthz` 只表示进程可响应。`readyz` 还会验证 SQLite、迁移版本/校验和及一次随机的 OS Keychain 写入、读取和删除；任何一项失败都会返回 503，后台任务也会停止领取新作业。

另开一个终端启动 Mobile Web 开发页面：

```bash
cd /Users/mingrui/Project/tantan
pnpm dev:web
```

浏览器访问 `http://127.0.0.1:2233`。本期只有 Mobile Web/PWA；宽屏也只显示居中的手机壳，不提供 PC 页面。

验收生产构建时先执行：

```bash
cd /Users/mingrui/Project/tantan
pnpm build:web
cd services/tantan-api
TANTAN_STATIC_DIR=/Users/mingrui/Project/tantan/apps/desktop/out/web \
  go run ./cmd/tantan-api serve
```

然后访问 `http://127.0.0.1:3000`。服务器部署时仍由 Go 监听 loopback，再由同机 HTTPS 反向代理转发；浏览器业务请求只允许同源 `/api`。

## 服务端 Gemini 配置

浏览器不再提供 Key、模型或 Endpoint 输入框。Go 服务固定使用：

- Provider：`google-gemini-openai`
- Endpoint：`https://generativelanguage.googleapis.com/v1beta/openai`
- Model：`gemini-3.5-flash-lite`

本机可在“钥匙串访问”中新建密码项目：服务名 `tantan.ai.provider`、账户 `google-gemini-openai`、密码为轮换后的 Key。服务器使用权限为 `0600` 的 Secret 文件，并只把文件路径交给 Go：

```bash
TANTAN_GEMINI_API_KEY_FILE=/绝对私密路径/gemini.key \
TANTAN_STATIC_DIR=/Users/mingrui/Project/tantan/apps/desktop/out/web \
  go run ./cmd/tantan-api serve
```

Key 值不得写进源码、配置 YAML、命令参数、普通环境变量、SQLite、浏览器存储或日志。此前贴进聊天的 Key 已暴露，必须在 Google 控制台作废并换新，不能再用于验收。

若本机网络必须经过代理，只允许显式的 loopback HTTP(S) 代理，并让 API 进程继承环境变量：

```bash
cd services/tantan-api
HTTPS_PROXY=http://127.0.0.1:7897 go run ./cmd/tantan-api serve
```

端口按本机代理实际监听端口替换。远程代理、带账号密码的代理、SOCKS、自定义路径/查询参数都会被拒绝。

真实 Google 翻译冒烟测试从 Go 服务相同的 Keychain 项读取 Key；也可显式指定私密文件。
测试不接收 Key 值环境变量或命令行 Key：

```bash
cd services/tantan-api
TANTAN_LIVE_AI=1 \
  go test ./internal/ai -run TestLiveGoogleTranslation -count=1 -v

# 服务器 Secret 文件方式
TANTAN_GEMINI_API_KEY_FILE=/绝对私密路径/gemini.key \
TANTAN_LIVE_AI=1 \
  go test ./internal/ai -run TestLiveGoogleTranslation -count=1 -v
```

## doctor

先停止 Tantan 服务，再运行：

```bash
cd services/tantan-api
go run ./cmd/tantan-api doctor
```

doctor 依次检查本地端口、数据目录权限、SQLite 完整性、迁移、OS Keychain、Folo 官方域名的可用 DNS 地址与 TLS。输出只包含固定状态和修复建议，不包含 API Key、会话、内容、原始路径或底层错误。

常见修复：

- `port`：停止占用 `127.0.0.1:3000` 的进程。
- `data_directory` / `sqlite`：确认数据目录可写；必要时按下一节恢复最近备份。
- `migrations`：不要手工改库，恢复由当前 Tantan 版本创建的备份。
- `keychain`：解锁系统钥匙串，并允许 Tantan 访问。
- `dns` / `tls`：检查网络、DNS、系统时间和证书。

## 备份

服务运行时可创建一致性备份，但输出文件必须尚不存在：

```bash
cd services/tantan-api
go run ./cmd/tantan-api backup \
  --output /绝对路径/tantan-backup.sqlite
```

命令使用 SQLite `VACUUM INTO` 生成快照，发布前执行完整性、迁移、校验和及关键表行数检查，文件权限为 `0600`。服务每个本地日期启动时还会在数据目录的 `backups` 下生成一次备份，并只保留最近 7 份。

备份不包含 OS Keychain 中的 Folo 会话、AI Provider Key 或游标签名密钥。

## 恢复

1. 完全停止 Tantan API，确认 `127.0.0.1:3000` 未被占用。
2. 执行恢复；来源还会额外通过 foreign-key 检查：

```bash
cd services/tantan-api
go run ./cmd/tantan-api restore \
  --input /绝对路径/tantan-backup.sqlite
```

恢复会先验证来源，再复制到临时文件并复核校验和、迁移、完整性及行数，最后原子替换数据库。原数据库存在时会保留同目录的 `tantan.sqlite.pre-restore-*` 恢复副本；命令输出会返回其精确路径。

如果出现 `SERVICE_RUNNING`，不要删除 WAL/SHM 文件，先正常停止服务。恢复失败且数据库尚未替换时，原文件保持不变。

## 本地数据清除

清除会永久移除本机同步内容、Topic、筛选、队列和作业；Folo 登录与 AI Key 还要分别从产品会话和 OS Keychain/服务器 Secret 文件中清除。

1. 在产品中退出 Folo 登录，并在 Keychain/Secret 管理器中删除 Gemini 项。
2. 停止 API，先做一份外部备份。
3. 只删除 doctor/启动配置所指向的那个 Tantan 数据目录；不要使用通配符或递归删除工作区、用户目录。

## 公共错误码

- `AUTH_REQUIRED`：本地会话缺失或失效。
- `VALIDATION_ERROR`：请求参数或 JSON 不符合合同。
- `VERSION_CONFLICT`：Topic/Filter 版本已变化，刷新后重试。
- `SERVICE_NOT_READY`：SQLite、迁移或 Keychain 未就绪。
- `LOCAL_STORAGE_ERROR`：本地库或作业状态不可用。
- `FOLO_UNAVAILABLE` / `FOLO_RATE_LIMITED`：允许的 Folo 数据接口暂时不可用。
- `AI_NOT_CONFIGURED` / `AI_PROVIDER_UNAVAILABLE` / `AI_OUTPUT_INVALID`：本地 AI 配置、连接或结构化输出失败。

错误响应、日志和诊断接口都不得包含 API Key、Folo 会话、文章正文或任意上游原始错误。
