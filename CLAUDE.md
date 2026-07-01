# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o builder ./cmd/builder

# Test
go test ./...

# Run a single test package
go test ./internal/config/...

# Install to PATH (Unix)
make install
```

## Architecture

`android-builder` is a CLI tool (`builder`) that orchestrates remote Android APK builds via GitHub Actions and hot-reload dev sessions via `flutter attach`. It is a Go module (`github.com/Maortz/android-builder`) using Cobra for commands.

### Request flow

1. **Auth** (`internal/auth/`) — Token is read from OS keyring (via `go-keyring`) with fallback to `~/.config/android-builder/token`. `builder auth github` runs GitHub OAuth Device Flow (`device.go`) to obtain a token without requiring a PAT.

2. **Init** (`cmd/builder/init.go`, `internal/workflow/`) — Detects the GitHub remote, writes `builder.json` (project config) and `.github/workflows/android-build.yml` (from the embedded template in `internal/workflow/templates/`).

3. **Build** (`cmd/builder/android.go`, `internal/build/coordinator.go`) — `Coordinator.Build()` drives the full pipeline: trigger workflow dispatch → poll for run start → poll for artifact → download artifact ZIP → extract APK to `dist/`. Progress is rendered via `internal/build/progress.go`.

4. **Dev session** (`cmd/builder/flutter.go`, `internal/dev/`) — Installs APK via `adb`, detects package name by scanning `adb shell pm list packages`, then starts `flutter attach`. `Watcher` (`watcher.go`) watches `lib/` for `.dart` changes and sends `r` to `flutter attach`'s stdin to trigger hot reload.

### Key files

| Path | Role |
|------|------|
| `cmd/builder/root.go` | Cobra root; `getGitHubClient()` and `loadConfig()` helpers used by all subcommands |
| `internal/config/types.go` | `Config` struct — shape of `builder.json` |
| `internal/build/coordinator.go` | End-to-end build orchestration |
| `internal/github/client.go` | GitHub REST API client (auth header, redirect-following download) |
| `internal/github/workflow.go` | Workflow dispatch, polling for run start and artifact availability |
| `internal/workflow/templates/android-build.yml` | GHA workflow template written by `builder init` |

### Config (`builder.json`)

Lives in the project root. Loaded by `config.Manager.Load()`. Required fields: `project`, `github.owner`, `github.repo`. The `github.branch` defaults to `master` if omitted (set in `coordinator.go`, not in the config itself).

### Auth storage precedence

OS keyring → `~/.config/android-builder/token` (plain file, mode 0600).
