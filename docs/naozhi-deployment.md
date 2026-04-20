# Naozhi Deployment Manual

Production deployment record + runbook for the naozhi EC2 node.

- **EC2 host**: `ubuntu@10.0.11.189:8180` (x86_64 / amd64)
- **Binary path**: `/usr/local/bin/naozhi`
- **Systemd unit**: `naozhi.service` (see `deploy/naozhi.service`)
- **Branch**: `naozhi-2.0`
- **Config**: `/root/.naozhi/config.yaml` on the EC2

## Standard deploy steps

```bash
# From repo root on the build host
go run ./tools/hashstatic                                    # regenerate internal/server/static/dist/
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/naozhi-phase-N ./cmd/naozhi/
file /tmp/naozhi-phase-N                                     # verify ELF x86-64

ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi /usr/local/bin/naozhi.pre-phase-N'
scp /tmp/naozhi-phase-N ubuntu@10.0.11.189:/tmp/naozhi-phase-N-keith
ssh ubuntu@10.0.11.189 'sudo install -m 755 /tmp/naozhi-phase-N-keith /usr/local/bin/naozhi && sudo systemctl restart naozhi'

ssh ubuntu@10.0.11.189 'sudo systemctl status naozhi --no-pager | head -20'
ssh ubuntu@10.0.11.189 'curl -sf http://localhost:8180/health'
```

`go run ./tools/hashstatic` MUST run before `go build` — the `//go:embed all:static` directive captures `internal/server/static/dist/` at compile time, so a stale or missing `dist/` ships a binary without hashed asset URLs.

## Rollback (universal)

```bash
ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi.pre-phase-N /usr/local/bin/naozhi && sudo systemctl restart naozhi'
```

The previous binary is always preserved as `/usr/local/bin/naozhi.pre-phase-N` right before install — use the most recent one.

---

## Changelog

### Phase 1 — Frontend dashboard split (2026-04-19)

- **Commit range**: `db6324d` → `e4f7334` (16 commits, Tasks 1-17 of the Phase 1 plan)
- **Tag**: `phase-1-frontend-split` (on `e4f7334`, local only — not pushed to origin)
- **Binary**: 38 MB (`/tmp/naozhi-phase1`, SHA256 `2f4f482ad82540413061ecd588979006080d7a18beaf5d1498a46f5debc890d8`)
- **Backup**: `/usr/local/bin/naozhi.pre-phase1` on EC2

**What changed**:
- Dashboard inline `<script>` / `<style>` extracted into ES modules under `internal/server/static/{js,css}/`
- Router shell + lazy per-view loading via dynamic `import()` (home/chat/knowledge/wiki/patrols/approvals/graph)
- Content-hash pipeline (`tools/hashstatic`) emits `internal/server/static/dist/` with fingerprinted filenames; dashboard HTML is rewritten to reference the hashed paths; served with `Cache-Control: public, max-age=31536000, immutable`
- First-paint JS budget measured at ~70 KB gzip (structural split only; further reduction is Phase 3)

**Deploy steps executed**:
1. `go run ./tools/hashstatic` — wrote manifest + 24 hashed files under `dist/`
2. `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/naozhi-phase1 ./cmd/naozhi/`
3. Backed up live binary to `/usr/local/bin/naozhi.pre-phase1`
4. `scp` + `sudo install -m 755` + `sudo systemctl restart naozhi`
5. Verified `/health` returns `status: ok`, systemctl reports `active (running)`
6. Spot-checked `/static/dist/manifest.json` entries, confirmed dashboard references `app.c2f077ac.js` with `Cache-Control: public, max-age=31536000, immutable`

**Rollback**:
```bash
ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi.pre-phase1 /usr/local/bin/naozhi && sudo systemctl restart naozhi'
```

**Perf note**: First-paint JS ≈ 70 KB gzip. This is the structural split baseline; additional shrink (tree-shake, code-split further, defer legacy) is deferred to Phase 3.

### Phase 3 — legacy.js feature split (2026-04-19)

**Tag:** `phase-3-legacy-split`
**Binary backup:** `/usr/local/bin/naozhi.pre-phase3`
**Deployed:** 2026-04-20 02:43 UTC on 10.0.11.189 (this host)

**What changed:**
- `internal/server/static/js/legacy.js`: 2728 → 1163 lines (−57%).
- `internal/server/static/js/features/`: 8 new lazy modules
  (context-panel, notif-enhance, bookmark, twin, replay, cron,
  file-hub, search), all loaded via dynamic `import()` end-oil shims.
- First-paint JS gzip: 70.1 KB → 55.3 KB (−21%). `legacy.js` alone
  shrank 27.9 KB → 13.2 KB gzip. Missed the <50 KB target by 5.3 KB;
  the remaining bulk is `views/chat.js` (25.2 KB gzip), a Phase 3
  non-goal. Follow-up chat.js split tracked as Phase 4.

**Deploy steps executed** (build host = production host; no SSH needed):
1. `go run ./tools/hashstatic` — wrote manifest + 32 hashed files under `dist/`
2. `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/naozhi-phase3 ./cmd/naozhi/`
3. `sudo cp /usr/local/bin/naozhi /usr/local/bin/naozhi.pre-phase3`
4. `sudo install -m 755 /tmp/naozhi-phase3 /usr/local/bin/naozhi`
5. `sudo systemctl restart naozhi`
6. Smoke: `/health` returned `status: ok`, `/dashboard` 200, all 8
   feature chunks return 200 with
   `Cache-Control: public, max-age=31536000, immutable`.

**Rollback:**
```bash
sudo cp /usr/local/bin/naozhi.pre-phase3 /usr/local/bin/naozhi && sudo systemctl restart naozhi
```
