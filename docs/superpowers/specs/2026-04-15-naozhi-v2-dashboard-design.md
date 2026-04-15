# Naozhi V2.0 设计规格文档

> **From AI Gateway to CTO Operating System**
>
> V1 解决了 "随时随地和 Claude 对话"。V2 解决 "AI 随时随地为你工作"。

---

## 1. 概述

### 1.1 定位转变

| | V1 (当前) | V2 (目标) |
|---|---|---|
| **定位** | AI Gateway | CTO Operating System |
| **交互模式** | 被动: 你问它答 | 主动: Patrol 巡逻 + 知识编译 + 审批工作流 |
| **知识** | 无沉淀, session 关了就没了 | 知识编译 (Karpathy 方法论) + Obsidian 集成 |
| **CLI 关系** | 独立: Dashboard 和 CLI 各自为政 | 打通: CLI session 双向同步, 统一检索 |
| **自动化** | Cron 定时任务 | Patrol 巡逻 + Approval 审批 + Proactive Insights |
| **可视化** | 对话流 | Home 仪表板 + 知识图谱 + 活动流 |

### 1.2 目标用户

- **Primary**: CTO / Tech Lead — 每天通过 Dashboard 管理多个 AI agent, 做架构决策, 审查代码, 监控基础设施
- **Secondary**: 团队成员 — 通过 IM (飞书/Slack) @naozhi 提问, Naozhi 先查 CTO 的知识库再回答

### 1.3 设计原则

1. **Dashboard First** — 浏览器是主战场, 不是 IM 的附属品
2. **Knowledge Compilation over RAG** — 借鉴 Karpathy LLM-Wiki 方法论, LLM 把原始素材一次性编译成结构化 wiki, 而非每次 RAG 切片检索
3. **Obsidian Native** — 直接读取和渲染用户的 Obsidian vault, 不要求导出/同步
4. **CLI Session Sync** — 与本地 Claude Code CLI 双向打通 (Dashboard 能看 CLI 工作, CLI 能 resume Dashboard 对话)
5. **Single Binary** — 保持 Go embed 单文件部署的简洁性, 前端演进不牺牲部署体验
6. **Progressive Enhancement** — 分阶段交付, 每阶段都有可用价值

### 1.4 成功指标

| 指标 | 目标 | 衡量方式 |
|------|------|---------|
| Dashboard 日活使用时长 | 提升 3x | 对比 V1 WebSocket 连接时长 |
| 历史知识检索命中率 | > 80% | 搜索查询有结果的比率 |
| Patrol 自动处理任务 | > 50% | 不需要人工干预完成的 patrol 执行次数 |
| 移动端 session 占比 | > 20% | 来自移动端 UA 的 session 数 |

---

## 2. 信息架构

### 2.1 导航结构

**桌面端 (7 tabs)**:
```
Home | Chat | Knowledge | Wiki | Patrols | Graph | Approvals
```

**移动端 (5 tabs)** — Wiki 和 Graph 合并到 Knowledge 的子视图:
```
Home | Chat | Knowledge | Patrols | Approvals
```

### 2.2 Home (CTO 仪表板)

首页是 CTO 的操控台, 一眼看到全局状态:

| Widget | 内容 |
|--------|------|
| **Today's Overview** | 4 张 stat 卡片: 活跃 sessions, Patrol 数, 今日成本, Wiki 页数 |
| **Quick Actions** | 4 个快捷按钮: New Chat / Patrols / Approvals / Graph |
| **Patrol Status** | 实时巡逻状态列表 (dot + name + status + time) |
| **Pending Approvals** | 紧急审批提醒 (红色, 可点击跳转) |
| **Activity Feed** | 全局活动流 — PR review, Wiki 编译, Cost 监控, CLI 完成, IM 团队问答 |
| **Recently Compiled** | 最近更新的 Wiki 页面 |

### 2.3 Chat (对话视图)

三栏布局:

| 区域 | 内容 |
|------|------|
| **左 Sidebar** | 搜索框 + "+" 新建按钮 + 标签筛选 (All/Review/Projects/CLI/Research) + 分组 session 列表 (Pinned/Today/CLI/Yesterday) |
| **中 对话** | 消息流 + 工具卡片 (可折叠) + Diff 渲染 + 代码高亮 + Running Banner |
| **右 Context Panel** | 三 tab: Saved (bookmarks) / Related (反向链接) / AI (元对话) |

### 2.4 Knowledge (Obsidian Vault 浏览器)

| 区域 | 内容 |
|------|------|
| **左** | Vault 路径配置 + 文件树 (目录折叠展开) |
| **中** | goldmark 渲染 Obsidian Markdown: frontmatter, [[wikilinks]], callouts, tables, task checkboxes |
| **右** | AI 对话面板 — 针对当前笔记提问, AI 结合笔记 + Wiki 知识回答 |

### 2.5 Wiki (知识编译)

| 区域 | 内容 |
|------|------|
| **左** | 编译页面列表, 按 Projects / Services / Customers 分组 |
| **中** | Wiki 页面渲染: compiled metadata (时间/来源数/实体) + 正文 + 来源标注 |
| **右** | Sources 溯源面板 (每条知识的来源 session/CLI/IM/Obsidian) + Lint 状态 |
| **操作** | Ingest (手动/Cron 触发编译) / Lint (矛盾检测) / Re-compile |

### 2.6 Patrols (自主巡逻)

卡片网格布局, 每卡片包含:
- 图标 + 名称 + 状态 badge (Running/Paused/Disabled)
- 描述 + cron 表达式 / 触发条件
- 操作按钮 (Pause/Resume/Edit/Logs)
- 最近执行日志 (时间 + 状态 + 摘要)

**预设 Patrol**: PR Sentinel, Cost Watchdog, Infra Health, Dependency Auditor

### 2.7 Graph (知识图谱)

- SVG 力导向图可视化
- 节点类型及颜色: Knowledge Hub (紫), AWS Service (蓝), Project/Customer (绿), Infrastructure (黄), Security Issue (红)
- 点击节点: 右侧面板展示详情 (描述 + 连接关系 + 来源数 + 操作: Open Wiki / View Sources)
- 图例 + 缩放控制

### 2.8 Approvals (审批队列)

- 全宽审批卡片列表
- 分级: urgent (红色左边框) / normal (黄色左边框)
- 每卡片: 图标 + 标题 + 来源 Patrol + 时间 + 详情 (terraform plan / PR info)
- 操作按钮: Approve (绿) / Reject (灰) / View Details
- 空状态: "All caught up!" checkmark

---

## 3. UI 组件设计

### 3.1 对话体验升级

#### 3.1.1 代码块

| 特性 | 实现 |
|------|------|
| 语法高亮 | Shiki WASM (CDN, 150+ 语言, 按需加载) |
| 行号 | 左侧灰色数字, 可选 |
| 头部 | 语言标签 + 文件名 (从工具参数提取) |
| Copy | 按钮, 点击反馈 "Copied!" (2s 恢复) |
| Insert | 将代码插入输入框 (可选) |

#### 3.1.2 工具调用卡片

- **折叠态** (默认): `[icon] [tool_name] [param_summary] [status ✓]` 单行
- **展开态** (点击): 显示完整输入/输出
- **Edit 工具**: 自动渲染 inline diff (green +lines / red -lines)
- **Agent 工具**: 显示子 agent 名称 + 描述 + 状态
- **动画**: 展开/折叠 max-height transition

#### 3.1.3 Diff 渲染

- 解析 Edit 事件的 `old_string` / `new_string` 字段
- Unified diff 视图: 绿色(+) / 红色(-) / 灰色(context)
- 头部: 文件路径 + 统计 (`+N -M`)
- 行号列

#### 3.1.4 长输出折叠

- 超过 N 行自动折叠 (可配置, 默认 10 行)
- 渐变遮罩 (transparent → background)
- 按钮: "▼ 展开全部 (N 项)" / "▲ 收起"
- CSS max-height + transition 动画

#### 3.1.5 消息 Bookmark

- AI 消息 hover 时右上角出现 🔖 按钮
- 点击: 保存到 Context Panel > Saved 标签
- 支持添加标签 (#security, #waf, #architecture)
- 存储: `~/.naozhi/bookmarks.json`
- 跨 session 可搜索

#### 3.1.6 Running Banner

- 对话顶部蓝色横幅 (仅在 Running 状态显示)
- 内容: 脉冲圆点 + 当前工具名/描述 + 耗时计时
- Sub-agent 行: 当有并行子 agent 时, 显示每个 agent 状态
- 自动消失: turn 完成后 fade out

### 3.2 导航与组织

#### 3.2.1 全局搜索 (Cmd+K / Ctrl+K)

- 全屏覆盖层 modal, backdrop blur
- 输入框自动聚焦, 实时过滤
- 结果按来源分组:
  - 🔖 Bookmarks
  - 🌐 Dashboard Sessions
  - 💻 CLI Sessions
  - 📖 Obsidian Vault
  - 📚 Wiki Pages
- 匹配文本高亮 (蓝色 bold)
- 键盘导航: ↑↓ 选择, Enter 跳转, ESC 关闭

#### 3.2.2 Session 标签筛选

横向滚动标签条:
```
[All] [🔍 Review] [📦 Projects] [💻 CLI] [📖 Research]
```
- 按 agent 类型自动归类 (从 `agent_commands` 配置映射)
- 选中标签高亮, 过滤 session 列表

#### 3.2.3 Session 时间分组

| 分组 | 条件 |
|------|------|
| 📌 Pinned | 用户手动置顶 |
| Today | 今天活跃的 session |
| 💻 CLI Sessions | 来自 Claude CLI 的 session (绿色终端图标) |
| Yesterday | 昨天的 session |
| This Week | 本周其他 |
| Earlier | 更早 (opacity 降低) |

#### 3.2.4 新建 Session Modal (Cmd+N)

- **Agent 四宫格**: General / Code Reviewer / Researcher / Planner
  - 每卡片: 图标 + 名称 + 描述 + 模型
  - 点击选中高亮, 单选
- **Project 绑定** (可选): 从 `projects.root` 自动发现的项目列表
  - 点击选中/取消, 单选
- **Working Directory**: 文本输入, 默认 `session.cwd`
- **Create**: 创建 session + 跳转到空白对话 + Welcome State

#### 3.2.5 Notification Center

- 右上角铃铛按钮 + badge 计数
- 点击弹出下拉面板 (绝对定位, z-index 高于其他覆盖层)
- 通知分级: urgent (红色左边框) / unread (蓝色背景) / read (默认)
- 通知类型:
  - ⚠️ Patrol 告警 (安全问题/异常/失败)
  - 📥 审批请求 (terraform/merge/deploy)
  - 📚 Wiki 编译完成
  - 💬 IM 团队问答 (Naozhi 代答)
  - 💰 Cost 报告
- "Mark all read" 一键清除
- 点击通知跳转到对应视图

### 3.3 Context Panel (右侧面板)

可折叠, 按钮切换显隐. 三个 tab:

#### 3.3.1 Saved (Bookmarks)

- 当前 session 的 bookmarks
- "Related from other sources" — 跨 Dashboard/CLI/IM/Obsidian 的相关 bookmarks
- 每条: 来源 badge (🌐 Dashboard / 💻 CLI / 💬 Feishu / 📖 Obsidian) + 内容摘要 + 标签 + 操作 (Jump / View / Resume)

#### 3.3.2 Related (反向链接)

- 基于当前对话话题, 自动关联其他来源的内容
- 匹配方式: Phase 1 关键词, Phase 2 语义
- 每条: 来源 badge + 摘要 + 时间

#### 3.3.3 AI Chat

- 针对当前 session 上下文的独立对话
- 可以问元问题: "还有什么遗留风险?", "和上周的讨论对比一下"
- AI 结合 session 内容 + Wiki 知识回答

---

## 4. 知识系统设计

### 4.1 Karpathy 三层架构

借鉴 [Karpathy LLM-Wiki](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) 的知识编译方法论:

```
┌─────────────────────────────────────────────────────┐
│ Layer 1: Raw Sources (不可变原始素材)                 │
│  Dashboard 对话 │ CLI Sessions │ Obsidian │ IM 消息  │
└────────────────────────┬────────────────────────────┘
                         │ LLM Ingest
                         ▼
┌─────────────────────────────────────────────────────┐
│ Layer 2: Compiled Wiki (LLM 编译的结构化知识)         │
│  ~/.naozhi/wiki/*.md                                │
│  实体页面 + [[wikilinks]] + 来源标注 + 全文索引       │
└────────────────────────┬────────────────────────────┘
                         │ Query / Lint / Browse
                         ▼
┌─────────────────────────────────────────────────────┐
│ Layer 3: Schema (规则 & 配置)                         │
│  CLAUDE.md 编译规则 │ Dashboard 配置 │ Lint Rules    │
└─────────────────────────────────────────────────────┘
```

**为什么"知识编译"优于 RAG**: 传统 RAG 每次查询都要切片→向量检索→拼上下文, 有信息丢失. Karpathy 方法是 LLM 读完原始素材后, 一次性生成结构化 wiki 页面 (带摘要、实体、交叉引用), 后续查询直接搜 wiki. 对于个人级知识库 (<1-2M tokens), 整个 wiki 甚至可以放进上下文窗口.

#### Layer 1: Raw Sources

| 来源 | 写入方式 | 数据内容 |
|------|---------|---------|
| Dashboard 对话 | WebSocket 事件流实时写入 | 消息文本 + 工具调用 + 结果 |
| CLI Sessions | `~/.claude/history.jsonl` 定期扫描 (60s) | 用户 prompt + project path + session UUID |
| Obsidian Vault | 配置路径直接读取 .md 文件 | Markdown 全文 |
| IM Messages | 现有 platform 适配器 | Feishu/Slack/Discord/WeChat 消息 |

#### Layer 2: Compiled Wiki

- 存储: `~/.naozhi/wiki/*.md`
- 文件格式: 标准 Markdown + YAML frontmatter (compiled time, sources, entities)
- 按实体/概念/项目组织: `aws-waf.md`, `cloudfront.md`, `zeelool-cdn.md`
- `[[wikilinks]]` 交叉引用, 自动维护反向链接
- 全文索引: bleve (纯 Go, 零外部依赖, 支持中文分词 jieba)

#### Layer 3: Schema

- `~/.naozhi/wiki/CLAUDE.md`: 知识编译规则 (分类约定, 实体命名, 质量标准)
- Dashboard 配置 (`config.yaml`): 展示路径, 标签过滤, Ingest 频率
- Lint Rules: 矛盾检测, 过期标记 (>N 天未验证), 孤立页清理

### 4.2 知识编译操作

#### 4.2.1 Ingest (编译)

```
触发方式:
  - 手动: Dashboard "🔄 Ingest" 按钮
  - 定时: Cron 表达式 (如 @every 6h)
  - 自动: 新消息/新 CLI session 触发 (可配置)

流程:
  1. 扫描 raw sources, 筛选出新增内容 (基于时间戳)
  2. 启动 Claude Code CLI 子进程 (复用现有 cli 包)
  3. Prompt: "读取以下新素材, 提取实体和关键结论, 更新 wiki 页面, 维护交叉引用"
  4. CLI 进程读取 wiki 目录 + 新素材, 输出更新后的 wiki 文件
  5. 更新全文索引
```

#### 4.2.2 Query (查询)

- 通过 Cmd+K 全局搜索
- 同时检索: wiki 编译页 + 原始 source + bookmarks
- Phase 1: 关键词搜索 (bleve BM25)
- Phase 2: 语义搜索 (向量嵌入, 可选)

#### 4.2.3 Lint (健康检查)

```
触发: 手动 / Cron (如每周一次)

检测项:
  - 矛盾信息: 两个 wiki 页面对同一实体的描述不一致
  - 过期结论: 超过 N 天未被新 source 验证的关键声明
  - 孤立页: 没有任何 incoming wikilink 的页面
  - 缺失来源: 编译内容没有对应的 raw source

输出: Lint 报告, 显示在 Wiki 视图右侧面板
```

### 4.3 Obsidian Vault 集成

#### 4.3.1 配置

```yaml
knowledge:
  obsidian:
    vault_path: "~/keith-space/Obsidian/vaults/Keith_Space_2026"
    include_paths: ["Things/", "page/", "journals/"]
    exclude_paths: [".obsidian/", "assets/", ".trash/"]
```

#### 4.3.2 渲染引擎

Go 侧使用 `goldmark` (CommonMark 兼容) + 自定义扩展:

| Obsidian 语法 | 渲染支持 | 实现方式 |
|--------------|---------|---------|
| `[[wikilinks]]` | ✓ 点击导航 | goldmark-wikilink 扩展 + 文件名→路径索引 |
| `> [!tip]` callouts | ✓ 彩色边框卡片 | 自定义 AST 扩展 |
| YAML frontmatter | ✓ 属性网格 | goldmark-frontmatter |
| Task checkboxes | ✓ 复选框 | CommonMark 扩展 |
| Tables | ✓ 响应式表格 | GFM table 扩展 |
| Dataview 查询 | ✗ 暂不支持 | 需要 Obsidian runtime |
| `![[embed]]` 图片 | Phase 2 | 需实现文件路径解析 |

#### 4.3.3 API

```
GET /api/vault/tree                    → 目录树 JSON
GET /api/vault/read?path=Things/...    → 渲染后 HTML
GET /api/vault/raw?path=Things/...     → 原始 Markdown
```

### 4.4 CLI Session 同步

#### 4.4.1 数据源

| 文件 | 内容 | 用途 |
|------|------|------|
| `~/.claude/history.jsonl` | 每条 prompt (timestamp, session UUID, project path) | 导入搜索索引, 展示 CLI 会话列表 |
| `~/.claude/sessions/*.json` | 会话元数据 (PID, cwd, startedAt) | 实时发现活跃 CLI 进程 |
| 现有 `discovery` 模块 | 扫描外部进程 | 进程发现 + takeover |

#### 4.4.2 同步策略

- **定期扫描** (60s): 读取 `history.jsonl` 新增行, 导入到统一搜索索引
- **实时发现**: 监控 `sessions/` 目录变化, 新进程出现时通知 Dashboard (WebSocket)
- **展示**: sidebar "💻 CLI Sessions" 独立分组, 绿色终端图标 `>_`

#### 4.4.3 操作

- **View**: 查看 CLI session 的对话历史 (从 session JSONL 文件还原)
- **Resume**: 在 Dashboard 中接续 CLI session (复用现有 takeover 机制: SIGTERM → `--resume`)

### 4.5 统一搜索引擎

#### 4.5.1 索引来源

| 来源 | 写入时机 | 数据内容 | 字段 |
|------|---------|---------|------|
| Dashboard 事件 | 实时 (WebSocket) | 消息 + 工具输出 | text, session_key, timestamp |
| CLI history | 60s 扫描 | prompt + project path | text, project, session_id, timestamp |
| Obsidian vault | 启动 + 文件变更 | .md 全文 | text, path, title, tags |
| Wiki 编译页 | Ingest 后 | 编译内容 + 元数据 | text, entities, sources, compiled_at |
| Bookmarks | 实时 | 片段 + 标签 | text, tags, source, session_key |

#### 4.5.2 技术选型

- **推荐**: `bleve` — 纯 Go, 零外部依赖 (无需 CGO), 支持中文分词
- **备选**: SQLite FTS5 — 更成熟但需要 CGO
- 索引存储: `~/.naozhi/search.bleve/`

#### 4.5.3 API

```
GET /api/search?q=WAF+误拦截&source=all
→ {
    "results": [
      {"source": "bookmark", "title": "WAF Rate Limit 调优", "match": "...误拦截率 3.2%→0.4%...", "score": 0.95},
      {"source": "dashboard", "title": "WAF Rule 优化", "match": "...SQLi rule 误判...", "score": 0.88},
      {"source": "cli", "title": "terraform-waf", "match": "...Count 模式...", "score": 0.82},
      {"source": "vault", "title": "AWS WAF 最佳实践", "match": "...排除列表...", "score": 0.75}
    ]
  }
```
## 5. 自主 Agent 系统 (Autonomous Agents)

Naozhi V1 的 cron 系统提供了基础的定时任务执行能力（`robfig/cron` + `internal/cron` 包）。V2 在此基础上构建完整的自主 Agent 框架：有状态追踪的 Patrol、人机协作的 Approval Gateway、以及主动推送洞察的 Proactive Insights 系统。

### 5.1 Patrol Mode (巡逻模式)

#### 5.1.1 概念

Patrol 是预配置的自动化 Agent 任务，与现有 Cron Job 的核心区别：

| 维度 | 现有 Cron Job | Patrol |
|------|-------------|--------|
| 状态追踪 | 仅记录 `last_result` / `last_error` | 完整生命周期状态机 + 执行历史 |
| 日志 | 单条结果覆盖式存储 | JSONL 持久化全历史 |
| 通知 | 仅推送到创建者 chat | 可路由到多目标 (Dashboard + IM) |
| 触发 | 仅 cron schedule | Schedule + Event webhook + Manual |
| 审批 | 无 | 可配置 Approval Gateway 门控 |
| MCP 集成 | 无 | 可声明依赖的 MCP server |
| 资源限制 | 全局 `max_jobs` | Per-patrol budget + timeout |

Patrol 基于现有 `internal/cron` 包扩展，复用 `robfig/cron` 调度器和 session router 消息发送机制。

#### 5.1.2 配置格式

Patrol 在 `config.yaml` 的 `patrols` 段声明：

```yaml
patrols:
  pr-review:
    trigger: "github:pr_opened"     # event-based 触发
    agent: code-reviewer            # 复用 agents 配置中的 agent
    model: sonnet                   # 可覆盖 agent 默认 model
    prompt: "Review this PR for security issues, code quality, and best practices"
    notify: [feishu, dashboard]     # 通知路由目标
    approval_required: false
    timeout: "5m"

  cost-alert:
    schedule: "@every 1h"           # cron 表达式 (复用 robfig/cron 语法)
    agent: general
    prompt: "Check AWS Cost Explorer. Alert if daily spend > $5 or anomalous pattern."
    notify: [feishu, dashboard]
    budget: "$5.00/day"             # Bedrock token 花费上限

  infra-health:
    schedule: "@every 30m"
    agent: general
    prompt: "Check EKS cluster health, RDS status, ALB targets, CloudFront error rates"
    notify: dashboard
    mcp_servers: ["aws"]            # 需要的 MCP server (传递给 CLI --mcp-server)

  dep-audit:
    schedule: "0 9 * * *"           # 标准 cron: 每天 09:00
    agent: general
    prompt: "Scan project dependencies for CVEs. Auto-create PR for critical fixes."
    auto_fix: true                  # 允许 agent 自主执行修复操作
    approval_required: true         # 修复操作需人工审批
    work_dir: "/home/naozhi/projects"
```

#### 5.1.3 数据结构

新增 `internal/patrol` 包，核心结构体：

```go
// internal/patrol/patrol.go

package patrol

import (
    "time"
)

// State 表示 Patrol 的生命周期状态
type State string

const (
    StateActive   State = "active"   // 按计划运行中
    StatePaused   State = "paused"   // 暂停调度，保留配置
    StateDisabled State = "disabled" // 完全禁用，不加载
    StateRunning  State = "running"  // 正在执行中 (瞬时状态)
)

// Patrol 代表一个自主巡逻任务
type Patrol struct {
    Name             string   `json:"name" yaml:"name"`
    Schedule         string   `json:"schedule,omitempty" yaml:"schedule,omitempty"`
    Trigger          string   `json:"trigger,omitempty" yaml:"trigger,omitempty"`
    Agent            string   `json:"agent" yaml:"agent"`
    Model            string   `json:"model,omitempty" yaml:"model,omitempty"`
    Prompt           string   `json:"prompt" yaml:"prompt"`
    Notify           []string `json:"notify" yaml:"notify"`
    ApprovalRequired bool     `json:"approval_required" yaml:"approval_required"`
    AutoFix          bool     `json:"auto_fix,omitempty" yaml:"auto_fix,omitempty"`
    Timeout          string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
    Budget           string   `json:"budget,omitempty" yaml:"budget,omitempty"`
    MCPServers       []string `json:"mcp_servers,omitempty" yaml:"mcp_servers,omitempty"`
    WorkDir          string   `json:"work_dir,omitempty" yaml:"work_dir,omitempty"`

    // 运行时状态 (不写入 config.yaml, 持久化到 store)
    State       State     `json:"state"`
    LastRun     *RunLog   `json:"last_run,omitempty"`
    TotalRuns   int64     `json:"total_runs"`
    TotalErrors int64     `json:"total_errors"`
    TotalCost   float64   `json:"total_cost"`
    CreatedAt   time.Time `json:"created_at"`
}

// RunLog 记录单次 Patrol 执行结果
type RunLog struct {
    ID        string    `json:"id"`         // 随机 16 字符 hex
    Timestamp time.Time `json:"timestamp"`
    Duration  string    `json:"duration"`   // e.g., "12.3s"
    Cost      float64   `json:"cost"`       // Bedrock token 费用 (USD)
    Status    RunStatus `json:"status"`     // ok / warn / error
    Summary   string    `json:"summary"`    // LLM 生成的执行摘要
    Detail    string    `json:"detail"`     // 完整输出 (truncated in list view)
    Error     string    `json:"error,omitempty"`
    EventData string    `json:"event_data,omitempty"` // webhook 触发时的原始 payload
}

type RunStatus string

const (
    RunOK    RunStatus = "ok"
    RunWarn  RunStatus = "warn"
    RunError RunStatus = "error"
)
```

#### 5.1.4 生命周期与状态机

```
                  ┌─────────┐
        ┌────────>│  Active  │<───────┐
        │         └─────┬───┘        │
        │               │ (schedule/event fires)
        │               v            │
        │         ┌─────────┐        │
        │         │ Running  │────────┘ (execution complete)
        │         └─────┬───┘
        │               │ (pause command)
        │               v
        │         ┌─────────┐
        └─────────│  Paused  │
  (resume command)└─────┬───┘
                        │ (disable command)
                        v
                  ┌─────────┐
                  │ Disabled │
                  └─────────┘
```

状态转换规则：
- Active -> Running: 调度器触发 / webhook 触发 / 手动触发
- Running -> Active: 执行完成（无论成功或失败）
- Active / Running -> Paused: 用户暂停 (正在执行的任务会先完成)
- Paused -> Active: 用户恢复
- Any -> Disabled: 用户禁用（从调度器中移除，保留配置和历史）

#### 5.1.5 触发类型

**Schedule 触发**

复用现有 `robfig/cron` 调度器。Patrol 的 schedule 字段支持：
- 标准 5 字段 cron 表达式：`"0 9 * * *"` (每天 09:00)
- `@every` 语法：`"@every 30m"`, `"@every 1h"`
- 预定义：`"@hourly"`, `"@daily"`, `"@weekly"`

```go
// 复用现有 cronParser
var cronParser = robfigcron.NewParser(
    robfigcron.Minute | robfigcron.Hour | robfigcron.Dom |
    robfigcron.Month | robfigcron.Dow | robfigcron.Descriptor,
)
```

**Event 触发**

新增 webhook 端点接收外部事件，匹配 patrol 配置中的 `trigger` 字段：

```
POST /api/webhooks/{patrol-name}
Content-Type: application/json

{
  "event": "pr_opened",
  "source": "github",
  "payload": { ... }   // 原始事件体，注入到 prompt context
}
```

trigger 格式：`{source}:{event_type}`，例如：
- `github:pr_opened` — GitHub PR 打开事件
- `github:issue_opened` — GitHub Issue 创建事件
- `cloudwatch:alarm` — CloudWatch 告警触发
- `custom:*` — 自定义事件（匹配任意 event_type）

webhook 端点验证逻辑：
1. 根据 URL path 中的 patrol name 查找 Patrol 配置
2. 校验 Patrol 状态为 Active
3. 如果配置了 trigger，验证 `source:event_type` 是否匹配
4. 将 payload 序列化为 JSON string，注入到 prompt 前缀

```go
// webhook 触发时的 prompt 构造
func buildEventPrompt(p *Patrol, payload json.RawMessage) string {
    return fmt.Sprintf("[Event: %s]\nPayload:\n```json\n%s\n```\n\n%s",
        p.Trigger, string(payload), p.Prompt)
}
```

**Manual 触发**

Dashboard 或 API 直接触发：

```
POST /api/patrols/{name}/trigger
Content-Type: application/json

{
  "context": "optional additional context for this run"  // 可选
}
```

#### 5.1.6 日志持久化

每个 Patrol 的执行日志写入独立的 JSONL 文件：

```
~/.naozhi/patrols/{patrol-name}/logs.jsonl
```

每行一条 `RunLog` JSON 记录，按时间顺序追加写入。文件轮转策略：
- 单文件最大 10MB，超过后归档为 `logs.{timestamp}.jsonl.gz`
- 保留最近 30 天的归档文件
- Dashboard 查询时从当前文件尾部读取（倒序展示）

```go
// internal/patrol/logwriter.go

type LogWriter struct {
    dir     string    // ~/.naozhi/patrols/{name}/
    mu      sync.Mutex
    file    *os.File
    size    int64
    maxSize int64     // 10 * 1024 * 1024
}

func (w *LogWriter) Append(log *RunLog) error {
    w.mu.Lock()
    defer w.mu.Unlock()

    data, err := json.Marshal(log)
    if err != nil {
        return err
    }
    data = append(data, '\n')

    if w.size+int64(len(data)) > w.maxSize {
        if err := w.rotate(); err != nil {
            return err
        }
    }

    n, err := w.file.Write(data)
    w.size += int64(n)
    return err
}

// ReadTail 从文件末尾读取最近 n 条日志
func (w *LogWriter) ReadTail(n int) ([]*RunLog, error) { ... }
```

#### 5.1.7 通知路由

Patrol 的 `notify` 字段支持多目标通知：

| 目标 | 格式 | 说明 |
|------|------|------|
| `dashboard` | Notification Center + Activity Feed | WebSocket 推送到 Hub |
| `feishu` | 结构化卡片消息 | 复用现有 `platform.Feishu` 适配器 |
| `slack` | Block Kit 消息 | 复用现有 `platform.Slack` 适配器 |
| `discord` | Embed 消息 | 复用现有 `platform.Discord` 适配器 |
| `weixin` | 文本消息 | 复用现有 `platform.WeChat` 适配器 |

通知消息格式（结构化）：

```json
{
  "type": "patrol_result",
  "patrol": "cost-alert",
  "status": "warn",
  "title": "[Patrol: cost-alert] AWS 费用异常",
  "summary": "当日累计费用 $7.23，超过阈值 $5.00。主要来源：Bedrock InvokeModel $4.12",
  "action_url": "/dashboard#patrol/cost-alert/run/abc123",
  "timestamp": "2026-04-15T14:30:00Z"
}
```

Dashboard 端通过 WebSocket Hub 推送 `patrol_event` 类型事件：

```go
// 通知推送到 Dashboard Hub
hub.Broadcast(Event{
    Type:    "patrol_event",
    Patrol:  p.Name,
    Status:  string(log.Status),
    Summary: log.Summary,
    RunID:   log.ID,
    Time:    log.Timestamp.UnixMilli(),
})
```

IM 端复用现有 `platform.ReplyWithRetry()` 机制（参见 `scheduler.notifyTarget()`），通知目标的 chatID 从 config 中读取或使用 patrol 专用通知频道：

```yaml
# config.yaml
patrol_notify:
  feishu_chat_id: "oc_xxxx"      # patrol 通知发送到的飞书群
  slack_channel: "#ops-alerts"    # patrol 通知发送到的 Slack channel
```

#### 5.1.8 执行机制

Patrol 执行复用现有 session router + CLI process 机制，流程如下：

```
Scheduler/Webhook/Manual 触发
  → PatrolManager.Execute(patrolName, eventPayload)
    → 构造 session key: "patrol:{patrolName}"
    → 查找 agent config (agents[p.Agent])
    → 设置 AgentOpts: model 覆盖, MCP server, workspace
    → router.GetOrCreate(ctx, key, opts)
    → sess.Send(ctx, prompt, nil, nil)   // 复用 ManagedSession.Send()
    → 解析结果, 判断 status (ok/warn/error)
    → 写入 RunLog 到 JSONL
    → 通知路由 (Dashboard + IM)
    → 如果 approval_required && auto_fix, 检查是否需要审批
```

```go
// internal/patrol/manager.go

type Manager struct {
    patrols   map[string]*Patrol
    mu        sync.RWMutex
    router    *session.Router
    agents    map[string]session.AgentOpts
    platforms map[string]platform.Platform
    hub       *server.Hub           // Dashboard WebSocket Hub
    approvals *approval.Manager     // Approval Gateway
    logDir    string                // ~/.naozhi/patrols/
    cron      *robfigcron.Cron      // 复用 robfig/cron 调度器
}

func (m *Manager) Execute(ctx context.Context, name string, eventPayload json.RawMessage) error {
    m.mu.RLock()
    p, ok := m.patrols[name]
    m.mu.RUnlock()
    if !ok {
        return fmt.Errorf("patrol %q not found", name)
    }
    if p.State == StateDisabled || p.State == StatePaused {
        return fmt.Errorf("patrol %q is %s", name, p.State)
    }

    // 标记为 Running
    m.setState(name, StateRunning)
    defer m.setState(name, StateActive)

    start := time.Now()
    prompt := p.Prompt
    if len(eventPayload) > 0 {
        prompt = buildEventPrompt(p, eventPayload)
    }

    // 复用 session router
    key := "patrol:" + name
    opts := m.agents[p.Agent]
    if p.Model != "" {
        opts.Model = p.Model
    }
    if p.WorkDir != "" {
        opts.Workspace = p.WorkDir
    }
    opts.Exempt = true  // patrol session 不计入 maxProcs

    timeout := m.parseTimeout(p.Timeout, 5*time.Minute)
    execCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    sess, _, err := m.router.GetOrCreate(execCtx, key, opts)
    if err != nil {
        log := m.recordError(p, start, err)
        m.notify(p, log)
        return err
    }

    result, err := sess.Send(execCtx, prompt, nil, nil)
    if err != nil {
        log := m.recordError(p, start, err)
        m.notify(p, log)
        return err
    }

    // 记录执行结果
    runLog := &RunLog{
        ID:        generateID(),
        Timestamp: start,
        Duration:  time.Since(start).Round(time.Millisecond).String(),
        Cost:      result.Cost,
        Status:    classifyResult(result.Text),
        Summary:   extractSummary(result.Text),
        Detail:    result.Text,
    }
    if len(eventPayload) > 0 {
        runLog.EventData = string(eventPayload)
    }

    m.writeLog(name, runLog)
    m.updateStats(name, runLog)
    m.notify(p, runLog)

    return nil
}
```

#### 5.1.9 REST API

```
GET    /api/patrols                    → 列出所有 Patrol (含状态、统计)
GET    /api/patrols/{name}             → 单个 Patrol 详情
PUT    /api/patrols/{name}/state       → 修改状态 (pause/resume/disable)
POST   /api/patrols/{name}/trigger     → 手动触发执行
GET    /api/patrols/{name}/logs        → 执行日志 (?limit=20&offset=0)
GET    /api/patrols/{name}/logs/{id}   → 单条日志详情
POST   /api/webhooks/{patrol-name}     → 外部 event webhook 触发
```

**列表响应示例：**

```json
{
  "patrols": [
    {
      "name": "cost-alert",
      "schedule": "@every 1h",
      "agent": "general",
      "state": "active",
      "notify": ["feishu", "dashboard"],
      "total_runs": 142,
      "total_errors": 3,
      "total_cost": 1.87,
      "last_run": {
        "id": "a3f8b2c1d4e5f6a7",
        "timestamp": "2026-04-15T14:00:00Z",
        "duration": "8.2s",
        "cost": 0.013,
        "status": "ok",
        "summary": "当日费用 $3.12，低于 $5.00 阈值，无异常。"
      },
      "next_run": "2026-04-15T15:00:00Z"
    }
  ]
}
```

---

### 5.2 Approval Gateway (审批网关)

#### 5.2.1 概念

Approval Gateway 是人机协作的关键组件。当 Patrol 或交互式 Agent 执行到高风险操作时，系统自动暂停执行，创建审批请求并等待人工确认。

适用场景：
- `terraform apply` — 基础设施变更
- `git push --force` — 代码仓库强制推送
- `kubectl delete` — K8s 资源删除
- `DROP TABLE` / `DELETE FROM` — 数据库变更
- `aws ec2 terminate-instances` — EC2 实例终止
- Patrol 配置 `approval_required: true` 的所有操作

#### 5.2.2 审批流程

```
Agent 执行任务
  │
  ├─ 检测到高风险操作 (关键字匹配 / agent 自主标记)
  │
  v
创建 Approval Request
  │
  ├─ 写入 ~/.naozhi/approvals.json
  ├─ 推送 Dashboard Notification (WebSocket)
  ├─ 推送 IM 通知 (飞书/Slack/Discord/微信)
  │
  v
Agent 暂停等待 (session 保持 Running 状态, stdin 不发送)
  │
  ├── 用户在 Dashboard 点击 Approve / Reject
  │   或
  ├── 用户在 IM 回复 "/approve {id}" / "/reject {id}"
  │   或
  ├── 超时自动 Reject (默认 30 分钟)
  │
  v
┌─ Approved → Agent 继续执行 (发送确认消息到 CLI stdin)
│
└─ Rejected → Agent 收到拒绝通知, 中止当前操作, 回复用户
```

#### 5.2.3 风险判断机制

**层级 1：关键字匹配（即时检测）**

在 `sess.Send()` 的输出流中实时检测高风险关键字：

```go
// internal/approval/detector.go

var dangerousPatterns = []struct {
    Pattern *regexp.Regexp
    Label   string
    Level   string // "critical" / "high"
}{
    {regexp.MustCompile(`terraform\s+apply`),           "terraform apply",      "critical"},
    {regexp.MustCompile(`git\s+push\s+--force`),        "force push",           "critical"},
    {regexp.MustCompile(`kubectl\s+delete`),             "kubectl delete",       "high"},
    {regexp.MustCompile(`(?i)DROP\s+TABLE`),             "DROP TABLE",           "critical"},
    {regexp.MustCompile(`(?i)DELETE\s+FROM`),            "DELETE FROM",          "high"},
    {regexp.MustCompile(`aws\s+\S+\s+terminate`),        "AWS terminate",        "critical"},
    {regexp.MustCompile(`aws\s+\S+\s+delete`),           "AWS delete",           "high"},
    {regexp.MustCompile(`rm\s+-rf\s+/`),                 "rm -rf root",          "critical"},
}

func DetectDanger(text string) (label string, level string, found bool) {
    for _, p := range dangerousPatterns {
        if p.Pattern.MatchString(text) {
            return p.Label, p.Level, true
        }
    }
    return "", "", false
}
```

**层级 2：Agent 自主标记**

通过 system prompt 引导 agent 在不确定时主动请求审批。在 patrol 和 agent session 的 system prompt 中追加：

```
When you are about to execute a destructive or irreversible operation, output the
following marker BEFORE executing:

[APPROVAL_NEEDED: brief description of the action and its impact]

This will pause execution and wait for human approval.
```

Agent 输出流中检测 `[APPROVAL_NEEDED: ...]` 标记，提取描述文本作为审批请求的 summary。

**层级 3：Per-patrol 配置**

```yaml
patrols:
  dep-audit:
    approval_required: true    # 该 patrol 的所有操作均需审批
```

当 `approval_required: true` 时，即使没有检测到危险关键字，agent 输出中包含任何 tool 调用结果都会触发审批（仅适用于 auto_fix 场景）。

#### 5.2.4 数据结构

```go
// internal/approval/approval.go

package approval

import "time"

type Status string

const (
    StatusPending  Status = "pending"
    StatusApproved Status = "approved"
    StatusRejected Status = "rejected"
    StatusExpired  Status = "expired"
    StatusExecuted Status = "executed"  // approved + 执行完成
)

type Urgency string

const (
    UrgencyNormal Urgency = "normal"
    UrgencyUrgent Urgency = "urgent"   // 需要立即关注
)

// Request 代表一个审批请求
type Request struct {
    ID         string  `json:"id"`          // "appr-" + 16 字符 hex
    Patrol     string  `json:"patrol"`      // patrol name (可为空, 表示交互式 session)
    SessionKey string  `json:"session_key"` // 关联的 session key
    Agent      string  `json:"agent"`       // agent ID
    Action     string  `json:"action"`      // 检测到的操作标签
    Summary    string  `json:"summary"`     // 操作摘要 (human-readable)
    Detail     string  `json:"detail"`      // 完整上下文 (terraform plan output 等)
    Impact     string  `json:"impact"`      // 影响评估 (e.g., "+$24.80/month")
    Urgency    Urgency `json:"urgency"`
    Level      string  `json:"level"`       // "critical" / "high"

    // 生命周期
    Status     Status    `json:"status"`
    CreatedAt  time.Time `json:"created_at"`
    ExpiresAt  time.Time `json:"expires_at"`   // 默认 created_at + 30min
    ApprovedBy string    `json:"approved_by,omitempty"`
    ApprovedAt *time.Time `json:"approved_at,omitempty"`
    RejectedBy string    `json:"rejected_by,omitempty"`
    RejectedAt *time.Time `json:"rejected_at,omitempty"`
}
```

**JSON 示例：**

```json
{
  "id": "appr-a3f8b2c1d4e5f6a7",
  "patrol": "infra-health",
  "session_key": "patrol:infra-health",
  "agent": "general",
  "action": "terraform apply",
  "summary": "Scale EKS node group workers 3 -> 5",
  "detail": "Terraform will perform the following actions:\n\n  # aws_eks_node_group.workers will be updated\n  ~ scaling_config {\n      ~ desired_size = 3 -> 5\n    }\n\nPlan: 0 to add, 1 to change, 0 to destroy.",
  "impact": "+$24.80/month (2x m5.large on-demand)",
  "urgency": "normal",
  "level": "critical",
  "status": "pending",
  "created_at": "2026-04-15T14:30:00Z",
  "expires_at": "2026-04-15T15:00:00Z",
  "approved_by": null,
  "approved_at": null
}
```

#### 5.2.5 Approval Manager

```go
// internal/approval/manager.go

type Manager struct {
    mu        sync.RWMutex
    requests  map[string]*Request  // id -> Request
    waiters   map[string]chan Status // id -> 等待审批结果的 channel
    storePath string                // ~/.naozhi/approvals.json
    hub       interface{ Broadcast(any) }
    platforms map[string]platform.Platform
    timeout   time.Duration         // 默认 30 分钟
}

// CreateRequest 创建审批请求并通知所有渠道
func (m *Manager) CreateRequest(req *Request) error {
    req.ID = "appr-" + generateID()
    req.Status = StatusPending
    req.CreatedAt = time.Now()
    req.ExpiresAt = time.Now().Add(m.timeout)

    m.mu.Lock()
    m.requests[req.ID] = req
    ch := make(chan Status, 1)
    m.waiters[req.ID] = ch
    m.mu.Unlock()

    m.save()
    m.notifyAll(req)

    // 启动超时 goroutine
    go m.expireAfter(req.ID, m.timeout)

    return nil
}

// WaitForDecision 阻塞等待审批结果 (用于 patrol/session 执行流)
func (m *Manager) WaitForDecision(ctx context.Context, id string) (Status, error) {
    m.mu.RLock()
    ch, ok := m.waiters[id]
    m.mu.RUnlock()
    if !ok {
        return "", fmt.Errorf("no waiter for approval %s", id)
    }

    select {
    case status := <-ch:
        return status, nil
    case <-ctx.Done():
        return StatusExpired, ctx.Err()
    }
}

// Approve 批准请求
func (m *Manager) Approve(id, approvedBy string) error {
    m.mu.Lock()
    req, ok := m.requests[id]
    if !ok {
        m.mu.Unlock()
        return fmt.Errorf("approval %s not found", id)
    }
    if req.Status != StatusPending {
        m.mu.Unlock()
        return fmt.Errorf("approval %s is already %s", id, req.Status)
    }

    now := time.Now()
    req.Status = StatusApproved
    req.ApprovedBy = approvedBy
    req.ApprovedAt = &now

    ch := m.waiters[id]
    m.mu.Unlock()

    m.save()
    if ch != nil {
        ch <- StatusApproved
    }

    // 通知 Dashboard
    m.hub.Broadcast(map[string]any{
        "type":   "approval_update",
        "id":     id,
        "status": "approved",
    })

    return nil
}

// Reject 拒绝请求
func (m *Manager) Reject(id, rejectedBy string) error {
    // 类似 Approve, 设置 StatusRejected + RejectedBy/RejectedAt
    ...
}
```

#### 5.2.6 与 Patrol 执行流的集成

在 `PatrolManager.Execute()` 中，对 `sess.Send()` 返回的结果进行审批检测：

```go
// 在 Execute() 中, Send() 之后
result, err := sess.Send(execCtx, prompt, nil, nil)
if err != nil { ... }

// 检查是否需要审批
if p.ApprovalRequired || detectApprovalMarker(result.Text) {
    label, level, _ := DetectDanger(result.Text)
    req := &Request{
        Patrol:     p.Name,
        SessionKey: key,
        Agent:      p.Agent,
        Action:     label,
        Summary:    extractApprovalSummary(result.Text),
        Detail:     result.Text,
        Level:      level,
    }

    m.approvals.CreateRequest(req)

    // 阻塞等待审批
    status, err := m.approvals.WaitForDecision(execCtx, req.ID)
    if status == StatusApproved {
        // 发送确认消息继续执行
        confirmResult, _ := sess.Send(execCtx, "Approved. Please proceed.", nil, nil)
        // ... 记录结果
    } else {
        // 发送中止消息
        sess.Send(execCtx, "Rejected. Please abort the current operation.", nil, nil)
        // ... 记录拒绝
    }
}
```

#### 5.2.7 REST API

```
GET    /api/approvals              → 列表 (支持 ?status=pending&limit=20)
GET    /api/approvals/{id}         → 单个审批详情
POST   /api/approvals/{id}/approve → 批准 (body: {"approved_by": "keith"})
POST   /api/approvals/{id}/reject  → 拒绝 (body: {"rejected_by": "keith", "reason": "..."})
GET    /api/approvals/stats        → 统计 (pending/approved/rejected 计数)
```

**列表响应示例：**

```json
{
  "approvals": [
    {
      "id": "appr-a3f8b2c1d4e5f6a7",
      "patrol": "infra-health",
      "action": "terraform apply",
      "summary": "Scale EKS nodes 3 -> 5",
      "urgency": "normal",
      "level": "critical",
      "status": "pending",
      "created_at": "2026-04-15T14:30:00Z",
      "expires_at": "2026-04-15T15:00:00Z"
    }
  ],
  "stats": {
    "pending": 1,
    "approved_today": 5,
    "rejected_today": 0
  }
}
```

#### 5.2.8 IM 审批命令

用户可以在 IM 平台直接审批，无需打开 Dashboard：

```
/approve appr-a3f8     → 批准 (ID 前缀匹配)
/reject appr-a3f8      → 拒绝
/approvals             → 列出待审批请求
```

IM 通知消息格式（以飞书为例）：

```
[审批请求] infra-health
操作: terraform apply
摘要: Scale EKS nodes 3 -> 5
影响: +$24.80/month
级别: critical

回复 /approve appr-a3f8 批准
回复 /reject appr-a3f8 拒绝
```

#### 5.2.9 存储

审批记录持久化到 `~/.naozhi/approvals.json`，格式为 JSON 数组，包含全部历史记录（用于审计追踪）。采用与 cron store 相同的原子写入模式（写入 `.tmp` 文件后 `os.Rename`）。

为防止文件无限增长，已完成的审批记录（approved/rejected/expired）在 7 天后迁移到归档文件 `~/.naozhi/approvals-archive.jsonl`。

---

### 5.3 Proactive Insights (主动洞察)

#### 5.3.1 概念

传统 AI 助手是被动响应式的——用户发问，AI 回答。Proactive Insights 让 Naozhi 主动发现并推送相关信息：

- 对话中提到的实体与其他 session / wiki 笔记有关联
- Patrol 结果中出现与历史数据不一致的异常
- 检测到未完成的 action items 或过期信息
- Wiki 中的知识出现矛盾（Wiki Lint）

这不是搜索引擎——是一个持续运行的关联分析引擎。

#### 5.3.2 触发场景与优先级

| 场景 | 触发条件 | 优先级 | 示例 |
|------|---------|--------|------|
| 实体关联 | 用户消息中的关键词命中 wiki/历史 session | P1 | 你提到 "EKS cluster"，3 小时前 infra-health patrol 检测到该集群 pending pods 增加 |
| Patrol 异常 | Patrol 结果与过去 7 天均值偏差 >2σ | P1 | cost-alert 检测到费用 $12.40，过去 7 天均值 $3.20 |
| Action Item 到期 | 会议纪要中的 action item 超过截止日期 | P2 | 3 天前会议的 action "更新 CDK stack" 尚未完成 |
| Wiki 矛盾 | Wiki 中两篇笔记对同一实体描述不一致 | P3 | wiki/eks-setup.md 说 node type 是 m5.large，但 wiki/cost-breakdown.md 说是 m5.xlarge |
| 上下文补充 | 当前对话涉及的主题在 wiki 中有详细文档 | P3 | 你在讨论 CloudFront invalidation，wiki 中有相关最佳实践文档 |

#### 5.3.3 实体提取

在 message handler 处理链中，对用户消息进行轻量级实体提取：

```go
// internal/insight/extractor.go

// ExtractEntities 从文本中提取技术实体
// Phase 1: 关键词匹配 (正则 + 词表)
// Phase 2: LLM 语义提取 (异步, 不阻塞响应)
func ExtractEntities(text string) []Entity {
    var entities []Entity

    // AWS 服务名
    for _, svc := range awsServices {
        if strings.Contains(strings.ToLower(text), strings.ToLower(svc)) {
            entities = append(entities, Entity{Type: "aws_service", Value: svc})
        }
    }

    // 资源标识符 (ARN, instance ID, cluster name)
    for _, pat := range resourcePatterns {
        if matches := pat.Re.FindAllString(text, -1); len(matches) > 0 {
            for _, m := range matches {
                entities = append(entities, Entity{Type: pat.Type, Value: m})
            }
        }
    }

    // 项目名 / 文件路径
    // ... 更多规则

    return entities
}

type Entity struct {
    Type  string `json:"type"`   // aws_service, resource_id, project, file_path, etc.
    Value string `json:"value"`
}
```

#### 5.3.4 关联查询

提取到实体后，查询以下来源寻找关联内容：

```go
// internal/insight/correlator.go

type Correlator struct {
    patrolMgr   *patrol.Manager    // 查询 patrol 执行历史
    sessionMgr  *session.Router    // 查询活跃 session 上下文
    wikiIndex   *wiki.SearchIndex  // 查询 wiki 全文索引 (V2 Wiki 功能)
    actionItems *actionitem.Store  // 查询 action items (V2 Meeting 功能)
}

type Insight struct {
    Type       string    `json:"type"`        // "related_patrol", "entity_link", "action_reminder", "wiki_conflict"
    Priority   string    `json:"priority"`    // "p1", "p2", "p3"
    Title      string    `json:"title"`
    Summary    string    `json:"summary"`
    SourceType string    `json:"source_type"` // "patrol", "wiki", "session", "meeting"
    SourceRef  string    `json:"source_ref"`  // 链接到来源
    Score      float64   `json:"score"`       // 相关性分数 (0-1)
    CreatedAt  time.Time `json:"created_at"`
}

// FindRelated 根据实体列表查找关联洞察
func (c *Correlator) FindRelated(ctx context.Context, entities []Entity) ([]Insight, error) {
    var insights []Insight

    // 查询 patrol 最近日志
    for _, e := range entities {
        logs := c.patrolMgr.SearchLogs(e.Value, 24*time.Hour)
        for _, log := range logs {
            if log.Status == patrol.RunWarn || log.Status == patrol.RunError {
                insights = append(insights, Insight{
                    Type:       "related_patrol",
                    Priority:   "p1",
                    Title:      fmt.Sprintf("Patrol [%s] 近期有相关告警", log.PatrolName),
                    Summary:    log.Summary,
                    SourceType: "patrol",
                    SourceRef:  fmt.Sprintf("/api/patrols/%s/logs/%s", log.PatrolName, log.ID),
                    Score:      0.8,
                })
            }
        }
    }

    // 查询 wiki 索引
    // ... 类似逻辑

    // 按 score 排序, 取 top 3
    sort.Slice(insights, func(i, j int) bool {
        return insights[i].Score > insights[j].Score
    })
    if len(insights) > 3 {
        insights = insights[:3]
    }

    return insights, nil
}
```

#### 5.3.5 推送机制

洞察以非阻塞方式注入对话流。在 `message handler` 的响应链中：

```
用户发送消息
  → message handler 开始处理
  → 异步: ExtractEntities() + FindRelated()
  → Agent 开始回复 (不等待洞察)
  → 洞察结果就绪后:
    → 如果 score > 0.6 且 Agent 回复尚未结束:
      → 通过 WebSocket 推送 "insight" 事件到 Dashboard
    → 如果 Agent 回复已结束:
      → 追加到下一条消息的 context 提示区域
```

Dashboard 前端渲染洞察卡片：

```json
{
  "type": "insight",
  "session_key": "feishu:direct:alice:general",
  "insights": [
    {
      "type": "related_patrol",
      "priority": "p1",
      "title": "Patrol [infra-health] 2 小时前检测到 EKS pending pods",
      "summary": "kube-system namespace 有 3 个 pod 处于 Pending 状态...",
      "source_ref": "/api/patrols/infra-health/logs/abc123",
      "score": 0.85
    }
  ]
}
```

IM 端：仅推送 P1 优先级的洞察，以避免消息噪音。格式：

```
💡 [Related] infra-health patrol 2 小时前检测到 EKS pending pods
详情: /dashboard#patrol/infra-health/logs/abc123
```

#### 5.3.6 Phase 1 与 Phase 2 对比

| 维度 | Phase 1 (V2.0) | Phase 2 (V2.1+) |
|------|----------------|-----------------|
| 实体提取 | 关键词 + 正则匹配 | LLM 语义提取 (通过 Bedrock) |
| 关联查询 | 精确匹配 + 前缀搜索 | Embedding 向量相似度搜索 |
| 查询源 | Patrol 日志 + 活跃 session | + Wiki 全文索引 + 历史 session |
| 延迟预算 | < 100ms (纯本地) | < 500ms (含 Bedrock 调用) |
| 推送策略 | Score > 0.7 才推送 | Score > 0.5, 但降低重复推送频率 |

---

## 6. 平台集成 (Platform Integrations)

Naozhi V1 已有完整的 IM 平台适配器架构（Feishu / Slack / Discord / WeChat）。V2 在此基础上扩展四个深度集成方向：GitHub 开发流程集成、AWS 控制台自然语言查询、可观测性告警桥接、以及会议智能。

### 6.1 GitHub Deep Integration

#### 6.1.1 功能概览

| 功能 | 说明 | 触发方式 |
|------|------|---------|
| PR 时间线 | Dashboard 内嵌展示 PR reviews, CI status, merge state | Dashboard UI 主动拉取 |
| One-click Fix | Agent 读取 review comments → 自动修复 → push | Dashboard 按钮 / IM 命令 |
| Issue → Session | 新 issue 自动创建 Agent 调研 session | Patrol webhook |
| Commit 知识编译 | Commit history + diff 纳入知识图谱 | 后台定期扫描 |

#### 6.1.2 架构设计

GitHub 集成采用双通道架构：

```
┌─────────────┐        ┌──────────────┐        ┌─────────────┐
│  GitHub.com  │──────>│  Patrol       │──────>│  Agent       │
│  (Webhooks)  │       │  (Webhook     │       │  (CLI        │
│              │       │   Endpoint)   │       │   Process)   │
└──────────────┘       └──────────────┘       └──────────────┘
                              │
                              v
                       ┌──────────────┐
                       │  gh CLI      │  (作为 Claude tool)
                       │  MCP Server  │
                       └──────────────┘
```

**数据获取层：** 复用 `gh` CLI 作为 Claude 的 tool。Agent 通过 `gh pr list`, `gh pr view`, `gh issue list` 等命令获取 GitHub 数据。不需要在 Naozhi 中实现 GitHub API 客户端——Agent 自己调用 `gh` 即可。

**事件驱动层：** GitHub Webhooks → Patrol webhook 端点 → 触发对应 Patrol。

#### 6.1.3 PR 时间线

Dashboard 前端通过 REST API 代理 `gh` 命令获取 PR 数据：

```
GET /api/github/prs?repo={owner/repo}&state=open
GET /api/github/prs/{number}?repo={owner/repo}
GET /api/github/prs/{number}/timeline?repo={owner/repo}
```

后端实现：

```go
// internal/server/dashboard_github.go

type GitHubHandlers struct {
    allowedRepos []string  // 配置允许访问的 repo 列表
}

func (h *GitHubHandlers) handlePRList(w http.ResponseWriter, r *http.Request) {
    repo := r.URL.Query().Get("repo")
    if !h.isAllowed(repo) {
        http.Error(w, "repo not allowed", http.StatusForbidden)
        return
    }

    // 调用 gh CLI (非 Agent, 直接执行)
    out, err := exec.CommandContext(r.Context(),
        "gh", "pr", "list", "--repo", repo, "--json",
        "number,title,state,author,createdAt,reviews,statusCheckRollup",
    ).Output()
    // ... 解析 JSON, 返回前端
}
```

PR 时间线数据结构（前端消费）：

```json
{
  "number": 42,
  "title": "Fix CloudFront cache invalidation",
  "state": "open",
  "author": "keith",
  "created_at": "2026-04-15T10:00:00Z",
  "reviews": [
    {
      "author": "reviewer1",
      "state": "CHANGES_REQUESTED",
      "body": "Please add error handling for 403 responses",
      "submitted_at": "2026-04-15T11:00:00Z"
    }
  ],
  "ci_status": {
    "state": "failure",
    "checks": [
      {"name": "go-test", "status": "completed", "conclusion": "success"},
      {"name": "lint", "status": "completed", "conclusion": "failure"}
    ]
  },
  "mergeable": false
}
```

#### 6.1.4 One-Click Fix PR

Dashboard 或 IM 触发 Agent 自动修复 PR review comments：

```
POST /api/github/prs/{number}/fix
Content-Type: application/json

{
  "repo": "owner/repo",
  "approval_required": true  // 可选: push 前需审批
}
```

执行流程：

```
1. gh pr view {number} --json reviews,comments
2. 提取所有 review comments (CHANGES_REQUESTED)
3. 构造 prompt: "Fix the following review comments on PR #{number}:\n{comments}"
4. 路由到 code-reviewer agent session
5. Agent 读取代码 → 修复 → git commit → (审批) → git push
6. 推送结果通知到 Dashboard + IM
```

IM 命令：`/fix-pr {repo}#{number}` 或 `/fix-pr {url}`

#### 6.1.5 Issue → Session 自动调研

配置 patrol 监听 GitHub issue 创建事件：

```yaml
patrols:
  issue-triage:
    trigger: "github:issue_opened"
    agent: researcher
    prompt: |
      A new issue has been opened. Analyze it:
      1. Categorize (bug/feature/question)
      2. Check if duplicate
      3. Assess priority
      4. If bug: attempt root cause analysis
      5. Summarize findings and recommend next steps
    notify: [feishu, dashboard]
    approval_required: false
```

GitHub webhook 配置：

```
POST https://naozhi.example.com/api/webhooks/issue-triage
Content-Type: application/json
X-GitHub-Event: issues
```

#### 6.1.6 Commit 知识编译

后台定期扫描项目 git log，提取 commit 信息纳入搜索索引：

```yaml
patrols:
  knowledge-compile:
    schedule: "@daily"
    agent: general
    prompt: |
      Scan git log for the past 24 hours across all projects.
      For each significant commit, extract: purpose, files changed, architectural impact.
      Update the knowledge index.
    work_dir: "/home/naozhi/projects"
```

---

### 6.2 AWS Console in Dashboard

#### 6.2.1 功能概览

让用户在 Dashboard 中用自然语言查询 AWS 资源状态，无需登录 AWS Console：

- "Show me running EC2 instances" → 表格展示
- "What's the RDS CPU usage in the last hour?" → 时间序列图表
- "How much did Bedrock cost this month?" → 费用明细
- "Terminate instance i-0abc..." → Approval Gateway 审批

#### 6.2.2 交互模式

```
用户在 Dashboard 发送消息
  → Agent 识别为 AWS 查询
  → Agent 调用 AWS MCP Server tools (或 aws CLI)
  → 返回结构化数据 (JSON)
  → 前端识别特定数据格式, 渲染为表格/图表
  → 危险操作 → Approval Gateway
```

#### 6.2.3 结构化输出识别

Agent 返回的文本中包含特定标记时，前端自动渲染为丰富组件：

**表格标记：**

```markdown
[TABLE]
| Instance ID | Type | State | Name |
|-------------|------|-------|------|
| i-0abc1234  | t4g.small | running | naozhi-prod |
| i-0def5678  | t4g.micro | stopped | naozhi-dev |
[/TABLE]
```

**图表标记：**

```markdown
[CHART type=line title="RDS CPU Utilization (1h)"]
timestamp,cpu_percent
2026-04-15T13:00,12.3
2026-04-15T13:05,15.1
2026-04-15T13:10,14.8
...
[/CHART]
```

**费用标记：**

```markdown
[COST_BREAKDOWN title="Bedrock Cost - April 2026"]
Service,Cost
Amazon Bedrock - InvokeModel,$45.23
Amazon Bedrock - InvokeModelWithResponseStream,$12.87
Total,$58.10
[/COST_BREAKDOWN]
```

前端解析逻辑：

```javascript
// Dashboard 前端 message 渲染
function renderMessage(text) {
    // 检测 [TABLE]...[/TABLE] 标记
    text = text.replace(/\[TABLE\]([\s\S]*?)\[\/TABLE\]/g, (match, table) => {
        return renderMarkdownTable(table);
    });

    // 检测 [CHART]...[/CHART] 标记
    text = text.replace(/\[CHART(.*?)\]([\s\S]*?)\[\/CHART\]/g, (match, attrs, data) => {
        const config = parseChartAttrs(attrs);
        return renderChart(config, parseCSV(data));
    });

    // 检测 [COST_BREAKDOWN]...[/COST_BREAKDOWN] 标记
    // ... 类似处理

    return renderMarkdown(text);
}
```

#### 6.2.4 Agent System Prompt 配置

在 `agents` 配置中为 AWS 查询场景设置专用 agent：

```yaml
agents:
  aws-console:
    model: sonnet
    args: ["--mcp-server", "aws"]
    system_prompt: |
      You are an AWS operations assistant. When querying AWS resources:
      1. Use the AWS MCP tools to fetch real-time data
      2. Format tabular results using [TABLE]...[/TABLE] markers
      3. Format time-series data using [CHART]...[/CHART] markers
      4. Format cost data using [COST_BREAKDOWN]...[/COST_BREAKDOWN] markers
      5. For destructive operations (terminate, delete, modify), output
         [APPROVAL_NEEDED: description] and wait for approval
      6. Always include resource IDs, regions, and timestamps
```

#### 6.2.5 危险操作保护

所有 AWS 变更操作（terminate, delete, modify, create）自动触发 Approval Gateway：

```go
// 在 AWS agent session 的输出流中检测
var awsDangerPatterns = []struct {
    Pattern *regexp.Regexp
    Label   string
}{
    {regexp.MustCompile(`terminate-instances`),     "EC2 Terminate"},
    {regexp.MustCompile(`delete-(bucket|stack|cluster)`), "AWS Delete"},
    {regexp.MustCompile(`modify-db-instance`),      "RDS Modify"},
    {regexp.MustCompile(`update-stack`),            "CloudFormation Update"},
}
```

#### 6.2.6 API 端点

```
POST /api/aws/query     → 自然语言查询 (路由到 aws-console agent)
GET  /api/aws/resources → 缓存的资源列表 (最近一次查询结果)
```

`/api/aws/query` 实际上是 `/api/sessions/send` 的语法糖，自动选择 `aws-console` agent 并设置工作目录。

---

### 6.3 Observability Bridge (可观测性桥接)

#### 6.3.1 功能概览

接收外部监控告警，自动创建 Agent session 进行根因调查：

| 来源 | 协议 | 说明 |
|------|------|------|
| CloudWatch Alarms | SNS → HTTPS webhook | AWS 原生告警 |
| Grafana | Webhook notification channel | 自托管监控 |
| Datadog | Webhook integration | SaaS 监控 |
| Custom | HTTP POST | 任意自定义告警源 |

#### 6.3.2 统一 Webhook 端点

所有告警源通过统一端点接入：

```
POST /api/webhooks/alert
Content-Type: application/json
X-Alert-Source: cloudwatch|grafana|datadog|custom

{
  "source": "cloudwatch",
  "alert_name": "EKS-CPU-High",
  "severity": "critical",
  "summary": "EKS cluster 'prod' CPU utilization > 80% for 5 minutes",
  "details": { ... },
  "timestamp": "2026-04-15T14:30:00Z"
}
```

也可以通过 Patrol webhook 端点接入（推荐方式）：

```yaml
patrols:
  alert-investigator:
    trigger: "alert:*"           # 匹配所有告警事件
    agent: general
    prompt: |
      An alert has been triggered. Investigate the root cause:
      1. Check the affected service's metrics and logs
      2. Identify potential causes
      3. Suggest remediation steps
      4. If auto-fix is possible and safe, propose the fix
    notify: [feishu, dashboard]
    mcp_servers: ["aws"]
    timeout: "10m"
```

#### 6.3.3 告警标准化

不同来源的告警格式需要标准化为统一的内部格式：

```go
// internal/alert/normalize.go

type Alert struct {
    Source    string            `json:"source"`
    Name     string            `json:"name"`
    Severity string            `json:"severity"`  // critical/warning/info
    Summary  string            `json:"summary"`
    Details  map[string]any    `json:"details"`
    Labels   map[string]string `json:"labels"`    // 用于路由和关联
    Time     time.Time         `json:"time"`
}

// NormalizeCloudWatch 将 CloudWatch SNS 消息标准化
func NormalizeCloudWatch(payload json.RawMessage) (*Alert, error) {
    var msg struct {
        AlarmName    string `json:"AlarmName"`
        NewState     string `json:"NewStateValue"`
        Reason       string `json:"NewStateReason"`
        Trigger      struct {
            MetricName string `json:"MetricName"`
            Namespace  string `json:"Namespace"`
        } `json:"Trigger"`
    }
    if err := json.Unmarshal(payload, &msg); err != nil {
        return nil, err
    }

    severity := "warning"
    if msg.NewState == "ALARM" {
        severity = "critical"
    }

    return &Alert{
        Source:   "cloudwatch",
        Name:     msg.AlarmName,
        Severity: severity,
        Summary:  msg.Reason,
        Details:  map[string]any{"metric": msg.Trigger.MetricName, "namespace": msg.Trigger.Namespace},
        Time:     time.Now(),
    }, nil
}

// NormalizeGrafana, NormalizeDatadog 类似实现
```

#### 6.3.4 调查结果推送

Agent 完成根因分析后，调查结果推送到多个渠道：

```go
// 调查结果结构
type InvestigationResult struct {
    AlertName   string   `json:"alert_name"`
    RootCause   string   `json:"root_cause"`
    Impact      string   `json:"impact"`
    Remediation []string `json:"remediation"`   // 建议的修复步骤
    AutoFixable bool     `json:"auto_fixable"`
    SessionKey  string   `json:"session_key"`   // 可以跳转到调查 session
}
```

推送格式（IM 结构化消息）：

```
[Alert Investigation] EKS-CPU-High
Root Cause: kube-system/coredns deployment scaled to 10 replicas due to HPA misconfiguration
Impact: Node CPU 82%, 3 pods pending
Remediation:
  1. kubectl edit hpa coredns -n kube-system (set maxReplicas=3)
  2. Monitor for 10 minutes
Auto-fix: Available (requires approval)

详情: /dashboard#session/patrol:alert-investigator
```

---

### 6.4 Meeting Intelligence (会议智能)

#### 6.4.1 功能概览

扩展现有 `internal/transcribe` 模块（当前仅支持飞书/Slack 语音消息的实时 STT），增加长音频文件处理和会议分析能力：

| 功能 | 说明 |
|------|------|
| 会议录音转写 | 支持上传 mp3/m4a/wav 文件，调用 Amazon Transcribe 批量转写 |
| 自动 Summary | LLM 生成会议摘要（议题、决策、争议点） |
| Action Items 提取 | 自动提取 action items，含负责人和截止日期 |
| Obsidian 集成 | 写入 Obsidian daily journal / meeting notes |
| Cron/Patrol 联动 | Action items 自动创建为 Patrol 任务追踪 |

#### 6.4.2 上传端点

```
POST /api/meeting/upload
Content-Type: multipart/form-data

Parameters:
  file: <audio file>              (required, max 100MB)
  title: "Weekly Standup"         (optional)
  date: "2026-04-15"             (optional, default today)
  participants: "keith,alice,bob" (optional)
  language: "zh-CN"              (optional, default from config)
  output: "obsidian"             (optional: obsidian / markdown / json)
```

响应（异步处理，返回 job ID）：

```json
{
  "job_id": "mtg-a3f8b2c1",
  "status": "processing",
  "estimated_duration": "2-5 minutes",
  "poll_url": "/api/meeting/mtg-a3f8b2c1"
}
```

#### 6.4.3 处理流水线

```
Upload → 存储到临时目录 → Amazon Transcribe (批量)
  → 转写文本 → LLM 分析 (Bedrock)
    → 生成 Summary
    → 提取 Action Items
    → 识别 Decisions
    → 识别参与者发言分布
  → 输出格式化结果
    → 写入 Obsidian (可选)
    → 创建 Action Item Patrol (可选)
    → 推送通知
```

#### 6.4.4 Amazon Transcribe 批量转写

扩展现有 `internal/transcribe` 包，新增批量转写支持：

```go
// internal/transcribe/batch.go

type BatchConfig struct {
    Region       string
    S3Bucket     string // 临时存储音频文件
    S3Prefix     string // "naozhi/meeting-audio/"
    Language     string // "zh-CN"
    OutputBucket string // 同 S3Bucket
    OutputPrefix string // "naozhi/meeting-transcripts/"
}

// StartBatchJob 上传音频到 S3 → 启动 Transcribe Job → 轮询结果
func (c *BatchConfig) StartBatchJob(ctx context.Context, audioPath, jobName string) (*TranscriptResult, error) {
    // 1. 上传音频到 S3
    s3Key := c.S3Prefix + filepath.Base(audioPath)
    if err := uploadToS3(ctx, c.S3Bucket, s3Key, audioPath); err != nil {
        return nil, fmt.Errorf("upload audio: %w", err)
    }

    // 2. 启动 Transcribe 批量作业
    input := &transcribeservice.StartTranscriptionJobInput{
        TranscriptionJobName: aws.String(jobName),
        LanguageCode:         types.LanguageCode(c.Language),
        Media: &types.Media{
            MediaFileUri: aws.String(fmt.Sprintf("s3://%s/%s", c.S3Bucket, s3Key)),
        },
        OutputBucketName: aws.String(c.OutputBucket),
        OutputKey:        aws.String(c.OutputPrefix + jobName + ".json"),
        Settings: &types.Settings{
            ShowSpeakerLabels:  aws.Bool(true),
            MaxSpeakerLabels:   aws.Int32(10),
        },
    }

    // 3. 轮询等待完成 (带退避)
    // 4. 下载并解析结果
    // 5. 清理临时 S3 文件

    return result, nil
}

type TranscriptResult struct {
    Text     string          `json:"text"`      // 完整文本
    Segments []Segment       `json:"segments"`  // 带说话人标签的分段
    Duration time.Duration   `json:"duration"`
}

type Segment struct {
    Speaker   string  `json:"speaker"`
    StartTime float64 `json:"start_time"`
    EndTime   float64 `json:"end_time"`
    Text      string  `json:"text"`
}
```

#### 6.4.5 LLM 会议分析

转写完成后，将文本发送给 LLM 进行结构化分析：

```go
// 分析 prompt
const meetingAnalysisPrompt = `Analyze the following meeting transcript and extract:

1. **Summary**: 3-5 bullet points covering the main topics discussed
2. **Decisions**: List of decisions made, with context
3. **Action Items**: Each with:
   - description
   - assignee (from participant list, or "unassigned")
   - due_date (if mentioned, otherwise "TBD")
   - priority (high/medium/low)
4. **Key Discussions**: Topics that had significant debate or discussion
5. **Participant Summary**: Brief note on each participant's main contributions

Meeting title: {title}
Date: {date}
Participants: {participants}

Transcript:
{transcript}

Output as JSON.`
```

分析结果数据结构：

```go
type MeetingAnalysis struct {
    Title       string       `json:"title"`
    Date        string       `json:"date"`
    Duration    string       `json:"duration"`
    Summary     []string     `json:"summary"`
    Decisions   []Decision   `json:"decisions"`
    ActionItems []ActionItem `json:"action_items"`
    Discussions []Discussion `json:"discussions"`
    Speakers    []Speaker    `json:"speakers"`
}

type Decision struct {
    Description string `json:"description"`
    Context     string `json:"context"`
}

type ActionItem struct {
    Description string `json:"description"`
    Assignee    string `json:"assignee"`
    DueDate     string `json:"due_date"`
    Priority    string `json:"priority"`
    Status      string `json:"status"`  // "open" / "done"
}

type Discussion struct {
    Topic   string `json:"topic"`
    Summary string `json:"summary"`
}

type Speaker struct {
    Name            string  `json:"name"`
    SpeakingPercent float64 `json:"speaking_percent"`
    MainTopics      string  `json:"main_topics"`
}
```

#### 6.4.6 Obsidian 输出

将分析结果写入 Obsidian vault（如果配置了 `obsidian.vault_path`）：

```yaml
# config.yaml
meeting:
  enabled: true
  s3_bucket: "naozhi-meeting-audio"
  obsidian:
    vault_path: "/home/naozhi/obsidian-vault"
    template: "meeting"       # 使用 meeting 模板
    folder: "Meetings"        # 写入目标文件夹
    daily_note_link: true     # 在 daily note 中添加链接
```

生成的 Obsidian Markdown 格式：

```markdown
---
title: Weekly Standup
date: 2026-04-15
participants:
  - keith
  - alice
  - bob
tags:
  - meeting
  - weekly
duration: 45min
---

# Weekly Standup - 2026-04-15

## Summary
- 讨论了 CloudFront demo 平台的进展，Part 1 已完成
- Keith 提出需要增加 Tag-Based Invalidation 功能
- 确认本周五前完成 EKS node group 扩容

## Decisions
1. CloudFront demo 平台采用 CDK 部署，放弃 Terraform 方案
2. EKS node group 扩容从 3 → 5 节点

## Action Items
- [ ] Keith: 完成 CloudFront Tag-Based Invalidation spec (Due: 2026-04-16) #high
- [ ] Alice: 更新 CDK stack 配置 (Due: 2026-04-17) #medium
- [ ] Bob: 测试 EKS 扩容后的性能 (Due: 2026-04-18) #medium

## Key Discussions
### CloudFront 部署方式
CDK vs Terraform 讨论。CDK 优势：与现有 AWS 资源更好集成，TypeScript 类型安全。
最终选择 CDK。

## Participants
| 参与者 | 发言占比 | 主要话题 |
|--------|---------|---------|
| Keith  | 45%     | CloudFront spec, 架构决策 |
| Alice  | 35%     | CDK 实现, EKS 配置 |
| Bob    | 20%     | 测试计划, 性能指标 |
```

#### 6.4.7 Action Items → Patrol 联动

分析完成后，高优先级 action items 可自动创建 Patrol 任务进行追踪：

```go
// 自动创建 action item 追踪 patrol
func (h *MeetingHandler) createActionItemPatrol(item ActionItem) {
    if item.Priority != "high" || item.DueDate == "TBD" {
        return
    }

    // 创建一次性 patrol, 在 due date 当天 09:00 触发
    schedule := fmt.Sprintf("0 9 %s * *", parseDayOfMonth(item.DueDate))
    patrol := &patrol.Patrol{
        Name:     fmt.Sprintf("action-%s", generateShortID()),
        Schedule: schedule,
        Agent:    "general",
        Prompt:   fmt.Sprintf("Action item reminder: %s\nAssignee: %s\nDue: %s\nPlease check if this has been completed.",
            item.Description, item.Assignee, item.DueDate),
        Notify:   []string{"feishu", "dashboard"},
    }
    // ... 注册到 PatrolManager
}
```

#### 6.4.8 API 端点汇总

```
POST   /api/meeting/upload              → 上传会议录音 (multipart)
GET    /api/meeting/{job-id}            → 查询处理状态和结果
GET    /api/meeting/history             → 历史会议列表 (?limit=20)
GET    /api/meeting/{job-id}/transcript → 获取原始转写文本
GET    /api/meeting/{job-id}/analysis   → 获取 LLM 分析结果
POST   /api/meeting/{job-id}/action-items/{index}/done → 标记 action item 完成
DELETE /api/meeting/{job-id}            → 删除会议记录
```

#### 6.4.9 配置

```yaml
# config.yaml
meeting:
  enabled: true
  provider: aws                     # 转写服务提供商
  region: us-east-1
  s3_bucket: "naozhi-meeting-audio" # 临时存储桶
  language: "zh-CN"                 # 默认语言
  max_file_size: "100MB"
  max_duration: "2h"                # 最大录音时长
  obsidian:
    vault_path: "/home/naozhi/obsidian-vault"
    folder: "Meetings"
    daily_note_link: true
  auto_patrol:                      # action item 自动追踪
    enabled: true
    priority_threshold: "high"      # 仅追踪 high priority items
```

---

### 6.5 集成依赖关系

```
                      ┌─────────────────────┐
                      │  Platform Adapters   │
                      │  (feishu/slack/...)  │
                      └──────────┬──────────┘
                                 │ 通知推送
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
          v                      v                      v
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  GitHub          │  │  AWS Console    │  │  Observability  │
│  Integration     │  │  Integration    │  │  Bridge         │
└────────┬────────┘  └────────┬────────┘  └────────┬────────┘
         │                    │                     │
         └────────────────────┼─────────────────────┘
                              │
                    ┌─────────v─────────┐
                    │  Patrol Manager   │
                    │  (调度 + 执行)     │
                    └─────────┬─────────┘
                              │
                    ┌─────────v─────────┐
                    │  Approval Gateway │
                    │  (审批 + 门控)     │
                    └─────────┬─────────┘
                              │
                    ┌─────────v─────────┐
                    │  Session Router   │
                    │  (现有核心)        │
                    └───────────────────┘
```

所有集成模块遵循统一模式：
1. **数据获取** — 通过 Agent tool 调用（gh CLI, aws CLI, MCP server）
2. **事件触发** — 通过 Patrol webhook 端点接收外部事件
3. **结果处理** — 通过 Patrol notification 路由推送
4. **危险操作** — 通过 Approval Gateway 门控
5. **人机交互** — 通过 Dashboard + IM 双通道
## 7. 移动端设计 (Mobile Web)

### 7.1 设计原则

- **PWA 响应式 web**, 不是 native app — 用户通过手机浏览器访问 `http://naozhi-server:8180`
- 同一份 HTML, 通过 CSS `@media (max-width: 768px)` 适配
- iOS/Android safe area insets (`env(safe-area-inset-*)`)
- 触控目标 >= 44px
- `-webkit-tap-highlight-color: transparent` 消除闪烁
- 已有 PWA manifest + service worker (V1 基础)

### 7.2 导航模式

**底部 Tab Bar** (5 tabs, 桌面端 7 个精简):

```
Home | Chat | Knowledge | Patrols | Approvals
```

- Wiki 和 Graph 合并到 Knowledge tab 的子视图 (tab 内切换)
- 顶部: Logo + 搜索按钮 + 通知铃铛
- Tab 图标 + 文字标签, active 状态蓝色高亮
- Approvals tab 有红点 badge (pending 数)

### 7.3 各 Tab 设计

#### Home Tab

| 元素 | 设计 |
|------|------|
| 问候语 | "Good afternoon, Keith" + 日期 |
| Stats | 横向滚动卡片 (sessions / patrols / cost) |
| Patrol 状态 | 紧凑列表: dot + name + status (单行) |
| 审批 Banner | 红色条, tap 跳转 Approvals |
| Activity Feed | 可滚动列表 (图标 + 文本 + 时间) |
| FAB | 右下角 "+" 浮动按钮 → 新建 session |

#### Chat Tab

**两级导航** (push/pop 转场动画):

**Session List** (一级):
- 搜索栏 + 标签 pills (横向滚动)
- Session 卡片: icon + name + meta + time
- 左滑删除, 长按置顶
- 分组: Pinned / Today / CLI / Yesterday

**Conversation** (二级, 从右侧 push 进入):
- 返回按钮 (← 左上角) + session 名 + agent + cost
- 消息: 用户气泡 (右) + AI 文本 (左)
- 工具卡片: 紧凑单行 (icon + name + status), tap 展开
- 代码块: 横向滚动 + Copy 按钮
- Diff: 紧凑 unified 视图
- Bookmark: tap AI 消息出现 🔖
- 输入: 文本框 + 🎙 麦克风 + ▲ 发送
- 语音: 按住说话 overlay (复用现有 WeChat 风格 voice-overlay)

#### Knowledge Tab

- 文件树: 全宽, 大触控目标 (48px 行高)
- tap 文件夹: 展开/折叠
- tap 文件: push 到渲染视图
- 渲染视图: Obsidian 内容 + 移动端排版 (更大字体, 表格横向滚动)
- 右下角 AI 浮动按钮 🤖 → 底部 sheet 对话

#### Patrols Tab

- 全宽 Patrol 卡片 (垂直堆叠)
- 每卡片: icon + name + badge + description + last run
- tap 展开显示日志
- "+" 按钮 (新建 Patrol)

#### Approvals Tab

- 全宽审批卡片
- urgent: 红色左边框
- 按钮: Approve (绿) / Reject (灰) / Details
- 处理完: 卡片滑出动画
- 全部完成: "All caught up!" 空状态 (✓ 图标)

### 7.4 新建 Session (手机端)

- FAB "+" → 底部 Sheet (从下方滑入)
- Agent 四宫格 (2x2, 大触控目标)
- Project 列表 (可选, 滚动)
- Create → 关闭 sheet + toast + 跳 Chat tab

### 7.5 通知 (手机端)

- 顶部铃铛 → 下拉面板 (全宽)
- urgent/unread/read 分级
- tap 跳转对应视图

---

## 8. 技术架构

### 8.1 前端架构演进

#### Phase 1: 增强现有单 HTML

保持 Go `embed.FS` 单文件架构, 通过 CDN 增强:

| 依赖 | 用途 | 加载方式 |
|------|------|---------|
| Shiki | 代码语法高亮 | CDN WASM, 按需加载语言包 |
| KaTeX | 数学公式 (已有) | CDN |
| Mermaid | 图表 (已有) | CDN |

所有新 UI 组件用原生 JS + CSS 实现. 预估 HTML 文件大小: ~200KB (当前 ~120KB).

#### Phase 2: 轻量化前端框架 (如需要)

如果单 HTML 超过 300KB 或组件复杂度导致维护成本过高:

- 候选: **Preact** (3KB) + **Vite** 构建
- Go 侧不变, 只是 `internal/server/static/` 从单文件变为构建产物目录
- `embed.FS` 仍然嵌入构建产物, 保持单二进制部署

### 8.2 后端新增模块

```
internal/
  knowledge/              # 新增: 知识系统
    vault.go              # Obsidian vault 文件读取 + goldmark 渲染
    vault_tree.go         # 目录树 JSON 生成
    wiki.go               # Wiki 编译页管理 (CRUD)
    search.go             # bleve 索引管理 (index/query/update)
    ingest.go             # Ingest 编排 (调用 CLI 子进程)
    lint.go               # Lint 健康检查
    bookmark.go           # Bookmark CRUD + 标签
    store.go              # bookmarks.json 持久化

  patrol/                 # 新增: 自主巡逻 (扩展现有 cron)
    patrol.go             # Patrol struct + lifecycle (Active/Paused/Disabled)
    executor.go           # 执行逻辑 (复用 session router + CLI process)
    webhook.go            # 外部事件接收 (/api/webhooks)
    approval.go           # 审批管理 (create/approve/reject)
    notification.go       # 通知路由 (Dashboard + IM)
    store.go              # patrols.json + approvals.json 持久化
```

### 8.3 新增 API 路由

```
# Knowledge
GET    /api/vault/tree                    # 目录树 JSON
GET    /api/vault/read?path=...           # 渲染后 HTML
GET    /api/vault/raw?path=...            # 原始 Markdown
GET    /api/search?q=...&source=...       # 统一搜索
GET    /api/wiki                          # Wiki 页面列表
GET    /api/wiki/{name}                   # 读取编译页
POST   /api/wiki/ingest                   # 触发 Ingest
POST   /api/wiki/lint                     # 触发 Lint
GET    /api/bookmarks                     # 列出 bookmarks
POST   /api/bookmarks                     # 创建 bookmark
DELETE /api/bookmarks/{id}                # 删除 bookmark

# Patrols
GET    /api/patrols                       # 列出所有 Patrol
POST   /api/patrols                       # 创建 Patrol
PUT    /api/patrols/{name}                # 更新 Patrol 配置
DELETE /api/patrols/{name}                # 删除 Patrol
POST   /api/patrols/{name}/trigger        # 手动触发执行
POST   /api/patrols/{name}/pause          # 暂停
POST   /api/patrols/{name}/resume         # 恢复
GET    /api/patrols/{name}/logs           # 执行日志

# Approvals
GET    /api/approvals                     # 列表 (支持 ?status=pending)
POST   /api/approvals/{id}/approve        # 批准
POST   /api/approvals/{id}/reject         # 拒绝
GET    /api/approvals/{id}                # 详情

# Webhooks
POST   /api/webhooks/{patrol-name}        # Patrol 事件触发
POST   /api/webhooks/alert                # 告警自动调查

# Notifications
GET    /api/notifications                 # 通知列表
POST   /api/notifications/read-all        # 标记全部已读
```

### 8.4 数据存储

```
~/.naozhi/
  sessions.json              # (已有) session 持久化
  cron.json                  # (已有) cron 任务
  bookmarks.json             # 新增: bookmark 数据
  patrols.json               # 新增: patrol 配置 + 状态
  approvals.json             # 新增: 审批历史
  notifications.json         # 新增: 通知列表
  search.bleve/              # 新增: 全文搜索索引目录
  wiki/                      # 新增: 编译 wiki 目录
    CLAUDE.md                #   编译规则 (Karpathy schema)
    aws-waf.md               #   编译页面
    cloudfront.md
    zeelool-cdn.md
    naozhi-architecture.md
    ...
  patrols/                   # 新增: patrol 日志
    pr-review/logs.jsonl
    cost-alert/logs.jsonl
    infra-health/logs.jsonl
    ...
```

**向后兼容**: 所有新增为独立文件, 不修改现有 `sessions.json` / `cron.json` 格式. 旧版本安全忽略新文件.

### 8.5 WebSocket 扩展

新增 server → client 消息类型:

```jsonc
// Patrol 执行事件
{"type": "patrol_event", "patrol": "pr-review", "status": "completed", "summary": "Reviewed PR #148"}

// 审批创建
{"type": "approval_created", "approval": {"id": "appr-001", "patrol": "infra-health", "action": "terraform apply", "urgency": "urgent"}}

// 审批处理
{"type": "approval_resolved", "id": "appr-001", "action": "approved"}

// 通知
{"type": "notification", "notification": {"title": "PR #146: Security Issue", "urgency": "urgent"}}

// Wiki 更新
{"type": "wiki_updated", "page": "aws-waf", "sources_added": 2}

// 搜索索引更新
{"type": "search_index_updated", "total_docs": 1234}

// CLI session 发现
{"type": "cli_session_discovered", "session_id": "...", "cwd": "~/infra/terraform-waf"}
```

### 8.6 性能考量

| 操作 | 目标延迟 | 策略 |
|------|---------|------|
| bleve 搜索 | < 50ms | 内存索引, BM25 排序 |
| goldmark 渲染 | < 10ms/文件 | 无外部依赖, 纯 Go |
| Vault 文件树 | < 100ms | 缓存 + 60s 刷新 (复用 project manager 模式) |
| Wiki Ingest | 异步 | CLI 子进程, 不阻塞主流程 |
| Dashboard 首页 | < 200ms | stat 数据内存聚合, 无 DB 查询 |

---

## 9. 实施路线图

### Phase 1: UI Quick Wins (1-2 周)

| 任务 | 影响 | 复杂度 |
|------|------|--------|
| 代码块语法高亮 (Shiki CDN) | 高 | 低 |
| 工具调用卡片化 (折叠/展开) | 高 | 低 |
| Diff 渲染 (Edit 事件解析) | 高 | 中 |
| 长输出折叠 | 中 | 低 |
| Cmd+K 全局搜索 (session 名 + 消息文本) | 高 | 中 |
| Session 标签筛选 + 时间分组 | 中 | 低 |
| 新建 Session Modal (Cmd+N) | 中 | 低 |
| Notification Center (前端 UI) | 中 | 低 |
| 移动端底部 tab bar + 手势优化 | 高 | 中 |

### Phase 2: Knowledge Layer (3-4 周)

| 任务 | 影响 | 复杂度 |
|------|------|--------|
| Obsidian vault 浏览器 (Go API + goldmark + 前端文件树) | 高 | 高 |
| Karpathy Wiki 引擎 (ingest + lint + 编译页管理) | 高 | 高 |
| CLI session 同步 (history.jsonl 扫描 + 统一索引) | 高 | 中 |
| 统一搜索引擎 (bleve 全文索引) | 高 | 中 |
| Bookmark 系统 (API + 前端 + Context Panel) | 中 | 中 |
| Context Panel (Saved / Related / AI 三 tab) | 中 | 中 |
| Home 仪表板 | 中 | 中 |

### Phase 3: Autonomous Agents (4-6 周)

| 任务 | 影响 | 复杂度 |
|------|------|--------|
| Patrol Mode (扩展 cron 包 + 状态 + 日志) | 高 | 高 |
| Approval Gateway (审批流 + IM 审批) | 高 | 高 |
| GitHub Webhook 集成 (PR Sentinel patrol) | 高 | 中 |
| Cost Watchdog (AWS Cost Explorer MCP) | 中 | 中 |
| Infra Health (AWS 资源状态 MCP) | 中 | 中 |
| Proactive Insights (关键词匹配版) | 中 | 中 |
| Notification 后端 (WebSocket 推送 + 持久化) | 中 | 低 |

### Phase 4: Platform & Moonshots (6-8 周)

| 任务 | 影响 | 复杂度 |
|------|------|--------|
| Knowledge Graph 可视化 (SVG 力导向图) | 中 | 高 |
| Observability Bridge (告警 → 自动调查) | 中 | 中 |
| Meeting Intelligence (长音频转录 + 摘要) | 中 | 高 |
| Decision Journal (自动 ADR 生成 → Obsidian) | 中 | 中 |
| Session Replay & Sharing | 低 | 高 |
| CTO Digital Twin (知识驱动代理回答) | 低 | 高 |

---

## 10. 竞品对比

| Feature | Naozhi V2 | Open WebUI | LobeChat | Claude.ai | Cursor |
|---------|-----------|------------|----------|-----------|--------|
| **Full CLI Agent** | ✓ Native subprocess | ✗ API wrapper | ✗ API wrapper | ✓ Web only | ✓ IDE only |
| **IM Integration** | ✓ 4 platforms | ✗ | ✗ | ✗ | ✗ |
| **Autonomous Patrols** | ✓ Event + Cron | ✗ | ✗ | ~ Routines (limited) | ✗ |
| **Knowledge Compilation** | ✓ Karpathy method | ~ RAG only | ~ RAG only | ✗ | ✗ |
| **Obsidian Integration** | ✓ Native render | ✗ | ✗ | ✗ | ✗ |
| **CLI Session Sync** | ✓ Bidirectional | ✗ | ✗ | ~ Teleport | ✗ |
| **Multi-Node NAT** | ✓ WebSocket reverse | ✗ | ✗ | ✗ | ✗ |
| **Knowledge Graph** | ✓ Force-directed | ✗ | ✗ | ✗ | ✗ |
| **Approval Gateway** | ✓ Multi-device | ✗ | ✗ | ✗ | ✗ |
| **Self-hosted** | ✓ Single binary | ✓ Docker | ✓ Docker | ✗ Cloud | ✗ Cloud |

### 护城河

Naozhi V2 的核心差异化不在于单个功能, 而在于**四路数据的交汇**:

1. **CLI Session** — 开发者在终端里做的工作
2. **Dashboard 对话** — 在浏览器里的 AI 交互
3. **IM 消息** — 在飞书/Slack 的团队沟通
4. **Obsidian Vault** — 个人知识沉淀

市面上没有任何产品同时打通这四路数据, 并通过知识编译统一检索. 这是 Naozhi 作为 "CTO Operating System" 的根基.
