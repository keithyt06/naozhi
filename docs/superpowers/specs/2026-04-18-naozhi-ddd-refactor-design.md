# Naozhi DDD 重构与性能优化 — Design Spec

> 日期: 2026-04-18
> 作者: Keith (CTO mode) + Claude
> 状态: Draft，待用户审阅

## 1. Executive Summary

Naozhi 已积累到 **38k 行 Go + 8.3k 行单文件 HTML** 的规模。后端按 handler 分文件已有雏形但缺少领域层；前端则是一个 7163 行的内联 `<script>` 吞下 7 个视图，首屏慢、维护成本高。

本次重构做三件事：

1. **下线 Meeting 功能** — 删除所有代码、API、UI、数据。
2. **前端模块化 + 按视图懒加载** — 把 `dashboard.html` 拆成 shell + ES modules + 每视图独立 chunk，首屏 JS 降到 < 1500 行。
3. **后端引入 DDD 分层** — 新增 `internal/domain/<context>/` 层，`internal/server/` 退化为薄 HTTP transport。

保持不变：**Go 原生 `net/http` + vanilla JS + 单二进制 embed**。**不**引入 React/Vue/打包工具链。这是刻意的 YAGNI 决策。

---

## 2. 当前状态（量化）

| 指标 | 数据 |
|---|---|
| Go 代码 | 38,161 行，17 个子包 |
| `internal/server/` | 11 个 `dashboard_*.go` handler 文件，`server.go` 17KB，`wshub.go` 20KB |
| 最大 Go 文件 | `session/router.go` 1536 行、`shim/server.go` 844、`cli/process.go` 811、`server/wshub.go` 734 |
| `dashboard.html` | **8309 行**（989 行 `<style>` + **7163 行单块 `<script>`** + HTML） |
| 前端视图 | 7 个：Home / Chat / Knowledge / Wiki / Patrols / Approvals / Meetings / Graph |
| 构建工具 | 无（JS 直接 embed 到二进制） |

**性能病根**：

- 前端一次性下载和解析 8.3k 行 HTML + 7k 行 JS，即使用户只用 Chat；
- 大量字符串拼接生成 HTML（`onclick="switchView('patrols',...)"`），切视图可能全量重渲；
- 样式和 JS 都不走 `Cache-Control: immutable`，每次内容变都全量失效。

---

## 3. 目标 / 非目标

### 目标

- **删除 Meeting 功能**（代码、数据、文档）。
- **首屏 JS 下载体积减半以上**，视图切换非首次 < 50ms。
- **后端领域边界清晰**：业务逻辑从 `internal/server/` 搬到 `internal/domain/`，handler 变薄（< 150 行/文件）。
- **测试不倒退**：重构每步都能 `go test ./...` 通过，不允许 big-bang。
- **保留部署简单性**：仍是单 Go 二进制，不引入 Node.js build。

### 非目标

- 不改 Claude CLI 代理层（LiteLLM、Bedrock）。
- 不改认证（Cognito/ALB）。
- 不引入前端框架（React、Vue、Svelte）。
- 不拆微服务（保持单体）。
- 不做 i18n、暗黑模式、A11y 专项（未来再议）。

---

## 4. 架构决策

### 4.1 为什么不上 SPA？

选项对比：

| 方案 | 改动 | 性能收益 | 风险 | CTO 判断 |
|---|---|---|---|---|
| A. Vanilla + ES Modules 拆分 | 中 | 高（懒加载 + 缓存） | 低 | **选它** |
| B. Alpine.js / htmx / Lit | 中+ | 高 | 中（引入依赖） | 否 |
| C. 完整 SPA（React/Vue/Svelte + Vite） | 大 | 高 | 高（构建链、部署、team 单人） | 否 |

**理由**：单人团队、已在生产、核心瓶颈是"一次性加载全部代码"。这个问题用**原生 ES modules + 动态 `import()` + `<template>`** 就能解决，不需要框架。框架带来的开发效率提升在 7 个视图的规模上并不显著，反而增加构建、部署、调试的复杂度。

### 4.2 DDD 分层

引入四层：

```
┌────────────────────────────────────────────┐
│ Transport  (internal/server/)              │  HTTP handlers, WS hub, middleware
├────────────────────────────────────────────┤
│ Application (internal/app/)                │  use-case orchestration (optional, 只在跨领域时用)
├────────────────────────────────────────────┤
│ Domain     (internal/domain/<context>/)    │  entities, value objects, domain services, repo interfaces
├────────────────────────────────────────────┤
│ Infrastructure (internal/infra/)           │  filesystem, bedrock, cognito, feishu adapters
└────────────────────────────────────────────┘
```

Domain 层**不允许** import transport/infrastructure。依赖方向始终向下。

### 4.3 Bounded Contexts

从现有 17 个包归并为 5 个 bounded context：

| Context | 当前包 | 职责 |
|---|---|---|
| **Session** | `cli`, `session`, `shim`, `dispatch`, `discovery`, `node`, `routing` | 运行 Claude CLI 会话、跨节点路由、进程 shim |
| **Knowledge** | `knowledge`, `graph`, `patrol` | 笔记库、wiki、决策日志、知识图谱、巡检规则 |
| **Workspace** | `project`, `cron` | 工作目录、定时任务 |
| **Integration** | `platform/*`, `transcribe`, `upstream` | 飞书、语音转写、上游拉取 |
| **Dashboard** | `server/` (thinned) | HTTP/WS transport only |

`config` 横切所有层，作为独立共享包。

---

## 5. Target 目录结构

### 5.1 Go 后端

```
internal/
  domain/
    session/          # Session aggregate, RouteSession service, repo interface
      session.go
      router.go       # 从 session/router.go 搬来，去掉 HTTP 依赖
      service.go
      ports.go        # interface: CLIRunner, NodeRegistry, Store
    knowledge/
      note.go
      wiki.go
      patrol.go
      decision.go
      service.go
      ports.go
    workspace/
      project.go
      cron.go
      service.go
      ports.go
    integration/
      feishu/
      transcribe/
  infra/
    store/            # filesystem / JSON persistence impls
    bedrock/          # Claude CLI + LiteLLM adapter
    cognito/
    feishu/           # feishu API impl
  server/             # transport only
    server.go
    wshub.go
    middleware.go
    handlers/
      session.go      # thin: parse req → call domain.SessionService → write resp
      knowledge.go
      files.go
      cron.go
      transcribe.go
      auth.go
      home.go
    static/           # frontend (see 5.2)
  config/
  cli/                # CLI adapter (bedrock) — 可搬到 infra/ 或保留
  shim/               # OS-level shim — infra 性质，可放 infra/shim/
```

**迁移规则**：每个 context 先建 `domain/` 骨架 + 接口，handler 保留原位但改成调用 domain，旧业务代码逐文件搬过去并删除。**一步一个测试**。

### 5.2 前端

```
internal/server/static/
  dashboard.html           # < 500 行：HTML shell + view slots + main module <script type="module" src="/js/app.js">
  css/
    base.css               # 全局变量、reset、layout
    components.css         # buttons、inputs、common widgets
    views/
      chat.css
      knowledge.css
      wiki.css
      patrols.css
      approvals.css
      graph.css
      home.css
  js/
    app.js                 # bootstrap: auth、ws、router
    core/
      router.js            # view registry + switchView + history API
      ws.js                # WebSocket hub client
      api.js               # fetch wrapper、authHeaders、retry
      state.js             # 轻量 pub/sub store
      html.js              # tagged template literal helper (no deps, < 50 lines)
      utils.js             # escAttr, escJs, debounce...
    views/
      home.js              # export { mount, unmount } — 默认首屏
      chat.js              # export { mount, unmount, onEvent } — 懒加载
      knowledge.js
      wiki.js
      patrols.js
      approvals.js
      graph.js
  sw.js                    # service worker：precache core + 按需缓存 views
```

**视图懒加载模式**：

```js
// router.js
const VIEWS = {
  chat:      () => import('./views/chat.js'),
  knowledge: () => import('./views/knowledge.js'),
  // ...
};
async function switchView(name) {
  const mod = await VIEWS[name]();
  currentView?.unmount();
  currentView = mod;
  await mod.mount(document.getElementById('view-slot'));
}
```

**渲染**：统一用一个 50 行的 `html` tagged template（无虚拟 DOM，按需 DOM diffing 后续可加 lit-html，但 v1 不加），消除 `'<div class="..." onclick="func(\'' + esc(x) + '\')">'` 这种字符串拼接。

**缓存策略**：Go 端给 `/css/*.css`、`/js/*.js` 打 content-hash（例如 `app.abc123.js`），`Cache-Control: public, max-age=31536000, immutable`；`dashboard.html` 短缓存 + ETag。

---

## 6. 分阶段执行

**每个 Phase 独立可 ship，且 `go test ./... && systemctl restart naozhi` 后服务正常。**

### Phase 0 — 删除 Meeting（1 天）

**Go 代码**：

- 删除 `internal/knowledge/meeting.go`、`internal/knowledge/meeting_processor.go`
- 删除 `internal/server/dashboard_meeting.go`
- `internal/server/server.go`：第 72 行字段 `meetingH *MeetingHandlers`、330-335 行初始化块
- `internal/server/dashboard.go`：第 155-159 行 3 条 `/api/meetings*` 路由注册
- `grep -rn "Meeting\|meetings" internal/` 兜底清残留

**前端（`dashboard.html`）**：

- 第 931-1001 行 `/* ===== Meeting Intelligence (Phase 4C) ===== */` CSS 整段
- 第 1060 行 Meetings nav 按钮
- 第 5580 行 `else if (view === 'meetings')` 分支
- 第 6906 开始的 Meeting Intelligence JS 整块（`mtMeetings` 全局、`renderMeetingView`、`loadMeetingList`、`loadMeetingDetail`、`handleMeetingUpload`）
- Home view 内 Meeting 相关 widget（若有）

**数据（生产 EC2，需确认）**：

```bash
ssh $EC2 "sudo systemctl stop naozhi && \
  sudo rm -f /root/.naozhi/meetings.json && \
  sudo rm -rf /root/.naozhi/meetings/ && \
  sudo systemctl start naozhi"
```

**文档**：

- `README.md` — Meeting Intelligence 段落删掉
- `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md` — 在顶部加 "> Meeting 功能于 2026-04-18 下线，本 spec 相关章节仅做历史参考" 标注
- 同样标注 `docs/superpowers/plans/2026-04-15-naozhi-v2-phase4-platform-moonshots.md`

**验证**：

```bash
go build ./... && go test ./... && \
  ! grep -rn -i "meeting" --include="*.go" internal/ && \
  ! grep -n -i "meeting" internal/server/static/dashboard.html
```

手机端访问 dashboard 切所有视图无控制台报错。

### Phase 1 — 前端拆分（3-5 天，性能最大收益）

**Step 1.1**：抽 CSS
- 把 `<style>` 块按视图拆到 `css/base.css` + `css/views/*.css`
- `dashboard.html` 只保留 `<link rel="stylesheet" href="/css/base.css">` + 关键 above-fold

**Step 1.2**：抽 core JS
- 把 `switchView`、WS、auth、api、utils 搬到 `js/core/*.js`
- `dashboard.html` 末尾只剩 `<script type="module" src="/js/app.js"></script>`

**Step 1.3**：每次搬一个视图到 `js/views/<name>.js`
- 顺序：home → chat → knowledge → wiki → patrols → approvals → graph
- 每搬一个跑一次 smoke test（用户点进去功能正常）
- 搬完后 `dashboard.html` 应 < 500 行

**Step 1.4**：接上动态 `import()` 懒加载 + content-hash 缓存

**性能目标**（Phase 1 完成）：
- 首屏 JS 下载 < 50 KB gzip（当前 ~120 KB）
- Chrome DevTools `PerformanceObserver` 测 LCP < 1.5s（本地），< 2.5s（移动 4G）
- 视图切换平均 < 50ms（除首次 import）

### Phase 2 — 后端 DDD 骨架（3 天）

**Step 2.1**：创建 `internal/domain/session/`
- 把 `session/router.go` 1536 行里**纯领域逻辑**搬进 `domain/session/router.go`
- 抽出 `NodeRegistry`、`Store`、`CLIRunner` 接口放到 `domain/session/ports.go`
- `internal/server/` 和 `internal/session/` 暂时共存，handler 改调用 domain service
- 旧 `internal/session/` 内容搬空后删除

**Step 2.2**：`internal/domain/knowledge/`
- 搬 `bookmark`、`decision`、`wiki`、`twin`、`search`、`patrol`
- meeting 已在 Phase 0 删除，搬迁时不带

**Step 2.3**：`internal/domain/workspace/` + `internal/domain/integration/`

**Step 2.4**：`internal/infra/` 接口实现
- `infra/store/` — JSON 文件持久化
- `infra/cli/` — claude CLI spawn
- `infra/platform/feishu/`
- `infra/transcribe/`

**验证**：每步 `go test ./...` 通过，`wire_gen.go` 或 `cmd/naozhi/main.go` 显式装配所有依赖。

### Phase 3 — server 层瘦身（2 天）

- `internal/server/handlers/` 每个 handler < 150 行
- `server.go` 只保留 server 生命周期 + 路由注册（从 400 行降到 < 200 行）
- `wshub.go` 保留（本来就是 transport）

### Phase 4 — 性能 polish（1-2 天）

- Service worker precache 核心 + 按需缓存视图
- `<link rel="modulepreload">` 预热最常用视图
- HTTP/2 适配（CloudFront 本来支持）
- 简单 metrics endpoint：前端用 `performance.mark` 上报 LCP/切换时间

**总工期估算**：10-13 个工作日（单人 + Claude 协作）。

---

## 7. 性能目标（SLA-like）

| 指标 | 当前（估） | Phase 1 后 | Phase 4 后 |
|---|---|---|---|
| 首屏 JS gzip | ~120 KB | < 50 KB | < 30 KB |
| 首屏 HTML | 8.3k 行 | < 500 行 | < 400 行 |
| LCP (本地 WiFi) | ~2s | < 1.5s | < 1s |
| LCP (移动 4G) | ~5s | < 2.5s | < 2s |
| 视图切换（非首次） | ~200ms | < 50ms | < 30ms |
| 单文件最大 Go | 1536 行 | 1536 行 | **< 600 行** |

---

## 8. 迁移与风险

### 风险 1：前端拆分过程功能回归

**缓解**：每搬一个视图都做手动 smoke test（桌面 + iOS Safari + Android Chrome）。保留 `dashboard.html` 的旧版备份分支 `naozhi-2.0-pre-split` 便于回滚。

### 风险 2：后端 DDD 迁移中间态循环依赖

**缓解**：严格单向依赖（domain ← server，domain → infra 接口）。每步只允许一个文件迁移。`go build` 挂就回滚该步。

### 风险 3：iOS Safari ES modules 兼容

**缓解**：Safari 10.1+ 支持原生 ES modules。当前 naozhi 用户场景（作者手机）已验证。不做 polyfill。

### 风险 4：content-hash 文件的 embed 方案

**缓解**：Go `embed` + 构建时 Makefile 脚本注入 hash 到 manifest.json。HTML 模板读 manifest 渲染 `<script src>` 的 URL。保持单二进制。

### 风险 5：测试覆盖不够

**缓解**：重构前先补关键路径集成测试（session 创建/路由/dispatch 至少 3 条 happy path）。测试不通过的部分暂不搬。

---

## 9. 本次 Spec 不涉及（Out of Scope）

- TypeScript 迁移（未来再议）
- i18n
- 暗黑模式美化
- 后端改 gRPC/protobuf
- 测试框架换 ginkgo 等
- 任何 SaaS 化/多租户

---

## 10. 验收标准

Phase 0 完成：`grep -ri "meeting" --include="*.go" --include="*.html" internal/ docs/` 返回空；dashboard 无 Meetings 按钮。

Phase 1 完成：`wc -l internal/server/static/dashboard.html` < 500；Chrome DevTools 看到 view chunk 按需加载。

Phase 2 完成：`tree internal/domain/` 存在且每个 context 有 `service.go` + `ports.go`；`internal/server/` 不再有业务逻辑 switch/case。

Phase 3 完成：`wc -l internal/server/*.go` 无单文件 > 600 行。

Phase 4 完成：性能表格达标。

---

## 11. 现有能力 Inventory（保留并强化）

先认清家底，再谈新建。以下模块**代码已在**，只是散在 `internal/knowledge/`、`internal/patrol/` 等，DDD 重构后搬到对应 bounded context，并借机统一接口和观测性。

| 模块 | 当前文件 | 价值 | 重构动作 |
|---|---|---|---|
| **Obsidian Vault** | `knowledge/vault.go` (11.8KB) | 最大的"第二大脑"入口：.md、wikilinks、callouts、math | 搬 `domain/knowledge/vault/`，加 inotify/polling 热更新；文件变更事件化 |
| **Wiki 编译** | `knowledge/wiki.go` + `cli_sync.go` | 把 CLI 对话/vault 编译成可查询页面 | 搬 `domain/knowledge/wiki/`，编译结果走事件 bus 触发 search/graph 重建 |
| **Full-text Search** | `knowledge/search.go` (9KB, **bleve**) | 已是 Bleve 生产级索引 | 保留 Bleve，包一层 `SearchPort` 接口；为 "Semantic Search"（见 §12）并行加 embedding index |
| **Decision Journal (ADR)** | `knowledge/decision.go` | 架构决策记录（Context/Decision/Consequences） | 搬 `domain/knowledge/decision/`；加 "关联 Session/Wiki" 反向链接 |
| **Bookmark** | `knowledge/bookmark.go` | 对话片段剪藏 | 搬 `domain/knowledge/bookmark/`；和 Decision、Wiki 打通标签体系 |
| **Knowledge Graph** | `graph/` (extractor + d3-force UI) | wikilinks → 图可视化 | 搬 `domain/knowledge/graph/`；Phase 1 前端拆分时把 d3 单独懒加载 |
| **Lint** | `knowledge/lint.go` | orphan/stale/empty/missing frontmatter 检测 | 搬 `domain/knowledge/lint/`；暴露为定时 cron + dashboard "Vault 健康" 视图 |
| **Ingest** | `knowledge/ingest.go` | 跨源索引 | 改成 event-driven：文件改动/新 bookmark → 增量 index |
| **CTO Digital Twin** | `knowledge/twin.go` (10KB) + `twin_delegate.go` | 模仿用户风格自动回复 patrol/approval | 搬 `domain/knowledge/twin/`；接 §12 的 Semantic Search 做 RAG |
| **Patrols** | `patrol/` (8 个文件) | 定时巡检 + 告警 + 执行器 + webhook | 搬 `domain/workspace/patrol/`；统一 `AlertNormalizer` 插件接口 |
| **Alert Webhook** | `patrol/alert.go` + `alert_handler.go` | CloudWatch/Grafana/Datadog 告警入口 | 搬 `domain/workspace/alert/`；加 Incident Timeline（见 §12） |
| **Approvals** | `patrol/approval*.go` | 待审批队列 | 搬 `domain/workspace/approval/`；Twin 自动分类 |
| **Notification** | `patrol/notification*.go` | 多渠道通知 | 搬 `domain/integration/notification/`；抽 `NotificationPort` |
| **Cron** | `cron/scheduler.go` (574 行) | 定时任务 | 搬 `domain/workspace/cron/`；为 §12 Daily Digest 复用 |
| **Session Replay** | `server/replay.go` (10KB) | 会话回放分享 | 搬 `domain/session/replay/`；加过期清理 |
| **Discovery** | `discovery/scanner.go` (661 行) | 外部进程发现 | 搬 `domain/session/discovery/`；扫描间隔自适应 |
| **Multi-node Routing** | `session/router.go` (1536 行) | 跨节点分发 | 搬 `domain/session/`；拆分文件（见 Phase 3） |
| **Platform Adapters** | `platform/feishu/` (601 行) + WeChat/Slack/Discord | IM 接入 | 搬 `domain/integration/<platform>/`；统一 `PlatformPort` |
| **Transcribe** | `transcribe/` | 语音转文字（AWS Transcribe） | 搬 `infra/transcribe/`；domain 侧只暴露 `Transcriber` 接口 |

**关键洞察**：naozhi 已经是"可用的第二大脑 + Agent 编排平台"。重构不是推倒重来，而是**把散落的能力归位**，让下一步新功能能用清晰的接口组合起来。

---

## 12. Think Big — 新功能矩阵

优先级排序：P0 = 本季度内做（ROI 高 + 复用已有模块）；P1 = 下一阶段；P2 = 探索级。每条带"复用了什么、新增什么、effort"。

### P0 — 本季度做

#### 12.1 Command Palette（⌘K 全局命令面板）

**场景**：VSCode/Linear/Obsidian 式全局搜索 + 动作入口。⌘K 弹出，键入即搜 sessions / wiki pages / bookmarks / decisions / approvals / commands。一键跳转或执行动作（新建 Session、切视图、打开 Wiki）。

**复用**：Bleve 多源索引已存在（`knowledge/search.go`）。

**新增**：`js/core/palette.js` ~300 行；后端 `/api/palette/search` 合并多源 hit；键盘 a11y。

**Effort**：2-3 天。

---

#### 12.2 Quick Capture（PWA Share Target + Web Clipper）

**场景**：手机/桌面 Chrome 任何页面点 Share → Naozhi，自动抓正文、抽标签、存入 vault 或 bookmark。桌面可装浏览器扩展（MV3）把选中文字+URL 一键发。

**复用**：Vault 写入、Bookmark、已安装的 `obsidian:defuddle` 技能（正文抽取）。

**新增**：PWA manifest 加 `share_target`；`POST /api/capture`；Chrome MV3 扩展（< 200 行，仅 action + fetch）；可选 Claude 摘要。

**Effort**：3-4 天。

---

#### 12.3 Daily Digest / Morning Briefing

**场景**：每日早晨 08:00 自动生成摘要：昨日对话要点、待处理 approvals、patrol 异常、今日 cron、重要 alert。推送到 Feishu + dashboard Home 顶部卡片 + 可选邮件。

**复用**：Cron（触发）、Session history、Approval/Patrol/Alert 数据源、Feishu 通知、Twin（用 CTO 口吻撰写）。

**新增**：`domain/workspace/digest/`：`DigestBuilder`（聚合器）+ `DigestRenderer`（模板）；`schedule.yaml` 条目；dashboard Home 顶部挂件。

**Effort**：3 天。

---

#### 12.4 Semantic Search（Bedrock Titan Embeddings + 向量库）

**场景**：当前 Bleve 是关键词匹配。加一层语义层："和这个决策类似的历史决策"、"和这个 alert 类似的过往 incident"、"帮我找所有关于 Cognito 坑的笔记"（模糊意图）。

**复用**：已有 Ingest 流水线（插入同步的 embedding 计算）、Twin（RAG 场景的首个用户）、Vault/Wiki/Decision 为语料。

**新增**：`infra/embeddings/bedrock_titan.go`（调 `amazon.titan-embed-text-v2:0`）；`infra/vector/sqlite_vss.go`（sqlite-vss 插件，< 100MB 向量走 sqlite 即可，不上 OpenSearch）；`domain/knowledge/semantic.go` 和 `SearchPort` 并列。

**Effort**：4-5 天。

---

#### 12.5 Engineering Rigor 基线（见 §13）

这不是"功能"是"地基"，但 P0，和 Phase 2 同步做。

---

### P1 — 下一阶段

#### 12.6 Cost / Token Observability

按 session / 按模型 / 按日周月的 token 成本。预算告警（跨阈值推通知）。区分 Opus/Sonnet/Haiku 单价。

**复用**：已有 per-session cost 字段（近期 commit）；Cron；Notification。  
**新增**：`domain/workspace/cost/` 聚合 + dashboard "Cost" 子视图。  
**Effort**：2 天。

---

#### 12.7 Incident Timeline（事件时间线）

所有 alerts / patrol 结果 / approval 决策 / session 关键事件 按时间轴展示。和 Graph 互补（Graph 看"知识关联"，Timeline 看"时间因果"）。

**复用**：Alert Webhook、Patrol、Approval、Session 事件。  
**新增**：`domain/workspace/timeline/` 事件存储（append-only JSON log + 按月分片）；`js/views/timeline.js`。  
**Effort**：3 天。

---

#### 12.8 Smart Mentions / Context Injection

Chat 输入框支持 `@wiki:page-name`、`@session:key`、`@decision:id`、`@bookmark:tag`。发送时后端把引用内容注入 prompt，形成标准"上下文组装"。

**复用**：Search、Vault、Session Store。  
**新增**：`js/views/chat.js` autocomplete；后端 `ContextAssembler`；token 预算裁剪。  
**Effort**：3 天。

---

#### 12.9 Session Templates / Prompt Library

可复用 prompt 模板（"做 AWS 架构 review"、"写 TAP 规划"、"写 ADR"）。模板从 vault markdown 加载（frontmatter 定义 input schema），dashboard 一键起会话。

**复用**：Vault。  
**新增**：`domain/session/template/`；dashboard "Templates" 子面板。  
**Effort**：2-3 天。

---

#### 12.10 Vault Git Sync

Vault 自动 `git commit` + `push` 到私有 GitHub 仓库，跨设备同步 + 版本历史 + Obsidian 本地客户端互通。

**复用**：Vault 文件系统。  
**新增**：`infra/git/`；Cron 触发 sync；冲突走 "last-writer-wins + 保留冲突文件"。  
**Effort**：2 天。

---

### P2 — 探索级

#### 12.11 Multi-modal Capture（截图 OCR + 相机直出）

PWA 相机 API 拍图 → 上传 → Bedrock Claude Vision OCR + 抽 action items → 入 vault。  
**Effort**：估 3-4 天。

#### 12.12 Offline-first PWA

Service Worker precache + IndexedDB 缓存 vault/sessions；离线可读、可记；联网合并。  
**Effort**：3 天。

#### 12.13 LiteLLM / Bedrock 可观测性 + Fallback

调用延迟/错误率监控，Opus 超时自动降级到 Sonnet。`/debug/model-metrics`。  
**Effort**：2-3 天。

#### 12.14 Privacy Lock

Session/Note 标记 `private`，不进 search index，UI 模糊化，二次认证可见。  
**Effort**：2 天。

#### 12.15 Shared Workspaces（多人协作）

**Out of scope by explicit decision**（§9 已拒）。记录在此仅作 future reference。

---

### 功能优先级可视

```
         High Impact
              ↑
12.1 ⌘K  ────── 12.3 Digest  ─── 12.4 Semantic Search
              │                         
12.8 Mentions  │   12.6 Cost     12.7 Timeline
              │                         
12.2 Capture  ─┼─── 12.9 Templates   12.10 Git Sync
              │                         
              │                 12.11 Multimodal
              │                 12.12 Offline
              │                 12.13 Model Obs
              └──────────────────────→ Low Impact
              Low Effort              High Effort
```

P0 四件套（⌘K / Capture / Digest / Semantic Search）合计 12-15 天，紧挨 Phase 4 之后进入 **Phase 5 — 能力扩展**。

---

## 13. Engineering Rigor（贯穿 Phase 2-3-5 的横切规范）

重构不加码严谨性就是白重构。以下规则在 Phase 2 落地，新代码必须遵守，老代码搬迁时"路过就修"。

### 13.1 Context 传递

**规则**：domain service 和 infra adapter 的**所有** I/O 方法首参 `ctx context.Context`；传递用户、trace、deadline、cancellation。HTTP handler 用 `r.Context()` 作为根 ctx。

**检查**：`grep -rn "func .*)(...)" internal/domain/ | grep -v "ctx context.Context"` 应近空。

### 13.2 领域错误

**规则**：每个 domain 包定义 sentinel errors：

```go
// domain/session/errors.go
var (
    ErrSessionNotFound     = errors.New("session not found")
    ErrSessionAlreadyExists = errors.New("session already exists")
    ErrNodeUnreachable     = errors.New("node unreachable")
)
```

Handler 用 `errors.Is` 映射 HTTP 状态：

```go
switch {
case errors.Is(err, session.ErrSessionNotFound): http.Error(w, "not found", 404)
case errors.Is(err, session.ErrNodeUnreachable): http.Error(w, "gateway", 502)
default: http.Error(w, "internal", 500)
}
```

禁止 handler 做字符串匹配 `strings.Contains(err.Error(), "not found")`。

### 13.3 结构化日志

**规则**：统一 `log/slog`，每条日志带 `request_id`、`session_key`（若有）、`user`（若有）。中间件注入 `request_id` 到 ctx，logger 从 ctx 取。

```go
func LoggerFromCtx(ctx context.Context) *slog.Logger {
    if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok { return l }
    return slog.Default()
}
```

### 13.4 输入验证

**规则**：handler 入口统一 `decodeAndValidate[T any](r *http.Request) (T, error)`，T 有 `Validate()` 方法。禁止裸 `json.NewDecoder(r.Body).Decode(&v)` 后直接用。

### 13.5 原子写入

**规则**：所有 JSON 持久化走统一 `atomicWriteJSON(path, v)`：写 `.tmp` → `fsync` → `rename`。`bookmark.go` 已有实现，提升到 `infra/store/atomic.go`。

### 13.6 并发模型

**规则**：

- 每个 store 一把 `sync.RWMutex`，**方法内**加锁，**不**返回持锁对象；
- 大临界区要拆（如 "全量索引 + 单条更新" 分两把锁）；
- goroutine 必须带 recover 中间件，panic 不能杀进程；
- 所有后台 worker 收到 ctx.Done 必须 return，测试用 `go test -race`。

### 13.7 依赖注入

**规则**：`cmd/naozhi/main.go` 保持"显式装配"，不引入 wire/fx；抽出 `internal/app/wire.go` 把 "构造 + 注入" 集中。Domain service 只依赖**接口**，infra 实现注入。

### 13.8 可观测性

**规则**：

- `/health` 保留（现有）；
- 新增 `/health/ready` — 依赖探活（vault 可读、store 可写、LiteLLM 可连）；
- 新增 `/metrics` — 暴露 Prometheus 格式：
  - `naozhi_http_requests_total{method,path,status}`
  - `naozhi_session_active`
  - `naozhi_tokens_total{model}`
  - `naozhi_cli_spawn_duration_seconds`（histogram）
- 新增 `/debug/pprof/*`（仅监听内网 SG）。

### 13.9 测试

**规则**：

- Domain 层覆盖率 > 80%（`go test -cover`）；
- HTTP handler 走 `httptest` 集成测试，覆盖成功 + 4xx + 5xx；
- `go test -race ./...` 无竞争告警；
- 关键 happy path 写 contract test（特别是跨节点 routing）。

### 13.10 配置校验与 fail-fast

**规则**：启动时 `config.Validate()` — 缺 naozDir、LiteLLM URL 不可达、vault dir 不存在等 → 进程退出 exit 2。不允许带病启动。

### 13.11 Secrets

**规则**：

- 当前阶段：`ANTHROPIC_API_KEY` 等保留在 `/root/.naozhi/claude-settings.json`（文件权限 0600）；
- P1 阶段：改从 AWS Secrets Manager 读，启动加载；
- 禁止日志打印含 key 的 env dump（grep 兜底）。

### 13.12 Rate Limiting

**规则**：`/api/transcribe/upload`、`/api/capture`、`/api/meetings/upload`（已删）等大 body 端点，按源 IP + Cognito user 限流（`golang.org/x/time/rate`，单进程 token bucket 即可）。

---

## 14. 下一步

本 spec 由用户 review 后，进入 **writing-plans** 流程：

- **Plan 0** — Meeting 下线（Phase 0）— 立即开工
- **Plan 1** — 前端拆分（Phase 1）
- **Plan 2** — 后端 DDD + Engineering Rigor 基线（Phase 2 + §13）
- **Plan 3** — server 瘦身 + 性能 polish（Phase 3 + 4）
- **Plan 5** — P0 新功能四件套（§12.1-12.4）

Plan 0 风险最低，可作为整个重构的热身。
