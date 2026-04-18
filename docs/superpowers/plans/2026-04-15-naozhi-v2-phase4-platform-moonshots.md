# Naozhi V2.0 Phase 4: Platform & Moonshots 实施计划

> ⚠️  **Meeting Intelligence 功能已于 2026-04-18 下线**（见
> `docs/superpowers/specs/2026-04-18-naozhi-ddd-refactor-design.md` §6
> Phase 0）。本 plan 中涉及 Meeting 的章节仅作历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Phase 1-3 构建的 Dashboard UI、Knowledge Layer 和 Patrol 自主 Agent 基础上，实现六大高级功能模块: Knowledge Graph 可视化、Observability Bridge 告警桥接、Meeting Intelligence 会议智能、Decision Journal 决策日志、Session Replay 回放分享、CTO Digital Twin 数字分身。这些功能将 Naozhi 从 "AI 对话管理工具" 推向 "CTO Operating System" 的完整形态。

**Architecture:** 所有新模块遵循 Naozhi 现有架构范式 -- Go 后端提供 REST API + WebSocket 事件，前端嵌入 `dashboard.html` 单文件，通过 Patrol/Session Router 复用 CLI 子进程执行 AI 推理。Knowledge Graph 和 Session Replay 属于纯展示层扩展，不引入新的外部依赖；Observability Bridge 和 Meeting Intelligence 扩展现有 `internal/transcribe` 和 Patrol webhook 机制；Decision Journal 和 CTO Digital Twin 建立在 Phase 2 Wiki 知识编译之上。

**Tech Stack:** Go stdlib, `d3-force` (CDN, knowledge graph), `goldmark` (Markdown 渲染), Amazon Transcribe (batch mode), bleve (全文索引), vanilla JavaScript + SVG.

**Spec:** `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md` (Sections 6-10)

**Phase Duration:** 6-8 weeks

**Base Directory:** `/root/keith-space/AWS/EC2-Workload/naozhi/`

---

## 依赖关系总览

本阶段所有任务均依赖 Phase 1 (UI Quick Wins) 的 Dashboard 基础设施。对 Phase 2 和 Phase 3 的具体依赖在每个任务中标注。

```
Phase 2 (Knowledge Layer)              Phase 3 (Autonomous Agents)
├── Wiki 编译引擎 (wiki.go)              ├── Patrol Manager (patrol/manager.go)
├── bleve 搜索索引 (search.go)           ├── Approval Gateway (approval/manager.go)
├── Obsidian vault 浏览器 (vault.go)     ├── Webhook 端点 (/api/webhooks)
├── Bookmark 系统 (bookmark.go)         ├── Notification 后端 (notification.go)
└── Context Panel (dashboard.html)       └── Proactive Insights (insight/)
         │                                        │
         v                                        v
Phase 4 (Platform & Moonshots)
├── A. Knowledge Graph (Tasks 1-5)     ← Phase 2 Wiki + Search
├── B. Observability Bridge (Tasks 6-8) ← Phase 3 Patrol + Webhook
├── C. Meeting Intelligence (Tasks 9-12) ← Phase 2 Obsidian + Phase 3 Patrol
├── D. Decision Journal (Tasks 13-15)   ← Phase 2 Wiki + Obsidian
├── E. Session Replay (Tasks 16-18)     ← Phase 1 Dashboard WebSocket
└── F. CTO Digital Twin (Tasks 19-21)   ← Phase 2 Wiki + Phase 3 Patrol + Insights
```

---

## 文件结构总览

### 新增文件

| File | Responsibility |
|------|---------------|
| `internal/graph/model.go` | Knowledge Graph 数据模型 (Node, Edge, GraphData) |
| `internal/graph/extractor.go` | 从 Wiki 页面提取实体和关系 |
| `internal/graph/api.go` | Graph REST API handlers |
| `internal/graph/extractor_test.go` | Extractor 单元测试 |
| `internal/alert/normalize.go` | 告警格式标准化器 (CloudWatch/Grafana/Datadog) |
| `internal/alert/normalize_test.go` | Normalizer 单元测试 |
| `internal/alert/investigator.go` | 告警自动调查流程编排 |
| `internal/alert/linking.go` | Alert-to-Patrol 关联逻辑 |
| `internal/transcribe/batch.go` | Amazon Transcribe 批量转写 |
| `internal/transcribe/batch_test.go` | Batch 转写单元测试 |
| `internal/meeting/handler.go` | 会议处理流水线 (upload -> transcribe -> analyze) |
| `internal/meeting/analysis.go` | LLM 会议分析 (summary, decisions, action items) |
| `internal/meeting/obsidian.go` | Obsidian journal 输出格式化 |
| `internal/meeting/actionlink.go` | Action item -> Patrol/Cron 联动 |
| `internal/meeting/handler_test.go` | 会议处理单元测试 |
| `internal/decision/extractor.go` | ADR 自动提取 (对话模式识别) |
| `internal/decision/renderer.go` | ADR 模板渲染 (Context -> Decision -> Consequences) |
| `internal/decision/store.go` | ADR 存储管理 (Obsidian vault / wiki 目录) |
| `internal/decision/extractor_test.go` | ADR 提取单元测试 |
| `internal/replay/timeline.go` | Session 事件时间线重建 |
| `internal/replay/viewer.go` | Replay API handlers |
| `internal/replay/share.go` | 分享链接生成 + token 验证 |
| `internal/replay/timeline_test.go` | Timeline 重建单元测试 |
| `internal/twin/assembler.go` | Knowledge-driven prompt 组装 |
| `internal/twin/router.go` | 团队提问委派路由 |
| `internal/twin/confidence.go` | 置信度评分 |
| `internal/twin/assembler_test.go` | Prompt 组装单元测试 |

### 修改文件

| File | Changes |
|------|---------|
| `internal/server/server.go` | 注册 Graph / Alert / Meeting / Decision / Replay / Twin 路由 |
| `internal/server/static/dashboard.html` | 新增 Graph tab SVG 可视化、Meeting 上传 UI、Decision Journal 视图、Replay 播放器、Twin 配置面板 |
| `internal/config/config.go` | 新增 `graph`, `alert`, `meeting`, `decision`, `replay`, `twin` 配置段 |
| `internal/patrol/manager.go` | 增加 alert linking 接口、action item patrol 创建方法 |
| `internal/knowledge/wiki.go` | 暴露实体提取 hook 供 Graph 模块消费 |
| `internal/knowledge/search.go` | 新增 Graph 数据查询方法 |
| `internal/transcribe/transcribe.go` | 重构公共接口，区分 streaming 和 batch mode |

---

## A. Knowledge Graph Visualization (知识图谱可视化)

Knowledge Graph 将 Phase 2 编译的 Wiki 知识从文本列表升级为交互式力导向图谱，让 CTO 以视觉化方式浏览实体间的关联关系。这是 Naozhi 区别于所有竞品的视觉差异化特性。

---

## Task 1: Graph Data Model (图谱数据模型)

**描述:** 定义 Knowledge Graph 的核心数据结构，包括 Node (实体节点) 和 Edge (关系边)。从 Wiki 编译页面的 frontmatter 和正文内容中提取实体及其关系，构建图数据。每个 Wiki 页面对应一个主节点，页面中的 `[[wikilinks]]` 构成边，实体类型从 frontmatter `entities` 字段或正文内容推断。

**Files:**
- Create: `internal/graph/model.go`
- Create: `internal/graph/extractor.go`
- Create: `internal/graph/extractor_test.go`
- Modify: `internal/knowledge/wiki.go` (暴露页面遍历接口)

**Estimated LOC:** ~450

**Dependencies:**
- **Phase 2:** `internal/knowledge/wiki.go` (Wiki 编译页管理, `[[wikilinks]]` 解析)
- **Phase 2:** `internal/knowledge/search.go` (bleve 索引中的实体字段)

**Acceptance Criteria:**
- [ ] `GraphData` 结构体包含 `Nodes []Node` 和 `Edges []Edge`，支持 JSON 序列化
- [ ] `Node` 包含 `ID`, `Label`, `Type` (knowledge_hub / aws_service / project / infrastructure / security_issue), `Size` (连接数权重), `WikiPage` (关联 wiki 路径)
- [ ] `Edge` 包含 `Source`, `Target`, `Label` (关系描述), `Weight`
- [ ] `ExtractGraph()` 遍历 `~/.naozhi/wiki/*.md`，从 frontmatter entities 提取节点，从 `[[wikilinks]]` 提取边
- [ ] 对同一实体的多次引用合并为单节点，边的 weight 反映引用频次
- [ ] 单元测试覆盖: 空 wiki 目录、单页面无 wikilink、多页面交叉引用、重复实体合并
- [ ] `go vet ./internal/graph/...` 无警告

---

## Task 2: Graph REST API

**描述:** 实现 Knowledge Graph 的 REST API 端点，提供完整图数据、按类型筛选节点、以及单节点详情查询。图数据在内存中缓存，Wiki 变更时通过 WebSocket `wiki_updated` 事件触发增量刷新。API 设计遵循 Naozhi 现有 REST 风格 (JSON 响应, 统一 error 格式)。

**Files:**
- Create: `internal/graph/api.go`
- Modify: `internal/server/server.go` (注册 `/api/graph/*` 路由)
- Modify: `internal/config/config.go` (新增 `graph` 配置段)

**Estimated LOC:** ~350

**Dependencies:**
- **Task 1:** `internal/graph/model.go`, `internal/graph/extractor.go`
- **Phase 2:** `internal/knowledge/wiki.go` (Wiki 页面 CRUD)
- **Phase 1:** `internal/server/server.go` (路由注册模式)

**Acceptance Criteria:**
- [ ] `GET /api/graph` 返回完整图数据 `{"nodes": [...], "edges": [...], "stats": {"total_nodes": N, "total_edges": M}}`
- [ ] `GET /api/graph/nodes?type=aws_service` 支持按节点类型筛选
- [ ] `GET /api/graph/nodes/{id}` 返回单节点详情，包含关联边列表和 wiki 页面内容摘要
- [ ] 图数据内存缓存，首次请求时构建，后续请求命中缓存 (< 10ms)
- [ ] Wiki Ingest 完成后自动触发图缓存刷新
- [ ] 配置 `graph.enabled: true/false` 控制功能开关
- [ ] 空 wiki 时返回空图 `{"nodes": [], "edges": []}`，不报错

---

## Task 3: Frontend SVG Force-Directed Graph (前端力导向图渲染)

**描述:** 在 Dashboard 的 Graph tab 中实现基于 SVG 的力导向图可视化。使用 `d3-force` (CDN 加载) 计算节点布局，用原生 SVG 渲染节点和连线。节点按类型着色 (设计规格 Section 2.7 定义的 5 种颜色)，节点大小反映连接数权重。支持鼠标拖拽节点、滚轮缩放、平移画布。

**Files:**
- Modify: `internal/server/static/dashboard.html` (新增 Graph tab 完整 UI)

**Estimated LOC:** ~800 (JavaScript + CSS + SVG)

**Dependencies:**
- **Task 2:** `/api/graph` API 就绪
- **Phase 1:** Dashboard tab 导航框架
- **External:** `d3-force` CDN (`https://cdn.jsdelivr.net/npm/d3-force@3/+esm`)

**Acceptance Criteria:**
- [ ] Graph tab 加载时调用 `GET /api/graph` 获取数据
- [ ] SVG 画布占满 tab 内容区，使用 `viewBox` 实现响应式缩放
- [ ] 节点渲染为圆形 + 标签文字，颜色按类型: Knowledge Hub (紫 #8B5CF6), AWS Service (蓝 #3B82F6), Project/Customer (绿 #10B981), Infrastructure (黄 #F59E0B), Security Issue (红 #EF4444)
- [ ] 节点半径: `Math.max(8, Math.min(24, 4 + node.size * 2))`
- [ ] 连线渲染为灰色半透明直线，hover 时显示关系标签
- [ ] d3-force simulation 包含: `forceLink`, `forceManyBody`, `forceCenter`, `forceCollide`
- [ ] 鼠标拖拽节点时固定该节点位置 (`fx`, `fy`)，释放后恢复自由
- [ ] 滚轮缩放 (0.1x - 5x 范围)，Ctrl+拖拽平移画布
- [ ] 左下角图例: 5 种节点类型 + 颜色说明
- [ ] 节点数 > 200 时自动启用 `alphaDecay(0.05)` 加速收敛

---

## Task 4: Graph Interaction (图谱交互)

**描述:** 为 Knowledge Graph 添加丰富的交互能力。点击节点弹出右侧详情面板，展示节点描述、连接关系列表、来源数量和操作按钮。支持按节点类型筛选 (顶部 filter pills)、搜索节点 (输入框实时过滤)、高亮选中节点的一阶邻居 (其余节点降低透明度)。

**Files:**
- Modify: `internal/server/static/dashboard.html` (Graph tab 交互增强)

**Estimated LOC:** ~600 (JavaScript + CSS)

**Dependencies:**
- **Task 3:** SVG 力导向图基础渲染
- **Task 2:** `/api/graph/nodes/{id}` 详情 API

**Acceptance Criteria:**
- [ ] 单击节点: 右侧滑出详情面板 (300px 宽，`transform: translateX` 动画)
- [ ] 详情面板内容: 节点名称 + 类型 badge + 描述 + 连接列表 (点击跳转) + 来源数 + "Open Wiki" / "View Sources" 按钮
- [ ] 双击节点: 以该节点为中心缩放到 2x
- [ ] 选中节点时: 一阶邻居保持 `opacity: 1`，其余节点和边 `opacity: 0.15`
- [ ] 顶部 filter pills: `[All] [Knowledge Hub] [AWS Service] [Project] [Infrastructure] [Security]`，点击切换隐藏/显示对应类型节点
- [ ] 搜索输入框: 实时过滤，匹配节点高亮 (描边闪烁)，不匹配节点降低透明度
- [ ] 空选 (点击画布空白处): 关闭详情面板，恢复所有节点透明度
- [ ] 移动端: 详情面板改为底部 sheet (从下方滑入)

---

## Task 5: Graph Integration with Wiki (图谱-Wiki 联动)

**描述:** 打通 Knowledge Graph 与 Wiki 视图的双向导航。在 Graph 详情面板点击 "Open Wiki" 按钮时，切换到 Wiki tab 并打开对应页面；反之，在 Wiki 页面视图中点击 "View in Graph" 按钮，切换到 Graph tab 并高亮定位到对应节点。同时在 Wiki 页面底部添加 "Related Graph" 小组件，展示当前页面在图谱中的局部子图 (该页面 + 一阶邻居)。

**Files:**
- Modify: `internal/server/static/dashboard.html` (Wiki -> Graph 导航 + Graph -> Wiki 导航 + 局部子图)

**Estimated LOC:** ~400 (JavaScript + CSS)

**Dependencies:**
- **Task 4:** Graph 交互完整
- **Phase 2:** Wiki 视图 (Dashboard Wiki tab 就绪)

**Acceptance Criteria:**
- [ ] Graph 详情面板 "Open Wiki" 按钮: 切换到 Wiki tab，调用 `/api/wiki/{name}` 加载页面内容
- [ ] Wiki 页面顶部 "View in Graph" 按钮: 切换到 Graph tab，自动选中对应节点 (设置 `selectedNode` + 滚动到视口中心)
- [ ] Wiki 页面底部 "Related Graph" minimap: 150px 高的 SVG 子图，仅包含当前页面节点 + 一阶邻居，无交互 (纯展示)
- [ ] minimap 中节点可点击，跳转到 Graph tab 完整视图
- [ ] Wiki 页面中的 `[[wikilinks]]` 在 Graph 中以高亮边的形式展示
- [ ] URL hash 联动: `#graph/node/{id}` 直接定位到指定节点

---

## B. Observability Bridge (可观测性桥接)

Observability Bridge 将外部监控告警 (CloudWatch, Grafana, Datadog) 接入 Naozhi 的 Patrol 系统，实现 "告警 -> 自动调查 -> 报告 -> 修复" 的闭环。这是 Naozhi 从 "AI 对话工具" 进化为 "运维自动化中枢" 的关键能力。

---

## Task 6: Alert Webhook Normalizer (告警标准化器)

**描述:** 实现多源告警的统一接收和格式标准化。CloudWatch Alarms 通过 SNS HTTPS 推送，Grafana 和 Datadog 通过 webhook notification channel 推送，三者的 JSON 格式各不相同。Normalizer 将它们转换为统一的内部 `Alert` 结构体，包含 source、name、severity、summary、labels 等标准字段。标准化后的 Alert 注入 Patrol webhook 端点触发自动调查。

**Files:**
- Create: `internal/alert/normalize.go`
- Create: `internal/alert/normalize_test.go`
- Modify: `internal/server/server.go` (注册 `/api/webhooks/alert` 路由)
- Modify: `internal/config/config.go` (新增 `alert` 配置段)

**Estimated LOC:** ~550

**Dependencies:**
- **Phase 3:** `internal/patrol/webhook.go` (Patrol webhook 端点, `POST /api/webhooks/{patrol-name}`)
- **Phase 3:** `internal/patrol/manager.go` (Patrol 执行流程)

**Acceptance Criteria:**
- [ ] `Alert` 结构体包含 `Source`, `Name`, `Severity` (critical/warning/info), `Summary`, `Details` (map), `Labels` (map), `Time`
- [ ] `NormalizeCloudWatch(payload)` 解析 SNS notification JSON，提取 AlarmName, NewStateValue, NewStateReason, Trigger.MetricName
- [ ] `NormalizeGrafana(payload)` 解析 Grafana webhook JSON，提取 ruleName, state, message, evalMatches
- [ ] `NormalizeDatadog(payload)` 解析 Datadog webhook JSON，提取 title, alert_type, body, tags
- [ ] `NormalizeCustom(payload)` 接受任意 JSON，从约定字段 `alert_name`, `severity`, `summary` 提取
- [ ] `POST /api/webhooks/alert` 端点: 根据 `X-Alert-Source` header 或 body `source` 字段路由到对应 normalizer
- [ ] 标准化成功后，将 Alert 序列化为 JSON payload，调用 Patrol Manager 的 event 触发方法
- [ ] 配置 `alert.enabled: true`, `alert.default_patrol: "alert-investigator"` 指定接收 Patrol
- [ ] 单元测试覆盖: CloudWatch ALARM/OK 状态、Grafana alerting/ok 状态、Datadog warning/error 类型、无效 JSON 格式
- [ ] 未知 source 返回 400 Bad Request，附带支持的 source 列表

---

## Task 7: Auto-Investigation Flow (自动调查流程)

**描述:** 实现告警触发后的全自动调查链路: 收到标准化告警 -> 创建调查 session -> 注入告警上下文到 prompt -> Agent 执行根因分析 -> 生成结构化报告 -> 推送到 Dashboard 和 IM。调查 session 复用 Patrol 执行机制，但使用专门的 `investigation` prompt 模板，引导 Agent 进行系统化排查 (指标检查 -> 日志分析 -> 关联事件 -> 根因假设 -> 修复建议)。

**Files:**
- Create: `internal/alert/investigator.go`
- Modify: `internal/patrol/manager.go` (新增 `ExecuteInvestigation` 方法)
- Modify: `internal/server/server.go` (注册 `/api/alerts/investigate` 手动触发端点)

**Estimated LOC:** ~500

**Dependencies:**
- **Task 6:** `internal/alert/normalize.go` (标准化 Alert 结构)
- **Phase 3:** `internal/patrol/manager.go` (Patrol 执行机制 + CLI 子进程)
- **Phase 3:** `internal/patrol/notification.go` (通知路由: Dashboard + IM)

**Acceptance Criteria:**
- [ ] `InvestigationConfig` 包含 `prompt_template` (可配置的调查 prompt), `timeout` (默认 10min), `mcp_servers` (调查所需的 MCP server 列表), `notify` 目标
- [ ] 调查 prompt 模板注入告警上下文: `[Alert: {source}:{name}]\nSeverity: {severity}\nSummary: {summary}\nDetails: {details_json}\n\n{investigation_prompt}`
- [ ] 默认调查 prompt 包含 5 步方法论: (1) 检查受影响服务的 metrics 和 logs, (2) 识别潜在原因, (3) 关联近期变更, (4) 评估影响范围, (5) 提出修复建议
- [ ] 调查 session key 格式: `investigation:{alert-name}:{timestamp}`，独立于常规 Patrol session
- [ ] 调查完成后生成 `InvestigationReport`: `AlertName`, `RootCause`, `Impact`, `Remediation` (steps list), `AutoFixable` (bool), `SessionKey`
- [ ] Report 通过 Notification 路由推送到 Dashboard Activity Feed + 配置的 IM 平台
- [ ] `POST /api/alerts/investigate` 支持手动触发调查 (传入 alert JSON body)
- [ ] 同一告警 5 分钟内重复触发时，复用已有调查 session 而非创建新的 (去重)
- [ ] 调查超时时记录中间结果，不丢弃已有分析内容

---

## Task 8: Alert-to-Patrol Linking (告警-巡逻关联)

**描述:** 建立告警与已有 Patrol 之间的自动关联关系。当告警到达时，根据告警的 labels (service name, resource type, namespace 等) 匹配可能相关的 Patrol 执行历史，将关联信息注入调查 prompt，帮助 Agent 获取更多上下文。同时在 Dashboard Patrol 详情页展示该 Patrol 近期关联的告警列表，形成 "告警 <-> Patrol" 双向索引。

**Files:**
- Create: `internal/alert/linking.go`
- Modify: `internal/alert/investigator.go` (调查 prompt 注入 Patrol 历史)
- Modify: `internal/patrol/manager.go` (暴露日志搜索接口 `SearchLogs`)
- Modify: `internal/server/static/dashboard.html` (Patrol 详情页增加关联告警卡片)

**Estimated LOC:** ~400

**Dependencies:**
- **Task 7:** `internal/alert/investigator.go` (调查流程)
- **Phase 3:** `internal/patrol/manager.go` (Patrol 日志持久化 + 查询)
- **Phase 3:** Proactive Insights `internal/insight/correlator.go` (实体匹配模式)

**Acceptance Criteria:**
- [ ] `LinkAlertToPatrols(alert *Alert) []PatrolMatch` 根据 Alert labels 匹配 Patrol: (1) 名称包含告警 service name, (2) 近期日志中提及告警 resource, (3) Patrol 的 MCP server 与告警 source 相关
- [ ] `PatrolMatch` 包含 `PatrolName`, `Relevance` (0-1 评分), `RecentLogs` (最近 3 条相关日志摘要)
- [ ] 匹配结果注入调查 prompt 的上下文区域: `[Related Patrol History]\n{patrol_name}: {log_summary}\n...`
- [ ] `GET /api/patrols/{name}/alerts` 返回该 Patrol 关联的历史告警列表 (最近 50 条)
- [ ] Alert → Patrol 和 Patrol → Alert 双向索引存储在内存 map 中，持久化到 `~/.naozhi/alert-links.json`
- [ ] Dashboard Patrol 详情页底部新增 "Related Alerts" 卡片区域
- [ ] 无匹配结果时正常执行调查，不注入空的 Patrol 上下文

---

## C. Meeting Intelligence (会议智能)

Meeting Intelligence 扩展现有 `internal/transcribe` 语音转写模块 (当前仅支持飞书/Slack 语音消息的实时 streaming STT)，增加长音频批量处理和 LLM 驱动的会议分析能力。产出物包括结构化摘要、决策记录、action items，并自动写入 Obsidian vault 和 Patrol 追踪系统。

---

## Task 9: Long Audio Transcription (长音频批量转写)

**描述:** 扩展 `internal/transcribe` 包，新增 Amazon Transcribe 批量转写模式。现有代码仅支持 streaming mode (适用于 <60s 的语音消息)，本 task 增加 batch mode: 上传音频到 S3 临时桶 -> 启动 Transcribe Job -> 轮询等待完成 -> 下载并解析结果 -> 清理临时文件。支持 speaker diarization (说话人分离)，为后续会议分析提供带说话人标签的分段文本。

**Files:**
- Create: `internal/transcribe/batch.go`
- Create: `internal/transcribe/batch_test.go`
- Modify: `internal/transcribe/transcribe.go` (重构公共接口，区分 streaming/batch)
- Modify: `internal/config/config.go` (新增 `meeting.s3_bucket`, `meeting.language` 配置)

**Estimated LOC:** ~550

**Dependencies:**
- **Phase 1:** `internal/transcribe/transcribe.go` (现有 streaming 转写基础架构)
- **External:** AWS SDK `transcribeservice` (已有 `transcribe` 包的 AWS 依赖)
- **External:** S3 临时存储桶 (配置项 `meeting.s3_bucket`)

**Acceptance Criteria:**
- [ ] `BatchTranscriber` 结构体包含 `Region`, `S3Bucket`, `S3Prefix`, `Language`, `OutputPrefix`
- [ ] `StartBatchJob(ctx, audioPath, jobName)` 完整实现: S3 upload -> StartTranscriptionJob -> poll with backoff -> download result -> parse -> cleanup S3
- [ ] 轮询策略: 初始 5s，指数退避至 30s，最大等待 `meeting.max_duration` (默认 2h)
- [ ] `TranscriptResult` 包含 `Text` (完整文本), `Segments []Segment` (speaker + start_time + end_time + text), `Duration`
- [ ] speaker diarization 配置: `ShowSpeakerLabels: true`, `MaxSpeakerLabels: 10`
- [ ] 支持音频格式: mp3, m4a, wav, flac, ogg (Amazon Transcribe 原生支持)
- [ ] 单元测试: mock S3/Transcribe API，验证上传/轮询/解析/清理流程
- [ ] 错误处理: S3 上传失败回滚、Transcribe Job 失败清理 S3 文件、context cancellation 中止轮询
- [ ] 重构 `transcribe.go` 暴露公共 `Transcriber` interface: `TranscribeStream(ctx, reader)` 和 `TranscribeBatch(ctx, filePath, jobName)`

---

## Task 10: Meeting Summary Generation (会议摘要生成)

**描述:** 实现 LLM 驱动的会议分析流水线。接收 Task 9 产出的转写文本，通过 CLI 子进程调用 LLM (复用 Naozhi session router)，提取结构化信息: 会议摘要 (3-5 条要点)、决策列表 (含上下文)、action items (含负责人/截止日期/优先级)、重要讨论话题、参与者发言分布。分析结果以 JSON 结构体存储，供后续 Obsidian 输出和 Patrol 联动消费。

**Files:**
- Create: `internal/meeting/handler.go` (流水线编排: upload -> transcribe -> analyze -> output)
- Create: `internal/meeting/analysis.go` (LLM 分析逻辑 + 数据结构)
- Create: `internal/meeting/handler_test.go`
- Modify: `internal/server/server.go` (注册 `/api/meeting/*` 路由)

**Estimated LOC:** ~650

**Dependencies:**
- **Task 9:** `internal/transcribe/batch.go` (批量转写结果)
- **Phase 3:** `internal/session/router.go` (复用 session router 发送 prompt 到 CLI)
- **Phase 3:** `internal/patrol/notification.go` (分析完成通知推送)

**Acceptance Criteria:**
- [ ] `POST /api/meeting/upload` 接收 multipart file + 元数据 (title, date, participants, language)，返回 `{"job_id": "mtg-xxx", "status": "processing"}`
- [ ] `GET /api/meeting/{job-id}` 返回处理状态: `processing` / `transcribing` / `analyzing` / `completed` / `failed`
- [ ] 处理流水线: 存储临时文件 -> BatchTranscriber -> 构造分析 prompt -> session.Send() -> 解析 JSON 响应
- [ ] `MeetingAnalysis` 结构体: `Title`, `Date`, `Duration`, `Summary []string`, `Decisions []Decision`, `ActionItems []ActionItem`, `Discussions []Discussion`, `Speakers []Speaker`
- [ ] 分析 prompt 包含: 会议标题、日期、参与者列表、完整转写文本，要求 JSON 格式输出
- [ ] `GET /api/meeting/history` 返回历史会议列表 (按日期倒序, 分页)
- [ ] `GET /api/meeting/{job-id}/analysis` 返回分析结果 JSON
- [ ] `GET /api/meeting/{job-id}/transcript` 返回原始转写文本
- [ ] 分析结果持久化到 `~/.naozhi/meetings/{job-id}.json`
- [ ] 音频文件最大 100MB，超过返回 413 Payload Too Large
- [ ] 处理完成后推送 WebSocket `meeting_completed` 事件到 Dashboard

---

## Task 11: Obsidian Journal Integration (Obsidian 日志集成)

**描述:** 将会议分析结果自动写入 Obsidian vault，生成符合 Obsidian 语法规范的会议笔记。笔记包含 YAML frontmatter (标题/日期/参与者/标签/时长)、Summary 摘要列表、Decisions 决策记录、Action Items 任务清单 (Obsidian task 语法 `- [ ]`)、讨论话题、参与者发言分布表格。可选在 daily note 中自动插入会议链接。

**Files:**
- Create: `internal/meeting/obsidian.go`
- Modify: `internal/meeting/handler.go` (流水线增加 Obsidian 输出步骤)
- Modify: `internal/config/config.go` (新增 `meeting.obsidian.*` 配置)

**Estimated LOC:** ~400

**Dependencies:**
- **Task 10:** `internal/meeting/analysis.go` (MeetingAnalysis 结构体)
- **Phase 2:** `internal/knowledge/vault.go` (Obsidian vault 路径配置 + 文件写入)

**Acceptance Criteria:**
- [ ] `WriteToObsidian(analysis *MeetingAnalysis, config ObsidianConfig) (string, error)` 生成 Markdown 文件并写入 vault
- [ ] 输出文件路径: `{vault_path}/{folder}/{date}-{title-slug}.md` (如 `Meetings/2026-04-15-weekly-standup.md`)
- [ ] Frontmatter 包含: `title`, `date`, `participants` (YAML list), `tags` (含 `meeting`), `duration`
- [ ] Action Items 使用 Obsidian task 语法: `- [ ] {assignee}: {description} (Due: {date}) #{priority}`
- [ ] 决策记录格式: 编号列表，每条含决策描述和上下文
- [ ] 参与者发言分布: Markdown 表格 `| 参与者 | 发言占比 | 主要话题 |`
- [ ] 配置 `meeting.obsidian.daily_note_link: true` 时，在当天 daily note 中追加 `[[{meeting-file}]]` 链接
- [ ] 文件已存在时追加时间后缀 (不覆盖): `2026-04-15-weekly-standup-2.md`
- [ ] vault_path 不存在或不可写时，跳过 Obsidian 输出并记录 warning 日志，不影响流水线其他步骤

---

## Task 12: Action Item -> Patrol/Cron Linking (Action Item 联动)

**描述:** 将会议中提取的高优先级 action items 自动关联到 Naozhi 的任务追踪体系。对于有明确截止日期的 action item，在截止日当天 09:00 自动创建一次性 Patrol 提醒任务；对于无截止日期的，加入 Dashboard Notification 待办列表。用户可在 Dashboard 手动标记 action item 完成或关联到已有 Patrol。

**Files:**
- Create: `internal/meeting/actionlink.go`
- Modify: `internal/patrol/manager.go` (新增 `CreateOneShot` 方法: 创建一次性 Patrol)
- Modify: `internal/meeting/handler.go` (流水线增加 action item 联动步骤)
- Modify: `internal/server/static/dashboard.html` (Meeting 详情页 action item 管理 UI)

**Estimated LOC:** ~400

**Dependencies:**
- **Task 10:** `internal/meeting/analysis.go` (ActionItem 结构体)
- **Phase 3:** `internal/patrol/manager.go` (Patrol 创建 + 调度)
- **Phase 3:** `internal/server/notification.go` (Notification 推送)

**Acceptance Criteria:**
- [ ] 配置 `meeting.auto_patrol.enabled: true`, `meeting.auto_patrol.priority_threshold: "high"` 控制自动联动
- [ ] 高优先级 + 有截止日期的 action item: 自动创建 Patrol，name 格式 `action-{short-id}`, schedule 为截止日 09:00
- [ ] Patrol prompt: "Action item reminder: {description}\nAssignee: {assignee}\nDue: {date}\nSource: {meeting title}\nPlease check if this has been completed."
- [ ] Patrol 执行后结果推送到 Dashboard + IM，包含原始 action item 上下文
- [ ] `POST /api/meeting/{job-id}/action-items/{index}/done` 标记 action item 完成，同时禁用对应 Patrol
- [ ] `POST /api/meeting/{job-id}/action-items/{index}/link-patrol` 手动关联到已有 Patrol
- [ ] Dashboard Meeting 详情页: action items 列表，每条含 checkbox + 状态 badge (open/linked/done) + 关联 Patrol 链接
- [ ] 截止日期已过的 action item 标记为 overdue (红色)，自动触发一次提醒

---

## D. Decision Journal (决策日志)

Decision Journal 自动从 Naozhi 对话历史中提取架构决策记录 (ADR - Architecture Decision Record)，按标准模板 (Context -> Decision -> Consequences) 格式化，持久化到 Obsidian vault 或 Wiki 目录。这让 CTO 的隐性决策知识显性化，形成可追溯的决策链。

---

## Task 13: ADR Auto-Extraction (ADR 自动提取)

**描述:** 实现从 Agent 对话文本中自动检测决策模式并提取 ADR 候选。检测策略分两层: (1) 关键词模式匹配 -- 识别 "决定/选择/采用/放弃/decided/chose/selected" 等决策信号词及其上下文; (2) LLM 语义提取 -- 对标记为候选的对话片段，调用 LLM 判断是否包含明确的技术决策并提取结构化信息。提取结果为 `ADRCandidate` 列表，每个候选包含决策描述、上下文背景、备选方案、选择理由。

**Files:**
- Create: `internal/decision/extractor.go`
- Create: `internal/decision/extractor_test.go`
- Modify: `internal/config/config.go` (新增 `decision` 配置段)

**Estimated LOC:** ~500

**Dependencies:**
- **Phase 2:** `internal/knowledge/search.go` (bleve 索引查询对话历史)
- **Phase 1:** Dashboard WebSocket 事件流 (实时对话内容获取)

**Acceptance Criteria:**
- [ ] `ADRCandidate` 结构体: `ID`, `Title`, `Context` (背景), `Decision` (决策内容), `Alternatives` (备选方案 []string), `Consequences` (影响), `Confidence` (0-1, 提取置信度), `SourceSession`, `SourceTimestamp`, `Status` (candidate/confirmed/rejected)
- [ ] Phase 1 关键词检测器: 正则匹配 `决定|选择|采用|放弃|chose|decided|selected|going with|opted for` 及其前后 3 句上下文
- [ ] Phase 2 LLM 提取: 将候选片段发送给 LLM，prompt 要求判断 "这段对话是否包含明确的技术/架构决策" 并提取 title/context/decision/alternatives/consequences
- [ ] `ExtractFromSession(sessionKey string) ([]ADRCandidate, error)` 从指定 session 提取
- [ ] `ExtractFromRecent(hours int) ([]ADRCandidate, error)` 扫描近 N 小时的所有 session
- [ ] Confidence < 0.5 的候选不推送，但存储为 `candidate` 状态供人工审核
- [ ] 配置 `decision.enabled: true`, `decision.auto_extract: true` (对话结束时自动扫描), `decision.keywords` (可自定义关键词列表)
- [ ] 单元测试覆盖: 明确决策对话、模糊讨论 (不应提取)、中英文混合对话、多决策同 session

---

## Task 14: ADR Template Rendering (ADR 模板渲染)

**描述:** 将 `ADRCandidate` 渲染为标准 ADR 格式的 Markdown 文档。ADR 模板遵循 Michael Nygard 的经典格式: Title + Date + Status + Context + Decision + Consequences，并扩展 Naozhi 特有字段: Source (来源 session 链接)、Alternatives (备选方案对比表)、Confidence (AI 提取置信度)。支持模板自定义和多语言 (中文/英文)。提供 Dashboard 预览和编辑界面，用户可在确认前修改 ADR 内容。

**Files:**
- Create: `internal/decision/renderer.go`
- Modify: `internal/server/server.go` (注册 `/api/decisions/*` 路由)
- Modify: `internal/server/static/dashboard.html` (Decision Journal 视图: 列表 + 预览 + 编辑)

**Estimated LOC:** ~550

**Dependencies:**
- **Task 13:** `internal/decision/extractor.go` (ADRCandidate 结构体)
- **Phase 2:** `internal/knowledge/vault.go` (Obsidian Markdown 渲染)

**Acceptance Criteria:**
- [ ] `RenderADR(candidate *ADRCandidate, lang string) string` 输出标准 ADR Markdown
- [ ] 模板结构: YAML frontmatter (title, date, status, tags) + `## Context` + `## Decision` + `## Alternatives` (表格) + `## Consequences` (正面/负面分列) + `## Source`
- [ ] `GET /api/decisions` 列出所有 ADR (candidate + confirmed), 支持 `?status=confirmed` 筛选
- [ ] `GET /api/decisions/{id}` 返回单条 ADR 详情 (含渲染后 Markdown)
- [ ] `PUT /api/decisions/{id}` 更新 ADR 内容 (用户编辑后保存)
- [ ] `POST /api/decisions/{id}/confirm` 将 candidate 标记为 confirmed
- [ ] `POST /api/decisions/{id}/reject` 将 candidate 标记为 rejected
- [ ] Dashboard Decision Journal 视图: 左侧列表 (按日期分组, candidate 黄色标记, confirmed 绿色) + 右侧渲染预览 + 编辑按钮
- [ ] 编辑模式: 切换为 textarea + 实时预览 (split view)
- [ ] ADR 编号自动递增: `ADR-0001`, `ADR-0002`...

---

## Task 15: ADR Storage (ADR 持久化)

**描述:** 实现 ADR 的持久化存储，支持两种目标: (1) Obsidian vault -- 写入配置的目录 (如 `Decisions/`)，利用 Obsidian 的文件管理和全文搜索; (2) Wiki 目录 -- 写入 `~/.naozhi/wiki/decisions/`，与知识编译系统集成。用户可配置使用哪种模式或两者同时写入。存储时维护 ADR 索引文件 (`decisions-index.json`)，记录全部 ADR 的元数据供列表查询。

**Files:**
- Create: `internal/decision/store.go`
- Modify: `internal/decision/renderer.go` (增加 write-to-disk 方法)
- Modify: `internal/knowledge/wiki.go` (Wiki 目录新增 `decisions/` 子目录支持)
- Modify: `internal/config/config.go` (新增 `decision.storage` 配置)

**Estimated LOC:** ~400

**Dependencies:**
- **Task 14:** `internal/decision/renderer.go` (ADR 渲染)
- **Phase 2:** `internal/knowledge/wiki.go` (Wiki 目录管理)
- **Phase 2:** `internal/knowledge/vault.go` (Obsidian vault 文件写入)

**Acceptance Criteria:**
- [ ] 配置 `decision.storage.mode: "obsidian"` / `"wiki"` / `"both"`
- [ ] `decision.storage.obsidian_folder: "Decisions"` (vault 内子目录)
- [ ] `decision.storage.wiki_folder: "decisions"` (wiki 内子目录)
- [ ] ADR 文件命名: `ADR-{number:04d}-{title-slug}.md` (如 `ADR-0003-use-cdk-over-terraform.md`)
- [ ] `decisions-index.json` 包含所有 ADR 元数据: id, number, title, status, date, source_session, file_path
- [ ] Confirmed ADR 写入磁盘时同步更新 bleve 搜索索引 (source type: `decision`)
- [ ] Obsidian 模式下 ADR 包含 `[[wikilinks]]` 到相关 wiki 页面
- [ ] Wiki 模式下 ADR 自动出现在 Wiki 编译页列表 (Decisions 分组)
- [ ] `DELETE /api/decisions/{id}` 删除 ADR 时: 从索引移除 + 从磁盘删除 + 从 bleve 移除
- [ ] Confirm 操作触发存储写入，candidate 状态不写磁盘 (仅存 `decisions-index.json`)
- [ ] 备份机制: 覆盖已有 ADR 文件前创建 `.bak` 副本

---

## E. Session Replay & Sharing (会话回放与分享)

Session Replay 将已完成的 Agent 会话转化为可时间线回放的交互体验，让用户回顾 AI 的思考过程和工具调用序列。Share 功能生成带 token 认证的只读链接，允许将有价值的会话分享给团队成员。

---

## Task 16: Event Timeline Reconstruction (事件时间线重建)

**描述:** 从 session 事件日志中重建完整的事件时间线。现有 WebSocket hub 记录的事件流包含 `message`, `tool_use`, `tool_result`, `thinking`, `cost_update` 等类型，每条带 timestamp。本 task 将这些事件解析为有序的 `TimelineEvent` 序列，标注每个事件的起止时间、持续时长、层级关系 (如 tool_use 包含其 tool_result 子事件)。重建的时间线是 Replay 播放器和 Share 功能的数据基础。

**Files:**
- Create: `internal/replay/timeline.go`
- Create: `internal/replay/timeline_test.go`

**Estimated LOC:** ~450

**Dependencies:**
- **Phase 1:** `internal/server/hub.go` (WebSocket 事件格式, `Event` 结构体)
- **Phase 1:** Dashboard WebSocket 事件历史 (`subscribe` with `after` timestamp)

**Acceptance Criteria:**
- [ ] `TimelineEvent` 结构体: `ID`, `Type` (message/tool_use/tool_result/thinking/cost), `StartTime`, `EndTime`, `Duration`, `Content` (text/JSON), `ParentID` (层级关系), `Metadata` (tool name, file path 等)
- [ ] `ReconstructTimeline(sessionKey string) ([]TimelineEvent, error)` 从 session 事件日志读取并排序
- [ ] 事件层级关系: `tool_use` 事件的子事件为对应的 `tool_result`，通过 event ID 匹配
- [ ] `thinking` 事件的 content 标记为 `redactable: true` (分享时可选隐藏)
- [ ] 时间线统计: `TotalDuration`, `ThinkingTime`, `ToolCallCount`, `TotalCost`
- [ ] 支持从 WebSocket 事件历史重建 (在线 session) 和从 Claude JSONL 文件重建 (离线 CLI session)
- [ ] 单元测试覆盖: 空 session、单条消息、多轮对话含 tool calls、并发 sub-agent 事件、事件乱序修正
- [ ] 大 session (>1000 events) 重建时间 < 100ms

---

## Task 17: Replay Viewer (回放播放器)

**描述:** 在 Dashboard 中实现 session 回放播放器 UI。播放器包含时间轴控制条 (scrub bar)、播放/暂停按钮、倍速控制 (1x/2x/4x)、当前时间指示器。主区域按时间顺序展示事件: 用户消息气泡、AI 文本 (打字机效果)、工具调用卡片 (折叠态 -> 展开态动画)、thinking 区域 (可折叠灰色块)。用户可以拖拽时间轴跳转到任意时间点，查看该时刻的对话状态。

**Files:**
- Create: `internal/replay/viewer.go` (Replay API handlers)
- Modify: `internal/server/server.go` (注册 `/api/replay/*` 路由)
- Modify: `internal/server/static/dashboard.html` (Replay 播放器完整 UI)

**Estimated LOC:** ~900 (Go ~200 + JavaScript ~500 + CSS ~200)

**Dependencies:**
- **Task 16:** `internal/replay/timeline.go` (TimelineEvent 序列)
- **Phase 1:** Dashboard 对话渲染组件 (代码块、工具卡片、Diff 渲染)

**Acceptance Criteria:**
- [ ] `GET /api/replay/{session-key}` 返回完整时间线 + 统计信息
- [ ] 时间轴 scrub bar: 底部固定，显示事件密度热力图 (颜色深浅表示该时段事件数量)
- [ ] 播放模式: Play 按钮启动自动播放，按时间间隔依次渲染事件
- [ ] 倍速控制: 1x (实时), 2x, 4x, 8x，按钮组切换
- [ ] AI 文本: 打字机效果 (逐字符渲染，速度 = 50 chars/s * 倍速)
- [ ] 工具调用: 折叠态出现 (200ms) -> 等待 -> 展开态显示结果 (300ms transition)
- [ ] thinking 区域: 灰色背景 collapsible block，默认折叠，可展开查看思考内容
- [ ] 拖拽跳转: 拖拽 scrub bar 到任意位置，立即渲染该时间点前的所有事件 (跳过动画)
- [ ] 当前播放位置指示: scrub bar 上蓝色圆点 + 时间显示 `03:24 / 12:45`
- [ ] session 不存在返回 404，事件为空返回空时间线 (提示 "No events to replay")
- [ ] Chat tab session 列表每条添加 "Replay" 按钮，点击进入 Replay 视图

---

## Task 18: Share Link Generation (分享链接生成)

**描述:** 为 session replay 生成可分享的只读链接。分享链接包含随机 token (32 字符 hex)，访问者无需 Dashboard 登录认证即可查看 replay (但受 token 有效期和只读限制)。分享时可选择是否包含 thinking 内容 (默认隐藏)、是否包含 cost 信息、链接有效期 (默认 7 天)。分享链接的 token 存储在 `~/.naozhi/shares.json` 中，过期自动清理。

**Files:**
- Create: `internal/replay/share.go`
- Modify: `internal/replay/viewer.go` (增加 share token 验证 middleware)
- Modify: `internal/server/server.go` (注册 `/share/{token}` 公开路由)
- Modify: `internal/server/static/dashboard.html` (Share 按钮 + 配置 modal + 分享页面模板)

**Estimated LOC:** ~500

**Dependencies:**
- **Task 17:** `internal/replay/viewer.go` (Replay 播放器)
- **Phase 1:** `internal/server/server.go` (认证 middleware 模式)

**Acceptance Criteria:**
- [ ] `POST /api/replay/{session-key}/share` 创建分享链接，body: `{"include_thinking": false, "include_cost": false, "expires_in": "7d"}`
- [ ] 返回 `{"share_url": "https://naozhi.example.com/share/{token}", "token": "abc...", "expires_at": "2026-04-22T..."}`
- [ ] Token 生成: `crypto/rand` 32 字节 hex 编码 (64 字符)
- [ ] `GET /share/{token}` 路由: 验证 token -> 检查过期 -> 加载 timeline -> 渲染只读 replay 页面
- [ ] 只读页面: 完整 replay 播放器但无 Send/Edit 功能，顶部显示 "Shared Session - Read Only" 横幅
- [ ] `include_thinking: false` 时，timeline 中 thinking 事件的 content 替换为 "[Thinking hidden]"
- [ ] 过期 token: 返回 410 Gone 页面，提示 "This shared session has expired"
- [ ] `GET /api/replay/{session-key}/shares` 列出该 session 的所有分享链接 (含状态)
- [ ] `DELETE /api/replay/{session-key}/shares/{token}` 撤销分享链接
- [ ] 后台每小时清理过期 share 记录，防止 `shares.json` 无限增长
- [ ] 存储: `~/.naozhi/shares.json`，格式 `[{"token": "...", "session_key": "...", "options": {...}, "created_at": "...", "expires_at": "..."}]`

---

## F. CTO Digital Twin (CTO 数字分身)

CTO Digital Twin 是 Naozhi V2 的最终形态 -- 一个基于 CTO 知识库训练的 AI 代理，能够代替 CTO 回答团队成员的技术问题。Digital Twin 不是简单的 RAG 检索，而是将 CTO 的 Wiki 知识、最近决策、偏好风格编译进 system prompt，让 AI 以 CTO 的视角和判断力回答问题。对于超出知识库覆盖范围或高不确定性的问题，系统自动标记 "需要 Keith 确认" 并上报。

---

## Task 19: Knowledge-Driven Prompt Assembly (知识驱动 Prompt 组装)

**描述:** 实现 CTO Digital Twin 的核心能力: 动态组装 system prompt。从 Wiki 编译知识库中检索与当前问题最相关的页面，结合最近的 ADR 决策记录和 CTO 的回答风格偏好，构建一个高度个性化的 system prompt。Prompt 结构: (1) 角色定义 ("你是 Keith 的 AI 代理..."), (2) 知识上下文 (相关 wiki 页面摘要), (3) 最近决策 (近 7 天的 confirmed ADR), (4) 回答风格指南 (从历史对话中提取的沟通特征)。

**Files:**
- Create: `internal/twin/assembler.go`
- Create: `internal/twin/assembler_test.go`
- Modify: `internal/config/config.go` (新增 `twin` 配置段)

**Estimated LOC:** ~500

**Dependencies:**
- **Phase 2:** `internal/knowledge/wiki.go` (Wiki 页面读取)
- **Phase 2:** `internal/knowledge/search.go` (bleve 搜索: 按相关性检索 wiki 页面)
- **Task 13-15:** `internal/decision/store.go` (ADR 索引查询)

**Acceptance Criteria:**
- [ ] `AssemblePrompt(question string) (string, error)` 根据问题动态生成 system prompt
- [ ] Prompt 结构分 4 段: `[Role]` + `[Knowledge Context]` + `[Recent Decisions]` + `[Response Style]`
- [ ] `[Knowledge Context]`: 用问题关键词查询 bleve 索引，取 top 5 wiki 页面摘要 (每页 < 500 tokens)
- [ ] `[Recent Decisions]`: 最近 7 天的 confirmed ADR 标题 + 决策摘要
- [ ] `[Response Style]`: 从配置文件读取 CTO 风格描述 (如 "回答简洁直接, 先给结论再解释, 偏好具体数据而非模糊描述")
- [ ] 总 prompt 长度控制: 若超过 `twin.max_prompt_tokens` (默认 8000 tokens)，按优先级截断 (先截 wiki, 再截 ADR, 不截 Role 和 Style)
- [ ] 配置 `twin.enabled: true`, `twin.name: "Keith"`, `twin.role: "AWS Solutions Architect"`, `twin.style: "..."`, `twin.max_prompt_tokens: 8000`
- [ ] 单元测试: 无相关 wiki 时返回基础 prompt (仅 Role + Style)、wiki 内容超长时正确截断、ADR 为空时跳过该段

---

## Task 20: Team Delegation Routing (团队提问委派路由)

**描述:** 实现 IM 平台 (飞书/Slack) 的团队提问自动处理链路。当团队成员在群聊中 @naozhi 提问时，系统先用 Task 19 组装的知识 prompt 尝试回答; 如果 Twin 有足够置信度，直接在群聊中回复 (标注 "[AI 代答]"); 如果置信度不足，标记问题为 "pending review" 并通知 CTO 审阅。CTO 可以在 Dashboard 或 IM 中审阅 AI 的草稿回答，修改后发送，或直接亲自回复。

**Files:**
- Create: `internal/twin/router.go`
- Modify: `internal/server/server.go` (注册 `/api/twin/*` 管理路由)
- Modify: `internal/server/static/dashboard.html` (Twin 审阅队列 UI)

**Estimated LOC:** ~550

**Dependencies:**
- **Task 19:** `internal/twin/assembler.go` (Prompt 组装)
- **Task 21:** `internal/twin/confidence.go` (置信度评分) -- 注: Task 20 和 21 可并行开发，初期 Task 20 使用简化的置信度判断
- **Phase 3:** `internal/patrol/notification.go` (通知推送)
- **Phase 1:** `internal/platform/*.go` (IM 平台适配器)

**Acceptance Criteria:**
- [ ] `TwinRouter` 拦截配置的群聊中 @naozhi 消息，先走 Twin 链路再 fallback 到常规 Agent
- [ ] 配置 `twin.delegate_chats: ["oc_xxxx", "#team-tech"]` 指定启用 Twin 的群聊列表
- [ ] Twin 回复流程: 收到问题 -> AssemblePrompt -> session.Send (Twin agent) -> 评估置信度 -> 回复或上报
- [ ] 高置信度回复格式: `[AI 代答] {answer}\n\n---\n基于 Keith 的知识库回答。如需确认请 @Keith`
- [ ] 低置信度上报: 推送到 Dashboard Twin Review Queue + IM 通知 CTO
- [ ] `GET /api/twin/queue` 返回待审阅问题列表 (question, draft_answer, confidence, source_chat, timestamp)
- [ ] `POST /api/twin/queue/{id}/send` CTO 审阅后发送回复 (可编辑 draft)
- [ ] `POST /api/twin/queue/{id}/dismiss` 驳回问题 (不回复)
- [ ] Dashboard Twin Review Queue: 卡片列表，每卡包含原始问题、AI 草稿回答 (可编辑)、置信度条、来源群聊、Send/Edit/Dismiss 按钮
- [ ] Twin session 独立于常规 Agent session，key 格式: `twin:{platform}:{chatID}:{questionID}`
- [ ] 配置 `twin.auto_reply_threshold: 0.8` -- 置信度 >= 0.8 自动回复，< 0.8 进入审阅队列

---

## Task 21: Confidence Scoring (置信度评分)

**描述:** 实现 Twin 回答的置信度评分系统。评分基于三个维度: (1) Knowledge Coverage -- Wiki 中是否有直接相关内容 (bleve 搜索 score), (2) Recency -- 相关知识的最后更新时间 (越近越高), (3) Specificity -- 问题的具体程度 vs Wiki 覆盖的粒度。三个维度加权平均得到最终 confidence score (0-1)。超过阈值自动回复，低于阈值进入审阅队列，极低置信度直接回复 "这个问题需要 Keith 亲自回答"。

**Files:**
- Create: `internal/twin/confidence.go`
- Modify: `internal/twin/router.go` (集成置信度评分到回复决策)
- Modify: `internal/server/static/dashboard.html` (Twin 审阅卡片增加置信度可视化)

**Estimated LOC:** ~400

**Dependencies:**
- **Task 19:** `internal/twin/assembler.go` (Wiki 检索结果用于 Knowledge Coverage 评分)
- **Phase 2:** `internal/knowledge/search.go` (bleve 搜索分数)
- **Task 13-15:** `internal/decision/store.go` (ADR 时间戳用于 Recency 评分)

**Acceptance Criteria:**
- [ ] `ScoreConfidence(question string, wikiResults []SearchResult, recentADRs []ADR) float64` 返回 0-1 置信度
- [ ] Knowledge Coverage (权重 0.5): `min(1.0, topScore * 1.2)` -- bleve top result score 标准化
- [ ] Recency (权重 0.3): 相关 wiki 页面最后编译时间距今的衰减函数 `exp(-days/30)` (30 天半衰期)
- [ ] Specificity (权重 0.2): 问题长度 + 是否包含具体实体 (AWS service name, 项目名等) -> 具体问题得高分
- [ ] 三级决策: `>= auto_reply_threshold` (默认 0.8) 自动回复, `>= review_threshold` (默认 0.3) 进入审阅, `< review_threshold` 直接回复 "需要 Keith 确认"
- [ ] Dashboard 置信度可视化: 三色条形图 (绿 > 0.8 / 黄 0.3-0.8 / 红 < 0.3) + 三个维度的分数明细
- [ ] 配置 `twin.confidence.auto_reply_threshold: 0.8`, `twin.confidence.review_threshold: 0.3`, `twin.confidence.weights: {coverage: 0.5, recency: 0.3, specificity: 0.2}`
- [ ] 无 wiki 内容时 Knowledge Coverage = 0，总分自动低于阈值，进入审阅
- [ ] 单元测试: 完美匹配 wiki 问题 (score ~0.9+)、无匹配问题 (score ~0.1)、过期知识 (recency 衰减)、模糊问题 (specificity 低)

---

## 工作量统计

| 模块 | Tasks | 新增 LOC | 修改 LOC | 总估算 | 建议工期 |
|------|-------|---------|---------|--------|---------|
| A. Knowledge Graph | 1-5 | ~2,200 | ~400 | ~2,600 | 1.5 周 |
| B. Observability Bridge | 6-8 | ~1,050 | ~400 | ~1,450 | 1 周 |
| C. Meeting Intelligence | 9-12 | ~1,600 | ~400 | ~2,000 | 1.5 周 |
| D. Decision Journal | 13-15 | ~1,050 | ~400 | ~1,450 | 1 周 |
| E. Session Replay | 16-18 | ~1,450 | ~400 | ~1,850 | 1.5 周 |
| F. CTO Digital Twin | 19-21 | ~1,050 | ~400 | ~1,450 | 1 周 |
| **Total** | **21** | **~8,400** | **~2,400** | **~10,800** | **7.5 周** |

---

## 并行执行建议

以下模块之间无代码依赖，可以并行开发:

```
Week 1-2:  A. Knowledge Graph (Tasks 1-5)  ||  B. Observability Bridge (Tasks 6-8)
Week 3-4:  C. Meeting Intelligence (Tasks 9-12)  ||  D. Decision Journal (Tasks 13-15)
Week 5-6:  E. Session Replay (Tasks 16-18)  ||  F. CTO Digital Twin (Tasks 19-21)
Week 7:    集成测试 + 文档 + Bug 修复
```

每个模块内部的 Tasks 应按序执行 (数据模型 -> API -> 前端 -> 交互)。

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| d3-force CDN 不可用 | Graph 无法渲染 | 在 embed.FS 中内嵌 d3-force minified (32KB) 作为 fallback |
| Amazon Transcribe 批量作业延迟高 | 会议处理超时 | 设置合理的 max_duration + 前端 progress bar + 后台异步通知 |
| LLM 会议分析输出格式不稳定 | JSON 解析失败 | 多次 retry + fallback 到纯文本摘要 + JSON schema 验证 |
| ADR 提取误报率高 | 用户信任下降 | 设置 confidence 阈值 + 候选状态需人工确认 + 提供 reject 操作 |
| Twin 自动回复错误 | 团队获取错误信息 | 高置信度阈值 (0.8) + 所有回复标注 "[AI 代答]" + 审阅队列兜底 |
| dashboard.html 单文件过大 | 加载缓慢 | 监控文件大小，超过 300KB 时启动 Phase 2 前端架构迁移 (Preact + Vite) |
