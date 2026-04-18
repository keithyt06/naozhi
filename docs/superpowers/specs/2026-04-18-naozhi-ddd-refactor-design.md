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

- 删除 `internal/knowledge/meeting.go`、`internal/knowledge/meeting_processor.go`
- 删除 `internal/server/dashboard_meeting.go`
- 清理 `internal/server/server.go` / `dashboard.go` 的注册
- 删除 `dashboard.html` 中 Meeting 相关 CSS（~930-1001 行段）、nav 按钮、`renderMeetingView`、`loadMeeting*`、`handleMeetingUpload` 函数
- 清理 EC2 上 `/root/.naozhi/meetings.json` 和 `meetings/` 目录
- 删除 docs 里 Meeting 相关描述
- **验证**：`go build`、`go test ./...`、手机访问 dashboard 切视图不报错

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

## 11. 下一步

本 spec 由用户 review 后，进入 **writing-plans** 流程，把 Phase 0-4 展开成可执行的实施 plan（预计 4 个独立 plan 文档，每个 phase 一个）。Phase 0 可以当天开工（风险最低、收益立竿见影：减代码、去负担）。
