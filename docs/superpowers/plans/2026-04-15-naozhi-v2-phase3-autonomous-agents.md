# Naozhi V2.0 Phase 3: Autonomous Agents 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标:** 将 Naozhi 从被动响应式 AI Gateway 升级为自主运行的 CTO Operating System。构建 Patrol 巡逻引擎（状态追踪 + JSONL 日志 + 多触发方式）、Approval Gateway（人机协作审批门控）、通知中心（WebSocket 推送 + 持久化）、以及 GitHub/Proactive Insights 集成，使 AI Agent 能自主执行任务并在关键节点请求人工审批。

**架构:** 新增 `internal/patrol` 包（核心巡逻引擎）和 `internal/approval` 包（审批网关），复用现有 `session.Router` + `cli.Wrapper` 执行链、`robfig/cron` 调度器、以及 `platform.Platform` 通知适配器。前端扩展 `dashboard.html` 新增 Patrols / Approvals / Notification Center 三大视图。

**Tech Stack:** Go (backend), vanilla JS (frontend), JSON/JSONL 文件持久化, WebSocket 实时推送

**Spec:** `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md` Section 5 + Section 6

**周期:** 4-6 weeks (20 tasks, 预估 ~6,800 LOC)

---

## 依赖关系图

```
Task 1 (Patrol struct)
  ├── Task 2 (Patrol store)        ─── Task 5 (Patrol REST API)
  ├── Task 3 (Patrol executor)     ─── Task 6 (Webhook receiver)
  │     └── Task 10 (Approval blocking)
  └── Task 4 (Patrol log system)

Task 7 (Approval model + store)
  ├── Task 8 (Approval REST API)
  ├── Task 9 (Approval detection)
  └── Task 10 (Approval blocking)

Task 11 (Notification model + store)
  ├── Task 12 (Notification WebSocket)
  ├── Task 13 (Notification REST API)
  └── Task 14 (IM notification routing)

Task 5, 6           ─── Task 15 (GitHub webhook)
Task 3, 11          ─── Task 16 (Proactive Insights)

Task 5              ─── Task 17 (Patrols frontend)
Task 8              ─── Task 18 (Approvals frontend)
Task 12, 13         ─── Task 19 (Notification Center UI)
Task 17, 18, 19     ─── Task 20 (Home dashboard integration)
```

---

## Backend — Patrol Package

### Task 1: Patrol 数据结构与生命周期

**描述:**

定义 patrol 核心数据结构和状态机。`Patrol` struct 承载配置（schedule/trigger/agent/prompt）和运行时状态（state/last_run/stats）。状态机管理四个状态: `Active` → `Running` → `Active`（正常循环），`Active/Running` → `Paused`（用户暂停），`Any` → `Disabled`（完全禁用）。

这是整个 Patrol 系统的基石——所有后续 task 都依赖此结构体定义。设计模式参考现有 `internal/cron/job.go` 的 `Job` struct，但增加了完整的生命周期状态追踪（cron `Job` 仅有 `Paused` bool，而 `Patrol` 需要 `Active/Paused/Disabled/Running` 四态）和统计聚合字段。

**Files:**
- Create: `internal/patrol/patrol.go`

**Estimated LOC:** 150

**Dependencies:** None

**Acceptance Criteria:**
- [ ] `State` 类型定义四个常量: `StateActive`, `StatePaused`, `StateDisabled`, `StateRunning`
- [ ] `Patrol` struct 包含配置字段（Name, Schedule, Trigger, Agent, Model, Prompt, Notify, ApprovalRequired, AutoFix, Timeout, Budget, MCPServers, WorkDir）和运行时字段（State, LastRun, TotalRuns, TotalErrors, TotalCost, CreatedAt）
- [ ] `RunLog` struct 记录单次执行结果: ID (16-char hex), Timestamp, Duration, Cost, Status (ok/warn/error), Summary, Detail, Error, EventData
- [ ] `RunStatus` 类型定义 `RunOK`, `RunWarn`, `RunError` 三个常量
- [ ] `generateID()` 函数复用 `cron/job.go` 中 `crypto/rand` 模式生成 16-char hex ID
- [ ] `Patrol.ValidateTransition(newState)` 方法实现状态转换合法性校验（例如 Disabled 不能直接到 Running）
- [ ] `cronParser` 变量复用 `cron/job.go` 中相同的 `robfigcron.NewParser` 配置，确保 schedule 表达式兼容
- [ ] `go vet ./internal/patrol/...` 通过

**Implementation Notes:**

`Patrol` 的 JSON tag 同时标注 `json` 和 `yaml`，因为配置从 `config.yaml` 读入（yaml tag），运行时状态序列化到 `patrols.json`（json tag）。这与 `cron.Job` 仅用 json tag 不同，因为 cron job 是 API 创建的动态对象，而 patrol 是 config 声明式定义的静态对象。

```go
// State 转换规则参考:
//   Active  → Running  (scheduler/webhook/manual 触发)
//   Running → Active   (执行完成)
//   Active  → Paused   (用户暂停)
//   Running → Paused   (暂停请求，等执行完成后生效)
//   Paused  → Active   (用户恢复)
//   Any     → Disabled (用户禁用)
```

---

### Task 2: Patrol Store — JSON 持久化

**描述:**

实现 patrol 运行时状态的 JSON 持久化层。配置信息从 `config.yaml` 加载（只读），运行时状态（state, last_run, total_runs 等）持久化到 `~/.naozhi/patrols.json`。采用与 `internal/cron/store.go` 完全相同的原子写入模式: 写入 `.tmp` 文件后 `os.Rename`，防止 crash 导致文件损坏。

Store 需要处理 config 与 runtime state 的合并逻辑: 每次启动时从 config 读取 patrol 定义，然后从 `patrols.json` 恢复 state/stats。如果 config 中删除了某个 patrol，对应的 runtime state 也应清理。如果 config 中新增了 patrol，初始状态为 `Active`。

**Files:**
- Create: `internal/patrol/store.go`

**Estimated LOC:** 130

**Dependencies:** Task 1

**Acceptance Criteria:**
- [ ] `savePatrols(path string, patrols map[string]*Patrol) error` 函数实现原子写入: `json.MarshalIndent` → 写入 `.tmp` → `os.Rename`
- [ ] `loadPatrols(path string) map[string]*Patrol` 函数从 JSON 文件加载，缺失或损坏时返回 nil 并 `slog.Warn`
- [ ] `mergeConfig(configPatrols map[string]*Patrol, stored map[string]*Patrol) map[string]*Patrol` 函数合并 config 定义与持久化状态: config 中新增的 patrol 初始 state 为 Active，config 中删除的 patrol 从结果中移除，已有的 patrol 保留 runtime 字段（State, LastRun, TotalRuns 等）
- [ ] 目录自动创建: `os.MkdirAll(filepath.Dir(path), 0700)`
- [ ] `go test ./internal/patrol/...` 包含 store 读写 + merge 逻辑的单元测试
- [ ] 文件权限 0600（与 cron store 一致）

**Implementation Notes:**

模式完全照搬 `internal/cron/store.go`。关键区别: cron store 只存 `[]*Job` 数组，patrol store 需要存 `map[string]*PatrolState`（只持久化运行时字段，不重复存储 config 字段）。这样 config.yaml 修改 prompt/schedule 后，下次启动自动生效，不会被 store 中的旧值覆盖。

```go
// PatrolState 仅持久化运行时字段，与 config 分离
type PatrolState struct {
    State       State     `json:"state"`
    LastRun     *RunLog   `json:"last_run,omitempty"`
    TotalRuns   int64     `json:"total_runs"`
    TotalErrors int64     `json:"total_errors"`
    TotalCost   float64   `json:"total_cost"`
    CreatedAt   time.Time `json:"created_at"`
}
```

---

### Task 3: Patrol Executor — 复用 Session Router + CLI Process

**描述:**

实现 patrol 的核心执行引擎。Patrol 执行复用现有 `session.Router.GetOrCreate()` + `ManagedSession.Send()` 机制——这是 Naozhi 最关键的设计决策: 不新建 CLI 管理层，而是把 patrol 视为一种特殊的 session（key 格式 `patrol:{name}`，exempt 标记为 true 不占用 maxProcs）。

执行流程: 构造 session key → 查找 agent config → 设置 AgentOpts（model 覆盖, MCP server, workspace）→ `router.GetOrCreate()` → `sess.Send()` → 解析结果 → 写入 RunLog → 通知路由。

这个 task 同时实现 `Manager` 结构体——整个 patrol 子系统的入口和协调者。

**Files:**
- Create: `internal/patrol/executor.go`
- Create: `internal/patrol/manager.go`

**Estimated LOC:** 350

**Dependencies:** Task 1, Task 2

**Acceptance Criteria:**
- [ ] `Manager` struct 包含: `patrols map[string]*Patrol`, `mu sync.RWMutex`, `router *session.Router`, `agents map[string]session.AgentOpts`, `platforms map[string]platform.Platform`, `hub` interface (WebSocket 推送), `logDir string`, `cron *robfigcron.Cron`, `storePath string`
- [ ] `ManagerConfig` struct 与 `cron.SchedulerConfig` 对齐: Router, Platforms, Agents, StorePath, LogDir
- [ ] `NewManager(cfg ManagerConfig) *Manager` 创建 manager，加载 config + 恢复 store，注册 schedule trigger 到 cron 调度器
- [ ] `Manager.Start()` 启动 cron 调度器，开始调度 Active 状态且有 schedule 的 patrol
- [ ] `Manager.Stop()` 停止调度器，保存 store（与 `cron.Scheduler.Stop()` 模式一致）
- [ ] `Manager.Execute(ctx, name, eventPayload)` 完整执行流: state 校验 → 标记 Running → 构造 prompt → `router.GetOrCreate` → `sess.Send` → 解析结果 → 写入 log → 通知 → 恢复 Active
- [ ] session key 格式: `patrol:{name}`，opts.Exempt = true（不计入 maxProcs，参考 `scheduler.execute()` 中 `opts.Exempt = true`）
- [ ] 支持 event prompt 构造: `buildEventPrompt(p, payload)` 将 webhook payload 注入 prompt 前缀
- [ ] 超时控制: 从 patrol config 解析 timeout 或使用默认 5 分钟，通过 `context.WithTimeout` 包装
- [ ] 结果分类: `classifyResult(text)` 通过关键词匹配判断 ok/warn/error（包含 "error"/"fail"/"critical" → error，包含 "warn"/"alert"/"异常" → warn，其他 → ok）
- [ ] 摘要提取: `extractSummary(text)` 截取结果前 200 字符作为 summary
- [ ] setState 方法更新状态后自动触发 store save
- [ ] `go vet ./internal/patrol/...` 通过

**Implementation Notes:**

`Manager.Execute()` 的核心逻辑直接对标 `cron.Scheduler.execute()`，但增加了:
1. 状态机管理（cron 只有 paused 检查）
2. event payload 注入（cron 没有 webhook 触发）
3. RunLog 写入（cron 只覆盖式存储 last_result）
4. 多目标通知（cron 只发送到创建者 chat）

注意 `sess.Send()` 是直接调用而非 `hub.sendWithBroadcast()`——这与 `cron.Scheduler.execute()` 中的注释一致: "Direct Send without sendWithBroadcast — cron jobs notify via onExecute callback instead." Patrol 也采用相同策略，通过自己的通知路由推送结果。

```go
// 参考 cron/scheduler.go 第 426-470 行的 execute() 函数
// patrol 执行流完全平行，区别在于 patrol 有状态机 + 日志 + 多目标通知
func (m *Manager) Execute(ctx context.Context, name string, eventPayload json.RawMessage) error {
    // ... 状态校验、标记 Running、构造 prompt、router.GetOrCreate、sess.Send ...
    // 关键: opts.Exempt = true，与 cron execute 第 435 行一致
    key := "patrol:" + name
    opts := m.agents[p.Agent]
    opts.Exempt = true
}
```

---

### Task 4: Patrol Log System — JSONL 持久化

**描述:**

实现每个 patrol 独立的 JSONL 日志系统。每个 patrol 的执行日志写入 `~/.naozhi/patrols/{patrol-name}/logs.jsonl`，每行一条 `RunLog` JSON 记录，按时间顺序追加写入。提供从文件尾部倒序读取的能力（Dashboard 展示最新日志）。

日志文件采用简单的大小限制轮转策略: 单文件超过 10MB 后归档为 `logs.{timestamp}.jsonl.gz`，保留最近 30 天归档。这比 cron 的覆盖式 `last_result` 存储提供了完整的执行历史追溯。

**Files:**
- Create: `internal/patrol/logwriter.go`

**Estimated LOC:** 200

**Dependencies:** Task 1

**Acceptance Criteria:**
- [ ] `LogWriter` struct 包含: `dir string`, `mu sync.Mutex`, `file *os.File`, `size int64`, `maxSize int64` (默认 10MB)
- [ ] `NewLogWriter(dir string) (*LogWriter, error)` 创建 writer，自动 `os.MkdirAll`，打开或创建 `logs.jsonl`
- [ ] `LogWriter.Append(log *RunLog) error` 追加写入: `json.Marshal` → 追加 `\n` → 大小检查 → 写入文件。超过 maxSize 时先调用 `rotate()`
- [ ] `LogWriter.rotate() error` 轮转: 关闭当前文件 → 重命名为 `logs.{unix_timestamp}.jsonl` → gzip 压缩 → 创建新 `logs.jsonl`
- [ ] `LogWriter.ReadTail(n int) ([]*RunLog, error)` 从文件末尾读取最近 n 条日志。实现方式: 从文件末尾向前扫描换行符，读取最后 n 行，逆序解析为 `[]*RunLog`
- [ ] `LogWriter.ReadPage(offset, limit int) ([]*RunLog, int, error)` 分页读取: 从文件头部读取，返回日志切片和总条数
- [ ] `LogWriter.Close() error` 关闭文件
- [ ] `cleanOldArchives(dir string, maxAge time.Duration)` 清理超过 maxAge 的归档文件
- [ ] 单元测试覆盖: 追加写入、尾部读取、轮转、归档清理

**Implementation Notes:**

`ReadTail` 的实现可以用简单的缓冲倒读策略: 从文件末尾每次读取 4KB 块，扫描 `\n` 计数行数。对于 10MB 以内的 JSONL 文件（每条 ~500 bytes，约 2 万条），性能完全够用。

如果后续需要更高效的读取，可以维护一个行偏移索引文件（`logs.idx`），但 Phase 3 不需要这个优化。

---

### Task 5: Patrol REST API — CRUD + Trigger + Pause/Resume + Logs

**描述:**

实现 patrol 的 HTTP REST API 端点，供 Dashboard 前端和外部系统调用。API 设计遵循 Naozhi 现有 REST 风格（参考 `/api/cron` 系列端点），使用 `net/http` 标准库路由。

所有端点注册到现有 `server.Server` 的 mux 上（参考 CLAUDE.md 中 `/api/cron (GET/POST/DELETE), /api/cron/pause, /api/cron/resume` 的注册方式）。认证复用现有 Dashboard 的 token 校验机制。

**Files:**
- Create: `internal/patrol/api.go`
- Modify: `internal/server/server.go` (注册 patrol API 路由)

**Estimated LOC:** 300

**Dependencies:** Task 1, Task 2, Task 3, Task 4

**Acceptance Criteria:**
- [ ] `GET /api/patrols` — 列出所有 patrol，包含状态、统计信息、下次执行时间（如有 schedule）。响应格式: `{"patrols": [...], "stats": {"active": N, "paused": N, "disabled": N}}`
- [ ] `GET /api/patrols/{name}` — 单个 patrol 详情，包含完整 config + runtime state
- [ ] `PUT /api/patrols/{name}/state` — 修改状态: body `{"state": "paused"|"active"|"disabled"}`。状态转换校验由 `Patrol.ValidateTransition()` 执行
- [ ] `POST /api/patrols/{name}/trigger` — 手动触发执行: body `{"context": "optional extra context"}`。异步执行，立即返回 `202 Accepted` + `{"run_id": "..."}`
- [ ] `GET /api/patrols/{name}/logs` — 执行日志列表: query `?limit=20&offset=0`。返回 `{"logs": [...], "total": N}`
- [ ] `GET /api/patrols/{name}/logs/{id}` — 单条日志详情（包含完整 detail 字段）
- [ ] `APIHandler` struct 持有 `*Manager` 引用，通过 `RegisterRoutes(mux *http.ServeMux)` 注册所有路由
- [ ] 错误响应统一格式: `{"error": "message"}`，HTTP status code 语义正确（400 参数错误, 404 未找到, 409 状态冲突, 500 内部错误）
- [ ] 在 `server.go` 中注册路由，位于现有 cron API 注册代码附近
- [ ] `go vet ./internal/patrol/...` 通过

**Implementation Notes:**

路由注册方式参考 `server.go` 中现有 cron API 的注册模式。Naozhi 使用标准库 `http.ServeMux`，通过 `mux.HandleFunc` 注册。Go 1.22+ 的 ServeMux 支持 `GET /api/patrols/{name}` 风格的路径参数（通过 `r.PathValue("name")`），如果当前版本不支持，则使用 URL path 手动解析。

trigger 端点的异步模式参考 `send.go` 中 `sessionSend()` 的 goroutine 模式: 立即返回 202，后台 goroutine 执行。

---

### Task 6: Webhook Receiver — 外部事件触发

**描述:**

实现 webhook 端点接收外部事件（GitHub, CloudWatch, 自定义），匹配 patrol 配置中的 `trigger` 字段并触发对应 patrol 执行。每个 patrol 有自己的 webhook URL: `POST /api/webhooks/{patrol-name}`。

这是 Patrol 从"被动定时"进化到"事件驱动"的核心能力。webhook 端点的设计要考虑安全性（防止未授权触发）和灵活性（不同来源的 payload 格式各异）。

**Files:**
- Create: `internal/patrol/webhook.go`
- Modify: `internal/patrol/api.go` (在 RegisterRoutes 中追加 webhook 路由)

**Estimated LOC:** 180

**Dependencies:** Task 1, Task 3, Task 5

**Acceptance Criteria:**
- [ ] `POST /api/webhooks/{patrol-name}` 端点接收外部事件
- [ ] 请求体解析为 `WebhookPayload` struct: `{"event": "pr_opened", "source": "github", "payload": {...}}`
- [ ] Patrol 查找与状态校验: 根据 URL path 中的 patrol name 查找配置，校验 state 为 Active
- [ ] Trigger 匹配逻辑: 如果 patrol 配置了 `trigger` 字段（格式 `{source}:{event_type}`），则验证 payload 的 source:event 与之匹配。支持通配符 `custom:*` 匹配任意 event_type。如果未配置 trigger（纯 schedule patrol），webhook 触发仍然生效（手动触发场景）
- [ ] Event prompt 构造: `buildEventPrompt(p, payload)` 将 webhook payload 序列化为 JSON 注入 prompt 前缀，格式 `[Event: {trigger}]\nPayload:\n```json\n{payload}\n```\n\n{original_prompt}`
- [ ] 异步执行: 验证通过后启动 goroutine 调用 `Manager.Execute()`，立即返回 `202 Accepted`
- [ ] GitHub webhook 特殊处理: 检测 `X-GitHub-Event` header，自动构造 source 为 "github"，event_type 为 header 值（e.g., "pull_request" → `github:pull_request`）
- [ ] 响应体: 成功返回 `{"status": "accepted", "patrol": "name", "run_id": "..."}`, 失败返回 `{"error": "..."}`
- [ ] 基础安全: 可选的 webhook secret 校验（config 中配置 `patrol_webhook_secret`），通过 HMAC-SHA256 验证（与 GitHub webhook secret 兼容）

**Implementation Notes:**

webhook 端点需要兼顾 Feishu 的 3s 响应限制要求（虽然 webhook 通常不走 Feishu，但保持异步模式是好习惯）。参考 CLAUDE.md: "Feishu requires 200 response within 3s. The webhook handler returns 200 immediately and processes asynchronously."

GitHub webhook 的 `X-GitHub-Event` header 值与 patrol trigger 的映射:
- `pull_request` + action `opened` → `github:pr_opened`
- `issues` + action `opened` → `github:issue_opened`
- `push` → `github:push`

---

## Backend — Approval System

### Task 7: Approval 数据模型 + Store

**描述:**

定义审批请求数据结构和 JSON 持久化层。审批系统是 Patrol 人机协作的核心: 当 agent 执行到高风险操作时，创建 `Request` 对象，暂停执行等待人工决策。

数据模型设计考虑审计追踪需求: 所有审批记录永久保留（completed 状态的记录 7 天后迁移到归档文件），每条记录包含完整的上下文（操作描述、影响评估、来源 patrol、时间线）。

**Files:**
- Create: `internal/approval/approval.go`
- Create: `internal/approval/store.go`

**Estimated LOC:** 200

**Dependencies:** None

**Acceptance Criteria:**
- [ ] `Status` 类型定义五个常量: `StatusPending`, `StatusApproved`, `StatusRejected`, `StatusExpired`, `StatusExecuted`
- [ ] `Urgency` 类型定义: `UrgencyNormal`, `UrgencyUrgent`
- [ ] `Request` struct 包含: ID ("appr-" + 16-char hex), Patrol, SessionKey, Agent, Action, Summary, Detail, Impact, Urgency, Level ("critical"/"high"), Status, CreatedAt, ExpiresAt (默认 +30min), ApprovedBy, ApprovedAt, RejectedBy, RejectedAt
- [ ] `saveApprovals(path, requests)` 原子写入（与 cron/store.go 模式一致）
- [ ] `loadApprovals(path)` 加载历史记录
- [ ] `archiveCompleted(path, archivePath, maxAge)` 将已完成且超过 maxAge 的记录迁移到 `approvals-archive.jsonl`（JSONL 追加写入）
- [ ] ID 生成格式: `"appr-" + generateID()`
- [ ] 单元测试覆盖 store CRUD + archive 逻辑

**Implementation Notes:**

与 patrol store 不同，approval store 是活跃数据: 需要频繁查询 pending 状态的记录。当前使用 in-memory map + JSON 文件持久化，与 cron store 模式一致。如果未来 pending 量很大（>1000），可以考虑切换到 SQLite，但 Phase 3 不需要。

归档策略: 7 天后迁移到 `~/.naozhi/approvals-archive.jsonl`（JSONL 追加，不是 JSON 数组）。归档文件不加载到内存，仅用于审计查询（如果需要的话，后续 task 实现）。

---

### Task 8: Approval REST API

**描述:**

实现审批请求的 HTTP REST API 端点。用户通过 Dashboard 或 API 查看 pending 审批、执行 approve/reject 操作。API 需要支持状态筛选（pending/approved/rejected）和统计数据（今日审批/拒绝计数）。

**Files:**
- Create: `internal/approval/api.go`
- Modify: `internal/server/server.go` (注册 approval API 路由)

**Estimated LOC:** 200

**Dependencies:** Task 7

**Acceptance Criteria:**
- [ ] `GET /api/approvals` — 列表: query `?status=pending&limit=20&offset=0`。响应包含 `approvals` 数组和 `stats` 对象 (`pending`, `approved_today`, `rejected_today`)
- [ ] `GET /api/approvals/{id}` — 单个审批详情（包含完整 detail 字段）
- [ ] `POST /api/approvals/{id}/approve` — 批准: body `{"approved_by": "keith"}`。校验状态必须为 pending，更新状态为 approved，通知 waiter channel
- [ ] `POST /api/approvals/{id}/reject` — 拒绝: body `{"rejected_by": "keith", "reason": "..."}`。校验状态必须为 pending，更新状态为 rejected，通知 waiter channel
- [ ] `GET /api/approvals/stats` — 统计: 返回 `{"pending": N, "approved_today": N, "rejected_today": N, "expired_today": N}`
- [ ] `ApprovalHandler` struct 持有 `*Manager` 引用，`RegisterRoutes(mux)` 注册路由
- [ ] approve/reject 操作触发 WebSocket broadcast（`approval_update` 事件）
- [ ] 已过期的 pending 请求返回 409 Conflict
- [ ] 在 `server.go` 中注册路由

**Implementation Notes:**

approve/reject 端点需要同时做两件事: (1) 更新持久化状态，(2) 通知 waiter channel 唤醒阻塞的 patrol 执行流。这个 channel 通知机制在 Task 10 实现，但 API 层需要预留接口调用。

---

### Task 9: Approval Detection — 关键字匹配 + Agent 自主标记

**描述:**

实现两层风险检测机制: (1) 关键字匹配——在 agent 输出流中实时检测高风险操作关键字（terraform apply, kubectl delete, DROP TABLE 等），(2) Agent 自主标记——检测 agent 输出中的 `[APPROVAL_NEEDED: ...]` 标记。

这是 Approval Gateway 的"感知层"——决定什么时候需要人工审批。关键字匹配是兜底机制（agent 不主动标记也能拦截），agent 标记是增强机制（agent 知道自己要做什么，主动请求审批）。

**Files:**
- Create: `internal/approval/detector.go`

**Estimated LOC:** 150

**Dependencies:** Task 7

**Acceptance Criteria:**
- [ ] `dangerousPatterns` 变量定义 8+ 条危险操作正则: `terraform\s+apply`, `git\s+push\s+--force`, `kubectl\s+delete`, `(?i)DROP\s+TABLE`, `(?i)DELETE\s+FROM`, `aws\s+\S+\s+terminate`, `aws\s+\S+\s+delete`, `rm\s+-rf\s+/`。每条包含 Pattern, Label, Level
- [ ] `DetectDanger(text string) (label, level string, found bool)` 函数遍历所有 pattern 匹配输入文本。首次匹配即返回（short-circuit）
- [ ] `DetectApprovalMarker(text string) (description string, found bool)` 函数检测 `[APPROVAL_NEEDED: ...]` 标记，提取方括号内的描述文本
- [ ] `NeedsApproval(text string, patrolConfig *Patrol) (label, level, description string, needed bool)` 综合判断函数: 先检查 approval marker，再检查 danger pattern，最后检查 patrol 的 `approval_required` 配置
- [ ] Level 分级: "critical" (terraform apply, DROP TABLE, rm -rf) 和 "high" (kubectl delete, DELETE FROM)
- [ ] 单元测试覆盖所有 pattern 的正向和负向匹配

**Implementation Notes:**

pattern 列表可配置化是 Phase 4 的优化方向。Phase 3 硬编码即可，因为高风险操作的种类是有限的。

`DetectApprovalMarker` 的正则: `\[APPROVAL_NEEDED:\s*(.+?)\]`

agent 自主标记通过 system prompt 注入实现。在 patrol 的 agent session 中追加 system prompt:
```
When you are about to execute a destructive or irreversible operation, output:
[APPROVAL_NEEDED: brief description of the action and its impact]
This will pause execution and wait for human approval.
```
这个 system prompt 注入在 Task 3 的 `Manager.Execute()` 中实现（通过 `opts.ExtraArgs` 追加 `--append-system-prompt`）。

---

### Task 10: Approval 阻塞/恢复机制 — Pause Patrol 等待审批

**描述:**

实现审批系统的核心协调逻辑: `Manager` 创建审批请求后，阻塞当前 patrol 执行 goroutine（通过 channel 等待），直到用户在 Dashboard/IM 做出 approve/reject 决策，或超时自动 reject。

这是最复杂的 task——需要协调三个并发流: (1) patrol 执行 goroutine (producer), (2) REST API / IM command handler (consumer), (3) 超时 goroutine (fallback)。

**Files:**
- Create: `internal/approval/manager.go`
- Modify: `internal/patrol/executor.go` (在 Execute 中集成审批检测与阻塞逻辑)

**Estimated LOC:** 300

**Dependencies:** Task 3, Task 7, Task 8, Task 9

**Acceptance Criteria:**
- [ ] `approval.Manager` struct 包含: `mu sync.RWMutex`, `requests map[string]*Request`, `waiters map[string]chan Status`, `storePath string`, `hub` broadcast interface, `platforms` map, `timeout time.Duration` (默认 30min)
- [ ] `Manager.CreateRequest(req *Request) error` — 生成 ID, 设置 pending 状态, 计算 ExpiresAt, 保存到 store, 创建 waiter channel, 推送通知到所有渠道, 启动超时 goroutine
- [ ] `Manager.WaitForDecision(ctx, id) (Status, error)` — 阻塞等待: `select { case status := <-ch: ... case <-ctx.Done(): ... }`。返回后清理 waiter channel
- [ ] `Manager.Approve(id, approvedBy) error` — 校验 pending 状态, 更新为 approved, 写入 channel 唤醒 waiter, 保存 store, 广播 WebSocket 事件
- [ ] `Manager.Reject(id, rejectedBy) error` — 类似 Approve, 状态设为 rejected
- [ ] `Manager.expireAfter(id, timeout)` — goroutine: sleep timeout 后检查状态，仍为 pending 则设为 expired 并写入 channel
- [ ] 在 `patrol/executor.go` 的 `Execute()` 中集成: `sess.Send()` 返回后调用 `NeedsApproval()` 检测，如需审批则 `CreateRequest()` + `WaitForDecision()`，根据结果发送 "Approved. Please proceed." 或 "Rejected. Please abort." 继续/中止执行
- [ ] 通知推送: `notifyAll(req)` 同时推送到 Dashboard Hub (WebSocket `approval_created` 事件) 和配置的 IM 平台
- [ ] waiter channel 使用 buffered channel (size 1) 防止 goroutine 泄漏（发送端不阻塞）
- [ ] 单元测试覆盖: approve 唤醒、reject 唤醒、超时过期、并发安全

**Implementation Notes:**

channel 协调模式的关键点:

1. waiter channel 必须是 `make(chan Status, 1)` — buffered size 1。如果 unbuffered，当 patrol goroutine 因为 context cancel 退出后，approve/reject 的 channel send 会永远阻塞（goroutine 泄漏）。

2. 超时 goroutine 必须先检查当前状态再写 channel。如果用户已经 approve/reject 了，超时 goroutine 不应该覆盖。

3. `WaitForDecision` 返回后应该清理 waiter map entry，防止内存泄漏。

patrol 执行流集成的关键代码路径:
```go
result, err := sess.Send(execCtx, prompt, nil, nil)
if err != nil { ... }

if label, level, desc, needed := NeedsApproval(result.Text, p); needed {
    req := &Request{
        Patrol: p.Name, SessionKey: key, Agent: p.Agent,
        Action: label, Summary: desc, Level: level,
    }
    m.approvals.CreateRequest(req)
    status, _ := m.approvals.WaitForDecision(execCtx, req.ID)
    if status == StatusApproved {
        sess.Send(execCtx, "Approved. Please proceed.", nil, nil)
    } else {
        sess.Send(execCtx, "Rejected. Please abort.", nil, nil)
    }
}
```

---

## Backend — Notifications

### Task 11: Notification 数据模型 + Store

**描述:**

定义通知中心的数据模型和 JSON 持久化层。通知系统是 patrol/approval 结果的汇聚展示层: 所有重要事件（patrol 告警、审批请求、wiki 编译完成、IM 代答等）统一收集，按时间倒序排列，支持已读/未读状态管理。

**Files:**
- Create: `internal/notification/notification.go`
- Create: `internal/notification/store.go`

**Estimated LOC:** 150

**Dependencies:** None

**Acceptance Criteria:**
- [ ] `Notification` struct 包含: ID (16-char hex), Type (patrol_alert/approval_request/wiki_compiled/im_answer/cost_report), Title, Summary, Urgency (urgent/normal), Read bool, SourceType (patrol/approval/wiki/im), SourceRef (URL/path), CreatedAt
- [ ] `Store` struct 包含: `mu sync.RWMutex`, `notifications []*Notification`, `maxItems int` (默认 500), `storePath string`
- [ ] `Store.Add(n *Notification)` — 追加通知，超过 maxItems 时移除最旧的已读通知
- [ ] `Store.List(limit, offset int, unreadOnly bool) ([]*Notification, int)` — 分页列表，支持仅未读筛选，返回总数
- [ ] `Store.MarkAllRead()` — 将所有通知标记为已读
- [ ] `Store.MarkRead(id string)` — 标记单条已读
- [ ] `Store.UnreadCount() int` — 返回未读通知数（Dashboard 铃铛 badge 用）
- [ ] `saveNotifications` / `loadNotifications` 原子写入/加载（与 cron/store.go 模式一致）
- [ ] 单元测试覆盖 Add + List + MarkRead + 容量淘汰

**Implementation Notes:**

通知存储选择 JSON 数组而非 JSONL，因为通知是有限集合（maxItems=500），需要频繁整体加载和更新已读状态。JSONL 适合追加写入的日志场景（patrol logs），不适合需要修改已有记录的通知场景。

容量淘汰策略: 优先移除最旧的已读通知。如果所有通知都未读（极端情况），则移除最旧的通知。

---

### Task 12: Notification WebSocket 推送

**描述:**

扩展现有 WebSocket Hub，新增通知相关的消息类型。当 patrol 执行完成、审批请求创建/处理、或其他重要事件发生时，通过 WebSocket 实时推送到所有已连接的 Dashboard 客户端。

这复用现有 `server.Hub` 的 `Broadcast()` 机制，只需要定义新的消息类型和在适当的触发点调用 broadcast。

**Files:**
- Modify: `internal/server/hub.go` (新增 broadcast 辅助方法)
- Modify: `internal/patrol/manager.go` (执行完成后推送 patrol_event)
- Modify: `internal/approval/manager.go` (审批创建/处理后推送 approval_created/approval_resolved)

**Estimated LOC:** 120

**Dependencies:** Task 3, Task 10, Task 11

**Acceptance Criteria:**
- [ ] `hub.BroadcastPatrolEvent(patrol, status, summary, runID)` — 推送 `{"type": "patrol_event", "patrol": "...", "status": "...", "summary": "...", "run_id": "...", "time": N}`
- [ ] `hub.BroadcastApprovalCreated(approval)` — 推送 `{"type": "approval_created", "approval": {...}}`
- [ ] `hub.BroadcastApprovalResolved(id, status)` — 推送 `{"type": "approval_resolved", "id": "...", "status": "approved|rejected|expired"}`
- [ ] `hub.BroadcastNotification(notification)` — 推送 `{"type": "notification", "notification": {...}}`
- [ ] 在 `Manager.Execute()` 完成后调用 `BroadcastPatrolEvent`
- [ ] 在 `approval.Manager.CreateRequest()` 中调用 `BroadcastApprovalCreated`
- [ ] 在 `approval.Manager.Approve()`/`Reject()` 中调用 `BroadcastApprovalResolved`
- [ ] Dashboard 前端接收新消息类型并更新对应 UI 组件（前端实现在 Task 19/20）

**Implementation Notes:**

`Hub.Broadcast()` 已存在，接收 `any` 类型参数并 JSON 序列化后发送给所有认证客户端。新增的 helper 方法只是构造特定格式的 map 并调用 `Broadcast()`，不需要修改 Hub 核心逻辑。

参考现有 `hub.BroadcastSessionReady()` 和 `hub.BroadcastSessionsUpdate()` 的模式。

---

### Task 13: Notification REST API

**描述:**

实现通知中心的 HTTP REST API。提供通知列表查询、标记已读、未读计数等端点。

**Files:**
- Create: `internal/notification/api.go`
- Modify: `internal/server/server.go` (注册通知 API 路由)

**Estimated LOC:** 100

**Dependencies:** Task 11

**Acceptance Criteria:**
- [ ] `GET /api/notifications` — 通知列表: query `?limit=20&offset=0&unread_only=false`。响应: `{"notifications": [...], "total": N, "unread_count": N}`
- [ ] `POST /api/notifications/read-all` — 标记全部已读。响应: `{"updated": N}`
- [ ] `POST /api/notifications/{id}/read` — 标记单条已读
- [ ] `NotificationHandler` struct 持有 `*Store` 引用，`RegisterRoutes(mux)` 注册路由
- [ ] 在 `server.go` 中注册路由

**Implementation Notes:**

通知 API 最简单——只有读取和标记已读。通知的写入由 patrol/approval 模块在内部完成（调用 `store.Add()`），不通过 API。

---

### Task 14: IM 通知路由 — 复用 Platform 适配器

**描述:**

实现 patrol 执行结果和审批请求到 IM 平台（飞书/Slack/Discord/微信）的通知推送。复用现有 `platform.Platform` 接口和 `platform.ReplyWithRetry()` 机制，参考 `cron/scheduler.go` 中 `notifyTarget()` 的实现。

同时新增 IM 审批命令: 用户可以在 IM 中直接 `/approve {id}` 或 `/reject {id}`，无需打开 Dashboard。

**Files:**
- Create: `internal/patrol/notify.go`
- Modify: `internal/server/dispatch.go` (新增 /approve, /reject, /approvals IM 命令路由)

**Estimated LOC:** 200

**Dependencies:** Task 3, Task 10, Task 11

**Acceptance Criteria:**
- [ ] `notifyPatrolResult(p *Patrol, log *RunLog, platforms, hub, notifStore)` — 根据 patrol 的 `notify` 配置，分发结果到 Dashboard (WebSocket) + IM 平台
- [ ] IM 通知消息格式: `[Patrol: {name}] {status_emoji} {summary}\n详情: /dashboard#patrol/{name}/run/{id}`
- [ ] 审批请求 IM 通知格式: `[审批请求] {patrol}\n操作: {action}\n摘要: {summary}\n级别: {level}\n\n回复 /approve {id_prefix} 批准\n回复 /reject {id_prefix} 拒绝`
- [ ] 在 dispatch.go 中新增命令路由: `/approve {id_prefix}` → 调用 `approval.Manager.Approve()`, `/reject {id_prefix}` → 调用 `approval.Manager.Reject()`, `/approvals` → 列出 pending 审批
- [ ] IM 平台推送目标从 config 读取: `patrol_notify.feishu_chat_id`, `patrol_notify.slack_channel`
- [ ] 通知长度适配: 使用 `platform.SplitText(text, maxLen)` 分段发送（与 cron `notifyIM` 一致）
- [ ] 推送失败时 `slog.Warn` 但不中断 patrol 执行流

**Implementation Notes:**

直接参考 `cron/scheduler.go` 第 489-533 行的 `notifyIM()` 和 `notifyTarget()` 函数。Patrol 的通知路由更灵活——`notify` 字段是 `[]string`，可以同时推送到多个目标（`["feishu", "dashboard", "slack"]`），而 cron 只推送到创建者 chat + 可选 notify target。

IM 审批命令的实现: 在 `dispatch.go` 的命令路由 switch 中新增 case，匹配 `/approve` 和 `/reject` 前缀。ID 前缀匹配逻辑参考 `cron.Scheduler.findByPrefix()`。

---

## Backend — Integrations

### Task 15: GitHub Webhook 集成 — PR Sentinel Patrol

**描述:**

实现 GitHub PR 事件到 Patrol 的端到端集成。当 GitHub webhook 推送 PR opened 事件时，触发 `pr-review` patrol，agent 自动审查 PR 并将结果推送到 Dashboard + IM。

这个 task 不需要新建代码——而是配置和验证 Task 6 的 webhook 机制在 GitHub 场景下的完整工作。唯一需要新增的是 GitHub webhook payload 的结构化解析，提取 PR number, title, repo, author 等信息注入 prompt。

**Files:**
- Create: `internal/patrol/github.go`
- Modify: `internal/patrol/webhook.go` (集成 GitHub payload 解析)

**Estimated LOC:** 150

**Dependencies:** Task 5, Task 6

**Acceptance Criteria:**
- [ ] `ParseGitHubWebhook(r *http.Request) (*GitHubEvent, error)` — 解析 GitHub webhook 请求: 从 `X-GitHub-Event` header 提取事件类型，从 body 提取 payload
- [ ] `GitHubEvent` struct 包含: EventType, Action, Repository (owner/name), PR (number/title/author/url/body), Issue (number/title/author), Sender
- [ ] `FormatGitHubContext(event *GitHubEvent) string` — 格式化 GitHub 事件为 human-readable prompt context。PR 场景示例: `"GitHub PR #42 opened by @keith in owner/repo\nTitle: Fix CloudFront cache\nURL: https://github.com/...\nBody: ..."`
- [ ] GitHub webhook secret 验证: 从 config 读取 `github.webhook_secret`，使用 `crypto/hmac` + SHA256 校验 `X-Hub-Signature-256` header
- [ ] 在 `webhook.go` 的请求处理链中: 检测到 `X-GitHub-Event` header 时走 GitHub 专用路径，否则走通用 webhook 路径
- [ ] 集成测试: 模拟 GitHub PR opened webhook payload，验证 patrol 被正确触发且 prompt 包含 PR 信息
- [ ] 支持的事件类型: `pull_request` (opened/reopened/synchronize), `issues` (opened), `push`

**Implementation Notes:**

GitHub payload 解析不需要完整的 GitHub API 类型定义——只提取 patrol prompt 需要的字段。使用 `json.RawMessage` + 手动解析关键字段，避免引入大型 GitHub SDK 依赖。

PR review patrol 的配置示例（写入文档但不自动创建）:
```yaml
patrols:
  pr-review:
    trigger: "github:pull_request"
    agent: code-reviewer
    prompt: "Review this PR for security issues, code quality, and best practices. Use gh CLI to read the PR diff."
    notify: [feishu, dashboard]
```

---

### Task 16: Proactive Insights — 实体提取 + Patrol 日志关联

**描述:**

实现 Phase 1 级别的 Proactive Insights: 关键词匹配实体提取 + Patrol 日志关联查询。当用户在 Dashboard 发送消息时，异步提取消息中的技术实体（AWS 服务名、资源 ID、项目名），然后查询 patrol 最近执行日志中是否有相关告警/异常，如果有则通过 WebSocket 推送 insight 卡片到 Dashboard。

这是"AI 主动推送洞察"的基础版本——纯关键词匹配，不涉及 LLM 语义理解（那是 Phase 2）。

**Files:**
- Create: `internal/insight/extractor.go`
- Create: `internal/insight/correlator.go`
- Modify: `internal/server/send.go` (在 sendWithBroadcast 中集成异步 insight 查询)

**Estimated LOC:** 250

**Dependencies:** Task 3, Task 11

**Acceptance Criteria:**
- [ ] `Entity` struct: `Type` (aws_service/resource_id/project/file_path), `Value`
- [ ] `ExtractEntities(text string) []Entity` — 关键词匹配提取: AWS 服务名词表（50+ 个，如 CloudFront, EKS, RDS, WAF, Lambda），AWS 资源 ID 正则（`i-[0-9a-f]+`, `arn:aws:...`, `sg-[0-9a-f]+` 等），项目名（从 `projects.root` 配置的目录名）
- [ ] `Insight` struct: Type (related_patrol/entity_link), Priority (p1/p2/p3), Title, Summary, SourceType (patrol), SourceRef, Score (0-1), CreatedAt
- [ ] `Correlator` struct 持有 `*patrol.Manager` 引用
- [ ] `Correlator.FindRelated(ctx, entities) ([]Insight, error)` — 查询 patrol 日志: 遍历所有 patrol 的最近 24h 日志，对 summary/detail 做关键词匹配，warn/error 状态的匹配项 score 更高。返回 top 3 按 score 排序
- [ ] 在 `send.go` 的 `sendWithBroadcast()` 中集成: 启动 goroutine 调用 `ExtractEntities` + `FindRelated`，不阻塞主 send 流程。如果有 score > 0.7 的 insight，通过 `hub.Broadcast` 推送 `{"type": "insight", "session_key": "...", "insights": [...]}`
- [ ] 延迟预算: 实体提取 < 5ms (纯正则/词表匹配), 关联查询 < 100ms (in-memory 日志扫描)
- [ ] 单元测试覆盖实体提取和关联查询逻辑

**Implementation Notes:**

实体提取的 AWS 服务名词表可以硬编码——AWS 服务名称变化频率很低。使用 `strings.ToLower` + `strings.Contains` 做匹配，比正则更快。

关联查询需要访问 patrol 日志。在 `Manager` 上暴露 `SearchLogs(keyword string, within time.Duration) []*RunLog` 方法，遍历所有 patrol 的 `LogWriter.ReadTail(100)` 结果做关键词匹配。

注意: insight 推送必须是非阻塞的——绝不能延迟用户消息的 agent 响应。使用 goroutine + channel/callback 机制，参考 `sendWithBroadcast` 中的 `onEvent` callback 模式。

---

## Frontend

### Task 17: Patrols 视图 — 卡片网格 + 状态 + 日志 + 操作

**描述:**

在 Dashboard 前端新增 Patrols 视图。采用卡片网格布局，每张卡片展示一个 patrol 的状态、描述、最近执行日志和操作按钮。视图通过 REST API 获取数据，通过 WebSocket 接收实时状态更新。

这是 Dashboard 顶栏导航中新增的第五个 tab（Home | Chat | Knowledge | Wiki | **Patrols** | Graph | Approvals）。

**Files:**
- Modify: `internal/server/static/dashboard.html` (新增 Patrols 视图)

**Estimated LOC:** 500

**Dependencies:** Task 5

**Acceptance Criteria:**
- [ ] 顶栏导航新增 "Patrols" tab，点击切换到 Patrols 视图
- [ ] 卡片网格布局: CSS Grid `repeat(auto-fill, minmax(320px, 1fr))`，响应式适配
- [ ] 每卡片包含: 状态圆点 (绿色 Active, 蓝色脉冲 Running, 黄色 Paused, 灰色 Disabled) + 名称 + 状态 badge + 描述 (truncated) + schedule/trigger 显示 + 操作按钮
- [ ] 操作按钮: Pause/Resume (toggle), Trigger (手动触发), Logs (展开日志面板)
- [ ] 日志面板: 点击 "Logs" 按钮后在卡片下方展开，显示最近 5 条 RunLog: 时间 + 状态 badge (ok 绿/warn 黄/error 红) + duration + summary。展开/折叠用 CSS `max-height` transition
- [ ] 点击单条日志行展开显示完整 detail (pre-formatted text block)
- [ ] API 调用: 页面加载时 `GET /api/patrols`，Logs 展开时 `GET /api/patrols/{name}/logs?limit=5`，操作按钮调用 `PUT /api/patrols/{name}/state` 或 `POST /api/patrols/{name}/trigger`
- [ ] WebSocket 监听 `patrol_event` 消息类型: 收到后更新对应卡片的状态圆点和最近执行记录，无需刷新
- [ ] Running 状态动画: 蓝色圆点 + CSS pulse animation
- [ ] 空状态: 无 patrol 时显示 "No patrols configured. Add patrols in config.yaml."
- [ ] 移动端适配: 卡片单列全宽 (`@media (max-width: 768px)`)

**Implementation Notes:**

前端遵循 Naozhi 现有 Dashboard 的实现风格: 原生 JS + CSS，无框架依赖。所有 UI 组件用 DOM 操作和 template literal 构建，事件绑定用 `addEventListener`。

状态 badge 的颜色方案:
- Active: `#10b981` (emerald-500)
- Running: `#3b82f6` (blue-500) + pulse
- Paused: `#f59e0b` (amber-500)
- Disabled: `#6b7280` (gray-500)

---

### Task 18: Approvals 视图 — 卡片列表 + Approve/Reject + 空状态

**描述:**

在 Dashboard 前端新增 Approvals 视图。全宽审批卡片列表，按 urgency 分级显示（urgent 红色左边框, normal 黄色左边框），每卡片包含操作详情和 Approve/Reject 按钮。

**Files:**
- Modify: `internal/server/static/dashboard.html` (新增 Approvals 视图)

**Estimated LOC:** 400

**Dependencies:** Task 8

**Acceptance Criteria:**
- [ ] 顶栏导航新增 "Approvals" tab，pending 数 > 0 时显示红色 badge 计数
- [ ] 全宽卡片列表布局: 垂直堆叠，每卡片左边框颜色编码 (urgent 红/normal 黄)
- [ ] 每卡片包含: 图标 (根据 action 类型) + 标题 (action label) + 来源 patrol 名 + 时间 (relative, "3 min ago") + detail 摘要 (truncated to 3 lines) + impact 文本 (如有) + level badge (critical 红/high 黄)
- [ ] 操作按钮: Approve (绿色) + Reject (灰色) + View Details (展开完整 detail)
- [ ] Approve 点击: 确认弹窗 "确认批准此操作?" → `POST /api/approvals/{id}/approve` → 卡片绿色高亮后滑出动画 (slide-out + fade)
- [ ] Reject 点击: 可选填写 reason → `POST /api/approvals/{id}/reject` → 卡片灰色后滑出
- [ ] View Details: 展开区域显示完整 detail (代码块渲染, terraform plan 等), 使用 `<pre>` 包裹
- [ ] 空状态: 全部审批处理完后显示大号 checkmark + "All caught up!" 文本 + 灰色说明 "No pending approvals"
- [ ] API 调用: 页面加载时 `GET /api/approvals?status=pending`，操作后刷新列表
- [ ] WebSocket 监听: `approval_created` → 在列表顶部插入新卡片 (slide-in 动画), `approval_resolved` → 移除对应卡片
- [ ] 状态筛选: 顶部 tab 条 "Pending | Approved | Rejected | All"，切换时 query status 参数
- [ ] 移动端适配: 卡片全宽, 操作按钮加大 (min-height 44px), 长 detail 折叠

**Implementation Notes:**

Approve/Reject 的乐观 UI 更新: 点击按钮后立即播放滑出动画，不等待 API 响应。如果 API 失败，回滚动画并显示 toast 错误。这样操作感更流畅。

detail 渲染需要处理 terraform plan 输出等结构化文本。使用 `<pre class="approval-detail">` + monospace 字体，不做 markdown 渲染（plan 输出的缩进和对齐很重要）。

---

### Task 19: Notification Center UI — 铃铛 + 下拉面板 + Badge + 导航

**描述:**

在 Dashboard 右上角实现 Notification Center: 铃铛图标 + 未读计数 badge + 点击弹出下拉通知面板。通知面板展示最近通知列表，按 urgency 分级高亮，点击通知跳转到对应视图（Patrols/Approvals/Wiki）。

**Files:**
- Modify: `internal/server/static/dashboard.html` (新增 Notification Center)

**Estimated LOC:** 350

**Dependencies:** Task 12, Task 13

**Acceptance Criteria:**
- [ ] 右上角铃铛按钮: SVG bell icon + 红色圆形 badge (显示未读数, > 99 显示 "99+", 0 时隐藏)
- [ ] 点击铃铛: 弹出下拉面板，绝对定位 (right: 0, top: 100%), z-index 高于其他覆盖层
- [ ] 面板头部: "Notifications" 标题 + "Mark all read" 按钮
- [ ] 通知列表: 每条通知包含——左边框颜色 (urgent 红/unread 蓝/read 无), 图标 (类型对应), 标题, 摘要 (truncated), 相对时间
- [ ] 通知类型图标: patrol_alert ⚠️, approval_request 📥, wiki_compiled 📚, im_answer 💬, cost_report 💰
- [ ] 点击通知: 标记已读 (`POST /api/notifications/{id}/read`) + 跳转到对应视图 (根据 sourceType: patrol → Patrols tab, approval → Approvals tab)
- [ ] "Mark all read" 按钮: `POST /api/notifications/read-all` → 清除所有蓝色高亮和 badge
- [ ] WebSocket 监听 `notification` 消息: 收到后在面板顶部插入新通知项 (slide-in), 更新 badge 计数
- [ ] 面板外点击关闭 (document click listener + `stopPropagation`)
- [ ] API 调用: 铃铛加载时 `GET /api/notifications?limit=20`, badge 数来自响应的 `unread_count`
- [ ] 空状态: 面板内显示 "No notifications" + 灰色 bell icon
- [ ] 移动端适配: 面板改为全宽下拉 (width: 100vw)

**Implementation Notes:**

badge 更新策略: WebSocket 连接建立后，收到任何 `notification` 消息都增加 badge 计数。面板打开时不自动标记已读（用户可能只是瞥一眼），只有点击具体通知或点击 "Mark all read" 才更新。

面板的定位: 使用 `position: fixed` 而非 `absolute`，避免被其他 overflow:hidden 容器裁剪。

---

### Task 20: Home Dashboard 集成 — Patrol 状态 + 审批 Widget

**描述:**

在 Home 仪表板（CTO 操控台）中集成 patrol 和 approval 的实时状态 widget。Home 是用户打开 Dashboard 的首屏，需要一眼看到: 活跃 patrol 数、正在运行的 patrol、pending 审批数、最近执行结果。

**Files:**
- Modify: `internal/server/static/dashboard.html` (扩展 Home 视图)

**Estimated LOC:** 350

**Dependencies:** Task 17, Task 18, Task 19

**Acceptance Criteria:**
- [ ] Today's Overview stat 卡片新增: "Active Patrols" (活跃 patrol 数) + "Pending Approvals" (待审批数, > 0 时红色)。数据来源: `GET /api/patrols` 的 stats + `GET /api/approvals/stats`
- [ ] **Patrol Status Widget**: 实时巡逻状态列表——每行: 状态圆点 (颜色同 Task 17) + patrol name + 状态文本 (Running/Active/Paused) + 最近执行时间。显示所有非 Disabled 的 patrol
- [ ] **Pending Approvals Widget**: 紧急审批提醒——如果有 pending 审批, 显示红色边框卡片列表 (最多 3 条), 每条: action label + source patrol + 相对时间 + "Review" 按钮 (点击跳转 Approvals tab)。无 pending 时显示绿色 "All clear"
- [ ] **Activity Feed 扩展**: 在现有 activity feed 中集成 patrol 事件——patrol 执行完成、审批创建/处理事件混合在全局时间线中
- [ ] Quick Actions 按钮新增: "Patrols" (跳转 Patrols tab) + "Approvals" (跳转 Approvals tab, badge 显示 pending 数)
- [ ] WebSocket 实时更新: 收到 `patrol_event` / `approval_created` / `approval_resolved` 消息时，自动更新对应 widget 数据，无需手动刷新
- [ ] 页面加载时并行请求: `GET /api/patrols` + `GET /api/approvals?status=pending` + `GET /api/notifications?limit=10`，Promise.all 等待完成后渲染
- [ ] 移动端适配: stat 卡片横向滚动, widget 垂直堆叠, Quick Actions 2x2 网格

**Implementation Notes:**

Home 页面是信息密度最高的视图——需要同时展示 sessions、patrols、approvals、notifications 的聚合数据。API 请求用 `Promise.all` 并行发送，避免瀑布式加载。

Patrol Status Widget 和 Pending Approvals Widget 放在 Home 页面的右栏（桌面端）或 stat 卡片下方（移动端）。设计上参考 spec 第 2.2 节 Home Widget 定义。

Activity Feed 的 patrol 事件格式:
```
🔄 [cost-alert] OK — 当日费用 $3.12，低于阈值  (2 min ago)
⚠️ [infra-health] WARN — EKS pending pods detected  (15 min ago)
📥 [infra-health] Approval created — terraform apply  (15 min ago)
✅ [infra-health] Approval approved by keith  (12 min ago)
```

---

## 实施时间线

```
Week 1-2: Backend Foundation
  Task 1  (Patrol struct)           → 0.5 day
  Task 2  (Patrol store)            → 0.5 day
  Task 7  (Approval model + store)  → 0.5 day
  Task 11 (Notification model)      → 0.5 day
  Task 4  (Patrol log system)       → 1 day
  Task 9  (Approval detection)      → 0.5 day
  Task 3  (Patrol executor)         → 2 days

Week 3: Backend API + Integration
  Task 5  (Patrol REST API)         → 1 day
  Task 6  (Webhook receiver)        → 1 day
  Task 8  (Approval REST API)       → 1 day
  Task 13 (Notification REST API)   → 0.5 day
  Task 10 (Approval blocking)       → 2 days

Week 4: Notification + Integration
  Task 12 (Notification WebSocket)  → 0.5 day
  Task 14 (IM notification routing) → 1 day
  Task 15 (GitHub webhook)          → 1 day
  Task 16 (Proactive Insights)      → 1.5 days

Week 5-6: Frontend
  Task 17 (Patrols view)            → 2 days
  Task 18 (Approvals view)          → 2 days
  Task 19 (Notification Center UI)  → 1.5 days
  Task 20 (Home dashboard)          → 2 days
  End-to-end testing + bugfix       → 2 days
```

## LOC 预估汇总

| 区域 | Tasks | 预估 LOC |
|------|-------|---------|
| Backend — Patrol Package | 1-6 | 1,310 |
| Backend — Approval System | 7-10 | 850 |
| Backend — Notifications | 11-14 | 570 |
| Backend — Integrations | 15-16 | 400 |
| Frontend | 17-20 | 1,600 |
| Server routing + config | 散布各 task | 200 |
| Tests | 各 task 含测试 | ~800 |
| **Total** | **20 tasks** | **~5,730** |

## 数据存储新增文件

```
~/.naozhi/
  patrols.json               # patrol 运行时状态
  approvals.json              # 审批请求 (活跃)
  approvals-archive.jsonl     # 审批记录归档
  notifications.json          # 通知列表
  patrols/                    # patrol 日志目录
    pr-review/logs.jsonl
    cost-alert/logs.jsonl
    infra-health/logs.jsonl
    dep-audit/logs.jsonl
```

## Config 新增字段

```yaml
# config.yaml 新增

patrols:
  pr-review:
    trigger: "github:pull_request"
    agent: code-reviewer
    prompt: "Review this PR..."
    notify: [feishu, dashboard]
  cost-alert:
    schedule: "@every 1h"
    agent: general
    prompt: "Check AWS Cost Explorer..."
    notify: [feishu, dashboard]

patrol_notify:
  feishu_chat_id: "oc_xxxx"
  slack_channel: "#ops-alerts"

github:
  webhook_secret: "${GITHUB_WEBHOOK_SECRET}"
  allowed_repos: ["owner/repo1", "owner/repo2"]
```

## WebSocket 新增消息类型

```jsonc
// Server → Client
{"type": "patrol_event", "patrol": "name", "status": "ok|warn|error", "summary": "...", "run_id": "...", "time": 1713200000000}
{"type": "approval_created", "approval": {"id": "appr-...", "patrol": "name", "action": "terraform apply", "urgency": "urgent"}}
{"type": "approval_resolved", "id": "appr-...", "status": "approved|rejected|expired"}
{"type": "notification", "notification": {"id": "...", "type": "patrol_alert", "title": "...", "urgency": "urgent"}}
{"type": "insight", "session_key": "...", "insights": [{"type": "related_patrol", "title": "...", "score": 0.85}]}
```

## REST API 新增端点汇总

```
# Patrols
GET    /api/patrols                       # 列出所有 Patrol
GET    /api/patrols/{name}                # 单个 Patrol 详情
PUT    /api/patrols/{name}/state          # 修改状态 (pause/resume/disable)
POST   /api/patrols/{name}/trigger        # 手动触发执行
GET    /api/patrols/{name}/logs           # 执行日志 (?limit=20&offset=0)
GET    /api/patrols/{name}/logs/{id}      # 单条日志详情

# Webhooks
POST   /api/webhooks/{patrol-name}        # 外部 event webhook 触发

# Approvals
GET    /api/approvals                     # 列表 (?status=pending&limit=20)
GET    /api/approvals/{id}                # 单个审批详情
POST   /api/approvals/{id}/approve        # 批准
POST   /api/approvals/{id}/reject         # 拒绝
GET    /api/approvals/stats               # 统计

# Notifications
GET    /api/notifications                 # 通知列表 (?limit=20&unread_only=false)
POST   /api/notifications/read-all        # 标记全部已读
POST   /api/notifications/{id}/read       # 标记单条已读
```
