# Folo2 小红书式 AI 信息流产品 PRD

> 文档定位：仅描述产品体验与前端改造目标，不包含技术实现方案。  
> 产品方向：在保留 Folo 原有账号、订阅、内容源、已读、收藏等基础能力的前提下，重新设计主要消费体验。  
> 核心目标：**把传统 RSS 阅读器改造成“只由我订阅的信息源构成的小红书式 AI 信息流”。**

---

# 1. 产品定义

## 1.1 一句话定义

> **把我的 Folo 订阅变成一个只属于我的“小红书”。**

## 1.2 核心价值

传统 RSS Reader 更像：

```text
先选择订阅源
      ↓
再查看这个源有哪些内容
```

Folo2 希望变成：

```text
我的全部订阅
      ↓
AI 理解内容
      ↓
按主题组织
      ↓
推荐排序
      ↓
双列瀑布流消费
```

用户仍然掌握全部信息来源，但不需要逐个 Feed 阅读。

---

# 2. 产品原则

1. **来源完全由用户控制**
   - 首页内容只来自用户自己的订阅。
   - 不建立陌生内容开放推荐池。

2. **首页 Content-centric**
   - 回答：“我现在最值得看什么？”

3. **订阅页 Source-centric**
   - 回答：“我订阅了谁？”

4. **AI 负责组织，不负责擅自扩充来源**
   - AI 可以分类、翻译、摘要、排序、筛选。
   - AI 不自动订阅陌生 Source。

5. **已读内容退出首页**
   - 首页承担“尚未消费的信息”。
   - 历史内容仍保留在 Source Detail、搜索、收藏中。

6. **用户可以真正把 Feed 看完**
   - 不强调 `999+` 未读焦虑。
   - 已经消费完的 Topic 应出现明确完成态。

---

# 3. 一级信息架构

底部 Navigation 固定：

```text
首页        订阅        设置
```

不增加其他一级 Tab。

---

# 4. 首页 / 发现

首页是整个产品最核心的页面。

## 4.1 Header

```text
Logo                 发现                 🔍   ✨
```

右上角两个入口必须独立：

### 🔍 搜索

解决：

> **“我知道自己要找什么。”**

### ✨ AI 智能筛选

解决：

> **“接下来整个首页给我看这种内容。”**

两者不能合并。

---

# 5. Topic 导航

Header 下方为横向滚动 Topic：

```text
推荐 | AI | Web3 | 3D | 时事政治 | 前端 | Agent | ...
```

规则：

- `推荐` 永远位于最左侧；
- `推荐` 默认选中；
- 当前 Topic 使用橙色文字；
- 当前 Topic 下方有短橙色 underline；
- Topic 表示**语义主题**；
- 不使用“文章 / 帖子 / 图片 / 视频”作为首页 Topic；
- 一个内容可以同时属于多个 Topic。

示例：

```text
AI
Agent
Claude Code
Codex
前端
3D
Web3
时事政治
```

---

# 6. 首页双列瀑布流

首页主体采用：

> **Pinterest / 小红书式双列 Masonry Feed**

要求：

- 深色背景；
- 两列；
- 卡片高度不同；
- 图片比例不同；
- Article / Post / Image / Video 混排；
- 快速扫读；
- 不要做成传统 RSS 的单列长列表。

整体视觉强调：

```text
内容封面
标题
短摘要
Source
时间
```

而不是：

```text
Feed 名称
未读数量
长正文
```

---

# 7. 首页卡片类型

## 7.1 Article Card

```text
┌──────────────┐
│ Cover        │
│              │
├──────────────┤
│ 标题          │
│ 两行摘要      │
│              │
│ avatar 来源 2h│
└──────────────┘
```

要求：

- 有封面优先显示封面；
- 标题控制在合理行数；
- 摘要保持短；
- 显示 Source Avatar；
- 显示相对发布时间。

---

## 7.2 Post Card

适合 X / Reddit 等帖子。

```text
┌──────────────┐
│ Post 正文摘录 │
│              │
│ Image        │
│ （可选）      │
│              │
│ avatar 来源 1h│
└──────────────┘
```

原则：

- 正文比传统文章标题更重要；
- 有图片可直接成为视觉主体；
- 外文 Post 优先展示中文译文。

---

## 7.3 Video Card

```text
┌──────────────┐
│ ▶ Thumbnail  │
│              │
├──────────────┤
│ 视频标题      │
│ avatar 来源 3h│
└──────────────┘
```

---

## 7.4 Image Card

```text
┌──────────────┐
│              │
│    Image     │
│              │
├──────────────┤
│ Caption      │
│ avatar 来源 4h│
└──────────────┘
```

---

# 8. 卡片降级规则

RSS / Feed 数据并不总是完整，所以页面不能依赖“每条内容都有大图”。

## 有 Cover

显示媒体卡。

## 无 Cover + 有长文本

显示纯文字卡：

```text
标题
摘要 / 正文摘录
Source · 时间
```

## 无 Cover + 内容很短

使用 Source 信息补足视觉：

```text
Source Avatar
Source Name

标题

时间
```

## 图片加载失败

自动退化为文字卡。

不得显示 broken image。

---

# 9. 推荐频道

`推荐`不是简单的时间线。

目标：

> 从用户全部未读订阅内容中，判断“现在什么最值得先看”。

考虑：

- 发布时间；
- 用户常看的 Topic；
- 用户常看的 Source；
- 收藏行为；
- 点击行为；
- 不感兴趣反馈；
- 内容质量；
- Source 多样性；
- Topic 多样性。

必须避免：

```text
Karpathy
Karpathy
Karpathy
Karpathy
```

或者：

```text
Claude Code
Claude Code
Claude Code
Claude Code
```

需要让整个 Feed 有自然的内容节奏。

---

# 10. 已读机制

已读是首页的重要产品规则。

## 10.1 默认规则

用户打开某条内容：

```text
Content
   ↓
Detail
   ↓
标记已读
```

返回首页后：

```text
推荐       → 不再出现
AI         → 不再出现
Agent      → 不再出现
其他 Topic → 不再出现
```

即：

> 一旦已读，默认从所有首页 Feed 中退出。

---

## 10.2 历史仍然存在

已读不会删除内容。

仍可通过：

```text
订阅 → Source Detail
搜索
收藏
阅读历史
```

找到。

---

# 11. “今天已经看完”状态

当一个 Topic 已经没有未读内容：

```text
今天的 AI 内容已经看完 ✓

新内容到达后会自动出现在这里。

[查看最近已读]
```

首页推荐全部消费完：

```text
今天值得看的内容已经看完 ✓

稍后有新内容时再回来看看。
```

这应该成为产品特色，而不是一个失败的 Empty State。

---

# 12. 搜索

点击：

```text
🔍
```

进入 Search。

placeholder：

```text
搜索文章、帖子、来源、Topic…
```

可搜索：

- 标题；
- 正文；
- 中文译文；
- Source；
- Topic；
- Tag。

搜索范围：

```text
已读
+
未读
```

搜索与首页推荐状态无关。

例如首页只展示未读，但搜索：

```text
Claude Code
```

仍然可以找到两个月前已经看过的内容。

---

# 13. AI 智能筛选

这是 Folo2 最重要的新增能力之一。

## 13.1 产品定义

AI 智能筛选不是：

```text
Search
```

也不是：

```text
生成一个临时搜索结果页
```

而是：

> **用自然语言重新配置整个首页。**

---

# 14. AI Filter Bottom Sheet

点击首页：

```text
✨
```

当前首页保留为背景并变暗。

底部弹出 Bottom Sheet。

结构：

```text
────────────

✨ AI 智能筛选

用自然语言描述你想看的内容，
AI 将为你生成个性化信息流。

┌────────────────────────────┐
│ 最近一周给我多推 Agent      │
│ Harness、Claude Code、Codex│
│ 的技术内容，不要融资新闻和  │
│ 入门教程                    │
│                      38/300│
└────────────────────────────┘

推荐主题

[AI Agent]
[3D 项目]
[前端新技术]
[只看英文一手来源]
[过滤营销内容]

[取消]          [✨ 生成信息流]
```

---

# 15. AI 筛选后的首页

假设默认：

```text
推荐 | AI | Web3 | 3D | 时事政治 | 前端 | Agent
```

用户输入：

```text
最近一周给我多推 Agent Harness、Claude Code、Codex 的技术内容，
不要融资新闻和入门教程。
```

首页重新变成：

```text
推荐 | Claude Code | Codex | Agent Harness | 工作流 | 源码分析
```

注意：

> **它仍然是首页。**

不能生成：

```text
AI 筛选结果页
```

---

# 16. AI Filter 必须改变的内容

生成信息流后，同时修改：

1. 推荐 Tab 的内容；
2. 推荐右侧 Topic；
3. Topic 顺序；
4. Topic 内内容；
5. 推荐权重；
6. Feed 中内容的整体方向。

例如：

```text
过滤前：

Web3
AI 新闻
3D
前端
时政
Agent

过滤后：

Claude Code
Codex
Agent Harness
Coding Agent Workflow
Context Engineering
源码分析
```

---

# 17. AI 筛选状态

筛选生效后，在 Topic 下方增加轻量状态栏：

```text
Claude Code × Codex × Agent Harness    [编辑] [重置]

近 7 天 · 未读 · 技术内容 · 已过滤融资 / 入门
```

不要让这一栏占据太多垂直空间。

---

# 18. 编辑 AI Filter

点击：

```text
编辑
```

再次打开 Bottom Sheet。

保留已有输入：

```text
最近一周给我多推 Agent Harness、Claude Code、Codex...
```

用户可以继续追加：

```text
只看源码分析和工程实践。
```

更新后重新计算首页。

---

# 19. 重置 AI Filter

点击：

```text
重置
```

恢复：

```text
推荐 | AI | Web3 | 3D | 时事政治 | 前端 | Agent
```

并恢复默认推荐。

重置不得影响：

- 订阅；
- 已读；
- 收藏；
- Source；
- 阅读历史。

---

# 20. 推荐负反馈

首页 Card 长按或 `...`：

```text
收藏
标记已读
不感兴趣
少推此类内容
少推这个来源
屏蔽这个来源
```

## 不感兴趣

让推荐系统理解：

```text
这一条不适合我
```

## 少推此类内容

针对 Topic / 内容方向。

## 少推这个来源

Source 仍然订阅，但首页降低曝光。

## 屏蔽这个来源

Source 仍在订阅管理中，但默认不进入推荐。

设置中允许恢复。

---

# 21. AI Topic

顶部 Topic 不应该是永久写死的一排分类。

采用：

```text
固定核心 Topic
+
AI 动态 Topic
```

AI 根据最近内容自动发现：

```text
Claude Code
Codex
Agent Harness
Robotics
Gaussian Splatting
...
```

要求：

- 名称简洁；
- 相似 Topic 合并；
- 不频繁抖动；
- 内容量过低可隐藏；
- 用户固定的 Topic 不自动消失。

---

# 22. Topic 管理

用户可以管理首页频道。

入口：

```text
设置 → 频道管理
```

或者 Topic 长按。

示例：

```text
频道管理

☰ AI             已固定
☰ Agent          已固定
☰ Claude Code
☰ 前端
☰ 3D
☰ 时事政治

隐藏频道
Web3
游戏
```

支持：

- 拖拽排序；
- 固定；
- 取消固定；
- 隐藏；
- 恢复。

`推荐`不能被删除。

---

# 23. 自动翻译

## 23.1 适用内容

重点支持：

```text
Article
Post
```

如果原内容主要语言不是中文：

```text
AI 自动翻译
        ↓
简体中文
```

---

## 23.2 首页

外文内容优先显示：

- 中文标题；
- 中文摘要；
- 中文帖子正文摘录。

Source 名称、作者名等保持原样。

---

## 23.3 详情页

提供：

```text
[译文] [原文]
```

默认：

```text
译文
```

用户可以随时切回原文。

翻译失败：

```text
直接显示原文
```

AI 处理不得阻止内容阅读。

---

# 24. AI 摘要

长 Article 支持：

```text
一句话摘要

3~5 条重点
```

详情页：

```text
AI 摘要
────────
• ...
• ...
• ...
```

首页仅使用简短摘要。

---

# 25. Article Detail

结构：

```text
←     Source                       ☆   ...

标题

作者 · Source · 发布时间

[译文] [原文]

Cover

AI 摘要

正文

图片

查看原文
```

行为：

- 进入后默认标记已读；
- 支持收藏；
- 支持原文 / 译文；
- 支持跳原始网页。

---

# 26. Post Detail

结构更接近社交帖子：

```text
←

Avatar  Author
        Source · Time

[译文] [原文]

Post Content

Images / Video

查看原帖
```

支持：

- 收藏；
- 已读；
- 原文 / 译文。

---

# 27. 订阅页

订阅页保留 Folo 的 Source-centric 思路。

页面：

```text
订阅

[文章] [帖子] [图片] [视频]

⭐ 收藏

⌄ X.com
  Andrej Karpathy
  Elon Musk
  Hamel Husain

⌄ Blogs
  Simon Willison
  Addy Osmani
  Armin Ronacher

⌄ Reddit
  LocalLLaMA

⌄ YouTube
  ...
```

视觉要求：

- 比首页更偏列表；
- 信息密度高；
- Source Avatar 较小；
- 分组可收起；
- 有新内容的 Source 可显示橙点。

---

# 28. 订阅页媒体筛选

顶部：

```text
文章 | 帖子 | 图片 | 视频
```

语义是：

> 根据 Source 的主要内容类型筛选订阅源。

不是首页 Feed 的媒体筛选器。

例如：

### 文章

```text
OpenAI Blog
Simon Willison
Addy Osmani
```

### 帖子

```text
Karpathy
Elon Musk
Reddit
```

### 视频

```text
YouTube
Bilibili
```

---

# 29. 收藏入口

订阅页顶部：

```text
⭐ 收藏
```

进入全部收藏内容。

收藏不受：

```text
readAt
```

过滤。

---

# 30. Source Detail

点击某个 Source：

```text
Andrej Karpathy

今天
- Post A
- Post B

昨天
- Post C

更早
- Post D
```

这里必须显示：

```text
已读
+
未读
```

这是用户回溯一个 Source 全部历史内容的地方。

---

# 31. 添加订阅

沿用 / 魔改 Folo 已有添加订阅体验。

用户可以输入：

```text
RSS URL
网站 URL
X URL / 用户
Reddit URL
YouTube URL
其他 Folo 支持的来源
```

系统展示 Source Preview：

```text
Avatar
Source Name
Platform
最近内容
```

用户确认：

```text
订阅
```

普通用户不需要感知 RSSHub Route、Cookie、Header 等底层概念。

---

# 32. 登录

登录沿用 Folo 账号体系。

用户在新前端看到：

```text
Continue with Google
```

其产品语义仍然是：

> 登录我的 Folo 账号。

登录成功后：

- 恢复原有订阅；
- 恢复收藏；
- 恢复已读状态；
- 恢复账号资料。

新前端不再额外创造一套“Folo2 Account”。

---

# 33. 设置页

整体继续采用深色分组卡片。

## AI

```text
自动翻译            ON
自动摘要            ON
主题分类            ON
推荐偏好            >
频道管理            >
```

## 阅读

```text
打开即标记已读      ON
字体大小            中 >
夜间模式随系统      ON
阅读历史            >
```

## 推荐

```text
已屏蔽来源          >
不感兴趣记录        >
重置推荐偏好        >
```

## 订阅

```text
导入 OPML           >
导出 OPML           >
```

## 账号

```text
Avatar
Google Account

退出登录
```

---

# 34. 关键 Empty / Error State

## 无订阅

```text
你的首页还没有内容

先添加一些你真正关心的信息源。

[添加订阅]
[导入 OPML]
```

## 首页全部已读

```text
今天值得看的内容已经看完 ✓
```

## Topic 全部已读

```text
今天的 Agent 内容已经看完 ✓
```

## 加载中

使用 Skeleton。

## AI 处理中

原文继续显示。

可轻量提示：

```text
AI 翻译中…
```

## AI 失败

保持原文，不阻塞。

## 内容同步异常

保留已有内容：

```text
暂时无法同步新内容
正在展示已有内容
```

---

# 35. 首页视觉规范

整体参考用户提供的视觉方向：

```text
Dark Mode
+
Xiaohongshu-like Masonry
+
Folo information density
```

颜色：

```text
页面背景：接近纯黑
卡片：深灰
Primary Text：白色
Secondary Text：灰色
Accent：橙色
```

橙色主要用于：

- 当前 Topic；
- Bottom Navigation Active；
- 未读点；
- 主要 CTA；
- AI Filter Generate；
- Toggle Active。

不要做成：

- SaaS Dashboard；
- 大面积渐变；
- 玻璃拟态；
- ChatGPT 式对话主页；
- 传统 RSS 单列列表。

---

# 36. 一次性交付页面

前端魔改最终至少需要具备：

```text
Login

Default Home
AI Filter Bottom Sheet
AI Filtered Home

Search

Article Detail
Post Detail

Subscriptions
Favorites
Source Detail
Add Subscription

Topic Management

Settings

Empty State
Loading State
Error State
```

---

# 37. 核心交互验收

## 首页

- [ ] 两列瀑布流；
- [ ] 混合 Article / Post / Image / Video；
- [ ] Topic 可切换；
- [ ] 推荐位于最左；
- [ ] 已读内容退出首页；
- [ ] 已全部消费显示“看完了”。

## 搜索

- [ ] 搜索与 AI Filter 是两个入口；
- [ ] 可以搜历史已读内容；
- [ ] 搜索不改变首页状态。

## AI Filter

- [ ] ✨ 打开 Bottom Sheet；
- [ ] 输入自然语言；
- [ ] 点击生成后仍停留首页；
- [ ] 推荐内容变化；
- [ ] Topic 变化；
- [ ] 有 Active Filter 状态；
- [ ] 支持编辑；
- [ ] 支持重置。

## 翻译

- [ ] 外文 Article / Post 首页优先中文；
- [ ] Detail 默认译文；
- [ ] 支持原文切换；
- [ ] AI 失败不阻塞阅读。

## 订阅

- [ ] 保持 Source-centric；
- [ ] Source 分组；
- [ ] 文章 / 帖子 / 图片 / 视频筛选；
- [ ] Source Detail 显示完整历史；
- [ ] 收藏入口明确。

## 登录

- [ ] 新前端可进入 Folo Google 登录；
- [ ] 登录后恢复原 Folo 账号数据；
- [ ] 不创建第二套产品账号概念。

---

# 38. 产品边界

当前产品不做：

- 社交社区；
- 评论；
- 点赞关系；
- UGC 发布；
- 创作者后台；
- 广告；
- 陌生内容开放推荐；
- AI Chat 作为一级入口；
- 重新设计新的账号体系；
- 强迫用户理解 RSSHub 技术细节。

---

# 39. 最终产品模型

```text
Folo Account
     ↓
我的全部订阅
     ↓
个人 Content Pool
     ↓
AI 理解 / 翻译 / Topic
     ↓
推荐 / 筛选 / 搜索
     ↓
小红书式双列信息流
```

四种核心消费方式：

```text
推荐
→ 系统决定我的订阅里先看什么

Topic
→ 按长期主题消费

AI 智能筛选
→ 用自然语言临时重构整个首页

搜索
→ 主动查找历史和当前内容
```

最终产品定义：

> **保留 Folo 的订阅与账号基础能力，把主要消费体验重新设计成一个只由用户自己订阅内容构成的小红书式 AI 信息流。**
