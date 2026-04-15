# Naozhi V2.0 Phase 1: UI Quick Wins 实施计划

> **时间范围**: 1-2 周
> **目标**: 在现有单 HTML 架构上显著提升对话体验、导航效率和移动端可用性
> **约束**: 不新增 Go 包, 不改后端架构, 所有改动在 `dashboard.html` + 少量 server 路由内完成

---

## 依赖关系总览

```
Task 1 (Shiki CDN) ──┐
Task 2 (工具卡片)  ───┤── 并行, 无依赖
Task 4 (长输出折叠) ──┘
            │
Task 3 (Diff 渲染) ── 依赖 Task 2 (工具卡片容器)
            │
Task 5 (全局搜索) ────── 独立
Task 6 (标签筛选) ────┐
Task 7 (时间分组)  ───┤── 并行, 共同改造 sidebar 渲染
Task 8 (新建 Modal) ──┘
            │
Task 9 (通知中心) ────── 独立
            │
Task 10 (移动端 Tab) ─┐
Task 11 (移动端手势) ──┤── 依赖 Task 6/7 的 sidebar 改造
Task 12 (移动端对话) ──┘── 依赖 Task 1/2/3 的对话组件
```

**建议执行顺序**: (1,2,4,5 并行) → (3,6,7,8,9 并行) → (10,11,12 并行)

---

## Task 1: 代码块语法高亮 (Shiki CDN)

**描述**: 替换当前的 `<pre class="md-pre">` 纯文本代码块为 Shiki WASM 语法高亮渲染, 添加行号、语言标签、Copy/Insert 按钮.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~120 (CSS) + ~80 (JS)

**依赖**: 无

**实现要点**:
1. 在 `<head>` 添加 Shiki CDN:
   ```html
   <script type="module">
     import { codeToHtml } from 'https://esm.sh/shiki@1.0.0/bundle/web'
   </script>
   ```
2. 修改现有 `renderMarkdown()` 函数中的代码块处理逻辑 (当前在 `md-pre` class 生成处)
3. 添加 `.code-block` 容器样式: 头部 (语言标签 + Copy 按钮) + 代码区 (行号 + 高亮)
4. Copy 按钮: `navigator.clipboard.writeText()`, 反馈 "Copied!" 2s 后恢复
5. 行号: CSS `counter-increment` 或预渲染 `<span class="ln">`
6. 降级: Shiki 加载失败时 fallback 到现有 `md-pre` 样式

**验收标准**:
- [ ] Go/Python/YAML/JSON/Terraform 等常见语言正确高亮
- [ ] 行号显示正确
- [ ] Copy 按钮点击后文本复制到剪贴板, 按钮文字变 "Copied!"
- [ ] 移动端代码块可横向滚动
- [ ] Shiki 加载失败时不影响页面功能

---

## Task 2: 工具调用卡片化

**描述**: 将当前 `tool_use` 事件从纯文本/图标渲染改为折叠式卡片, 显示工具名、参数摘要、执行状态, 点击可展开查看详情.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~80 (CSS) + ~100 (JS)

**依赖**: 无

**实现要点**:
1. 修改 `appendEvent()` 函数中 `type === 'tool_use'` 和 `type === 'tool_result'` 的渲染逻辑
2. 卡片 HTML 结构:
   ```html
   <div class="tool-card">
     <div class="tool-hdr" onclick="toggleTool(this)">
       <span class="t-icon">{icon}</span>
       <span class="t-name">{toolName}</span>
       <span class="t-detail">{paramSummary}</span>
       <span class="t-status">✓</span>
       <span class="t-expand">▼</span>
     </div>
     <div class="tool-body">{fullOutput}</div>
   </div>
   ```
3. 工具图标映射: Read→📄, Edit→✏️, Bash→💻, Grep→🔍, Glob→📂, Agent→🤖, Write→📝
4. 参数摘要: Read/Edit→文件路径, Bash→命令前20字符, Grep→搜索模式, Agent→描述
5. 展开/折叠: `max-height` transition + `overflow:hidden`
6. 保持与现有事件流的兼容 (tool_use 和 tool_result 需要配对)

**验收标准**:
- [ ] Read/Edit/Bash/Grep/Agent 等工具显示为折叠卡片
- [ ] 折叠态: 单行, 显示工具名 + 参数摘要 + ✓
- [ ] 展开态: 显示完整输出, 平滑动画
- [ ] 移动端卡片正确显示

---

## Task 3: Edit 工具 Diff 渲染

**描述**: 当 Edit 工具执行时, 解析 `old_string` 和 `new_string` 字段, 在工具卡片展开区域渲染 unified diff 视图.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~60 (CSS) + ~80 (JS)

**依赖**: Task 2 (工具卡片容器)

**实现要点**:
1. 在工具卡片的 `tool-body` 区域, 检测 `tool_name === 'Edit'`
2. 从 tool_use input 中提取 `old_string`, `new_string`, `file_path`
3. 生成 diff:
   ```javascript
   function renderDiff(oldStr, newStr) {
     const oldLines = oldStr.split('\n');
     const newLines = newStr.split('\n');
     // Simple line-by-line diff (不需要完整 LCS, 因为 old/new 通常很短)
     // 标记: 删除行 (红), 新增行 (绿), 相同行 (灰)
   }
   ```
4. Diff HTML:
   ```html
   <div class="diff-block">
     <div class="diff-hdr"><span class="diff-file">{filePath}</span><span class="diff-stat">+N -M</span></div>
     <div class="diff-line add">+ new line</div>
     <div class="diff-line del">- old line</div>
     <div class="diff-line ctx">  context</div>
   </div>
   ```
5. 统计: 在卡片折叠态的 status 位置显示 `+N -M`

**验收标准**:
- [ ] Edit 工具的 old_string/new_string 正确解析并渲染 diff
- [ ] 绿色新增行, 红色删除行, 灰色上下文
- [ ] 文件路径和 +N/-M 统计显示在头部
- [ ] 折叠态显示 `✓ +3 -1` 格式的摘要

---

## Task 4: 长输出折叠

**描述**: AI 消息中超过 N 行 (默认 10) 的文本内容自动折叠, 显示渐变遮罩和展开按钮.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~30 (CSS) + ~40 (JS)

**依赖**: 无

**实现要点**:
1. 在 `appendEvent()` 渲染 text 事件后, 检查内容行数
2. 超过阈值时包裹 `<div class="collapse-wrap">`:
   ```html
   <div class="collapse-wrap" style="max-height:200px">
     {content}
     <div class="collapse-grad">
       <button class="collapse-btn" onclick="toggleCollapse(this)">▼ 展开全部 (N 项)</button>
     </div>
   </div>
   ```
3. CSS: `max-height` + `overflow:hidden` + `transition`, gradient overlay `linear-gradient(transparent, var(--bg))`
4. Toggle: 点击切换 `max-height:none` + 更新按钮文字 "▲ 收起"

**验收标准**:
- [ ] 超过 10 行的文本自动折叠
- [ ] 渐变遮罩正确显示
- [ ] 点击 "展开全部" 展开, 按钮变 "收起"
- [ ] 再次点击收起, 平滑动画

---

## Task 5: Cmd+K 全局搜索

**描述**: 添加全屏搜索覆盖层, 支持搜索 session 名称和消息内容. 快捷键 Cmd+K / Ctrl+K 触发.

**文件修改**: `internal/server/static/dashboard.html`, `internal/server/dashboard_session.go` (可选: 添加消息搜索 API)

**预估 LOC**: ~100 (CSS) + ~120 (JS) + ~30 (Go, 可选)

**依赖**: 无

**实现要点**:
1. 搜索覆盖层 HTML (插入到 body 末尾):
   ```html
   <div class="search-overlay" id="search-overlay">
     <div class="search-modal">
       <div class="sm-header"><input placeholder="Search everything..." autofocus></div>
       <div class="sm-body">{grouped results}</div>
     </div>
   </div>
   ```
2. 快捷键: `document.addEventListener('keydown', e => { if ((e.metaKey||e.ctrlKey) && e.key==='k') ... })`
3. Phase 1 搜索范围 (前端): 
   - Session 名称/prompt 匹配 (从 `allSessionsCache` 过滤)
   - 已加载的消息内容匹配 (从 `sessionsData[key].events` 搜索)
4. 结果分组: Sessions / Messages, 每条显示 title + match highlight + meta
5. 键盘导航: ↑↓ 选择, Enter 跳转, ESC 关闭
6. 动画: modal `scale(.97) → scale(1)` + `opacity 0→1`

**验收标准**:
- [ ] Cmd+K 打开搜索, ESC 关闭
- [ ] 输入时实时过滤 session 列表
- [ ] 点击结果跳转到对应 session
- [ ] 键盘 ↑↓ + Enter 导航可用

---

## Task 6: Session 标签筛选

**描述**: 在 sidebar session 列表上方添加横向滚动标签条, 支持按 agent 类型过滤.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~40 (CSS) + ~50 (JS)

**依赖**: 无

**实现要点**:
1. 在 `.session-list` 上方插入标签条:
   ```html
   <div class="tag-filter">
     <span class="tag-pill active" data-filter="all">All</span>
     <span class="tag-pill" data-filter="review">🔍 Review</span>
     <span class="tag-pill" data-filter="project">📦 Projects</span>
     <span class="tag-pill" data-filter="cli">💻 CLI</span>
     <span class="tag-pill" data-filter="research">📖 Research</span>
   </div>
   ```
2. 标签从 `agent_commands` 配置自动生成 (可通过 `/api/sessions` 的 agent 字段推断)
3. 过滤逻辑: 在 `renderSessions()` 函数中根据 `activeFilter` 筛选
4. CSS: `overflow-x:auto; scrollbar-width:none; gap:4px; display:flex`

**验收标准**:
- [ ] 标签条横向可滚动
- [ ] 点击标签过滤 session 列表
- [ ] 选中标签高亮, 其余灰色
- [ ] "All" 显示全部

---

## Task 7: Session 时间分组

**描述**: 将 session 列表按时间分组显示: Pinned / Today / CLI Sessions / Yesterday / Earlier.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~60 (JS)

**依赖**: 无 (但建议与 Task 6 一起做)

**实现要点**:
1. 修改 `renderSessions()` 函数, 按以下逻辑分组:
   ```javascript
   groups: [
     { key: 'pinned', label: '📌 Pinned', filter: s => s.pinned },
     { key: 'today', label: 'Today', filter: s => isToday(s.lastActive) && !s.pinned },
     { key: 'cli', label: '💻 CLI Sessions', filter: s => s.source === 'cli' },
     { key: 'yesterday', label: 'Yesterday', filter: s => isYesterday(s.lastActive) },
     { key: 'earlier', label: 'Earlier', filter: s => isEarlier(s.lastActive) }
   ]
   ```
2. 每组显示 `<div class="section-header">{label}</div>` (复用已有 `.section-header` 样式)
3. Earlier 组 session 降低 opacity (0.65)
4. CLI sessions: 绿色终端图标 `>_`

**验收标准**:
- [ ] Session 列表按时间分组, 有分组标题
- [ ] Pinned 永远在最上面
- [ ] CLI sessions 有独立分组和绿色图标
- [ ] Earlier sessions 视觉降低

---

## Task 8: 新建 Session Modal (Cmd+N)

**描述**: 添加新建 session 弹窗, 支持选择 Agent、绑定 Project、设置工作目录.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~80 (CSS) + ~100 (JS)

**依赖**: 无

**实现要点**:
1. Modal HTML (参考现有 `.modal` 样式):
   - Agent 四宫格 (General / Code Reviewer / Researcher / Planner), 从 `availableAgents` 动态生成
   - Project 列表 (从 `projectsData` 动态生成, 可选)
   - Working Directory 输入框 (默认 `defaultWorkspace`)
2. 触发: Cmd+N 快捷键 + sidebar 的 "+" 按钮
3. 创建逻辑: 复用现有 `createNewSession()` 函数, 传入选择的 agent 和 project
4. 创建后: 关闭 modal → 新 session 出现在列表顶部 → 自动选中 → 显示空白对话态

**验收标准**:
- [ ] Cmd+N 打开 modal, ESC 关闭
- [ ] Agent 四宫格点击选择, 单选高亮
- [ ] Project 列表点击选中/取消
- [ ] Create 按钮创建 session 并自动跳转

---

## Task 9: Notification Center

**描述**: 右上角添加通知铃铛 + badge 计数 + 下拉面板, 显示系统通知.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~60 (CSS) + ~70 (JS)

**依赖**: 无

**实现要点**:
1. 在 sidebar header 的 `.hdr-btns` 中添加铃铛按钮 (复用 `.hdr-btn` 样式)
2. 通知面板: 绝对定位下拉, z-index 高于其他元素
3. Phase 1 通知来源 (前端生成):
   - Session 完成 (turn_result 事件)
   - Session 错误 (error 事件)
   - 进程发现 (discovered sessions)
   - Cron 任务执行 (已有事件)
4. 通知数据: `notifications = [{title, desc, time, read, urgency}]`, 内存存储
5. "Mark all read" 清除 badge
6. 点击通知跳转到对应 session

**验收标准**:
- [ ] 铃铛显示未读计数 badge
- [ ] 点击弹出下拉面板
- [ ] 通知按 urgency 分级显示 (urgent/unread/read)
- [ ] "Mark all read" 清除 badge
- [ ] 点击面板外部自动关闭

---

## Task 10: 移动端底部 Tab Bar

**描述**: 在 `@media (max-width: 768px)` 下, 将顶部 sidebar 导航替换为底部 tab bar.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~80 (CSS) + ~40 (JS)

**依赖**: Task 6, Task 7 (sidebar 改造完成后)

**实现要点**:
1. HTML: 在 `body` 末尾添加底部 tab bar (桌面端 `display:none`):
   ```html
   <div class="mobile-tab-bar">
     <div class="m-tab" data-tab="sessions">💬 Chat</div>
     <div class="m-tab" data-tab="cron">⏰ Cron</div>
     <div class="m-tab" data-tab="files">📁 Files</div>
     <div class="m-tab" data-tab="discover">🔍 More</div>
   </div>
   ```
2. CSS: `position:fixed; bottom:0; left:0; right:0; padding-bottom:env(safe-area-inset-bottom)`
3. Tab 切换: 控制 sidebar 内不同面板的显隐 (sessions/cron/files/discovered)
4. 主内容区: `padding-bottom` 留出 tab bar 空间
5. 保持现有 `mobile-list-view` 的 sidebar/main 切换逻辑

**验收标准**:
- [ ] 移动端显示底部 tab bar
- [ ] 桌面端隐藏
- [ ] Tab 切换对应视图
- [ ] Safe area 正确 (iPhone 底部)

---

## Task 11: 移动端手势优化

**描述**: 为移动端 session 列表添加左滑删除和长按置顶手势.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~100 (JS) + ~30 (CSS)

**依赖**: Task 10 (移动端 tab bar)

**实现要点**:
1. 左滑删除: 参考现有 `touchstart/touchmove/touchend` 事件处理 (dashboard.html 已有 `.session-card[data-key]{touch-action:pan-y}` 和 `.swiping` 样式)
2. 扩展现有 swipe 逻辑: 
   - 滑动超过 80px: 显示红色删除背景
   - 释放时确认删除 (dismiss session)
3. 长按置顶: `setTimeout(500ms)` 触发 pin/unpin
4. Haptic feedback: `navigator.vibrate(10)` (如果支持)

**验收标准**:
- [ ] 左滑 session 卡片显示红色删除区域
- [ ] 释放后 session 被移除
- [ ] 长按 500ms 后 session 置顶/取消置顶
- [ ] 桌面端不触发手势

---

## Task 12: 移动端对话视图优化

**描述**: 优化移动端的对话体验: 紧凑工具卡片、代码块横向滚动、Bookmark 点击显示.

**文件修改**: `internal/server/static/dashboard.html`

**预估 LOC**: ~60 (CSS)

**依赖**: Task 1 (代码高亮), Task 2 (工具卡片), Task 3 (Diff)

**实现要点**:
1. 工具卡片移动端: 单行紧凑模式, `@media(max-width:768px) .tool-card .tool-hdr { font-size:11px; padding:5px 8px }`
2. 代码块: `overflow-x:auto; -webkit-overflow-scrolling:touch`
3. Bookmark 按钮: 移动端改为 tap 出现 (不是 hover), 3s 后自动隐藏
4. 输入区: 更大的触控目标 (min-height:42px), 圆角输入框
5. 消息气泡: 移动端 `max-width:85%` (桌面 75%)

**验收标准**:
- [ ] 工具卡片移动端紧凑显示
- [ ] 代码块可横向滑动
- [ ] Tap AI 消息出现 Bookmark 按钮
- [ ] 输入区触控目标 >= 44px
