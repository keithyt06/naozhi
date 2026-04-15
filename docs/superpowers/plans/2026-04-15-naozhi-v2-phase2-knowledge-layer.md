# Naozhi V2.0 Phase 2: Knowledge Layer 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标:** 实现 Naozhi V2.0 知识系统的完整后端与前端，将 Naozhi 从"对话工具"升级为"知识操控台"。包含 Obsidian vault 浏览、Wiki 知识编译、bleve 全文检索、CLI session 同步、Bookmark 系统，以及 Home 仪表板和 Context Panel 前端视图。

**架构定位:** 新增 `internal/knowledge` 包，平行于现有 `internal/cron`、`internal/discovery`，不侵入已有模块的接口。`knowledge` 包通过 `server` 包注册 REST API 路由，复用 `cron/store.go` 的原子写入模式做 JSON 持久化，复用 `discovery` 包的 JSONL 解析能力做 CLI session 同步。

**技术栈:** Go stdlib + `goldmark` (CommonMark 渲染) + `bleve` (全文索引) + 原生 JavaScript (Dashboard 前端)。

**Spec 文档:** `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md` (Section 4: Knowledge System)

**工期:** 3-4 周，16 个任务，预估总新增 ~5,200 LOC (Go ~3,200 + JS/HTML ~2,000)。

---

## 模块依赖图 (新增部分)

Phase 2 在现有模块依赖树上新增 `knowledge` 分支。`knowledge` 包不依赖 `platform`、`cli`、`session` 等核心模块，仅被 `server` 包消费，保持依赖方向单一：

```
cmd/naozhi/main.go
  -> config        (已有)
  -> cli           (已有)
  -> session       (已有)
  -> platform      (已有)
  -> server        (已有, 扩展路由注册)
  |    -> knowledge/vault       新增: Obsidian vault 读取 + goldmark 渲染
  |    -> knowledge/wiki        新增: Wiki 编译页 CRUD
  |    -> knowledge/ingest      新增: Ingest 编排 (调用 CLI 子进程)
  |    -> knowledge/lint        新增: Lint 健康检查
  |    -> knowledge/bookmark    新增: Bookmark CRUD + 标签
  |    -> knowledge/search      新增: bleve 索引管理
  -> cron           (已有)
  -> discovery      (已有, 扩展 history.jsonl 扫描)
  -> project        (已有)
  -> connector      (已有)
  -> ...
```

关键设计约束：
- `knowledge` 包内各子模块之间可互相引用 (同属一个 Go package)，但整体只被 `server` 层消费
- 所有数据存储于 `~/.naozhi/` 下，与现有 `sessions.json`、`cron.json` 并列，互不干扰
- goldmark 渲染在 Go 侧完成，返回 HTML 给前端，避免前端引入重量级 Markdown 库

---

## 文件结构总览

### 新建文件

| 文件 | 职责 | 预估 LOC |
|------|------|---------|
| `internal/knowledge/vault.go` | Obsidian vault 配置加载、Markdown 文件读取、goldmark 渲染管线 | ~280 |
| `internal/knowledge/vault_tree.go` | Vault 目录树扫描、JSON 序列化、缓存 | ~180 |
| `internal/knowledge/wiki.go` | Wiki 编译页 CRUD (读取/列表 `~/.naozhi/wiki/*.md`) | ~200 |
| `internal/knowledge/ingest.go` | Ingest 编排引擎 (构造 prompt、调用 CLI 子进程) | ~220 |
| `internal/knowledge/lint.go` | Lint 健康检查 (矛盾/过期/孤立页检测) | ~250 |
| `internal/knowledge/bookmark.go` | Bookmark 数据结构 + CRUD + 标签管理 | ~180 |
| `internal/knowledge/store.go` | bookmarks.json 持久化 (复用 cron/store 原子写入模式) | ~100 |
| `internal/knowledge/search.go` | bleve 索引初始化、文档写入、查询接口 | ~350 |
| `internal/server/dashboard_knowledge.go` | Knowledge REST API handlers (vault + wiki + bookmark + search + ingest + lint) | ~400 |
| `internal/knowledge/vault_test.go` | Vault 渲染 + 目录树单元测试 | ~200 |
| `internal/knowledge/search_test.go` | bleve 索引/查询单元测试 | ~150 |
| `internal/knowledge/bookmark_test.go` | Bookmark CRUD 单元测试 | ~120 |

### 修改文件

| 文件 | 变更内容 | 预估 Delta |
|------|---------|-----------|
| `internal/server/server.go` | 新增 `knowledgeH *KnowledgeHandlers` 字段，在 `New()` 中初始化，在 `registerDashboard()` 中注册路由 | +40 |
| `internal/server/dashboard.go` | 在 `registerDashboard()` 中追加 knowledge 相关路由注册 | +30 |
| `internal/discovery/scanner.go` | 扩展 `history.jsonl` 定期扫描，提供 `ScanHistory()` 导出函数 | +80 |
| `internal/server/static/dashboard.html` | 新增 Knowledge/Wiki/Home 视图、Context Panel、Bookmark UI、Search 集成 | +2,000 |
| `go.mod` | 新增 `github.com/yuin/goldmark` + 扩展 + `github.com/blevesearch/bleve/v2` | +6 |
| `config.yaml` / `internal/config/config.go` | 新增 `knowledge` 配置段 | +30 |

---

## Task 1: Obsidian Vault 文件树 API

**描述:** 实现 vault 目录树的扫描与 JSON 序列化。Dashboard 的 Knowledge 视图左侧需要展示 Obsidian vault 的文件树结构，支持目录折叠展开。目录树在内存中缓存，每 60 秒刷新一次（复用 `project.Manager` 的定期扫描模式）。

**新建文件:**
- `internal/knowledge/vault_tree.go`

**预估 LOC:** ~180

**依赖:** 无外部依赖，仅依赖 Go stdlib (`os`, `filepath`, `sort`, `sync`, `time`)

**struct 定义与函数签名:**

```go
// internal/knowledge/vault_tree.go
package knowledge

import (
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
    "time"
)

// VaultConfig 持有 Obsidian vault 的路径与过滤规则。
// 由 config.yaml 的 knowledge.obsidian 段加载。
type VaultConfig struct {
    VaultPath    string   `yaml:"vault_path"`
    IncludePaths []string `yaml:"include_paths"` // e.g., ["Things/", "page/"]
    ExcludePaths []string `yaml:"exclude_paths"` // e.g., [".obsidian/", "assets/"]
}

// TreeNode 表示文件树中的一个节点（目录或文件）。
type TreeNode struct {
    Name     string      `json:"name"`
    Path     string      `json:"path"`     // 相对于 vault root 的路径
    Type     string      `json:"type"`     // "dir" or "file"
    Size     int64       `json:"size,omitempty"`
    ModTime  int64       `json:"mod_time,omitempty"` // unix ms
    Children []*TreeNode `json:"children,omitempty"`
}

// VaultTree 管理 vault 目录树的扫描与缓存。
type VaultTree struct {
    cfg       VaultConfig
    mu        sync.RWMutex
    root      *TreeNode
    lastScan  time.Time
    scanTTL   time.Duration // 默认 60s
}

// NewVaultTree 创建目录树管理器。
func NewVaultTree(cfg VaultConfig) *VaultTree

// Scan 执行一次完整的目录树扫描，更新内存缓存。
// 遵循 include_paths / exclude_paths 过滤规则。
func (vt *VaultTree) Scan() error

// Tree 返回当前缓存的目录树。如果缓存过期则触发 Scan。
func (vt *VaultTree) Tree() (*TreeNode, error)

// isExcluded 检查路径是否被排除。
func (vt *VaultTree) isExcluded(relPath string) bool

// buildTree 递归构建目录树节点。
func (vt *VaultTree) buildTree(absPath, relPath string) (*TreeNode, error)
```

**与现有模块的集成:**

- 缓存刷新模式复用 `project.Manager.Scan()` 的设计：`Tree()` 检查 `lastScan + scanTTL` 是否过期，过期则调用 `Scan()`
- 排序规则：目录在前，文件在后，各自按名称字母序
- 仅扫描 `.md` 文件和目录，忽略 `.obsidian/`、图片等非 Markdown 文件

**验收标准:**
- [ ] `VaultTree.Scan()` 能正确扫描指定 vault 路径下的 `.md` 文件和目录
- [ ] `include_paths` 和 `exclude_paths` 过滤规则生效
- [ ] 目录树 JSON 结构正确：目录在前、文件在后，嵌套层级正确
- [ ] 缓存 TTL 生效：60 秒内重复调用 `Tree()` 不触发磁盘扫描
- [ ] 不存在的 vault 路径返回明确错误，不 panic

---

## Task 2: Obsidian Markdown 渲染引擎 (goldmark)

**描述:** 使用 `goldmark` 库在 Go 侧将 Obsidian Markdown 渲染为 HTML。需要支持 Obsidian 特有语法：`[[wikilinks]]`、`> [!tip]` callout、YAML frontmatter、GFM tables、task checkboxes。渲染结果直接返回给前端，前端无需引入 Markdown 解析库。

**新建文件:**
- `internal/knowledge/vault.go`

**预估 LOC:** ~280

**新增依赖 (go.mod):**
- `github.com/yuin/goldmark` — CommonMark 渲染核心
- `github.com/yuin/goldmark-meta` — YAML frontmatter 解析
- `github.com/13rac1/goldmark-wikilink` — `[[wikilink]]` 语法支持 (如无合适库则自行实现 AST 扩展)
- `github.com/yuin/goldmark/extension` — GFM tables, strikethrough, task list

**struct 定义与函数签名:**

```go
// internal/knowledge/vault.go
package knowledge

import (
    "bytes"
    "os"
    "path/filepath"
    "strings"

    "github.com/yuin/goldmark"
    "github.com/yuin/goldmark/extension"
    "github.com/yuin/goldmark/parser"
    "github.com/yuin/goldmark/renderer/html"
)

// VaultReader 负责 vault 文件的读取与渲染。
type VaultReader struct {
    cfg VaultConfig
    md  goldmark.Markdown // 预配置的 goldmark 渲染管线
}

// NewVaultReader 创建 vault 文件阅读器，初始化 goldmark 管线。
// goldmark 管线配置:
//   - GFM extensions (table, strikethrough, task list)
//   - YAML frontmatter
//   - Wikilink 自定义扩展 (渲染为 <a data-wiki="target">)
//   - Callout 自定义扩展 (渲染为 <div class="callout callout-{type}">)
//   - Unsafe HTML rendering (允许嵌入 HTML)
func NewVaultReader(cfg VaultConfig) *VaultReader

// ReadRendered 读取 vault 中的 .md 文件，渲染为 HTML 返回。
// path 是相对于 vault root 的路径。
// 返回渲染后的 HTML string 和 frontmatter metadata map。
func (vr *VaultReader) ReadRendered(relPath string) (htmlContent string, meta map[string]interface{}, err error)

// ReadRaw 读取原始 Markdown 内容。
func (vr *VaultReader) ReadRaw(relPath string) ([]byte, error)

// validatePath 校验路径安全性 (防止 path traversal 攻击)。
// 确保解析后的绝对路径仍在 vault root 下。
func (vr *VaultReader) validatePath(relPath string) (string, error)

// --- Callout 自定义扩展 ---

// calloutTransformer 是 goldmark AST transformer，
// 将 > [!type] 格式的 blockquote 转换为 callout 节点。
type calloutTransformer struct{}

// calloutRenderer 将 callout 节点渲染为带 CSS class 的 <div>。
type calloutRenderer struct{}
```

**goldmark 管线配置细节:**

```go
md := goldmark.New(
    goldmark.WithExtensions(
        extension.GFM,           // Table, Strikethrough, TaskList, Autolink
        // frontmatter 扩展
        // wikilink 自定义扩展
    ),
    goldmark.WithParserOptions(
        parser.WithAutoHeadingID(),
    ),
    goldmark.WithRendererOptions(
        html.WithUnsafe(), // Obsidian 笔记中可能有内嵌 HTML
    ),
)
```

**Wikilink 渲染策略:**

Obsidian 的 `[[target|display]]` 语法渲染为:
```html
<a href="#" data-wiki="target" class="wiki-link">display</a>
```
前端 JavaScript 拦截 `data-wiki` 属性的点击事件，调用 `/api/vault/read?path=target.md` 加载目标文件。如果找不到 wikilink 目标，CSS 标记为红色断链 (`class="wiki-link broken"`)。

**Callout 渲染策略:**

Obsidian 的 `> [!tip] Title` 语法渲染为:
```html
<div class="callout callout-tip">
  <div class="callout-title">Tip: Title</div>
  <div class="callout-content">...</div>
</div>
```
支持的 callout 类型：`tip`, `note`, `warning`, `danger`, `info`, `example`, `quote`, `abstract`, `todo`, `success`, `question`, `failure`, `bug`。

**验收标准:**
- [ ] 标准 Markdown (headings, lists, code blocks, links, images) 正确渲染
- [ ] GFM tables 渲染为 `<table>` 并支持对齐
- [ ] Task checkboxes 渲染为 `<input type="checkbox" disabled>`
- [ ] `[[wikilink]]` 和 `[[target|display]]` 渲染为带 `data-wiki` 属性的链接
- [ ] `> [!tip]` callout 渲染为带正确 CSS class 的 `<div>`
- [ ] YAML frontmatter 解析并以 metadata map 形式返回
- [ ] path traversal 攻击 (`../../etc/passwd`) 被拦截
- [ ] 不存在的文件返回明确错误

---

## Task 3: Vault REST API 路由

**描述:** 在 `server` 包中注册 Obsidian vault 的三个 REST API 端点，复用 `dashboard.go` 中的 handler 注册模式。所有端点需要经过 auth middleware 保护。

**新建文件:**
- `internal/server/dashboard_knowledge.go` (本 task 仅实现 vault 相关 handlers，后续 task 追加)

**修改文件:**
- `internal/server/server.go` — 新增 `knowledgeH` 字段
- `internal/server/dashboard.go` — 在 `registerDashboard()` 中注册路由

**预估 LOC:** ~120 (本 task 部分)

**函数签名:**

```go
// internal/server/dashboard_knowledge.go
package server

import (
    "net/http"

    "github.com/naozhi/naozhi/internal/knowledge"
)

// KnowledgeHandlers 聚合知识系统的所有 HTTP handler。
type KnowledgeHandlers struct {
    vaultTree   *knowledge.VaultTree
    vaultReader *knowledge.VaultReader
    wikiMgr     *knowledge.WikiManager    // Task 4
    bookmarkMgr *knowledge.BookmarkStore  // Task 7
    searchIdx   *knowledge.SearchIndex    // Task 8
    ingestEng   *knowledge.IngestEngine   // Task 5
    lintEng     *knowledge.LintEngine     // Task 6
}

// --- Vault API ---

// handleVaultTree 返回 vault 目录树 JSON。
// GET /api/vault/tree
func (h *KnowledgeHandlers) handleVaultTree(w http.ResponseWriter, r *http.Request)

// handleVaultRead 渲染 vault 中的 .md 文件为 HTML。
// GET /api/vault/read?path=Things/some-note.md
// 响应: {"html": "<h1>...</h1>", "meta": {"title": "...", "tags": [...]}}
func (h *KnowledgeHandlers) handleVaultRead(w http.ResponseWriter, r *http.Request)

// handleVaultRaw 返回 vault 中 .md 文件的原始 Markdown。
// GET /api/vault/raw?path=Things/some-note.md
// 响应: text/markdown
func (h *KnowledgeHandlers) handleVaultRaw(w http.ResponseWriter, r *http.Request)
```

**路由注册 (在 `dashboard.go` 的 `registerDashboard()` 中追加):**

```go
// Knowledge API routes
if s.knowledgeH != nil {
    s.mux.HandleFunc("GET /api/vault/tree", auth(s.knowledgeH.handleVaultTree))
    s.mux.HandleFunc("GET /api/vault/read", auth(s.knowledgeH.handleVaultRead))
    s.mux.HandleFunc("GET /api/vault/raw", auth(s.knowledgeH.handleVaultRaw))
}
```

**复用的现有模式:**

参照 `dashboard.go` 中 `s.filesH`、`s.cronH` 的注册方式：
1. 在 `Server` struct 中添加 `knowledgeH *KnowledgeHandlers` 字段
2. 在 `New()` 构造函数中，根据 config 是否配置了 `knowledge.obsidian.vault_path` 来决定是否初始化
3. `registerDashboard()` 中用 `if s.knowledgeH != nil` 守卫避免 nil pointer

**server.go 变更:**

```go
// Server struct 新增字段
knowledgeH *KnowledgeHandlers

// New() 中初始化 (在 return 之前)
if cfg.Knowledge.Obsidian.VaultPath != "" {
    vaultCfg := knowledge.VaultConfig{
        VaultPath:    cfg.Knowledge.Obsidian.VaultPath,
        IncludePaths: cfg.Knowledge.Obsidian.IncludePaths,
        ExcludePaths: cfg.Knowledge.Obsidian.ExcludePaths,
    }
    s.knowledgeH = &KnowledgeHandlers{
        vaultTree:   knowledge.NewVaultTree(vaultCfg),
        vaultReader: knowledge.NewVaultReader(vaultCfg),
    }
}
```

**验收标准:**
- [ ] `GET /api/vault/tree` 返回正确的目录树 JSON
- [ ] `GET /api/vault/read?path=...` 返回渲染后的 HTML + frontmatter metadata
- [ ] `GET /api/vault/raw?path=...` 返回原始 Markdown (Content-Type: text/markdown)
- [ ] 未配置 vault_path 时，API 路由不注册，不影响现有功能
- [ ] 路径校验生效：path traversal 返回 400
- [ ] 未认证请求返回 401 (经过 `auth` middleware)

---

## Task 4: Wiki 页面管理

**描述:** 实现 Wiki 编译页的 CRUD 操作。Wiki 页面存储在 `~/.naozhi/wiki/*.md`，每个页面是一个带 YAML frontmatter 的 Markdown 文件。frontmatter 中记录编译时间、来源数量、实体列表等元数据。WikiManager 提供列表、读取、创建/更新接口，供 Ingest 引擎和 Dashboard 使用。

**新建文件:**
- `internal/knowledge/wiki.go`

**修改文件:**
- `internal/server/dashboard_knowledge.go` — 追加 wiki handler

**预估 LOC:** ~200 (wiki.go) + ~80 (handlers)

**struct 定义与函数签名:**

```go
// internal/knowledge/wiki.go
package knowledge

import (
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"
)

// WikiPage 表示一个编译后的 Wiki 页面。
type WikiPage struct {
    Name       string                 `json:"name"`        // 文件名 (不含 .md)
    Title      string                 `json:"title"`       // frontmatter title
    Category   string                 `json:"category"`    // Projects / Services / Customers
    CompiledAt string                 `json:"compiled_at"` // ISO 8601
    Sources    int                    `json:"sources"`     // 来源数量
    Entities   []string               `json:"entities"`    // 提及的实体列表
    Tags       []string               `json:"tags"`
    Size       int64                  `json:"size"`        // 文件大小
    ModTime    int64                  `json:"mod_time"`    // unix ms
    Meta       map[string]interface{} `json:"meta,omitempty"` // 完整 frontmatter
}

// WikiManager 管理 ~/.naozhi/wiki/ 目录下的编译页面。
type WikiManager struct {
    wikiDir string // ~/.naozhi/wiki/
    md      goldmark.Markdown
}

// NewWikiManager 创建 Wiki 管理器。
// wikiDir 默认为 ~/.naozhi/wiki/，不存在则自动创建。
func NewWikiManager(wikiDir string) (*WikiManager, error)

// List 列出所有 wiki 页面的元数据 (不含正文)。
// 遍历 wikiDir 下所有 .md 文件，解析 frontmatter。
func (wm *WikiManager) List() ([]WikiPage, error)

// Read 读取指定 wiki 页面，渲染为 HTML。
// 返回渲染后 HTML + 元数据。
func (wm *WikiManager) Read(name string) (htmlContent string, page WikiPage, err error)

// ReadRaw 返回原始 Markdown。
func (wm *WikiManager) ReadRaw(name string) ([]byte, error)

// Write 创建或更新 wiki 页面 (由 Ingest 引擎调用)。
// 使用原子写入 (tmp + rename) 防止写入中断导致数据损坏。
func (wm *WikiManager) Write(name string, content []byte) error

// Delete 删除 wiki 页面。
func (wm *WikiManager) Delete(name string) error

// parseFrontmatter 解析 .md 文件的 YAML frontmatter 提取元数据。
func parseFrontmatter(data []byte) (map[string]interface{}, []byte, error)
```

**REST API handlers (追加到 dashboard_knowledge.go):**

```go
// GET /api/wiki — 列出所有 wiki 页面
func (h *KnowledgeHandlers) handleWikiList(w http.ResponseWriter, r *http.Request)

// GET /api/wiki/{name} — 读取单个 wiki 页面 (渲染 HTML)
func (h *KnowledgeHandlers) handleWikiRead(w http.ResponseWriter, r *http.Request)
```

**Wiki 页面 frontmatter 格式:**

```yaml
---
title: AWS WAF Best Practices
category: Services
compiled_at: "2026-04-15T10:30:00Z"
sources: 5
entities:
  - AWS WAF
  - CloudFront
  - Rate Limiting
tags:
  - security
  - waf
  - cdn
---
```

**与现有模块的集成:**

- 目录创建与文件读写复用 `cron/store.go` 的 `os.MkdirAll` + `os.WriteFile(tmp) -> os.Rename` 原子写入模式
- `WikiManager` 在 `server.New()` 中初始化，挂载到 `KnowledgeHandlers`
- goldmark 渲染管线复用 Task 2 中 `VaultReader` 的配置 (提取为共享 helper)

**验收标准:**
- [ ] `List()` 遍历 `~/.naozhi/wiki/` 下所有 `.md` 文件并正确解析 frontmatter
- [ ] `Read()` 返回渲染后的 HTML，wikilinks 在 wiki 页面间正确链接
- [ ] `Write()` 使用原子写入，中断不损坏已有文件
- [ ] `GET /api/wiki` 返回按 `compiled_at` 倒序排列的页面列表
- [ ] `GET /api/wiki/{name}` 返回 HTML + 元数据
- [ ] wiki 目录不存在时自动创建

---

## Task 5: Ingest 编排引擎

**描述:** Ingest 引擎负责触发"知识编译"——扫描 raw sources (Dashboard 对话、CLI sessions、Obsidian vault) 中的新内容，调用 Claude CLI 子进程将其编译为结构化 wiki 页面。这是 Karpathy 三层架构中 Layer 1 -> Layer 2 的关键流程。

Ingest 不自行调用 LLM API，而是复用 Naozhi 现有的 CLI 子进程机制：构造 prompt，通过 `session.Router.GetOrCreate()` 获取一个专用 session，发送编译指令。这保证了与现有系统的一致性 (相同的 model 配置、相同的 watchdog 机制)。

**新建文件:**
- `internal/knowledge/ingest.go`

**预估 LOC:** ~220

**struct 定义与函数签名:**

```go
// internal/knowledge/ingest.go
package knowledge

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"
)

// IngestStatus 表示 Ingest 引擎的当前状态。
type IngestStatus string

const (
    IngestIdle    IngestStatus = "idle"
    IngestRunning IngestStatus = "running"
)

// IngestResult 记录一次 Ingest 执行的结果。
type IngestResult struct {
    StartedAt    time.Time `json:"started_at"`
    FinishedAt   time.Time `json:"finished_at"`
    Duration     string    `json:"duration"`
    PagesUpdated int       `json:"pages_updated"`
    PagesCreated int       `json:"pages_created"`
    SourcesRead  int       `json:"sources_read"`
    Error        string    `json:"error,omitempty"`
}

// IngestEngine 编排知识编译流程。
type IngestEngine struct {
    mu          sync.Mutex
    status      IngestStatus
    lastResult  *IngestResult
    wikiDir     string              // ~/.naozhi/wiki/
    vaultCfg    VaultConfig         // Obsidian vault 配置
    sendFn      IngestSendFunc      // 发送 prompt 到 CLI session 的函数
}

// IngestSendFunc 是发送消息到 CLI session 的抽象函数。
// server 包在初始化时注入实际实现 (通过 session.Router)。
// 参数: ctx, prompt string
// 返回: response text, error
type IngestSendFunc func(ctx context.Context, prompt string) (string, error)

// NewIngestEngine 创建 Ingest 引擎。
func NewIngestEngine(wikiDir string, vaultCfg VaultConfig, sendFn IngestSendFunc) *IngestEngine

// Run 执行一次知识编译。
// 1. 扫描 raw sources 中的新增内容 (基于 wiki 页面的 compiled_at 时间戳)
// 2. 构造 ingest prompt (包含新素材 + 现有 wiki 页面列表)
// 3. 调用 sendFn 发送到 CLI session
// 4. 解析 CLI 输出，更新 wiki 页面
// 同一时间只允许一个 Ingest 任务运行 (mu 保护)。
func (ie *IngestEngine) Run(ctx context.Context) (*IngestResult, error)

// Status 返回当前状态和最近一次执行结果。
func (ie *IngestEngine) Status() (IngestStatus, *IngestResult)

// buildIngestPrompt 构造编译 prompt。
// 包含: wiki 目录现有页面列表 + 新增 raw source 摘要 + 编译规则。
func (ie *IngestEngine) buildIngestPrompt(newSources []rawSource) string

// rawSource 表示一条原始素材。
type rawSource struct {
    Source    string `json:"source"`    // "dashboard", "cli", "vault", "im"
    Title     string `json:"title"`
    Content   string `json:"content"`
    Timestamp int64  `json:"timestamp"`
}
```

**与现有模块的集成:**

- `sendFn` 由 `server` 包在初始化时注入，内部通过 `session.Router.GetOrCreate()` 创建专用 session key `"ingest:knowledge"`，调用 `sess.Send()`
- 这与 `cron/scheduler.go` 的 `execute()` 方法模式一致：构造 session key -> GetOrCreate -> Send -> 处理结果
- Ingest session 设置 `opts.Exempt = true`，不计入 `maxProcs` 容量
- CLI 子进程的工作目录设置为 `~/.naozhi/wiki/`，使其可以直接读写 wiki 文件

**REST API handler:**

```go
// POST /api/wiki/ingest — 触发手动 Ingest
func (h *KnowledgeHandlers) handleWikiIngest(w http.ResponseWriter, r *http.Request)

// GET /api/wiki/ingest/status — 查询 Ingest 状态
func (h *KnowledgeHandlers) handleWikiIngestStatus(w http.ResponseWriter, r *http.Request)
```

**验收标准:**
- [ ] `Run()` 能扫描新增 raw sources 并构造合理的 ingest prompt
- [ ] 通过 `sendFn` 正确调用 CLI 子进程执行编译
- [ ] 同一时间只有一个 Ingest 任务运行，重复触发返回 "already running"
- [ ] `POST /api/wiki/ingest` 异步触发 Ingest 并立即返回 status
- [ ] Ingest 完成后 `lastResult` 记录执行统计
- [ ] Ingest session 不计入 `maxProcs` 容量

---

## Task 6: Lint 健康检查引擎

**描述:** Lint 引擎检测 wiki 知识库的健康状态：矛盾信息 (两个页面对同一实体描述不一致)、过期结论 (超过 N 天未被新 source 验证)、孤立页 (没有入链的 wiki 页面)、缺失来源 (编译内容无对应 raw source)。Lint 结果在 Wiki 视图的右侧面板展示。

Lint 的"矛盾信息"检测同样委托给 CLI 子进程 (LLM 判断)，而"孤立页"和"过期"检测则是纯 Go 逻辑。

**新建文件:**
- `internal/knowledge/lint.go`

**预估 LOC:** ~250

**struct 定义与函数签名:**

```go
// internal/knowledge/lint.go
package knowledge

import (
    "context"
    "regexp"
    "strings"
    "sync"
    "time"
)

// LintSeverity 表示 lint 问题的严重程度。
type LintSeverity string

const (
    LintWarn  LintSeverity = "warning"
    LintError LintSeverity = "error"
    LintInfo  LintSeverity = "info"
)

// LintIssue 表示一个 lint 检测到的问题。
type LintIssue struct {
    Type     string       `json:"type"`     // "contradiction", "stale", "orphan", "missing_source"
    Severity LintSeverity `json:"severity"`
    Page     string       `json:"page"`     // 涉及的 wiki 页面名
    Related  string       `json:"related,omitempty"` // 关联的页面 (矛盾检测时)
    Message  string       `json:"message"`
    Detail   string       `json:"detail,omitempty"`
}

// LintResult 记录一次 Lint 执行的结果。
type LintResult struct {
    RunAt    time.Time   `json:"run_at"`
    Duration string      `json:"duration"`
    Issues   []LintIssue `json:"issues"`
    Stats    LintStats   `json:"stats"`
}

// LintStats 统计各类问题的数量。
type LintStats struct {
    TotalPages     int `json:"total_pages"`
    Contradictions int `json:"contradictions"`
    StalePages     int `json:"stale_pages"`
    OrphanPages    int `json:"orphan_pages"`
    MissingSources int `json:"missing_sources"`
}

// LintEngine 执行 wiki 健康检查。
type LintEngine struct {
    mu         sync.Mutex
    wikiDir    string
    lastResult *LintResult
    staleDays  int            // 超过此天数视为过期，默认 30
    sendFn     IngestSendFunc // 复用 Ingest 的 send 机制 (矛盾检测需 LLM)
}

// NewLintEngine 创建 Lint 引擎。
func NewLintEngine(wikiDir string, staleDays int, sendFn IngestSendFunc) *LintEngine

// Run 执行一次完整的 Lint 检查。
// 1. checkOrphans: 扫描所有 wiki 页面的 wikilinks，找出无入链的页面
// 2. checkStale: 检查 frontmatter 中的 compiled_at 是否超过 staleDays
// 3. checkMissingSources: 检查 frontmatter 中的 sources 是否有效
// 4. checkContradictions: (可选, 需 LLM) 将所有页面摘要发送给 CLI 检测矛盾
func (le *LintEngine) Run(ctx context.Context) (*LintResult, error)

// LastResult 返回最近一次 Lint 结果。
func (le *LintEngine) LastResult() *LintResult

// checkOrphans 找出没有任何入链 wikilink 的页面。
func (le *LintEngine) checkOrphans(pages []WikiPage, contents map[string][]byte) []LintIssue

// checkStale 找出 compiled_at 超过 staleDays 的页面。
func (le *LintEngine) checkStale(pages []WikiPage) []LintIssue

// extractWikilinks 从 Markdown 内容中提取 [[wikilink]] 引用。
var wikilinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)
func extractWikilinks(content []byte) []string
```

**REST API handler:**

```go
// POST /api/wiki/lint — 触发 Lint
func (h *KnowledgeHandlers) handleWikiLint(w http.ResponseWriter, r *http.Request)

// GET /api/wiki/lint — 查询最近 Lint 结果
func (h *KnowledgeHandlers) handleWikiLintResult(w http.ResponseWriter, r *http.Request)
```

**验收标准:**
- [ ] `checkOrphans()` 正确识别无入链的 wiki 页面 (除 index/CLAUDE.md 外)
- [ ] `checkStale()` 正确识别 `compiled_at` 超过 staleDays 的页面
- [ ] wikilink 提取正则正确匹配 `[[target]]` 和 `[[target|display]]` 两种格式
- [ ] `POST /api/wiki/lint` 触发异步 Lint 并返回 status
- [ ] `GET /api/wiki/lint` 返回最近一次 Lint 结果 (包含 issues 和 stats)
- [ ] 空 wiki 目录下运行 Lint 不报错

---

## Task 7: Bookmark 系统

**描述:** 实现消息 Bookmark 功能，用户在 Dashboard 中可以保存任意 AI 消息片段到 `~/.naozhi/bookmarks.json`。每条 bookmark 包含来源 session、内容摘要、标签列表。Bookmarks 跨 session 可搜索，展示在 Context Panel 的 Saved tab 中。

**新建文件:**
- `internal/knowledge/bookmark.go`
- `internal/knowledge/store.go`

**预估 LOC:** ~180 (bookmark.go) + ~100 (store.go)

**struct 定义与函数签名:**

```go
// internal/knowledge/bookmark.go
package knowledge

import (
    "strings"
    "time"
)

// Bookmark 表示一个保存的消息片段。
type Bookmark struct {
    ID         string   `json:"id"`          // 16 字符 hex
    SessionKey string   `json:"session_key"` // 来源 session
    Source     string   `json:"source"`      // "dashboard", "cli", "vault", "im"
    Content    string   `json:"content"`     // 保存的文本内容 (truncated)
    Summary    string   `json:"summary"`     // 前 120 字符摘要
    Tags       []string `json:"tags"`        // 用户标签 (如 #security, #waf)
    CreatedAt  int64    `json:"created_at"`  // unix ms
    EventIndex int      `json:"event_index"` // 原始消息在 session 事件流中的索引
}

// BookmarkStore 管理 bookmark 的 CRUD 和持久化。
type BookmarkStore struct {
    mu        sync.RWMutex
    bookmarks map[string]*Bookmark // id -> Bookmark
    storePath string               // ~/.naozhi/bookmarks.json
}

// NewBookmarkStore 创建 bookmark store，从磁盘加载已有数据。
func NewBookmarkStore(storePath string) *BookmarkStore

// Add 添加一个新 bookmark。
func (bs *BookmarkStore) Add(b *Bookmark) error

// Delete 删除一个 bookmark。
func (bs *BookmarkStore) Delete(id string) error

// List 列出所有 bookmarks，支持可选的 tag 过滤。
// 按 created_at 倒序排列。
func (bs *BookmarkStore) List(filterTags []string) []*Bookmark

// ListBySession 列出指定 session 的 bookmarks。
func (bs *BookmarkStore) ListBySession(sessionKey string) []*Bookmark

// Search 在 bookmark content 中搜索关键词。
func (bs *BookmarkStore) Search(query string) []*Bookmark

// save 持久化到磁盘 (原子写入)。
func (bs *BookmarkStore) save() error

// load 从磁盘加载。
func (bs *BookmarkStore) load()
```

**store.go (持久化):**

```go
// internal/knowledge/store.go
package knowledge

// 复用 cron/store.go 的原子写入模式:
// 1. json.MarshalIndent(entries)
// 2. os.WriteFile(path + ".tmp", data, 0600)
// 3. os.Rename(path+".tmp", path)

func saveBookmarks(path string, bookmarks map[string]*Bookmark) error
func loadBookmarks(path string) map[string]*Bookmark
```

**REST API handlers:**

```go
// GET    /api/bookmarks              — 列出 bookmarks (?tags=security,waf&session=...)
// POST   /api/bookmarks              — 创建 bookmark
// DELETE /api/bookmarks/{id}         — 删除 bookmark
func (h *KnowledgeHandlers) handleBookmarkList(w http.ResponseWriter, r *http.Request)
func (h *KnowledgeHandlers) handleBookmarkCreate(w http.ResponseWriter, r *http.Request)
func (h *KnowledgeHandlers) handleBookmarkDelete(w http.ResponseWriter, r *http.Request)
```

**验收标准:**
- [ ] `Add()` 正确生成唯一 ID 并持久化到磁盘
- [ ] `Delete()` 删除后立即从内存和磁盘移除
- [ ] `List()` 支持 tag 过滤，结果按时间倒序
- [ ] `Search()` 在 bookmark content 中做 case-insensitive 子串匹配
- [ ] 持久化使用原子写入 (tmp + rename)，不会因中断导致文件损坏
- [ ] 启动时正确加载已有 bookmarks.json
- [ ] REST API 三个端点功能正确

---

## Task 8: bleve 全文搜索索引

**描述:** 使用 bleve 创建统一全文搜索索引，索引来源包括 Dashboard 事件、CLI history、Obsidian vault、Wiki 编译页、Bookmarks。bleve 是纯 Go 实现 (不需要 CGO)，支持 BM25 排序，索引存储在 `~/.naozhi/search.bleve/`。

**新建文件:**
- `internal/knowledge/search.go`
- `internal/knowledge/search_test.go`

**新增依赖 (go.mod):**
- `github.com/blevesearch/bleve/v2`

**预估 LOC:** ~350 (search.go) + ~150 (test)

**struct 定义与函数签名:**

```go
// internal/knowledge/search.go
package knowledge

import (
    "context"
    "log/slog"
    "sync"
    "time"

    "github.com/blevesearch/bleve/v2"
)

// SearchSource 标识搜索结果的来源。
type SearchSource string

const (
    SourceDashboard SearchSource = "dashboard"
    SourceCLI       SearchSource = "cli"
    SourceVault     SearchSource = "vault"
    SourceWiki      SearchSource = "wiki"
    SourceBookmark  SearchSource = "bookmark"
)

// SearchDoc 是写入 bleve 索引的文档结构。
type SearchDoc struct {
    ID         string       `json:"id"`
    Source     SearchSource `json:"source"`
    Title      string       `json:"title"`
    Content    string       `json:"content"`    // 全文内容 (用于搜索)
    SessionKey string       `json:"session_key,omitempty"`
    Path       string       `json:"path,omitempty"`       // vault 文件路径
    Tags       []string     `json:"tags,omitempty"`
    Timestamp  int64        `json:"timestamp"`             // unix ms
}

// SearchResult 是搜索结果的单条记录。
type SearchResult struct {
    Source     SearchSource `json:"source"`
    Title      string       `json:"title"`
    Match      string       `json:"match"`      // 匹配的文本片段 (高亮)
    Score      float64      `json:"score"`       // BM25 分数
    Path       string       `json:"path,omitempty"`
    SessionKey string       `json:"session_key,omitempty"`
    Timestamp  int64        `json:"timestamp"`
}

// SearchResponse 是搜索 API 的响应。
type SearchResponse struct {
    Query    string         `json:"query"`
    Total    int            `json:"total"`
    Duration string         `json:"duration"` // e.g., "12ms"
    Results  []SearchResult `json:"results"`
}

// SearchIndex 管理 bleve 全文搜索索引。
type SearchIndex struct {
    mu        sync.RWMutex
    index     bleve.Index
    indexPath string // ~/.naozhi/search.bleve/
}

// NewSearchIndex 打开或创建 bleve 索引。
// 索引 mapping 配置:
//   - content 字段: text 类型, 启用分词 (standard analyzer)
//   - title 字段: text 类型, 权重 boost 2.0
//   - source 字段: keyword 类型 (精确过滤)
//   - tags 字段: keyword 类型
//   - timestamp 字段: datetime 类型 (范围查询)
func NewSearchIndex(indexPath string) (*SearchIndex, error)

// Index 索引一个文档 (创建或更新)。
func (si *SearchIndex) Index(doc SearchDoc) error

// IndexBatch 批量索引文档 (用于初始化导入)。
func (si *SearchIndex) IndexBatch(docs []SearchDoc) error

// Search 执行全文搜索。
// source 为空表示搜索所有来源，否则按来源过滤。
// limit 默认 20。
func (si *SearchIndex) Search(query string, source SearchSource, limit int) (*SearchResponse, error)

// Delete 从索引中删除文档。
func (si *SearchIndex) Delete(id string) error

// DocCount 返回索引中的文档总数。
func (si *SearchIndex) DocCount() (uint64, error)

// Close 关闭索引 (用于 graceful shutdown)。
func (si *SearchIndex) Close() error

// buildMapping 构建 bleve 索引 mapping。
func buildMapping() *mapping.IndexMappingImpl
```

**索引初始化策略:**

首次启动时，需要将已有的 wiki 页面和 vault 文件导入索引。采用"启动时异步批量索引"策略：

```go
// 在 server.Start() 中异步初始化索引
go func() {
    // 1. 索引 wiki 页面
    wikiPages, _ := wikiMgr.List()
    for _, page := range wikiPages {
        content, _ := wikiMgr.ReadRaw(page.Name)
        searchIdx.Index(SearchDoc{
            ID:      "wiki:" + page.Name,
            Source:  SourceWiki,
            Title:   page.Title,
            Content: string(content),
            Tags:    page.Tags,
            Timestamp: page.ModTime,
        })
    }
    // 2. 索引 vault 文件 (扫描文件树)
    // 3. 索引 bookmarks
    slog.Info("search index initialized", "docs", searchIdx.DocCount())
}()
```

**与现有模块的集成:**

- `SearchIndex` 在 `server.New()` 中初始化，挂载到 `KnowledgeHandlers`
- 在 `server.Start()` 的 context cancel 时调用 `searchIdx.Close()` (graceful shutdown)
- Bookmark 的 Add/Delete 操作同步更新搜索索引
- Wiki 的 Write/Delete 操作同步更新搜索索引

**验收标准:**
- [ ] bleve 索引正确创建在 `~/.naozhi/search.bleve/`
- [ ] `Index()` 和 `IndexBatch()` 正确写入文档
- [ ] `Search()` 返回按 BM25 score 排序的结果
- [ ] source 过滤生效：指定 source 只返回该来源的结果
- [ ] 搜索延迟 < 50ms (1000 文档规模)
- [ ] `Close()` 正确关闭索引，不丢失数据
- [ ] 索引目录不存在时自动创建

---

## Task 9: 统一搜索 API

**描述:** 实现 `/api/search` 端点，聚合 bleve 索引中的多来源搜索结果。前端通过 Cmd+K 全局搜索调用此 API，结果按来源分组展示。

**修改文件:**
- `internal/server/dashboard_knowledge.go` — 追加 search handler

**预估 LOC:** ~80

**函数签名:**

```go
// GET /api/search?q=WAF+误拦截&source=all&limit=20
//
// 响应:
// {
//   "query": "WAF 误拦截",
//   "total": 12,
//   "duration": "8ms",
//   "results": [
//     {"source": "bookmark", "title": "WAF Rate Limit 调优", "match": "...误拦截率 3.2%→0.4%...", "score": 0.95},
//     {"source": "wiki", "title": "AWS WAF Best Practices", "match": "...排除列表...", "score": 0.75},
//     ...
//   ]
// }
func (h *KnowledgeHandlers) handleSearch(w http.ResponseWriter, r *http.Request)
```

**参数说明:**
- `q` (required): 搜索关键词
- `source` (optional): 过滤来源，`all`(默认) / `dashboard` / `cli` / `vault` / `wiki` / `bookmark`
- `limit` (optional): 返回条数，默认 20，最大 100

**与现有模块的集成:**

- 路由注册: `s.mux.HandleFunc("GET /api/search", auth(s.knowledgeH.handleSearch))`
- handler 内部直接调用 `h.searchIdx.Search(query, source, limit)`
- 空查询返回 400，未初始化 searchIdx 时返回 503

**验收标准:**
- [ ] `GET /api/search?q=test` 返回正确的搜索结果
- [ ] source 过滤参数生效
- [ ] limit 参数生效，超出上限时截断到 100
- [ ] 空 query 返回 400 Bad Request
- [ ] 响应中包含 duration 字段，反映实际搜索耗时
- [ ] 结果中的 match 字段包含匹配文本片段

---

## Task 10: CLI Session 同步

**描述:** 扩展现有 `discovery` 包，定期扫描 `~/.claude/history.jsonl` 中的新增 prompt 记录，导入到 bleve 搜索索引。这打通了 CLI 工作和 Dashboard 搜索的数据通道——用户在 CLI 中的工作可以在 Dashboard 的全局搜索中被检索到。

当前 `discovery` 包已有 `Scan()` 函数扫描活跃 CLI 进程和 `LoadHistory()` 函数解析 JSONL 文件。本 task 在此基础上增加 `ScanHistory()` 函数，从 `~/.claude/projects/*/` 下的所有 JSONL 文件中提取 user prompt，导入搜索索引。

**修改文件:**
- `internal/discovery/history.go` — 新增 `ScanHistory()` 导出函数

**新建文件:**
- 无 (逻辑添加到现有文件中)

**预估 LOC:** ~80 (delta)

**函数签名:**

```go
// internal/discovery/history.go (追加)

// HistoryEntry 表示从 CLI JSONL 中提取的一条 prompt 记录。
// 用于导入搜索索引。
type HistoryEntry struct {
    SessionID string `json:"session_id"`
    Project   string `json:"project"`   // 项目目录名 (从路径中提取)
    Prompt    string `json:"prompt"`    // 用户 prompt 文本
    Timestamp int64  `json:"timestamp"` // unix ms
}

// ScanHistory 扫描 claudeDir/projects/ 下所有 JSONL 文件，
// 提取 user prompt 记录，返回比 sinceMs 更新的条目。
//
// 扫描策略:
// 1. 遍历 claudeDir/projects/*/  下所有 .jsonl 文件
// 2. 只处理 mtime > sinceMs 的文件 (增量扫描)
// 3. 每个文件只读取尾部 (与 extractLastPromptUncached 类似的 tail read)
// 4. 提取所有 type=user 的消息
func ScanHistory(claudeDir string, sinceMs int64) ([]HistoryEntry, error)
```

**与现有模块的集成:**

- `ScanHistory()` 复用 `discovery` 包中已有的 `projDirName()`、`extractUserText()` 等 helper
- `server` 包在后台 goroutine 中定期 (60s) 调用 `ScanHistory()`，将结果批量写入 `SearchIndex`
- 扫描间隔复用 `discoveryCache.startLoop()` 的 ticker 模式

**在 server 中的调度:**

```go
// 在 server.Start() 中启动 CLI history 同步 loop
if s.knowledgeH != nil && s.knowledgeH.searchIdx != nil {
    go s.startHistorySyncLoop(ctx)
}

func (s *Server) startHistorySyncLoop(ctx context.Context) {
    var lastScanMs int64
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            entries, err := discovery.ScanHistory(s.claudeDir, lastScanMs)
            if err != nil {
                slog.Warn("cli history scan", "err", err)
                continue
            }
            if len(entries) == 0 {
                continue
            }
            // 批量导入搜索索引
            docs := make([]knowledge.SearchDoc, 0, len(entries))
            for _, e := range entries {
                docs = append(docs, knowledge.SearchDoc{
                    ID:         "cli:" + e.SessionID + ":" + fmt.Sprintf("%d", e.Timestamp),
                    Source:     knowledge.SourceCLI,
                    Title:      e.Project,
                    Content:    e.Prompt,
                    SessionKey: e.SessionID,
                    Timestamp:  e.Timestamp,
                })
            }
            s.knowledgeH.searchIdx.IndexBatch(docs)
            lastScanMs = time.Now().UnixMilli()
            slog.Debug("cli history synced", "entries", len(entries))
        }
    }
}
```

**验收标准:**
- [ ] `ScanHistory()` 正确遍历 `~/.claude/projects/*/` 下的 JSONL 文件
- [ ] 增量扫描生效: 只处理 mtime > sinceMs 的文件
- [ ] 提取的 prompt 文本正确，包含完整用户消息
- [ ] 60 秒定期同步 loop 正常运行，不泄漏 goroutine
- [ ] 同步结果正确写入 bleve 索引，可通过 `/api/search` 检索
- [ ] `~/.claude/` 目录不存在时优雅跳过

---

## Task 11: Knowledge 前端视图 (Obsidian 文件树 + 内容渲染)

**描述:** 在 `dashboard.html` 中实现 Knowledge tab 视图，包含三栏布局: 左侧文件树 + 中间内容渲染区 + 右侧 AI 对话面板。文件树通过 `/api/vault/tree` 加载，点击文件通过 `/api/vault/read` 渲染内容。需要处理 wikilink 点击导航、callout 样式、frontmatter 属性网格等 Obsidian 特色渲染。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~400 (JS + CSS + HTML)

**前端组件结构:**

```
Knowledge View
├── Left Panel: Vault File Tree
│   ├── Vault path display
│   ├── Recursive folder tree (click to expand/collapse)
│   └── File items (click to load content)
├── Center Panel: Content Renderer
│   ├── Breadcrumb navigation
│   ├── Frontmatter properties grid
│   ├── Rendered HTML content (from /api/vault/read)
│   └── Wikilink click handler (intercept data-wiki links)
└── Right Panel: AI Chat (placeholder, Phase 3)
    └── "Ask about this note" button
```

**关键实现细节:**

1. **文件树递归渲染:** 参照现有 session list 的 DOM 操作模式，用原生 JS 递归生成 `<ul>/<li>` 树结构，`<li>` 带 `data-path` 属性
2. **内容区 CSS:** Obsidian-like 排版（正文最大宽度 720px 居中，行高 1.8，代码块等宽字体），callout 用 CSS 变量控制颜色
3. **Wikilink 导航:** 通过事件委托拦截 `a[data-wiki]` 点击，调用 `loadVaultFile(target + ".md")`，更新 URL hash `#knowledge/path/to/file.md`
4. **Frontmatter 属性网格:** 解析 meta 对象，渲染为 key-value 网格 (类似 Obsidian 的 properties 面板)
5. **移动端适配:** `@media (max-width: 768px)` 时文件树全宽展示，点击文件 push 到内容视图 (隐藏文件树)

**验收标准:**
- [ ] Knowledge tab 在导航栏中可点击切换
- [ ] 文件树正确加载 vault 目录结构，支持目录折叠/展开
- [ ] 点击 .md 文件加载并渲染内容
- [ ] Wikilink `[[target]]` 点击后导航到目标文件
- [ ] Callout 块显示为带颜色边框的卡片 (tip=green, warning=yellow, danger=red)
- [ ] Frontmatter 属性以网格形式展示在内容区顶部
- [ ] 移动端布局自适应 (文件树/内容切换)
- [ ] 加载中显示 loading spinner

---

## Task 12: Wiki 前端视图

**描述:** 在 `dashboard.html` 中实现 Wiki tab 视图，展示知识编译页面列表和内容。左侧按 Category 分组展示页面列表，中间渲染 wiki 页面内容 + 编译元数据，右侧展示 Sources 溯源面板和 Lint 状态。提供 Ingest 和 Lint 触发按钮。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~350 (JS + CSS + HTML)

**前端组件结构:**

```
Wiki View
├── Left Panel: Page List
│   ├── Category groups (Projects / Services / Customers)
│   ├── Page items (name + compiled_at + sources count)
│   └── Ingest button (triggers POST /api/wiki/ingest)
├── Center Panel: Page Content
│   ├── Compiled metadata bar (compiled_at, sources, entities)
│   ├── Rendered wiki HTML (from /api/wiki/{name})
│   └── Wikilink navigation (inter-wiki links)
└── Right Panel: Sources & Lint
    ├── Sources tab: 每条知识的来源溯源 (session/CLI/Obsidian)
    ├── Lint tab: Lint issues list (warnings/errors)
    └── Lint trigger button (POST /api/wiki/lint)
```

**关键实现细节:**

1. **页面列表:** 调用 `GET /api/wiki`，按 `category` 字段分组，每组标题可折叠
2. **编译元数据条:** 横向布局: `Compiled: 2h ago | Sources: 5 | Entities: WAF, CloudFront, ...`
3. **Ingest 按钮:** 点击后调用 `POST /api/wiki/ingest`，按钮变为 spinner + "Compiling..." 状态
4. **Lint 面板:** 调用 `GET /api/wiki/lint` 渲染 issue 列表，每条显示 severity badge + page + message

**验收标准:**
- [ ] Wiki tab 在导航栏中可点击切换
- [ ] 页面列表按 Category 分组展示，点击页面加载内容
- [ ] 编译元数据条正确显示 compiled_at (相对时间)、sources 数量、entities 列表
- [ ] Ingest 按钮触发编译并显示进度状态
- [ ] Lint 按钮触发检查并在右侧面板显示结果
- [ ] Wiki 内的 `[[wikilinks]]` 在 wiki 页面间正确导航
- [ ] 空 wiki (无页面) 显示引导信息："Run your first Ingest to compile knowledge"

---

## Task 13: Home 仪表板视图

**描述:** 实现 Home tab 作为 CTO 操控台首页，一眼看到全局状态。包含 stats widgets (4 张统计卡片)、activity feed (全局活动流)、quick actions (快捷按钮)。统计数据从现有 API 聚合 (`/api/sessions`, `/api/cron`, wiki count, bookmark count)。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~350 (JS + CSS + HTML)

**前端组件结构:**

```
Home View
├── Greeting Header: "Good afternoon, Keith" + date
├── Stats Row (4 cards):
│   ├── Active Sessions (from /api/sessions count)
│   ├── Cron Jobs (from /api/cron count)
│   ├── Today's Cost (from sessions cost sum)
│   └── Wiki Pages (from /api/wiki count)
├── Quick Actions Row (4 buttons):
│   ├── New Chat → 跳转 Chat tab + open new session modal
│   ├── Search → 打开 Cmd+K overlay
│   ├── Knowledge → 跳转 Knowledge tab
│   └── Wiki → 跳转 Wiki tab
├── Activity Feed (scrollable list):
│   ├── Session events (user messages, agent completions)
│   ├── Cron execution results
│   ├── Wiki compilation events
│   └── CLI session discoveries
└── Recently Compiled (wiki pages):
    └── Last 5 updated wiki pages with compile time
```

**数据获取策略:**

Home 视图的 stats 数据从已有 API 聚合，不需要新增后端端点：

```javascript
async function loadHomeStats() {
    const [sessions, crons, wiki] = await Promise.all([
        fetch('/api/sessions').then(r => r.json()),
        fetch('/api/cron').then(r => r.json()),
        fetch('/api/wiki').then(r => r.json()).catch(() => ({pages: []})),
    ]);

    const activeSessions = sessions.sessions?.filter(s => s.state === 'running').length || 0;
    const totalCost = sessions.sessions?.reduce((sum, s) => sum + (s.total_cost || 0), 0) || 0;
    const cronCount = crons.jobs?.length || 0;
    const wikiCount = wiki.pages?.length || 0;

    // 更新 DOM
    document.getElementById('stat-sessions').textContent = activeSessions;
    document.getElementById('stat-crons').textContent = cronCount;
    document.getElementById('stat-cost').textContent = '$' + totalCost.toFixed(2);
    document.getElementById('stat-wiki').textContent = wikiCount;
}
```

**Activity Feed:**

Activity feed 通过 WebSocket 实时更新，复用现有 Hub 的 event 机制。在前端维护一个固定大小 (50 条) 的事件队列，新事件 unshift 到顶部。

**验收标准:**
- [ ] Home tab 作为默认 landing page 展示
- [ ] 4 张 stat 卡片显示正确的实时数据
- [ ] Quick actions 按钮点击跳转到对应视图
- [ ] Activity feed 实时更新 (通过 WebSocket)
- [ ] Recently Compiled 展示最近 5 个更新的 wiki 页面
- [ ] 移动端 stats 卡片横向滚动，activity feed 全宽
- [ ] 问候语根据当前时间显示 Good morning/afternoon/evening

---

## Task 14: Context Panel (右侧面板)

**描述:** 实现 Chat 视图的右侧 Context Panel，包含三个 tab: Saved (bookmarks)、Related (反向链接)、AI (元对话)。Panel 可折叠，通过按钮切换显隐。Saved tab 展示当前 session 的 bookmarks 及跨来源的相关 bookmarks。Related tab 基于当前对话话题做关键词匹配 (Phase 1 实现)。AI tab 作为 placeholder 留待 Phase 3。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~350 (JS + CSS + HTML)

**前端组件结构:**

```
Context Panel (right sidebar, collapsible)
├── Toggle Button (fixed, right edge)
├── Tab Bar: [Saved] [Related] [AI]
├── Saved Tab:
│   ├── Current Session Bookmarks
│   │   └── Bookmark card (content + tags + time)
│   └── Related Bookmarks (from other sessions)
│       └── Source badge (Dashboard/CLI/IM/Vault) + content
├── Related Tab:
│   ├── Keyword-matched related content
│   └── Source badge + summary + time
└── AI Tab (placeholder):
    └── "Ask about this session" input
```

**关键实现细节:**

1. **Panel 折叠:** CSS `transform: translateX(100%)` + transition 动画，固定宽度 320px
2. **Saved tab 数据:** 调用 `GET /api/bookmarks?session={currentSessionKey}` 获取当前 session 的 bookmarks
3. **Related tab 数据 (Phase 1):** 从当前对话最近 3 条消息中提取关键词，调用 `GET /api/search?q={keywords}&limit=5` 获取关联内容
4. **Source badge 样式:** 使用 emoji + 颜色区分来源 (Dashboard=蓝, CLI=绿, IM=紫, Vault=橙)

**验收标准:**
- [ ] Context Panel 通过按钮可折叠/展开
- [ ] Saved tab 正确显示当前 session 的 bookmarks
- [ ] Related tab 展示基于关键词匹配的关联内容
- [ ] AI tab 展示 placeholder 文字
- [ ] Tab 切换有视觉反馈 (active 下划线)
- [ ] 移动端 panel 以底部 sheet 形式出现

---

## Task 15: Bookmark UI (消息保存交互)

**描述:** 在 Chat 视图中实现 Bookmark 交互: AI 消息 hover 时右上角出现保存按钮，点击后将消息片段保存到 Context Panel 的 Saved tab。支持添加标签。Bookmark 通过 `POST /api/bookmarks` 持久化。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~200 (JS + CSS)

**前端交互流程:**

```
1. AI 消息 hover → 右上角出现 bookmark 按钮 (position: absolute)
2. 点击按钮 → 弹出小 popover:
   - 选中的文本内容预览 (截断到 200 字符)
   - Tags 输入框 (逗号分隔, 如 "security, waf")
   - Save 按钮
3. Save → POST /api/bookmarks {session_key, content, tags, event_index}
4. 成功 → 按钮变为实心图标 (已保存状态)
5. Context Panel Saved tab 实时刷新
```

**关键实现细节:**

1. **事件委托:** 在消息容器上绑定 `mouseenter`/`mouseleave`，动态显示/隐藏 bookmark 按钮
2. **内容提取:** 从 AI 消息的 DOM 节点中提取 `textContent`，保留关键结构
3. **Popover 定位:** 使用 `position: absolute` 相对于消息容器定位
4. **已保存状态:** 加载 session 的 bookmarks 后，对已保存的消息标记图标

**验收标准:**
- [ ] AI 消息 hover 时显示 bookmark 按钮，离开时隐藏
- [ ] 点击按钮弹出 popover，预览内容和 tags 输入
- [ ] Save 正确调用 API 并持久化
- [ ] 已保存的消息显示实心图标状态
- [ ] Context Panel Saved tab 在保存后刷新
- [ ] Tags 支持逗号分隔输入，自动去空格
- [ ] 删除 bookmark (从 Context Panel 操作) 后消息图标恢复空心

---

## Task 16: 全局搜索 UI (Cmd+K 集成)

**描述:** 实现 Cmd+K / Ctrl+K 全局搜索 overlay，调用 `/api/search` 获取多来源搜索结果。结果按来源分组展示 (Bookmarks / Dashboard / CLI / Vault / Wiki)，支持键盘导航和回车跳转。这是知识系统的"最后一公里"——将所有索引数据通过统一入口暴露给用户。

**修改文件:**
- `internal/server/static/dashboard.html`

**预估 LOC:** ~350 (JS + CSS + HTML)

**前端组件结构:**

```
Search Overlay (Cmd+K)
├── Backdrop (blur + semi-transparent)
├── Search Modal (centered, max-width 600px):
│   ├── Input (autofocus, placeholder "Search everything...")
│   ├── Results (grouped by source):
│   │   ├── Bookmarks group (icon + results)
│   │   ├── Dashboard Sessions group
│   │   ├── CLI Sessions group
│   │   ├── Obsidian Vault group
│   │   └── Wiki Pages group
│   └── Footer: keyboard hints (arrows, enter, esc)
└── Keyboard handlers: ↑↓ navigate, Enter jump, Esc close
```

**关键实现细节:**

1. **触发:** 全局 `keydown` 监听 `(e.metaKey || e.ctrlKey) && e.key === 'k'`，`e.preventDefault()` 阻止浏览器默认行为
2. **搜索去抖:** 输入后 300ms debounce，调用 `GET /api/search?q={input.value}&limit=20`
3. **结果分组:** 按 `source` 字段分组渲染，每组有图标 header
4. **键盘导航:** 维护 `selectedIndex` 状态，↑↓ 移动高亮，Enter 执行跳转
5. **跳转逻辑:**
   - `bookmark` → 打开对应 session + 滚动到 event_index
   - `dashboard` → 打开对应 session
   - `cli` → 打开 CLI session 详情
   - `vault` → 切换到 Knowledge tab + 加载文件
   - `wiki` → 切换到 Wiki tab + 加载页面
6. **匹配高亮:** 结果中的 `match` 文本用 `<mark>` 标签高亮搜索关键词
7. **空状态:** 无结果时显示 "No results for '{query}'" + 搜索建议

**验收标准:**
- [ ] Cmd+K (Mac) / Ctrl+K (Windows/Linux) 打开搜索 overlay
- [ ] 输入文字后 300ms 自动搜索
- [ ] 结果按来源分组展示，匹配文本高亮
- [ ] ↑↓ 键盘导航正确移动选中状态
- [ ] Enter 跳转到正确的目标视图
- [ ] Esc 关闭 overlay
- [ ] 移动端通过搜索按钮 (右上角) 打开
- [ ] 搜索中显示 loading indicator
- [ ] 无结果时显示友好的空状态

---

## 路由注册汇总

以下是 Phase 2 所有新增 API 路由的完整列表，全部注册在 `dashboard.go` 的 `registerDashboard()` 中，经 `auth` middleware 保护:

```go
// Knowledge API routes (Phase 2)
if s.knowledgeH != nil {
    // Vault (Task 1-3)
    s.mux.HandleFunc("GET /api/vault/tree", auth(s.knowledgeH.handleVaultTree))
    s.mux.HandleFunc("GET /api/vault/read", auth(s.knowledgeH.handleVaultRead))
    s.mux.HandleFunc("GET /api/vault/raw", auth(s.knowledgeH.handleVaultRaw))

    // Wiki (Task 4-6)
    s.mux.HandleFunc("GET /api/wiki", auth(s.knowledgeH.handleWikiList))
    s.mux.HandleFunc("GET /api/wiki/{name}", auth(s.knowledgeH.handleWikiRead))
    s.mux.HandleFunc("POST /api/wiki/ingest", auth(s.knowledgeH.handleWikiIngest))
    s.mux.HandleFunc("GET /api/wiki/ingest/status", auth(s.knowledgeH.handleWikiIngestStatus))
    s.mux.HandleFunc("POST /api/wiki/lint", auth(s.knowledgeH.handleWikiLint))
    s.mux.HandleFunc("GET /api/wiki/lint", auth(s.knowledgeH.handleWikiLintResult))

    // Bookmarks (Task 7)
    s.mux.HandleFunc("GET /api/bookmarks", auth(s.knowledgeH.handleBookmarkList))
    s.mux.HandleFunc("POST /api/bookmarks", auth(s.knowledgeH.handleBookmarkCreate))
    s.mux.HandleFunc("DELETE /api/bookmarks", auth(s.knowledgeH.handleBookmarkDelete))

    // Search (Task 8-9)
    s.mux.HandleFunc("GET /api/search", auth(s.knowledgeH.handleSearch))
}
```

---

## 数据存储汇总

Phase 2 新增的所有数据文件，位于 `~/.naozhi/` 下：

```
~/.naozhi/
  sessions.json           # (已有, 不修改)
  cron.json               # (已有, 不修改)
  bookmarks.json          # 新增: Bookmark 数据 (Task 7)
  search.bleve/           # 新增: bleve 全文索引目录 (Task 8)
  wiki/                   # 新增: Wiki 编译页目录 (Task 4)
    CLAUDE.md             #   编译规则 (Karpathy schema)
    aws-waf.md            #   编译页面
    cloudfront.md
    ...
```

**向后兼容:** 所有新增为独立文件和目录，不修改现有 `sessions.json` / `cron.json` 的格式。旧版本 binary 安全忽略新文件。

---

## go.mod 新增依赖

```
github.com/yuin/goldmark                     # CommonMark 渲染
github.com/yuin/goldmark/extension           # GFM tables, task list (goldmark 内置)
github.com/yuin/goldmark-meta                # YAML frontmatter
github.com/blevesearch/bleve/v2             # 全文搜索索引
```

注意: `bleve/v2` 在纯 Go 模式下 (无 CGO) 会使用内置的 `moss` 存储引擎。如果需要中文分词，可后续添加 `github.com/yanyiwu/gojieba` (需 CGO) 或使用 bleve 的 `unicode` tokenizer 作为 Phase 1 方案。

---

## 任务依赖关系

```
Task 1 (vault_tree.go)  ─┐
                          ├──> Task 3 (REST routes) ──> Task 11 (Knowledge UI)
Task 2 (vault.go)       ─┘

Task 4 (wiki.go)        ─┬──> Task 5 (ingest.go)
                          └──> Task 6 (lint.go)     ──> Task 12 (Wiki UI)

Task 7 (bookmark.go)    ─────────────────────────────> Task 15 (Bookmark UI)

Task 8 (search.go)      ─┬──> Task 9 (search API)  ──> Task 16 (Search UI)
                          └──> Task 10 (CLI sync)

Task 13 (Home UI)        (独立, 仅依赖已有 API)
Task 14 (Context Panel)  (依赖 Task 7 bookmark API)
```

**建议执行顺序:**

- **Week 1:** Task 1, 2, 3 (Vault 基础) + Task 7, 8 (Bookmark + Search 基础)
- **Week 2:** Task 4, 5, 6 (Wiki 引擎) + Task 9, 10 (Search API + CLI sync)
- **Week 3:** Task 11, 12 (Knowledge + Wiki 前端) + Task 13 (Home)
- **Week 4:** Task 14, 15, 16 (Context Panel + Bookmark UI + Search UI) + 集成测试 + bug fix

可并行的任务组:
- Task 1+2 并行 (两者独立)
- Task 4+7+8 并行 (三者独立)
- Task 5+6 并行 (仅共享 wikiDir 路径)
- Task 11+12+13 并行 (三个独立前端视图)
- Task 14+15+16 并行 (三个独立前端组件)

---

## Config 变更

在 `config.yaml` 中新增 `knowledge` 配置段:

```yaml
knowledge:
  obsidian:
    vault_path: "~/keith-space/Obsidian/vaults/Keith_Space_2026"
    include_paths: ["Things/", "page/", "journals/"]
    exclude_paths: [".obsidian/", "assets/", ".trash/"]
  wiki:
    dir: "~/.naozhi/wiki"          # 默认值, 通常不需要配置
  search:
    index_path: "~/.naozhi/search.bleve"  # 默认值
  bookmarks:
    store_path: "~/.naozhi/bookmarks.json" # 默认值
```

对应的 Go config struct:

```go
// internal/config/config.go (新增)
type KnowledgeConfig struct {
    Obsidian struct {
        VaultPath    string   `yaml:"vault_path"`
        IncludePaths []string `yaml:"include_paths"`
        ExcludePaths []string `yaml:"exclude_paths"`
    } `yaml:"obsidian"`
    Wiki struct {
        Dir string `yaml:"dir"`
    } `yaml:"wiki"`
    Search struct {
        IndexPath string `yaml:"index_path"`
    } `yaml:"search"`
    Bookmarks struct {
        StorePath string `yaml:"store_path"`
    } `yaml:"bookmarks"`
}
```

---

## Graceful Shutdown 扩展

在 `cmd/naozhi/main.go` 的 shutdown 序列中追加:

```
现有 shutdown 序列:
1. Cancel context
2. Stop cron scheduler
3. Wait for running sessions (30s)
4. Save session store
5. Close all processes
6. Shutdown WebSocket hub + platforms

新增 (在步骤 6 之后):
7. Close bleve search index (searchIdx.Close())
```

bleve 的 `Close()` 会 flush 内存中的 pending writes 到磁盘，确保索引数据不丢失。
