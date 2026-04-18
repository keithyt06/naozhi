# Phase 0: Meeting Feature Removal — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Completely remove the Meeting Intelligence feature (backend handlers + domain code + frontend UI + persisted data + doc references) so the codebase is clean for the Phase 1 frontend split and Phase 2 DDD refactor.

**Architecture:** Pure deletion — no new abstractions. Backend: drop 3 files + surgical edits to `server.go` and `dashboard.go`. Frontend: cut 4 sections from `dashboard.html` (CSS block, nav button, switchView case, JS block). Data: remove `~/.naozhi/meetings.json` and `~/.naozhi/meetings/` on EC2. Docs: remove README section, add deprecation banner to historical spec/plan files.

**Tech Stack:** Go (`net/http`, `embed`), vanilla JS in single HTML file, systemd on Ubuntu EC2.

**TDD note:** This plan is a deletion — there are no new behaviours to test. Verification steps rely on (a) `go build`, (b) `go test ./...` not regressing, (c) `grep` returning empty for residual references, (d) manual smoke test of the live dashboard.

---

## File Structure

### Files to delete

| Path | Reason |
|---|---|
| `internal/knowledge/meeting.go` (116 lines) | `Meeting`, `MeetingStore` types + JSON store |
| `internal/knowledge/meeting_processor.go` (189 lines) | Async audio ingest + transcribe pipeline |
| `internal/server/dashboard_meeting.go` (150 lines) | `MeetingHandlers` HTTP handlers |

### Files to modify

| Path | Change |
|---|---|
| `internal/server/server.go` | Remove field (line 72), remove init block (lines 329-336) |
| `internal/server/dashboard.go` | Remove routes (lines 155-160) |
| `internal/server/static/dashboard.html` | Remove CSS (931-958), nav button (1060), switchView case (5580), JS block (6906-7041) |
| `README.md` | Remove Meeting Intelligence feature description |
| `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md` | Add deprecation banner |
| `docs/superpowers/plans/2026-04-15-naozhi-v2-phase4-platform-moonshots.md` | Add deprecation banner |

### Runtime data to purge (EC2)

- `/root/.naozhi/meetings.json`
- `/root/.naozhi/meetings/` (audio files directory)

---

## Task 1: Baseline verification

**Files:** none modified.

- [ ] **Step 1.1: Confirm clean working tree**

Run: `git status`
Expected: branch `naozhi-2.0`, no uncommitted meeting-related changes. Any unrelated `M`/untracked files that existed before (docs, plans) are acceptable — note them.

- [ ] **Step 1.2: Confirm baseline build passes**

Run: `go build ./...`
Expected: exits 0, no output.

- [ ] **Step 1.3: Confirm baseline tests pass**

Run: `go test ./... 2>&1 | tail -20`
Expected: all green. If any pre-existing failures unrelated to meeting, record them so we can distinguish later.

- [ ] **Step 1.4: Snapshot the meeting surface**

Run:
```bash
grep -rn -i "meeting" --include="*.go" --include="*.html" internal/ | wc -l
```
Expected: ~145 matches (sanity check — this number should drop to 0 by the end of the plan).

---

## Task 2: Remove meeting HTTP routes

**Files:** Modify `internal/server/dashboard.go:155-160`.

- [ ] **Step 2.1: Read surrounding context**

Run: `sed -n '150,165p' internal/server/dashboard.go`

Expect to see:
```go
	// Meeting API
	if s.meetingH != nil {
		s.mux.HandleFunc("GET /api/meetings", auth(s.meetingH.handleList))
		s.mux.HandleFunc("POST /api/meetings/upload", auth(s.meetingH.handleUpload))
		s.mux.HandleFunc("GET /api/meetings/", auth(s.meetingH.handleGet))
	}
```

- [ ] **Step 2.2: Delete the block**

Edit `internal/server/dashboard.go` — remove exactly these 6 lines (including the comment `// Meeting API` and the closing `}`).

- [ ] **Step 2.3: Verify**

Run: `grep -n "meeting\|Meeting" internal/server/dashboard.go`
Expected: no output.

---

## Task 3: Remove meeting handler init from server

**Files:** Modify `internal/server/server.go:72` and `internal/server/server.go:329-336`.

- [ ] **Step 3.1: Remove the struct field**

Open `internal/server/server.go` and find line 72 (`meetingH    *MeetingHandlers`). Delete that line.

Context (line 68-76 before):
```go
	knowledgeH  *KnowledgeHandlers
	graphH      *graph.Handlers
	meetingH    *MeetingHandlers   // ← delete this line
	bookmarkH   *BookmarkHandlers
	decisionH   *DecisionHandlers
```

- [ ] **Step 3.2: Remove the init block**

Find lines 329-336 (the `// Meeting handlers` comment and the `if naozDir != ""` block that initialises `meetingStore` / `audioDir` / `processor` / `s.meetingH`). Delete all 8 lines including the leading blank line.

Context before:
```go
		s.graphH = graph.NewHandlers(wiki.Dir())
	}

	// Meeting handlers (initialized when naozDir is available)
	if naozDir != "" {
		meetingStore := knowledge.NewMeetingStore(filepath.Join(naozDir, "meetings.json"))
		audioDir := filepath.Join(naozDir, "meetings")
		processor := knowledge.NewMeetingProcessor(meetingStore, opts.Transcriber, audioDir)
		s.meetingH = NewMeetingHandlers(meetingStore, processor)
	}

	// Replay handlers (C2: persist shares to disk)
```

After:
```go
		s.graphH = graph.NewHandlers(wiki.Dir())
	}

	// Replay handlers (C2: persist shares to disk)
```

- [ ] **Step 3.3: Verify**

Run: `grep -n "meeting\|Meeting" internal/server/server.go`
Expected: no output.

- [ ] **Step 3.4: Check for unused imports**

Run: `go build ./internal/server/ 2>&1`
Expected: fails with "undefined: MeetingHandlers" (the handler files are still around) — that's OK, Task 4 fixes it. If it fails with "imported and not used" for `filepath`, check whether another line in server.go still uses it (it will — lots of other init code uses filepath). No import removal needed.

---

## Task 4: Delete `dashboard_meeting.go`

**Files:** Delete `internal/server/dashboard_meeting.go`.

- [ ] **Step 4.1: Delete the file**

Run: `rm internal/server/dashboard_meeting.go`

- [ ] **Step 4.2: Verify no dangling references**

Run: `grep -rn "MeetingHandlers\|NewMeetingHandlers" internal/server/`
Expected: no output.

- [ ] **Step 4.3: Build**

Run: `go build ./internal/server/ 2>&1`
Expected: still fails — the `knowledge.NewMeetingStore` / `knowledge.NewMeetingProcessor` references were already removed in Task 3, so this should now succeed. If it fails for other reasons, stop and investigate.

If it succeeds, move on.

---

## Task 5: Delete meeting domain files

**Files:** Delete `internal/knowledge/meeting.go`, `internal/knowledge/meeting_processor.go`.

- [ ] **Step 5.1: Delete files**

Run:
```bash
rm internal/knowledge/meeting.go internal/knowledge/meeting_processor.go
```

- [ ] **Step 5.2: Verify no dangling references in knowledge package**

Run: `grep -n "Meeting" internal/knowledge/*.go`
Expected: no output.

- [ ] **Step 5.3: Full build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 5.4: Full test suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: same green baseline as Task 1. Any new failures → investigate before proceeding.

---

## Task 6: Commit backend removal

**Files:** staged changes from Tasks 2-5.

- [ ] **Step 6.1: Review staged diff**

Run:
```bash
git status
git diff --stat
```

Expected in diff-stat:
- `internal/server/server.go` (~9 lines removed)
- `internal/server/dashboard.go` (~6 lines removed)
- `internal/server/dashboard_meeting.go` deleted
- `internal/knowledge/meeting.go` deleted
- `internal/knowledge/meeting_processor.go` deleted

- [ ] **Step 6.2: Stage and commit**

Run:
```bash
git add internal/server/server.go \
        internal/server/dashboard.go \
        internal/server/dashboard_meeting.go \
        internal/knowledge/meeting.go \
        internal/knowledge/meeting_processor.go
git commit -m "$(cat <<'EOF'
refactor: remove Meeting Intelligence backend (Phase 0 step 1/4)

Delete MeetingStore, MeetingProcessor, MeetingHandlers and the
associated HTTP routes (GET /api/meetings, POST /api/meetings/upload,
GET /api/meetings/{id}). Frontend dashboard.html still references these
endpoints — cleaned in next commit.

Phase 0 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds. Build still passes (tests may temporarily fail if any test references meeting — there are none; we verified in Task 1).

---

## Task 7: Remove meeting JS block from `dashboard.html`

**Files:** Modify `internal/server/static/dashboard.html:6906-7041`.

- [ ] **Step 7.1: Read block boundaries**

Run:
```bash
sed -n '6904,6908p' internal/server/static/dashboard.html
sed -n '7039,7044p' internal/server/static/dashboard.html
```

Expected start (line 6906):
```
/* ===== Phase 4C: Meeting Intelligence ===== */
```
Expected end (right before line 7042):
```
/* ===== Task 14: Context Panel ===== */
```

- [ ] **Step 7.2: Delete lines 6906-7041 inclusive**

Do this in one edit. The range contains: `mtMeetings`/`mtCurrentId` globals, `renderMeetingView()`, `loadMeetingList()`, `loadMeetingDetail()`, `handleMeetingUpload()`, helpers. After deletion, line 6905 (which is blank/comment) should directly precede what was line 7042 (`/* ===== Task 14: Context Panel ===== */`).

- [ ] **Step 7.3: Verify**

Run: `grep -n "mtMeetings\|renderMeetingView\|loadMeetingList\|loadMeetingDetail\|handleMeetingUpload" internal/server/static/dashboard.html`
Expected: no output.

---

## Task 8: Remove meeting CSS block

**Files:** Modify `internal/server/static/dashboard.html:931-958`.

- [ ] **Step 8.1: Confirm block boundaries**

Run:
```bash
sed -n '929,932p' internal/server/static/dashboard.html
sed -n '957,961p' internal/server/static/dashboard.html
```

Expected start (line 931):
```
/* ===== Meeting Intelligence (Phase 4C) ===== */
```
Expected line 959 or 960 to be the next `/* ===== ... ===== */` block for a different feature.

- [ ] **Step 8.2: Delete lines 931-958**

Inclusive of both the start comment and the last CSS rule before the next comment.

- [ ] **Step 8.3: Verify**

Run: `grep -n "mt-item\|mt-upload-btn\|mt-list-title\|mt-empty\|Meeting Intelligence" internal/server/static/dashboard.html`
Expected: no output.

---

## Task 9: Remove meeting nav button and switchView case

**Files:** Modify `internal/server/static/dashboard.html:1060` and `:5580`.

- [ ] **Step 9.1: Remove nav button**

Find and delete this single line (was line 1060 before Task 8 — recalculate if edits shifted lines):

```html
      <button class="vb-btn" data-view="meetings" onclick="switchView('meetings',this)">Meetings</button>
```

- [ ] **Step 9.2: Remove switchView case**

Find and delete this single line (was 5580):

```js
    else if (view === 'meetings') renderMeetingView();
```

If this line is part of an `else if` chain, verify the chain still compiles: the preceding and following `else if` branches should join cleanly.

- [ ] **Step 9.3: Final verify**

Run:
```bash
grep -n -i "meeting" internal/server/static/dashboard.html
```
Expected: no output.

- [ ] **Step 9.4: Sanity check line counts**

Run: `wc -l internal/server/static/dashboard.html`
Expected: roughly `8309 - 164 = ~8145` lines.

---

## Task 10: Local smoke test of frontend

**Files:** none modified.

- [ ] **Step 10.1: Build binary**

Run:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/naozhi ./cmd/naozhi/
```
Expected: exits 0, `bin/naozhi` exists.

- [ ] **Step 10.2: Quick grep pass for embedded HTML**

Run:
```bash
strings bin/naozhi | grep -i "meeting" | head
```
Expected: no matches (the static HTML is embedded, so any residual "meeting" text would appear here).

If matches appear, revisit Tasks 7-9.

---

## Task 11: Commit frontend removal

- [ ] **Step 11.1: Review diff**

Run: `git diff --stat internal/server/static/dashboard.html`
Expected: ~164 lines removed.

- [ ] **Step 11.2: Commit**

Run:
```bash
git add internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
refactor: remove Meeting Intelligence frontend (Phase 0 step 2/4)

Drop the Meetings nav tab, switchView case, CSS block, and the ~135
lines of Meeting JS (mtMeetings state, renderMeetingView, loadMeetingList,
loadMeetingDetail, handleMeetingUpload). dashboard.html drops from 8309
to ~8145 lines. No more /api/meetings* calls anywhere in the frontend.

Phase 0 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Update README

**Files:** Modify `README.md`.

- [ ] **Step 12.1: Locate Meeting section**

Run: `grep -n -i "meeting" README.md`

Look at line numbers and read context around each with `sed -n '<line-10>,<line+10>p' README.md` to understand the boundaries.

- [ ] **Step 12.2: Remove Meeting Intelligence paragraph(s)**

Delete any `### Meeting Intelligence` (or equivalent) heading and its content. If Meeting is only mentioned inline (e.g. in a feature list), remove that bullet.

- [ ] **Step 12.3: Verify**

Run: `grep -n -i "meeting" README.md`
Expected: no output.

---

## Task 13: Add deprecation banners to historical docs

**Files:** Modify two historical docs (do **not** delete — they are history).

- [ ] **Step 13.1: Banner for dashboard-design spec**

Prepend a banner immediately after the H1 heading in `docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md`:

```markdown
> ⚠️  **Meeting Intelligence 功能已于 2026-04-18 下线**（见
> `docs/superpowers/specs/2026-04-18-naozhi-ddd-refactor-design.md` §6
> Phase 0）。本 spec 中涉及 Meeting 的章节仅作历史参考。
```

- [ ] **Step 13.2: Banner for phase4-moonshots plan**

Same banner style prepended to `docs/superpowers/plans/2026-04-15-naozhi-v2-phase4-platform-moonshots.md` after its H1.

- [ ] **Step 13.3: Verify banners exist**

Run:
```bash
head -10 docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md
head -10 docs/superpowers/plans/2026-04-15-naozhi-v2-phase4-platform-moonshots.md
```
Expected: both show the banner near the top.

---

## Task 14: Commit docs

- [ ] **Step 14.1: Commit**

Run:
```bash
git add README.md \
        docs/superpowers/specs/2026-04-15-naozhi-v2-dashboard-design.md \
        docs/superpowers/plans/2026-04-15-naozhi-v2-phase4-platform-moonshots.md
git commit -m "$(cat <<'EOF'
docs: remove Meeting from README, add deprecation banners (Phase 0 step 3/4)

README no longer advertises Meeting Intelligence. Historical v2 spec
and phase4 plan keep their meeting content for history, with a banner
pointing to the 2026-04-18 DDD refactor spec that deprecated it.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Final verification (local)

**Files:** none modified.

- [ ] **Step 15.1: Grep audit**

Run:
```bash
grep -rn -i "meeting" --include="*.go" --include="*.html" internal/ docs/superpowers/specs/2026-04-18* docs/superpowers/plans/2026-04-18*
```
Expected: **empty output** (the only remaining references should be in the deprecation banners + historical v2 docs, which are outside this grep scope by design).

- [ ] **Step 15.2: Build + test**

Run:
```bash
go build ./... && go test ./... 2>&1 | tail -10
```
Expected: build OK, tests green.

- [ ] **Step 15.3: `go vet`**

Run: `go vet ./...`
Expected: no issues.

---

## Task 16: Deploy to EC2

**Files:** deploy binary to production.

- [ ] **Step 16.1: Build production binary**

Run:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/naozhi ./cmd/naozhi/
ls -la bin/naozhi
```
Expected: `bin/naozhi` exists, ~20-40 MB.

- [ ] **Step 16.2: Upload**

Run:
```bash
PEM=/root/keith-space/AWS/AWS-Keith-Network/keith-secret.pem
EC2=ubuntu@10.0.11.189
scp -i $PEM bin/naozhi $EC2:/tmp/naozhi
```
Expected: upload succeeds.

- [ ] **Step 16.3: Replace + restart**

Run:
```bash
ssh -i $PEM $EC2 "sudo systemctl stop naozhi && \
  sudo mv /tmp/naozhi /usr/local/bin/naozhi && \
  sudo chmod +x /usr/local/bin/naozhi && \
  sudo systemctl start naozhi && \
  sleep 2 && sudo systemctl is-active naozhi"
```
Expected: prints `active`.

- [ ] **Step 16.4: Health check**

Run: `ssh -i $PEM $EC2 "curl -s http://localhost:8180/health"`
Expected: `{"cli_available":true,...,"status":"ok",...}`.

- [ ] **Step 16.5: API surface check**

Run: `ssh -i $PEM $EC2 "curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8180/api/meetings"`
Expected: `404` (route is gone — good). Any 200/500 → stop and investigate.

---

## Task 17: Purge EC2 meeting data (DESTRUCTIVE — requires confirmation)

**Files:** delete `/root/.naozhi/meetings.json` + `/root/.naozhi/meetings/` on EC2.

**⚠️  Destructive step — STOP and confirm with user before running.** Once deleted, meeting transcripts and audio are gone forever.

- [ ] **Step 17.1: Inspect what will be deleted**

Run:
```bash
ssh -i $PEM $EC2 "sudo ls -la /root/.naozhi/meetings.json /root/.naozhi/meetings/ 2>&1 | head -20 && sudo du -sh /root/.naozhi/meetings 2>/dev/null"
```
Report the output to the user for review.

- [ ] **Step 17.2: Backup first**

Run:
```bash
ssh -i $PEM $EC2 "sudo tar czf /root/.naozhi/meetings-backup-\$(date +%Y%m%d).tgz \
  /root/.naozhi/meetings.json /root/.naozhi/meetings/ 2>&1 | tail -5 && \
  sudo ls -la /root/.naozhi/meetings-backup-*.tgz"
```
Expected: tarball exists. If tar fails because the files don't exist at all (clean server), skip Steps 17.3-17.4.

- [ ] **Step 17.3: Confirm with user**

Ask: "About to delete `/root/.naozhi/meetings.json` and `/root/.naozhi/meetings/` on EC2. Backup saved to `/root/.naozhi/meetings-backup-YYYYMMDD.tgz`. Proceed?" Wait for explicit yes.

- [ ] **Step 17.4: Delete**

Run:
```bash
ssh -i $PEM $EC2 "sudo rm -f /root/.naozhi/meetings.json && \
  sudo rm -rf /root/.naozhi/meetings/ && \
  sudo ls -la /root/.naozhi/ | grep meeting"
```
Expected: last grep returns only the backup `.tgz` file.

---

## Task 18: Production smoke test + final commit tag

**Files:** no code changes, tag the commit.

- [ ] **Step 18.1: Manual dashboard smoke test**

From user's phone or browser:
1. Load `https://naozhi.keithyu.cloud/dashboard`
2. Confirm no **Meetings** tab in the top nav
3. Click through remaining tabs: Chat, Knowledge, Wiki, Patrols, Approvals, Graph — each should render without console errors
4. Open browser DevTools → Network → refresh → confirm no failed `/api/meetings*` requests

Report PASS/FAIL to user.

- [ ] **Step 18.2: Check logs for residual errors**

Run:
```bash
ssh -i $PEM $EC2 "sudo journalctl -u naozhi -n 100 --no-pager | grep -i 'meeting\|error' | head"
```
Expected: only historical log entries predating this restart; no new errors.

- [ ] **Step 18.3: Tag the commit**

Run:
```bash
git tag -a phase-0-meeting-removed -m "Phase 0 complete: Meeting Intelligence removed (backend + frontend + data + docs)"
```

- [ ] **Step 18.4: Update deployment manual changelog**

Add a one-line entry to `/root/keith-space/AWS/EC2-Workload/naozhi-deployment.md` under Changelog:

```markdown
### 2026-04-18 — Meeting 功能下线

- 删除后端 3 个文件（`knowledge/meeting*.go` + `server/dashboard_meeting.go`）
- 删除 `server.go`/`dashboard.go` 的 meeting 注册
- `dashboard.html` 移除 Meetings tab、CSS、135 行 JS（8309 → ~8145 行）
- EC2 purge `/root/.naozhi/meetings.json` + `meetings/`（backup `.tgz` 保留）
- 上线验证：`/api/meetings` 返回 404，手机端各视图无报错
```

Commit in the parent `keith-space` repo (not the naozhi repo):

```bash
cd /root/keith-space
git add AWS/EC2-Workload/naozhi-deployment.md
git commit -m "docs: naozhi deployment — record Phase 0 meeting removal

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Acceptance

Phase 0 is complete when **all** of the following hold:

- [ ] `go build ./... && go test ./...` green (Task 15.2)
- [ ] `grep -rn -i "meeting" --include="*.go" --include="*.html" internal/` empty (Task 15.1)
- [ ] EC2 `curl /health` returns `status:ok` (Task 16.4)
- [ ] EC2 `curl /api/meetings` returns 404 (Task 16.5)
- [ ] Dashboard has no Meetings tab, all other tabs work (Task 18.1)
- [ ] EC2 `/root/.naozhi/meetings.json` + `meetings/` deleted (Task 17.4)
- [ ] Commit `phase-0-meeting-removed` tagged (Task 18.3)
- [ ] `naozhi-deployment.md` changelog entry committed (Task 18.4)

## Next

Upon sign-off, Phase 1 (frontend split) writing-plans session begins.
