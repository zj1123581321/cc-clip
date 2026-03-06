# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Does

cc-clip bridges your local Mac clipboard to a remote Linux server over SSH, so `Ctrl+V` image paste works in remote Claude Code sessions. It uses an xclip/wl-paste shim that transparently intercepts only Claude Code's clipboard calls and fetches image data through an SSH tunnel from a local daemon.

```
Local Mac clipboard → pngpaste → HTTP daemon (127.0.0.1:18339) → SSH RemoteForward → xclip shim → Claude Code
```

## Build & Test Commands

```bash
make build                          # Build binary with version from git tags
make test                           # Run all tests (go test ./... -count=1)
make vet                            # Run go vet
go test ./internal/tunnel/ -v -run TestFetchImageRoundTrip  # Single test
make release-local                  # Build for all platforms (dist/)
```

Version is injected via `-X main.version=$(VERSION)` ldflags. The `version` variable in `cmd/cc-clip/main.go` defaults to `"dev"`.

## Architecture

### Data Flow

1. **daemon** (`internal/daemon/`) — HTTP server on loopback, reads Mac clipboard via `pngpaste`, serves images at `GET /clipboard/type` and `GET /clipboard/image`. Auth via Bearer token + User-Agent whitelist.
2. **tunnel** (`internal/tunnel/`) — Client-side HTTP calls through the SSH-forwarded port. `Probe()` checks TCP connectivity. `Client.FetchImage()` downloads and saves with timestamp+random filename.
3. **shim** (`internal/shim/template.go`) — Bash script templates for xclip and wl-paste. Intercepts two specific invocation patterns Claude Code uses, fetches via curl through tunnel, falls back to real binary on any failure.
4. **connect** (`cmd/cc-clip/main.go:cmdConnect`) — Orchestrates deployment via SSH master session: detect remote arch → incremental binary upload (hash-based skip) → install shim → sync token → verify tunnel. Supports `--force`, `--token-only` flags.
5. **ssh** (`internal/shim/ssh.go`) — `SSHSession` wraps a ControlMaster SSH connection. Single passphrase prompt; all subsequent `Exec()` and `Upload()` calls reuse the master.
6. **deploy** (`internal/shim/deploy.go`) — `DeployState` tracks binary hash, version, shim status on the remote. JSON file at `~/.cache/cc-clip/deploy-state.json`. `NeedsUpload()` / `NeedsShimInstall()` enable incremental deploys.
7. **pathfix** (`internal/shim/pathfix.go`) — Auto-detects remote shell (bash/zsh/fish) and injects `~/.local/bin` PATH marker into rc file with `# cc-clip-managed` guards.
8. **service** (`internal/service/launchd.go`) — macOS launchd integration: `Install()`, `Uninstall()`, `Status()`. Generates plist for auto-start daemon.

### Key Design Decisions

- **Shim is a bash script, not a binary** — installed to `~/.local/bin/` with PATH priority over `/usr/bin/xclip`. Uses `which -a` to find the real binary, skipping its own directory.
- **Token is the daemon's token** — `cc-clip serve` generates a single token; `connect` reads it from the file and sends it to remote. Never generate a second token.
- **Binary-safe image transfer** in shim — `_cc_clip_fetch_binary()` uses `mktemp` + `curl -o tmpfile` + `cat tmpfile`, not shell variables (which strip NUL bytes) or `exec curl` (which prevents fallback). After curl succeeds, `[ ! -s "$tmpfile" ]` guards against empty responses (e.g., HTTP 204), returning exit code 10 to trigger fallback instead of outputting empty data.
- **Server-side empty guard** — `handleClipboardImage` checks `len(data) == 0` after `ImageBytes()` and returns 204, preventing 200 with empty body even if the clipboard reader returns empty data without error.
- **Exit codes are segmented** (`internal/exitcode/`) — 0 success, 10-13 business errors (no image, tunnel down, bad token, download failed), 20+ internal. Business codes trigger transparent fallback in the shim.
- **Platform clipboard** — `clipboard_darwin.go` (pngpaste), `clipboard_linux.go` (xclip/wl-paste), `clipboard_windows.go` (PowerShell, not shipped in releases yet).

### Token Lifecycle

`token.Manager` holds the session in memory. `LoadOrGenerate(ttl)` reuses an unexpired token from disk, or generates a new one. Token file at `~/.cache/cc-clip/session.token` (chmod 600) stores `token\nexpires_at_rfc3339`. `ReadTokenFileWithExpiry()` returns both token and expiry. `token.TokenDirOverride` exists for test isolation — tests set it to `t.TempDir()` to avoid polluting the real cache directory. `--rotate-token` flag forces new token generation ignoring existing.

### Test Patterns

- `internal/daemon/server_test.go` uses a mock `ClipboardReader` — no real clipboard access needed.
- `internal/tunnel/fetch_test.go` uses `newIPv4TestServer(t, handler)` which forces IPv4 binding and calls `t.Skipf` (not panic) if binding fails in restricted environments.
- `internal/shim/install_test.go` uses temp directories to test shim installation without touching real PATH.

### Shim Interception Patterns

The shim only intercepts these exact Claude Code invocations:
- xclip: `*"-selection clipboard"*"-t TARGETS"*"-o"*` and `*"-selection clipboard"*"-t image/"*"-o"*`
- wl-paste: `*"--list-types"*` and `*"--type"*"image/"*`

Everything else passes through to the real binary via `exec`.

## Cross-Architecture Binary Delivery

When `connect` detects a different remote arch (e.g., Mac arm64 → Linux amd64), it tries in order:
1. Download matching binary from GitHub Releases (needs non-`dev` version)
2. Cross-compile locally (needs Go toolchain + source)
3. Fail with actionable `--local-bin` instruction

## Known Pitfalls

- **SSH ControlMaster + RemoteForward**: If the user has `ControlMaster auto` globally, a pre-existing master connection without `RemoteForward` will be reused. The tunnel silently fails. Fix: set `ControlMaster no` and `ControlPath none` on hosts that need `RemoteForward`.
- **Token rotation on daemon restart**: Mitigated by token persistence — `LoadOrGenerate` reuses unexpired tokens. Use `cc-clip connect <host> --token-only` if only the token changed.
- **Empty image race condition**: The clipboard can change between the TARGETS check (returns "image") and the image fetch (returns 204 No Content). `curl -sf` treats 204 as success → shim outputs empty bytes → Claude Code API rejects empty base64. Guarded by `[ ! -s "$tmpfile" ]` check in `_cc_clip_fetch_binary()`.
- **Remote xclip must exist**: The shim hardcodes the real xclip path at install time. If xclip is not installed on the remote, the shim fallback fails with "No such file or directory".
- **`~/.local/bin` PATH priority**: The shim only works if `~/.local/bin` comes before `/usr/bin` in PATH. Non-interactive SSH commands may not source `.bashrc`, so the `connect` command's `which xclip` check can show the wrong result. Interactive shells (where Claude Code runs) typically source `.bashrc` correctly.

## Files That Need Coordinated Changes

- Adding a new API endpoint: `daemon/server.go` (handler) + `tunnel/fetch.go` (client method) + `shim/template.go` (bash interception pattern)
- Changing token format: `token/token.go` + `shim/connect.go:WriteRemoteToken` + shim templates (`_cc_clip_read_token`)
- Adding a new exit code: `exitcode/exitcode.go` + `cmd/cc-clip/main.go:classifyError` + shim templates (return codes)
