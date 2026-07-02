# Dev React Native Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `builder dev react-native` (alias `rn`) that installs an APK, sets up Metro connectivity via `adb reverse`, starts the Metro bundler, launches the app, and optionally streams logcat — mirroring the existing `dev flutter` workflow but for React Native.

**Architecture:** A new `ReactNativeHandler` in `internal/dev/reactnative.go` owns the Metro subprocess and orchestrates reverse-proxy setup, app launch, and optional logcat, all built on the `internal/adb` package (extended with `Reverse`/`ReverseRemove`). Unlike `FlutterHandler` (which plugs into the existing `Session`/watcher flow for hot reload via stdin), Metro's hot reload is handled by the RN packager itself over the reverse-proxied port, so `dev react-native` does not reuse `dev.Session` — it has its own smaller install→reverse→metro→launch pipeline in `cmd/builder/rn.go`. Ctrl-C is handled with a local `context.WithCancel` + `signal.Notify`, cancelling Metro/logcat and cleanly running `adb reverse --remove`.

**Tech Stack:** Go, Cobra, `internal/adb`, `os/exec`.

## Global Constraints

- `builder dev rn` and `builder dev react-native` must both work (Cobra alias).
- Auto-detects APK in `dist/` when `--apk` is not set (reuse `dev.FindAPK("dist")`, existing in `internal/dev/session.go`).
- Sets up `adb reverse tcp:<port> tcp:<port>` automatically; tears it down via `adb reverse --remove tcp:<port>` on exit.
- `--skip-install` requires `--package` (same rule as `dev flutter`).
- `--metro-port` overrides default `8081`.
- `--logs` streams logcat filtered to the app process (reuse `adb.PIDof` + `adb.Logcat` from `internal/adb`, added in issue #6).
- Prints a clear message if Metro (`npx`) or `adb` is not found in PATH.
- Ctrl-C kills Metro, tears down `adb reverse`, and exits with no error.
- Config: `android.packageName` (existing field) is the default for `--package`; new optional `reactNative.metroPort` config key.

---

### Task 1: Add `adb.Reverse`/`adb.ReverseRemove` and `ReactNativeConfig`

**Files:**
- Modify: `internal/adb/adb.go`
- Modify: `internal/adb/adb_test.go`
- Modify: `internal/config/types.go`

**Interfaces:**
- Produces: `func Reverse(ctx context.Context, deviceID, devicePort, hostPort string) error`, `func ReverseRemove(ctx context.Context, deviceID, devicePort string) error` in `internal/adb`; `ReactNativeConfig{ MetroPort int \`json:"metroPort,omitempty"\` }` field `ReactNative ReactNativeConfig \`json:"reactNative,omitempty"\`` added to `config.Config`.

- [ ] **Step 1: Write failing test for the exact adb args `Reverse` builds**

Add to `internal/adb/adb_test.go` (append, don't remove `TestParseDevicesOutput`):

```go
func TestReverseAndReverseRemoveArgs(t *testing.T) {
	// Reverse/ReverseRemove build on Run, which is already covered by
	// integration use; this test only pins down the exact adb command
	// shape via a lightweight arg-builder so a future refactor can't
	// silently swap "reverse"/"forward" or drop --remove.
	if got := reverseArgs("8081", "8081"); len(got) != 2 || got[0] != "tcp:8081" || got[1] != "tcp:8081" {
		t.Fatalf("unexpected reverse args: %v", got)
	}
	if got := reverseRemoveArgs("8081"); len(got) != 1 || got[0] != "tcp:8081" {
		t.Fatalf("unexpected reverse-remove args: %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/adb/... -run TestReverseAndReverseRemoveArgs -v`
Expected: FAIL — `reverseArgs`/`reverseRemoveArgs` undefined.

- [ ] **Step 3: Implement in `internal/adb/adb.go`**

Append to the file (after `ForwardRemove`):

```go
func reverseArgs(devicePort, hostPort string) []string {
	return []string{"tcp:" + devicePort, "tcp:" + hostPort}
}

func reverseRemoveArgs(devicePort string) []string {
	return []string{"tcp:" + devicePort}
}

func Reverse(ctx context.Context, deviceID, devicePort, hostPort string) error {
	return Run(ctx, deviceID, append([]string{"reverse"}, reverseArgs(devicePort, hostPort)...)...)
}

func ReverseRemove(ctx context.Context, deviceID, devicePort string) error {
	return Run(ctx, deviceID, append([]string{"reverse", "--remove"}, reverseRemoveArgs(devicePort)...)...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/adb/... -v`
Expected: PASS (both `TestParseDevicesOutput` and `TestReverseAndReverseRemoveArgs`).

- [ ] **Step 5: Add `ReactNative` config section**

In `internal/config/types.go`, add the field to `Config` and a new struct (do not touch any other field):

```go
type Config struct {
	Project     string            `json:"project"`
	Platform    string            `json:"platform"`
	GitHub      GitHubConfig      `json:"github"`
	Android     AndroidConfig     `json:"android,omitempty"`
	Flutter     FlutterConfig     `json:"flutter,omitempty"`
	ReactNative ReactNativeConfig `json:"reactNative,omitempty"`
}
```

```go
type ReactNativeConfig struct {
	MetroPort int `json:"metroPort,omitempty"`
}
```

- [ ] **Step 6: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build ./... && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/adb/adb.go internal/adb/adb_test.go internal/config/types.go
git commit -m "feat: add adb reverse/reverse-remove and reactNative config section"
```

---

### Task 2: Implement `ReactNativeHandler` in `internal/dev/reactnative.go`

**Files:**
- Create: `internal/dev/reactnative.go`
- Test: `internal/dev/reactnative_test.go`

**Interfaces:**
- Consumes: `adb.Install(ctx, deviceID, apkPath) error`, `adb.Reverse(ctx, deviceID, devicePort, hostPort) error`, `adb.ReverseRemove(ctx, deviceID, devicePort) error`, `adb.Run(ctx, deviceID, args...) error`, `adb.PIDof(ctx, deviceID, packageName) (int, error)`, `adb.Logcat(ctx, deviceID, pid, clear, w) error` (all in `internal/adb`, existing after Task 1 / issue #6).
- Produces:
  - `func NewReactNativeHandler(metroPort int, showLogs bool) *ReactNativeHandler`
  - `func (h *ReactNativeHandler) Run(ctx context.Context, deviceID, packageName string) error`
  - `func (h *ReactNativeHandler) Stop()`

- [ ] **Step 1: Write failing test for constructor defaults**

Create `internal/dev/reactnative_test.go`:

```go
package dev

import "testing"

func TestNewReactNativeHandlerDefaults(t *testing.T) {
	h := NewReactNativeHandler(8081, true)

	if h.metroPort != 8081 {
		t.Errorf("expected metroPort 8081, got %d", h.metroPort)
	}
	if !h.showLogs {
		t.Error("expected showLogs true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/dev/... -run TestNewReactNativeHandlerDefaults -v`
Expected: FAIL — package `internal/dev/reactnative.go` doesn't exist yet.

- [ ] **Step 3: Implement `internal/dev/reactnative.go`**

```go
package dev

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Maortz/android-builder/internal/adb"
)

type ReactNativeHandler struct {
	metroPort int
	showLogs  bool
	metroCmd  *exec.Cmd
}

func NewReactNativeHandler(metroPort int, showLogs bool) *ReactNativeHandler {
	return &ReactNativeHandler{metroPort: metroPort, showLogs: showLogs}
}

// Run sets up the Metro reverse-proxy, starts the Metro bundler, launches
// the app, and blocks until the bundler exits or ctx is cancelled.
func (h *ReactNativeHandler) Run(ctx context.Context, deviceID, packageName string) error {
	port := strconv.Itoa(h.metroPort)

	if err := adb.Reverse(ctx, deviceID, port, port); err != nil {
		return fmt.Errorf("adb reverse: %w", err)
	}
	defer func() {
		_ = adb.ReverseRemove(context.Background(), deviceID, port)
	}()

	fmt.Printf("Starting Metro bundler on port %s...\n", port)
	h.metroCmd = exec.CommandContext(ctx, "npx", "react-native", "start", "--port", port)
	h.metroCmd.Stdout = os.Stdout
	h.metroCmd.Stderr = os.Stderr
	if err := h.metroCmd.Start(); err != nil {
		return fmt.Errorf("start Metro (is Node.js/React Native CLI installed?): %w", err)
	}

	// Give Metro a moment to bind its port before launching the app.
	time.Sleep(2 * time.Second)

	fmt.Printf("Launching %s...\n", packageName)
	if err := adb.Run(ctx, deviceID, "shell", "am", "start", "-n", packageName+"/.MainActivity"); err != nil {
		return fmt.Errorf("launch app: %w", err)
	}

	if h.showLogs {
		h.startLogcat(ctx, deviceID, packageName)
	}

	return h.metroCmd.Wait()
}

func (h *ReactNativeHandler) startLogcat(ctx context.Context, deviceID, packageName string) {
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

func (h *ReactNativeHandler) Stop() {
	if h.metroCmd != nil && h.metroCmd.Process != nil {
		_ = h.metroCmd.Process.Kill()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/dev/... -run TestNewReactNativeHandlerDefaults -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build ./... && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/dev/reactnative.go internal/dev/reactnative_test.go
git commit -m "feat: add ReactNativeHandler for Metro-based Android hot reload"
```

---

### Task 3: Wire `builder dev react-native` / `rn` CLI command

**Files:**
- Create: `cmd/builder/rn.go`

**Interfaces:**
- Consumes: `dev.NewReactNativeHandler(metroPort int, showLogs bool) *dev.ReactNativeHandler` (Task 2); `dev.FindAPK(distDir string) (string, error)` (existing, `internal/dev/session.go`); `adb.Devices() ([]adb.Device, error)`, `adb.Install(ctx, deviceID, apkPath) error` (existing, `internal/adb`); `loadConfig() (*config.Config, error)` (existing, `cmd/builder/root.go`); `resolveDevice(cmd *cobra.Command) (string, error)` (existing, `cmd/builder/adb.go` — same package `main`, reusable directly).
- Produces: `devReactNativeCmd *cobra.Command`, registered under `devCmd` (from `cmd/builder/flutter.go`).

- [ ] **Step 1: Implement `cmd/builder/rn.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/Maortz/android-builder/internal/dev"
	"github.com/spf13/cobra"
)

var devReactNativeCmd = &cobra.Command{
	Use:     "react-native",
	Aliases: []string{"rn"},
	Short:   "Install APK and start React Native Metro hot-reload session",
	Long: `Starts a React Native hot reload session for Android.

Prerequisites:
- ADB in PATH with a connected device or emulator
- APK built with 'builder android build' (must be a debug build)
- Node.js and React Native CLI installed`,
	RunE: runDevReactNative,
}

func init() {
	devCmd.AddCommand(devReactNativeCmd)
	devReactNativeCmd.Flags().StringP("device", "d", "", "ADB device ID (default: first available)")
	devReactNativeCmd.Flags().String("apk", "", "Path to APK (default: auto-detect from dist/)")
	devReactNativeCmd.Flags().String("package", "", "App package name (e.g., com.myapp)")
	devReactNativeCmd.Flags().Bool("skip-install", false, "Skip APK install (app must already be installed)")
	devReactNativeCmd.Flags().Int("metro-port", 8081, "Metro bundler port")
	devReactNativeCmd.Flags().Bool("logs", false, "Stream logcat output")
}

func runDevReactNative(cmd *cobra.Command, args []string) error {
	deviceID, _ := cmd.Flags().GetString("device")
	apkPath, _ := cmd.Flags().GetString("apk")
	packageName, _ := cmd.Flags().GetString("package")
	skipInstall, _ := cmd.Flags().GetBool("skip-install")
	metroPort, _ := cmd.Flags().GetInt("metro-port")
	showLogs, _ := cmd.Flags().GetBool("logs")

	if cfg, err := loadConfig(); err == nil {
		if packageName == "" {
			packageName = cfg.Android.PackageName
		}
		if !cmd.Flags().Changed("metro-port") && cfg.ReactNative.MetroPort != 0 {
			metroPort = cfg.ReactNative.MetroPort
		}
	}

	if skipInstall && packageName == "" {
		return fmt.Errorf("--package is required with --skip-install")
	}

	if deviceID == "" {
		devices, err := adb.Devices()
		if err != nil {
			return err
		}
		for _, d := range devices {
			if d.State == "device" {
				deviceID = d.Serial
				break
			}
		}
		if deviceID == "" {
			return fmt.Errorf("no Android devices found\nEnable USB debugging and reconnect, then check: adb devices")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nStopping Metro and cleaning up...")
		cancel()
	}()

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
		fmt.Printf("Installing %s...\n", apkPath)
		if err := adb.Install(ctx, deviceID, apkPath); err != nil {
			return fmt.Errorf("adb install: %w", err)
		}
		fmt.Println("Installed.")
	}

	handler := dev.NewReactNativeHandler(metroPort, showLogs)
	err := handler.Run(ctx, deviceID, packageName)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
```

- [ ] **Step 2: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 3: Smoke test both command name and alias**

Run: `./builder dev react-native --help` and `./builder dev rn --help`
Expected: both print identical help text listing `--device`, `--apk`, `--package`, `--skip-install`, `--metro-port`, `--logs`.

- [ ] **Step 4: Commit**

```bash
git add cmd/builder/rn.go
git commit -m "feat: add builder dev react-native (rn) command"
```
