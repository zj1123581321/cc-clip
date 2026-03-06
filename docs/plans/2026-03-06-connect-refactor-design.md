# Design: connect Refactor + Daemon Service

**Date:** 2026-03-06
**Status:** Approved
**Issues:** #1, #2, #3, #4, #5

## Goal

Resolve all 6 known pain points in a single coordinated effort, executed in parallel by 3 agent tracks.

## Pain Points Addressed

| # | Problem | Solution |
|---|---------|----------|
| 1 | Daemon restart requires full re-connect | Token persistence across restarts |
| 2 | connect re-compiles/uploads every time | Incremental deploy with remote state file |
| 3 | SSH passphrase prompted multiple times | Single SSH master session within connect |
| 4 | Remote PATH not auto-configured | Idempotent shell rc injection |
| 5 | Daemon has no auto-start | launchd plist integration |
| 6 | Multiple terminal windows needed | Daemon as launchd service + one-shot connect |

## Design

### 1. Token Persistence (Agent A)

**Current:** `cc-clip serve` always generates a new token. Restart = new token = must re-connect.

**New:** `serve` checks for existing token file (`~/.cache/cc-clip/session.token`). If present and not expired, reuse it. Only generate new token when:
- No token file exists
- Token has expired (past TTL)
- User explicitly requests: `cc-clip serve --rotate-token`

**Changes:**
- `token.Manager.Generate()` → `token.Manager.LoadOrGenerate()`
- `token.ReadTokenFile()` gains expiry check (embed `expires_at` in token file or use file mtime + TTL)
- Token file format: line 1 = token string, line 2 = ISO8601 expiry timestamp
- `cc-clip serve --rotate-token` forces new token generation

### 2. Daemon Service (Agent A)

**New commands:**
- `cc-clip service install` — write `~/Library/LaunchAgents/com.cc-clip.daemon.plist`, load via `launchctl`
- `cc-clip service uninstall` — unload and remove plist
- `cc-clip service status` — check if launchd job is running

**Plist spec:**
- `RunAtLoad: true`
- `KeepAlive: true`
- `StandardOutPath: ~/Library/Logs/cc-clip.log`
- `StandardErrorPath: ~/Library/Logs/cc-clip.log`
- Program: absolute path to cc-clip binary
- Arguments: `["cc-clip", "serve"]`

**No Linux systemd for now** — daemon only runs on local Mac.

### 3. Connect Single SSH Session (Agent B)

**Current:** `connect` spawns separate `ssh` / `scp` processes for each step (6+ SSH connections).

**New:** `connect` establishes one SSH ControlMaster at the start, all subsequent operations reuse it.

```
connect start:
  1. Start temporary SSH master: ssh -fN -o ControlMaster=yes -o ControlPath=/tmp/cc-clip-%C <host>
  2. All RemoteExec/UploadBinary calls use: ssh -o ControlPath=/tmp/cc-clip-%C <host>
  3. On exit (success or failure): ssh -O exit -o ControlPath=/tmp/cc-clip-%C <host>
```

**Changes:**
- New `internal/shim/ssh.go`: `SSHSession` struct managing master lifecycle
- `SSHSession.Start(host)` — establishes master, returns control path
- `SSHSession.Exec(cmd)` — runs command via master
- `SSHSession.Upload(local, remote)` — scp via master
- `SSHSession.Close()` — kills master (deferred in connect)
- Refactor `RemoteExec`, `UploadBinary` to use `SSHSession`

### 4. Incremental Deploy (Agent B)

**Current:** Every `connect` re-compiles and re-uploads binary, even if unchanged.

**New:** Remote state file at `~/.cache/cc-clip/deploy.json`:

```json
{
  "binary_hash": "sha256:abc123...",
  "binary_version": "v0.1.0-1-gcc6b0b2",
  "shim_installed": true,
  "shim_target": "xclip",
  "path_fixed": true
}
```

**Connect flow:**
1. Read remote `deploy.json` (if exists)
2. Compare `binary_hash` with local binary hash
3. If match → skip compile + upload
4. If mismatch or missing → full deploy
5. Always sync token (cheap operation)
6. Always verify tunnel

**Flags:**
- `--force` — ignore state, full redeploy
- `--token-only` — explicit token-only sync (skip all checks)

### 5. PATH Auto-Fix (Agent C)

**Current:** Prints WARNING and manual instructions.

**New:** Idempotent injection into remote shell rc file.

**Marker-based injection:**
```bash
# >>> cc-clip PATH (do not edit) >>>
export PATH="$HOME/.local/bin:$PATH"
# <<< cc-clip PATH (do not edit) <<<
```

**Logic:**
1. Detect remote shell: check `$SHELL` on remote
2. Target file: `~/.bashrc` for bash, `~/.zshrc` for zsh
3. Check if marker block already exists → skip if present
4. Append marker block
5. `--no-path-fix` flag to disable

**Uninstall:** `cc-clip uninstall` removes the marker block from rc file.

### 6. Doctor/Status Enhancements (Agent C)

- `doctor` checks deploy.json consistency
- `doctor` checks token expiry
- `status` shows launchd service state (if installed)
- `doctor --host` verifies PATH priority on remote

## Agent Team Execution

### Agent A: Token Persistence + Daemon Service
- `internal/token/token.go` — LoadOrGenerate, expiry-aware token file
- `internal/token/token_test.go` — tests for reuse, expiry, rotation
- `cmd/cc-clip/main.go` — `service install/uninstall/status` subcommands
- `internal/service/launchd.go` — plist generation, launchctl calls
- `internal/service/launchd_test.go`

### Agent B: Connect SSH Session + Incremental Deploy
- `internal/shim/ssh.go` — SSHSession (master lifecycle)
- `internal/shim/ssh_test.go`
- `internal/shim/deploy.go` — remote state file read/write, hash comparison
- `internal/shim/deploy_test.go`
- `cmd/cc-clip/main.go` — refactor cmdConnect to use SSHSession + incremental

### Agent C: PATH Auto-Fix + Doctor Enhancements
- `internal/shim/pathfix.go` — marker-based rc injection
- `internal/shim/pathfix_test.go`
- `internal/doctor/` — enhanced checks
- Regression tests for all changed flows

## Verification

Each agent runs tests independently. After all three merge:
- `make test` passes
- `make vet` passes
- Manual E2E: `cc-clip serve` → `cc-clip connect` → verify all 6 pain points resolved
