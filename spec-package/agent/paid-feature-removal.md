# Folo 付费与服务端 AI 改造策略 v2

## 结论

Tantan 保留 RSS 与内容能力，也保留对用户有价值的翻译、摘要、分类和智能筛选交互；这些 AI 能力只能调用 Go 中间层和站点所有者在服务端配置的 Key。Folo 会员、额度、充值和官方付费 AI 不出现在产品中，也不允许通过代理访问。

## 必须保留

- Folo 账号身份和用户资料。
- RSS Subscription、Feed、Entry、Source、List、Read、Collection/Favorite 数据与交互。
- `packages/internal/store/src/modules/subscription/**`，这里的 subscription 是 RSS 订阅。
- Folo Mobile 的首页外导航、栈式详情、订阅、发现、设置、通用外观与可访问性模式。
- 翻译、摘要、Topic 分类和 AI Filter 的 UI 入口，但 Provider 改为 `/api/tantan/v1` 的服务端 Gemini 能力。

## 必须移除或屏蔽

- Plan、Power、Wallet、Upgrade、Stripe/payment subscription、Referral、会员 badge 和付费 CTA。
- Folo AI Chat、Folo AI endpoint、Folo AI quota/credit、购买额度和会员解锁判断。
- Folo AI 设置远程同步；AI Provider/Key 由 Go 服务管理，浏览器只得到 metadata。
- 所有直接 `api.folo.is` 或 Gemini 请求；浏览器只调用同源 `/api`。

## 禁止做法

- 禁止按 `subscription`、`upgrade`、`plan` 等字符串宽泛删除文件或导出。
- 禁止为让编译通过而删除 RSS store、Feed/Entry 类型、用户身份或非付费设置。
- 禁止把 Folo token 或 AI Key 放入 localStorage、IndexedDB、URL、日志、HAR、fixture、构建产物或 Git。
- 禁止在被 route policy 拒绝后由浏览器直连 Folo 作为 fallback。

## 代码判定流程

对每个候选符号先回答：

1. 它代表 RSS subscription 还是 payment subscription？前者保留。
2. 它是 UI 付费门槛，还是内容领域本身？仅移除门槛与支付能力。
3. 它是否调用 Folo AI？若是，把有价值的用户交互接到 Go 服务端 Gemini API；无价值的 Folo AI Chat 产品入口删除。
4. 它是否被 Home、Subscription、Discover、Detail、Settings 任一 Mobile Web 路径消费？消费中的非付费能力必须保留。

每个移除由 import/route/network 扫描证明：付费入口计数为 0、禁用 Folo 路由出站计数为 0、RSS subscription 存在性测试通过、服务端 AI 核心流程通过且浏览器 Key 字段为 0。
