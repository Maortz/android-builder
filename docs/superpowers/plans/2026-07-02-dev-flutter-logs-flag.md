# Dev Flutter --logs Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--logs` flag to `builder dev flutter` that streams adb logcat (filtered to the app's PID) to stderr alongside the `flutter attach` session.

**Architecture:** `FlutterHandler.Attach(ctx, deviceID, packageName)` already receives `packageName` (currently unused, bound to `_`). Add a `showLogs bool` field to `FlutterHandler`; when true and `packageName != ""`, `Attach` resolves the PID via `adb.PIDof` and runs `adb.Logcat` in a goroutine tied to `ctx`, writing to `os.Stderr`. If PID resolution fails, print a warning and continue without logcat (never fail the session).

**Tech Stack:** Go, Cobra, `internal/adb` (added in issue #6: `adb.PIDof(ctx, deviceID, packageName) (int, error)`, `adb.Logcat(ctx, deviceID, pid int, clear bool, w io.Writer) error`).

## Global Constraints

- No `--logs` flag → behavior identical to today (no regression).
- Logcat output goes to stderr, not stdout, so it doesn't interleave with `flutter attach`'s stdout relay.
- If package name can't be determined, print a warning and continue (do not return an error from `Attach`).
- Logcat must stop when the session ends (ctx cancellation via `exec.CommandContext` already handles this — no separate teardown needed).

---

### Task 1: Add `showLogs` to `FlutterHandler` and stream logcat in `Attach`

**Files:**
- Modify: `internal/dev/flutter.go`
- Test: `internal/dev/flutter_test.go` (new file)

**Interfaces:**
- Consumes: `adb.PIDof(ctx context.Context, deviceID, packageName string) (int, error)` and `adb.Logcat(ctx context.Context, deviceID string, pid int, clear bool, w io.Writer) error` (both existing in `internal/adb/adb.go`, added in issue #6).
- Produces: `func NewFlutterHandler(noAttach, noWatch, showLogs bool, watchCfg *config.WatchConfig) *FlutterHandler` — **signature changes** (adds `showLogs bool` as the 3rd parameter, before `watchCfg`). All callers must be updated in this task.

- [ ] **Step 1: Write failing test for the new constructor signature**

Create `internal/dev/flutter_test.go`:

```go
package dev

import (
	"testing"

	"github.com/Maortz/android-builder/internal/config"
)

func TestNewFlutterHandlerSetsShowLogs(t *testing.T) {
	watchCfg := &config.WatchConfig{Dirs: []string{"lib"}}

	h := NewFlutterHandler(false, false, true, watchCfg)

	if !h.showLogs {
		t.Fatal("expected showLogs to be true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/dev/... -run TestNewFlutterHandlerSetsShowLogs -v`
Expected: FAIL — `NewFlutterHandler` has wrong arity, or `showLogs` field doesn't exist.

- [ ] **Step 3: Update `internal/dev/flutter.go`**

Replace the full file contents:

```go
package dev

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/Maortz/android-builder/internal/config"
)

type FlutterHandler struct {
	noAttach bool
	noWatch  bool
	showLogs bool
	watchCfg *config.WatchConfig
	cmd      *exec.Cmd
	stdin    io.WriteCloser
}

func NewFlutterHandler(noAttach, noWatch, showLogs bool, watchCfg *config.WatchConfig) *FlutterHandler {
	return &FlutterHandler{noAttach: noAttach, noWatch: noWatch, showLogs: showLogs, watchCfg: watchCfg}
}

func (h *FlutterHandler) Attach(ctx context.Context, deviceID, packageName string) error {
	args := []string{"attach", "--device-id", deviceID}

	if h.noAttach {
		fmt.Printf("\nRun manually:\n  flutter %s\n", strings.Join(args, " "))
		return nil
	}

	if h.showLogs {
		h.startLogcat(ctx, deviceID, packageName)
	}

	fmt.Println("\nStarting flutter attach...")
	h.cmd = exec.CommandContext(ctx, "flutter", args...)

	stdin, err := h.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	h.stdin = stdin

	stdout, err := h.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	h.cmd.Stderr = h.cmd.Stdout

	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("flutter attach: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	if !h.noWatch {
		w := NewWatcher(h.watchCfg, func() {
			if h.stdin != nil {
				_, _ = fmt.Fprintln(h.stdin, "r")
				fmt.Printf("[%s] Hot reload\n", time.Now().Format("15:04:05"))
			}
		})
		go w.Run(ctx)
	}

	return h.cmd.Wait()
}

func (h *FlutterHandler) startLogcat(ctx context.Context, deviceID, packageName string) {
	if packageName == "" {
		fmt.Println("Warning: package name unknown, skipping --logs (pass --package to enable)")
		return
	}
	go func() {
		pid, err := adb.PIDof(ctx, deviceID, packageName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve PID for %s, skipping logcat: %v\n", packageName, err)
			return
		}
		if err := adb.Logcat(ctx, deviceID, pid, false, os.Stderr); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Warning: logcat stream ended: %v\n", err)
		}
	}()
}

func (h *FlutterHandler) Stop() {
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/dev/... -run TestNewFlutterHandlerSetsShowLogs -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dev/flutter.go internal/dev/flutter_test.go
git commit -m "feat: stream logcat in FlutterHandler.Attach when showLogs is set"
```

---

### Task 2: Wire `--logs` flag through `cmd/builder/flutter.go`

**Files:**
- Modify: `cmd/builder/flutter.go`

**Interfaces:**
- Consumes: `dev.NewFlutterHandler(noAttach, noWatch, showLogs bool, watchCfg *config.WatchConfig) *dev.FlutterHandler` (Task 1).

- [ ] **Step 1: Add the `--logs` flag and pass it through**

In `cmd/builder/flutter.go`, update `init()` to register the flag:

```go
func init() {
	devFlutterCmd.Flags().StringP("device", "d", "", "ADB device ID (default: first available)")
	devFlutterCmd.Flags().String("apk", "", "Path to APK (default: auto-detect from dist/)")
	devFlutterCmd.Flags().String("package", "", "App package name (e.g. com.example.app)")
	devFlutterCmd.Flags().Bool("skip-install", false, "Skip APK install (requires --package)")
	devFlutterCmd.Flags().Bool("no-attach", false, "Print flutter attach command instead of running")
	devFlutterCmd.Flags().Bool("no-watch", false, "Disable file-change hot reload")
	devFlutterCmd.Flags().Bool("logs", false, "Stream logcat output alongside flutter attach")
	devCmd.AddCommand(devFlutterCmd)
}
```

Then in `runDevFlutter`, read the flag and pass it to the constructor:

```go
func runDevFlutter(cmd *cobra.Command, args []string) error {
	deviceID, _ := cmd.Flags().GetString("device")
	apkPath, _ := cmd.Flags().GetString("apk")
	packageName, _ := cmd.Flags().GetString("package")
	skipInstall, _ := cmd.Flags().GetBool("skip-install")
	noAttach, _ := cmd.Flags().GetBool("no-attach")
	noWatch, _ := cmd.Flags().GetBool("no-watch")
	showLogs, _ := cmd.Flags().GetBool("logs")

	watchCfg := &config.WatchConfig{
		Dirs:     []string{"lib"},
		Patterns: []string{".dart"},
		Ignore:   []string{".g.dart", ".freezed.dart"},
		Debounce: 100,
	}

	if cfg, err := loadConfig(); err == nil {
		if len(cfg.Flutter.Watch.Dirs) > 0 {
			watchCfg = &cfg.Flutter.Watch
		}
		if packageName == "" {
			packageName = cfg.Android.PackageName
		}
	}

	if skipInstall && packageName == "" {
		return fmt.Errorf("--package is required with --skip-install")
	}

	if !skipInstall {
		if apkPath == "" {
			var err error
			apkPath, err = dev.FindAPK("dist")
			if err != nil {
				return err
			}
		}
		if _, err := os.Stat(apkPath); os.IsNotExist(err) {
			return fmt.Errorf("APK not found: %s", apkPath)
		}
		fmt.Printf("APK: %s\n", apkPath)
	}

	handler := dev.NewFlutterHandler(noAttach, noWatch, showLogs, watchCfg)
	session := dev.NewSession(deviceID, apkPath, handler)
	session.SetSkipInstall(skipInstall, packageName)
	return session.Start(cmd.Context())
}
```

- [ ] **Step 2: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds; all tests pass (no other `NewFlutterHandler` call sites exist yet outside this file and the test from Task 1).

- [ ] **Step 3: Smoke test the flag is registered**

Run: `./builder dev flutter --help`
Expected: help output lists `--logs` with description "Stream logcat output alongside flutter attach".

- [ ] **Step 4: Commit**

```bash
git add cmd/builder/flutter.go
git commit -m "feat: add --logs flag to builder dev flutter"
```
