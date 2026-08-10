# Tantan 部署到 Cloudflare Containers

这套部署只包含 Mobile Web/PWA、Go API 和 SQLite，不包含 Electron、原生 App 或 `apps/mobile` 产物。Cloudflare Worker 是唯一公网入口；它校验 Cloudflare Access 登录邮箱后，才把请求转给一个固定命名的 Container。

Cloudflare Container 的磁盘是临时的，因此镜像内置 Litestream：SQLite 每 5 秒增量复制到私有 R2；新容器没有数据库时会先从 R2 恢复，再启动 Go。不要删除 R2 Bucket，也不要把容器文件系统当作备份。

## 1. 需要准备

- 一个已接入 Cloudflare 的域名，例如 `tantan.example.com`。
- Workers Paid 套餐，Cloudflare Containers 不能部署在免费套餐。
- Cloudflare Zero Trust Access，只允许你的唯一邮箱。
- 一个私有 R2 Bucket，默认名 `tantan-data`。
- 一个仅有该 Bucket 读写权限的 R2 API Token。
- 一个新的 Gemini API Key。发到聊天、截图或日志里的旧 Key 必须先作废。

先在本机授权 Wrangler：

```bash
pnpm --dir deploy/cloudflare exec wrangler login
pnpm --dir deploy/cloudflare exec wrangler r2 bucket create tantan-data
```

## 2. 配置 Cloudflare Access

在 Cloudflare Zero Trust 后台进入 `Access > Applications`，新建 Self-hosted 应用：

1. 域名填写 Tantan 的完整域名。
2. Allow Policy 只允许你的唯一邮箱，不要加入 `Everyone`。
3. 记下应用的 `Application Audience (AUD) Tag`。
4. 记下团队域名，例如 `https://你的团队.cloudflareaccess.com`。

即使 Access 配错，Worker 仍会再次验证 JWT 的签发方、AUD 和邮箱；不匹配只返回 403。不要删除 Worker 里的二次校验。

## 3. 填写非敏感配置

编辑 `deploy/cloudflare/wrangler.jsonc`，替换下面字段：

- `routes[0].pattern`：Tantan 完整域名，不带 `https://`。
- `TEAM_DOMAIN`：Access 团队域名，带 `https://`，末尾不带 `/`。
- `POLICY_AUD`：Access 应用 AUD Tag。
- `OWNER_EMAIL`：唯一允许登录的邮箱。
- `TANTAN_PUBLIC_ORIGIN`：`https://` 加 Tantan 完整域名。
- `R2_ACCOUNT_ID`：Cloudflare Account ID。
- `R2_BUCKET_NAME`：R2 Bucket 名。

这些值可以提交 Git，但不能把任何 Key、令牌或密码写进该文件。部署命令会先拒绝占位值、错误域名和错误邮箱。

## 4. 注入 Secret

Secret 通过 Cloudflare Worker Secrets 在部署时注入 Container 环境，不进入 Dockerfile、镜像层、Git、SQLite、浏览器或前端构建产物。

生成并保存稳定的 32 字节数据库主密钥：

```bash
openssl rand -base64 32 | pnpm --dir deploy/cloudflare exec wrangler secret put TANTAN_MASTER_KEY_B64
```

请把同一个主密钥另外离线保存一份。它丢失或被替换后，R2 中数据库保存的 Folo 会话将无法解密。

生成 Worker 到 Go 的内部网关 Secret：

```bash
openssl rand -base64 32 | pnpm --dir deploy/cloudflare exec wrangler secret put TANTAN_GATEWAY_SECRET
```

其余 Secret 使用命令后按提示粘贴，不要写入命令行参数：

```bash
pnpm --dir deploy/cloudflare exec wrangler secret put TANTAN_GEMINI_API_KEY
pnpm --dir deploy/cloudflare exec wrangler secret put R2_ACCESS_KEY_ID
pnpm --dir deploy/cloudflare exec wrangler secret put R2_SECRET_ACCESS_KEY
```

Gemini endpoint 和模型由 Go 固定为：

- `https://generativelanguage.googleapis.com/v1beta/openai`
- `gemini-3.5-flash-lite`

R2 Secret 在 Cloudflare Dashboard 的 `R2 Object Storage > Manage R2 API Tokens` 创建，只授予指定 Bucket 的 Object Read & Write。Folo 一次性令牌不属于部署 Secret，不要把它写进 Cloudflare 或 Docker 环境变量。

## 5. 构建并部署

```bash
pnpm --dir deploy/cloudflare validate:config
pnpm --dir deploy/cloudflare typecheck
pnpm --dir deploy/cloudflare test
pnpm --dir deploy/cloudflare deploy
```

Wrangler 会构建仓库根目录的 `Dockerfile`，上传 Worker 和 Linux `amd64` Container 镜像，并保持最多一个名为 `tantan-owner` 的实例。不要再部署 Caddy、Cloudflare Tunnel、腾讯云 CVM 或公开 Go 的 8080 端口。

部署后验证：

```bash
curl -I https://你的域名
pnpm --dir deploy/cloudflare exec wrangler tail
```

未通过 Access 时应进入 Cloudflare 登录页；非 owner 邮箱或伪造请求必须得到 403。通过后 `/api/healthz`、首页和 PWA 应正常加载。

## 6. 首次连接 Folo

Folo one-time token 只在服务器首次绑定或 Folo 会话失效时使用一次。Go 会立即把它兑换为 Folo 会话，用数据库主密钥加密保存到 SQLite；随后 SQLite 会复制到 R2。手机永远不需要打开控制台或粘贴令牌。

1. 在电脑浏览器登录需要同步的同一个 `https://app.folo.is` 账号。
2. 打开开发者工具 Console，执行：

```javascript
fetch("https://api.folo.is/better-auth/one-time-token/generate", { credentials: "include" })
  .then((r) => r.json())
  .then(({ token }) => prompt("按 ⌘C 复制令牌，然后点确定", token))
```

3. 复制弹窗里的短时令牌，打开 `https://你的域名/login?setup=1`，粘贴并完成绑定。
4. Android/iOS Chrome 以后只需通过 Cloudflare Access 登录 owner 邮箱，Tantan 会自动创建自己的 HttpOnly 会话，不再显示令牌页面。

若 Folo 会话以后失效，在电脑重新执行以上四步。一次性令牌不得发到聊天、截图、日志或 Git；已经暴露的令牌视为无效。

## 7. 更新、恢复和轮换

- 更新应用：重新执行 `pnpm --dir deploy/cloudflare deploy`。
- 容器重启：Litestream 自动从 `tantan-data/single-user/tantan.sqlite` 恢复。
- 突然断电或强制终止：最坏可能丢失最近约 5 秒尚未复制的 SQLite 修改；正常停止会在退出前做最终同步。
- 轮换 Gemini Key：重新执行对应 `wrangler secret put`，然后重新部署。
- 不要随意轮换 `TANTAN_MASTER_KEY_B64`。需要轮换时必须先实现会话重加密迁移。
- R2 Access Key 泄漏时，在 R2 后台撤销旧 Token、写入新 Secret 并重新部署。

## 8. 上线安全检查

- `workers_dev` 保持 `false`，避免产生第二个绕过自定义域名 Access 的公网入口。
- Access Policy 只允许 owner 邮箱，Worker 中 `OWNER_EMAIL` 与它完全一致。
- R2 Bucket 保持私有，R2 Token 只允许该 Bucket 读写。
- Docker 镜像、`wrangler.jsonc`、Git、浏览器存储和日志中均不存在 Secret。
- `TANTAN_MASTER_KEY_B64` 有离线备份，R2 Bucket 有保留策略和费用告警。
- `/login?setup=1` 只有通过 Access 的 owner 能访问；不同 Folo 账号无法覆盖已有 owner 绑定。
