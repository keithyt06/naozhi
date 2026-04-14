# Naozhi File Hub — 设计规格文档

## 概述

File Hub 是 Naozhi 的文件系统访问层，让用户在任意设备（手机/电脑）上浏览服务器文件、上传本地文件到服务器、下载服务器文件到本地，并将文件路径无缝注入 AI 聊天会话。

### 核心价值

Naozhi 当前是一个"文件盲"的 AI 网关——用户能和 Claude 对话，但无法看到服务器上有什么文件，也无法精准地告诉 Claude 操作哪个文件。File Hub 解决这个"文件盲墙"问题：

- **浏览**：在任意设备上查看服务器目录结构
- **定位**：选中文件路径一键注入聊天输入框，精准指引 AI
- **传输**：双向文件传输（上传本地文件 + 下载服务器文件）
- **联动**：与会话、项目、多节点等现有功能深度集成

### 差异化定位

| 能力 | VS Code Remote | scp/sftp | Naozhi File Hub |
|------|---------------|----------|-----------------|
| 手机可用 | 否 | 否 | 是 |
| 无需 SSH/端口 | 否 | 否 | 是 |
| 路径→AI 聊天联动 | 否 | 否 | 是 |
| 多节点统一浏览 | 否 | 否 | 是 |
| IM 命令查看 | 否 | 否 | 是 |
| 零配置 | 否（需装扩展） | 否（需 SSH key） | 是（开箱即用） |

---

## 版本规划

### V1（本次实现范围）

- 后端文件 REST API（list/stat/upload/download/mkdir/delete）
- Dashboard File Hub 弹出式模态框（浏览/上传/下载/管理）
- `/ls` 聊天命令（四平台 + Dashboard 通用）
- 路径插入聊天输入框
- Dashboard 导航栏新增 Files 标签

### V1.1（后续迭代）

- 文件预览（语法高亮、Markdown 渲染、图片/CSV 直接查看）
- AI 操作足迹（标记 Claude 读过/改过/新建的文件）
- 快捷收藏夹
- 文件名搜索（glob 模式）

### V2（长期）

- Live Tail 实时日志追踪
- URL 直传（服务器直接 wget）
- Git 状态叠加
- AI 快捷操作菜单（右键文件 → 让 AI review/解释/写测试）
- 拖拽文件到聊天

### V3（愿景）

- 跨节点文件传输（EC2 ↔ MacBook 穿越 NAT）
- 临时分享链接（类 S3 Pre-signed URL）
- 存储可视化（磁盘用量 treemap）

---

## V1 详细设计

### 1. 后端 REST API

新增 `internal/server/dashboard_files.go` 文件处理器，注册在 `/api/files` 路由组下。

#### 1.1 端点定义

| 端点 | 方法 | 功能 |
|------|------|------|
| `/api/files/list` | GET | 列出指定目录内容 |
| `/api/files/stat` | GET | 获取单个文件/目录的元信息 |
| `/api/files/upload` | POST | multipart 上传文件到指定目录 |
| `/api/files/download` | GET | 流式下载指定文件 |
| `/api/files/mkdir` | POST | 创建目录 |
| `/api/files/delete` | DELETE | 删除文件或空目录 |

#### 1.2 list 端点

**请求**：`GET /api/files/list?path=/home/ec2-user/workspace`

**响应**：
```json
{
  "path": "/home/ec2-user/workspace",
  "entries": [
    {
      "name": "my-project",
      "type": "dir",
      "size": 0,
      "item_count": 12,
      "mod_time": "2026-04-14T10:00:00Z"
    },
    {
      "name": "config.yaml",
      "type": "file",
      "size": 1234,
      "mod_time": "2026-04-13T08:30:00Z"
    }
  ]
}
```

**行为**：
- `path` 参数必填，必须为绝对路径
- 返回的 entries 排序：目录在前、文件在后，各自按名称字母序
- 隐藏文件（以 `.` 开头）默认不返回，可通过 `?hidden=true` 显示
- 目录条目包含 `item_count`（直接子项数量）
- 如果 path 不存在或不是目录，返回 404

#### 1.3 stat 端点

**请求**：`GET /api/files/stat?path=/home/ec2-user/workspace/config.yaml`

**响应**：
```json
{
  "name": "config.yaml",
  "path": "/home/ec2-user/workspace/config.yaml",
  "type": "file",
  "size": 1234,
  "mod_time": "2026-04-14T10:00:00Z",
  "permissions": "rw-r--r--"
}
```

#### 1.4 upload 端点

**请求**：`POST /api/files/upload`，Content-Type: `multipart/form-data`

表单字段：
- `dest`：目标目录路径（必填）
- `file`：文件本体（支持多个 `file` 字段实现多文件上传）

**响应**：
```json
{
  "uploaded": [
    {
      "name": "data.csv",
      "path": "/home/ec2-user/workspace/data.csv",
      "size": 52428800
    }
  ]
}
```

**行为**：
- 使用 `http.MaxBytesReader` 限制单次请求总大小为 100MB
- 目标目录不存在时返回 400
- 同名文件直接覆盖（不做版本控制）
- 写入使用临时文件 + rename 原子操作，避免写入中断导致损坏文件

#### 1.5 download 端点

**请求**：`GET /api/files/download?path=/home/ec2-user/workspace/report.pdf`

**行为**：
- 设置 `Content-Disposition: attachment; filename="report.pdf"`
- 使用 `http.ServeFile` 或 `io.Copy` 流式输出，不将整个文件读入内存
- 目录不可下载，返回 400
- 文件不存在返回 404

#### 1.6 mkdir 端点

**请求**：`POST /api/files/mkdir`
```json
{
  "path": "/home/ec2-user/workspace/new-dir"
}
```

**行为**：
- 使用 `os.MkdirAll` 支持创建多级目录
- 目录已存在时幂等返回 200

#### 1.7 delete 端点

**请求**：`DELETE /api/files/delete?path=/home/ec2-user/workspace/old-file.txt`

**行为**：
- 文件：直接删除
- 目录：仅删除空目录（`os.Remove`），非空目录返回 400 错误提示
- 不支持递归删除（安全考虑）

#### 1.8 认证

复用现有 Dashboard token 机制：
- 如果配置了 `dashboard_token`，所有 `/api/files/*` 端点需要 Bearer token 或有效 cookie
- 未配置 token 时无需认证（与现有行为一致）

#### 1.9 安全

- 不做路径白名单限制（用户已明确选择信任模式，与 `--dangerously-skip-permissions` 一致）
- 使用 `filepath.Clean` 清理路径，防止 path traversal 中的冗余 `..`
- 上传文件大小限制 100MB
- 删除操作不支持递归，防止误删
- 上传写入使用临时文件 + atomic rename

#### 1.10 错误响应

统一错误格式：
```json
{
  "error": "directory not found",
  "path": "/home/ec2-user/nonexistent"
}
```

HTTP 状态码：
- 400：参数错误、目标不存在、非空目录删除
- 404：文件/目录不存在
- 413：上传文件超过 100MB 限制
- 500：服务器内部错误

---

### 2. `/ls` 聊天命令

#### 2.1 命令格式

```
/ls                    → 列出当前会话 cwd
/ls /home/ec2-user     → 列出指定绝对路径
/ls ./src              → 相对于当前会话 cwd 的相对路径
/ls ../                → 上级目录
```

#### 2.2 实现位置

在 `internal/dispatch/commands.go` 中新增 `/ls` 命令处理器。与 `/help`、`/cd` 等命令一致，直接返回系统消息，不经过 Claude CLI 会话。

#### 2.3 路径解析

- 无参数：使用当前会话的 `cwd`（从 session managed 中获取）
- 绝对路径：直接使用
- 相对路径：基于当前会话 `cwd` 解析（`filepath.Join(session.cwd, arg)`）
- 无活跃会话时：使用 `session.cwd` 配置默认值

#### 2.4 输出格式

```
📂 /home/ec2-user/workspace/naozhi

  📁 cmd/                          6 items    Apr 14
  📁 internal/                    12 items    Apr 14
  📁 docs/                         8 items    Apr 13
  📄 config.yaml              1.2K            Apr 13
  📄 go.mod                   892B            Apr 11
  📄 Makefile                 2.1K            Apr 10

10 items (5 dirs, 5 files)
```

**格式规则**：
- 目录在前、文件在后，各自按名称字母序
- 目录显示子项数量，文件显示 human-readable 大小
- 日期只显示月+日（节省空间）
- 超过 50 条时截断，显示 `... and N more items`
- 隐藏文件默认不显示

#### 2.5 平台差异

- **IM 平台（飞书/Slack/Discord/微信）**：纯文本输出，用户手动复制路径
- **Dashboard**：增强交互——目录名可点击继续展开、文件可一键复制路径、底部"在 File Hub 打开"跳转按钮

#### 2.6 错误处理

- 路径不存在：返回 `❌ 路径不存在: /path/to/nowhere`
- 路径不是目录：返回 `❌ 不是目录: /path/to/file.txt`
- 权限不足：返回 `❌ 权限不足: /root/secret`

---

### 3. Dashboard File Hub UI

#### 3.1 入口

File Hub 作为可复用的弹出式模态框组件，有多个触发入口：

| 入口 | 位置 | 打开时的初始路径 |
|------|------|-----------------|
| Dashboard 导航栏 📁 Files | 顶部导航标签 | `session.cwd` 配置值 |
| 会话详情页 📁 按钮 | 会话标题栏旁 | 该会话的 cwd |
| Project 管理页 📁 按钮 | 项目名旁 | 该项目的根目录 |
| Dashboard `/ls` 结果中 "在 File Hub 打开" | /ls 响应底部 | /ls 展示的当前路径 |

#### 3.2 模态框布局

**桌面端**：居中模态框，宽度 ~70% 屏幕，高度 ~80%

**手机端**：全屏底部抽屉（100% 宽高），顶部有拖拽手柄

**通用结构**：
```
┌──────────────────────────────────────────────┐
│  [节点选择器]  📁 home / ec2-user / workspace  [路径输入框]  ✕  │  ← 头部
│  [来自: feishu:general 会话 ← 选中路径将插入该会话]           │  ← 上下文标记
├──────────────────────────────────────────────┤
│  ☐ 📁 cmd/                    —   6 items  Apr 14  │  ← 文件列表
│  ☐ 📁 internal/               —  12 items  Apr 14  │
│  ☐ 📄 config.yaml          1.2K    yaml    Apr 13  │
│  ☐ 📄 go.mod                892B    go      Apr 11  │
│  ...                                                │
├──────────────────────────────────────────────┤
│  [📋 插入路径]  [⬆️ 上传]  [⬇️ 下载]  [📁 新建]  [🗑️ 删除]   │  ← 工具栏
│                                       已选 1 项 · 7 items  │
└──────────────────────────────────────────────┘
```

#### 3.3 头部

- **节点选择器**：下拉菜单，列出 Primary + 所有已连接的远程节点。切换节点时文件列表刷新为目标节点的文件系统。V1 仅支持 Primary 节点，多节点浏览预留 UI 位置。
- **面包屑导航**：路径各段可点击跳转上级。桌面端完整显示，手机端可横向滚动。
- **路径输入框**：可直接输入绝对路径回车跳转。
- **关闭按钮**：✕ 关闭模态框。

#### 3.4 上下文标记

当 File Hub 从特定会话/项目打开时，显示来源上下文：
- `来自: feishu:general 会话 ← 选中路径将插入该会话聊天框`
- 当从导航栏直接打开（无特定会话上下文）时，不显示此标记，"插入路径"按钮改为"复制路径到剪贴板"

#### 3.5 文件列表

每行显示：
- 勾选框（支持多选）
- 图标（📁/📄）
- 文件名（目录蓝色可点击，文件白色）
- 大小（文件显示 human-readable，目录显示 `—`）
- 类型标签（文件扩展名简写：yaml、go、md 等）
- 修改日期

**交互**：
- 点击目录 → 进入子目录
- 点击文件 → 选中（高亮），勾选 checkbox
- 长按/右键 → 操作菜单（复制路径、下载、删除）

**排序**：目录在前、文件在后，各自按名称字母序。

#### 3.6 底部工具栏

| 按钮 | 功能 | 条件 |
|------|------|------|
| 📋 插入路径到聊天 | 将选中项路径插入聊天输入框，模态框自动关闭 | 有会话上下文时显示 |
| 📋 复制路径 | 复制路径到剪贴板 | 无会话上下文时替代"插入路径" |
| ⬆️ 上传 | 打开本地文件选择器或拖拽区域，上传到当前浏览目录 | 始终可用 |
| ⬇️ 下载 | 下载选中文件 | 有文件被选中时可用 |
| 📁 新建 | 弹出输入框，在当前目录创建新文件夹 | 始终可用 |
| 🗑️ 删除 | 删除选中的文件/空目录，二次确认 | 有项被选中时可用 |

工具栏右侧显示选中数量和总条目数。

**手机端**：主操作（插入路径）单独一行全宽，其余按钮 3 列网格排列。

#### 3.7 上传交互

点击"上传"按钮后，文件列表区域切换为上传视图：

- **拖拽区域**（桌面端）/ **点击选择**（手机端）
- 支持多文件同时上传
- 手机端支持：相册、文件、拍照
- 最大 100MB 提示
- 上传进度条：文件名 + 百分比 + 进度条
- 完成提示：文件名 + 保存路径
- 上传完成后自动刷新文件列表

#### 3.8 路径插入行为

点击"插入路径到聊天"后：
1. 模态框自动关闭
2. 选中项的完整路径以文本形式插入到聊天输入框的光标位置
3. 多选时路径以空格分隔
4. 用户可在路径前后自由编辑补充指令再发送

#### 3.9 Dashboard 导航栏变化

现有导航标签：Sessions | Discover | Projects | Cron

新增：Sessions | Discover | Projects | Cron | **Files**

点击 Files 标签等同于从导航栏入口打开 File Hub 模态框。

---

### 4. 技术实现

#### 4.1 后端新增文件

| 文件 | 职责 |
|------|------|
| `internal/server/dashboard_files.go` | 文件 API handler 组（list/stat/upload/download/mkdir/delete） |

路由注册在 `internal/server/server.go` 的 `registerDashboard()` 中，跟随现有模式。

#### 4.2 前端修改

| 文件 | 改动 |
|------|------|
| `internal/server/static/dashboard.html` | 新增 File Hub 模态框组件、导航标签、触发按钮、/ls 增强渲染 |

前端使用 vanilla JavaScript（与现有 dashboard.html 一致，无框架），File Hub 模态框作为 DOM 组件动态创建。

#### 4.3 命令处理

| 文件 | 改动 |
|------|------|
| `internal/dispatch/commands.go` | 新增 `/ls` 命令处理器 |

`/ls` 命令复用 `dashboard_files.go` 中的 list 逻辑（提取为共享函数），但输出为格式化文本而非 JSON。

#### 4.4 数据流

**文件浏览（Dashboard）**：
```
Dashboard File Hub → GET /api/files/list → Go handler → os.ReadDir → JSON response → 渲染列表
```

**文件浏览（IM /ls）**：
```
飞书消息 "/ls ./src" → dispatch → /ls handler → os.ReadDir → 格式化文本 → platform.Reply
```

**文件上传**：
```
Dashboard 文件选择器 → POST /api/files/upload (multipart) → Go handler → 临时文件 → atomic rename → JSON response
```

**文件下载**：
```
Dashboard 点击下载 → GET /api/files/download?path=... → Go handler → http.ServeFile → 流式传输 → 浏览器保存
```

**路径插入**：
```
Dashboard File Hub 选中文件 → 点击"插入路径" → JS 获取路径字符串 → 插入 chatInput.value → 关闭模态框
```

---

### 5. 配置

不需要新增配置项。File Hub 使用现有配置：

- `server.dashboard_token`：复用认证机制
- `session.cwd`：作为 File Hub 和 `/ls` 的默认路径

---

### 6. 测试策略

#### 6.1 后端单元测试

- `dashboard_files_test.go`：测试所有 6 个端点的正常路径和错误路径
  - list: 正常目录、空目录、不存在的路径、隐藏文件过滤
  - stat: 文件、目录、不存在
  - upload: 单文件、多文件、超大文件（超 100MB）、目标不存在
  - download: 正常下载、文件不存在、目录下载（应报错）
  - mkdir: 新建、已存在（幂等）、多级
  - delete: 文件、空目录、非空目录（应报错）

#### 6.2 命令测试

- `/ls` 命令解析：无参数、绝对路径、相对路径、不存在路径

#### 6.3 E2E 测试

- `test/e2e/file-hub.test.js`（Playwright）：
  - 打开 File Hub 模态框
  - 目录导航（面包屑跳转、点击子目录）
  - 上传文件并确认出现在列表中
  - 下载文件
  - 路径插入到聊天输入框
  - 手机视口下的响应式布局

---

### 7. 后续版本功能参考

以下功能不在 V1 范围内，但设计时需为其预留扩展点：

#### V1.1 预留

- **文件预览**：list 响应已包含文件类型信息，前端可据此选择预览组件
- **AI 操作足迹**：需要在 `cli/process.go` 的事件处理中提取 tool_use 的文件路径，存入会话元数据。前端通过现有 WebSocket 事件流获取
- **收藏夹**：新增 `~/.naozhi/bookmarks.json`，前端在模态框头部渲染收藏标签
- **文件搜索**：list API 扩展 `?search=*.go&recursive=true` 参数

#### V2 预留

- **Live Tail**：新增 WebSocket 消息类型 `file-tail`，后端对指定文件做 `fsnotify` + 增量读取
- **URL 直传**：新增 `POST /api/files/fetch-url` 端点
- **Git 状态叠加**：list API 响应扩展 `git_status` 字段（M/A/D/?）
- **AI 快捷操作**：前端右键菜单生成预填消息，调用现有 send API

#### V3 预留

- **跨节点浏览**：文件 API 增加 `?node=macbook` 参数，通过现有多节点 relay 协议转发请求
- **临时分享链接**：新增 `POST /api/files/share` 生成带 HMAC token 的限时 URL
