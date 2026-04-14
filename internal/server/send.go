// send.go contains sendWithBroadcast, the canonical wrapper for sending
// messages to a session with dashboard state notifications.
//
// All entry points that send user messages (IM, HTTP API, WebSocket) should
// use this rather than calling sess.Send directly, so the dashboard receives
// running/ready state transitions. The only exception is cron (internal/cron),
// which runs in a separate package and uses sess.Send directly since cron
// jobs are background tasks with their own notification path (BroadcastCronResult).
package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/naozhi/naozhi/internal/cli"
	"github.com/naozhi/naozhi/internal/discovery"
	"github.com/naozhi/naozhi/internal/session"
)

// sendWithBroadcast wraps sess.Send with dashboard state broadcasts.
// Broadcasts "running" before send, and the final session snapshot after send.
// This is the canonical implementation; Server.sendWithBroadcast delegates here.
//
// sess must be non-nil; callers must check the error from GetOrCreate first.
func (h *Hub) sendWithBroadcast(
	ctx context.Context,
	key string,
	sess *session.ManagedSession,
	text string,
	images []cli.ImageData,
	onEvent cli.EventCallback,
) (*cli.SendResult, error) {
	// Notify ALL dashboard clients that this session is running so they can
	// auto-subscribe. Uses BroadcastSessionReady (sends to all authenticated
	// clients) instead of broadcastState (only subscribed clients), because
	// for new sessions nobody is subscribed yet.
	h.BroadcastSessionReady(key)
	h.BroadcastSessionsUpdate()

	result, err := sess.Send(ctx, text, images, onEvent)

	// Broadcast final state (ready or suspended) after Send completes.
	if rs := h.router.GetSession(key); rs != nil {
		snap := rs.Snapshot()
		h.broadcastState(key, snap.State, snap.DeathReason)
	}
	h.BroadcastSessionsUpdate()

	return result, err
}

// sendWithBroadcast is a nil-safe delegation to Hub.sendWithBroadcast.
// When the dashboard is not registered (hub is nil, e.g. in tests or headless mode),
// falls back to a direct sess.Send without broadcasts.
//
// sess must be non-nil; callers must check the error from GetOrCreate first.
func (s *Server) sendWithBroadcast(
	ctx context.Context,
	key string,
	sess *session.ManagedSession,
	text string,
	images []cli.ImageData,
	onEvent cli.EventCallback,
) (*cli.SendResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("sendWithBroadcast: session is nil")
	}
	if s.hub != nil {
		return s.hub.sendWithBroadcast(ctx, key, sess, text, images, onEvent)
	}
	return sess.Send(ctx, text, images, onEvent)
}

// sendParams holds parsed input for a session send request.
// Both HTTP and WebSocket callers construct this after their own input parsing.
type sendParams struct {
	Key       string
	Text      string
	Images    []cli.ImageData
	Workspace string
	ResumeID  string
}

// sessionSend validates and dispatches a send request.
// Returns (true, nil) if the request was a /clear or /new reset.
// Returns (false, err) if validation failed (workspace forbidden, etc.).
// Returns (false, nil) if accepted — a background goroutine handles the send.
//
// onAsyncError is called from the goroutine if GetOrCreate or guard timeout
// fails; it may be nil (HTTP path has no back-channel after 202).
func (h *Hub) sessionSend(p sendParams, onAsyncError func(string)) (bool, error) {
	key := p.Key

	// Handle slash commands locally — CLI built-in doesn't work in stream-json,
	// and dispatch-layer commands (/ls, /help, /cd, /pwd) only run for IM platforms.
	trimmed := strings.TrimSpace(p.Text)
	if trimmed == "/clear" || trimmed == "/new" {
		h.router.Reset(key)
		h.BroadcastSessionsUpdate()
		return true, nil
	}

	// Dashboard commands (/ls, /help, /cd, /pwd) are handled by callers
	// (Hub.handleSend / SendHandler.handleSend) BEFORE calling sessionSend,
	// because each caller has a different response mechanism (WS vs HTTP).

	// Workspace validation
	var validatedWorkspace string
	if p.Workspace != "" {
		wsPath, err := validateWorkspace(p.Workspace, h.allowedRoot)
		if err != nil {
			return false, err
		}
		validatedWorkspace = wsPath
		if idx := strings.LastIndexByte(key, ':'); idx >= 0 {
			h.router.SetWorkspace(key[:idx], wsPath)
		}
	}

	// Resume registration
	if p.ResumeID != "" && discovery.IsValidSessionID(p.ResumeID) {
		ws := validatedWorkspace
		if ws == "" {
			ws = h.router.DefaultWorkspace()
		}
		h.router.RegisterForResume(key, p.ResumeID, ws, "")
	}

	// Guard acquire/interrupt
	acquired := h.guard.TryAcquire(key)
	needInterrupt := !acquired
	if needInterrupt {
		h.router.InterruptSession(key)
		slog.Info("send: interrupted running session", "key", key)
	}

	// Background send
	text, images := p.Text, p.Images
	go func() {
		sendStart := time.Now()
		if needInterrupt {
			if !h.guard.AcquireTimeout(h.ctx, key, 2*time.Second) {
				slog.Error("send: interrupt timed out", "key", key)
				if onAsyncError != nil {
					onAsyncError("session busy, interrupt timed out")
				}
				return
			}
		}
		defer h.guard.Release(key)
		defer h.router.NotifyIdle()

		opts := buildSessionOpts(key, h.agents, h.projectMgr)
		sess, status, err := h.router.GetOrCreate(h.ctx, key, opts)
		if err != nil {
			slog.Error("send: get session", "key", key, "err", err)
			if onAsyncError != nil {
				onAsyncError(err.Error())
			}
			return
		}
		if status != session.SessionExisting {
			slog.Info("send: session spawned", "key", key, "status", status, "elapsed", time.Since(sendStart).Round(time.Millisecond))
		}

		if _, err := h.sendWithBroadcast(h.ctx, key, sess, text, images, nil); err != nil {
			slog.Error("send: send", "key", key, "err", err)
		} else if h.scheduler != nil && strings.HasPrefix(key, "cron:") {
			if err := h.scheduler.SetJobPrompt(strings.TrimPrefix(key, "cron:"), text); err != nil {
				slog.Warn("send: set cron prompt", "key", key, "err", err)
			}
		}
		slog.Info("send: turn complete", "key", key, "elapsed", time.Since(sendStart).Round(time.Millisecond))
	}()

	return false, nil
}

// handleDashboardCommand handles slash commands that the dispatch layer handles
// for IM platforms but that Dashboard messages bypass. Returns (result, true)
// if the command was handled, ("", false) otherwise.
func (h *Hub) handleDashboardCommand(key, trimmed string) (string, bool) {
	// Extract chat key (strip agent suffix) for workspace lookup
	chatKey := key
	if idx := strings.LastIndexByte(key, ':'); idx >= 0 {
		chatKey = key[:idx]
	}

	switch {
	case trimmed == "/help":
		help := "可用命令:\n" +
			"  /help — 显示此帮助\n" +
			"  /new — 重置会话\n" +
			"  /clear — 重置会话（同 /new）\n" +
			"  /ls [路径] — 列出目录内容\n" +
			"  /cd <路径> — 切换工作目录\n" +
			"  /pwd — 显示当前工作目录\n" +
			"  /rename [名称] — 命名当前会话（空清除）"
		// Append agent commands if any are configured
		if len(h.agentCmds) > 0 {
			help += "\n\n可用 Agent:"
			for cmd, agentID := range h.agentCmds {
				help += "\n  /" + cmd + " → " + agentID
			}
		}
		help += "\n\n💡 项目管理和定时任务请使用顶栏面板按钮"
		return help, true

	case trimmed == "/pwd":
		ws := h.router.GetWorkspace(chatKey)
		return "当前工作目录: " + ws, true

	case strings.HasPrefix(trimmed, "/cd "):
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "/cd"))
		if path == "" {
			return "用法: /cd <目录路径>", true
		}
		if strings.HasPrefix(path, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[1:])
			}
		}
		var absPath string
		if filepath.IsAbs(path) {
			absPath = filepath.Clean(path)
		} else {
			absPath = filepath.Join(h.router.GetWorkspace(chatKey), path)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			return "❌ 目录不存在: " + absPath, true
		}
		if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
			absPath = resolved
		}
		h.router.SetWorkspace(chatKey, absPath)
		h.router.ResetChat(chatKey)
		h.BroadcastSessionsUpdate()
		return "工作目录已切换到: " + absPath + "\n所有会话已重置。", true

	case trimmed == "/ls" || strings.HasPrefix(trimmed, "/ls "):
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/ls"))
		cwd := h.router.GetWorkspace(chatKey)

		var target string
		switch {
		case arg == "":
			target = cwd
		case strings.HasPrefix(arg, "~"):
			if home, err := os.UserHomeDir(); err == nil {
				target = filepath.Join(home, arg[1:])
			} else {
				return "❌ 无法获取主目录", true
			}
		case filepath.IsAbs(arg):
			target = filepath.Clean(arg)
		default:
			target = filepath.Join(cwd, arg)
		}

		info, err := os.Stat(target)
		if err != nil {
			return "❌ 路径不存在: " + target, true
		}
		if !info.IsDir() {
			return "❌ 不是目录: " + target, true
		}

		entries, err := os.ReadDir(target)
		if err != nil {
			return "❌ 权限不足: " + target, true
		}

		var dirs, files []os.DirEntry
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if e.IsDir() {
				dirs = append(dirs, e)
			} else {
				files = append(files, e)
			}
		}

		var sb strings.Builder
		sb.WriteString("📂 " + target + "\n\n")
		const maxItems = 50
		shown := 0

		for _, e := range dirs {
			if shown >= maxItems {
				break
			}
			sub, _ := os.ReadDir(filepath.Join(target, e.Name()))
			count := 0
			for _, se := range sub {
				if !strings.HasPrefix(se.Name(), ".") {
					count++
				}
			}
			fmt.Fprintf(&sb, "  📁 %s/", e.Name())
			if count > 0 {
				fmt.Fprintf(&sb, "    %d items", count)
			}
			sb.WriteString("\n")
			shown++
		}
		for _, e := range files {
			if shown >= maxItems {
				break
			}
			fi, _ := e.Info()
			if fi != nil {
				fmt.Fprintf(&sb, "  📄 %s    %s\n", e.Name(), formatSize(fi.Size()))
			} else {
				fmt.Fprintf(&sb, "  📄 %s\n", e.Name())
			}
			shown++
		}
		total := len(dirs) + len(files)
		if total > maxItems {
			fmt.Fprintf(&sb, "  ... and %d more items\n", total-maxItems)
		}
		fmt.Fprintf(&sb, "\n%d items (%d dirs, %d files)", total, len(dirs), len(files))

		slog.Info("dashboard /ls", "path", target, "items", total)
		return sb.String(), true

	case trimmed == "/rename" || strings.HasPrefix(trimmed, "/rename "):
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/rename"))
		if arg == "" {
			h.router.RenameSession(key, "")
			h.BroadcastSessionsUpdate()
			return "Session 名称已清除", true
		}
		h.router.RenameSession(key, arg)
		h.BroadcastSessionsUpdate()
		return "Session 已命名: " + arg, true

	default:
		return "", false
	}
}
