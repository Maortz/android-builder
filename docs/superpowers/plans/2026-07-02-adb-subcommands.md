# ADB Subcommands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `builder adb {devices,install,logcat,forward}` and consolidate ADB logic into a shared `internal/adb` package, reused by `internal/dev`.

**Architecture:** Extract `listDevices`, `adbRun`, and PID/aapt lookups currently inline in `internal/dev/session.go` into a new `internal/adb` package with typed functions (`Devices`, `Install`, `PIDof`, `Forward`, `Logcat`). `internal/dev/session.go` is refactored to call `adb.Devices()` / `adb.Run()` instead of its own private helpers (no duplicated logic). A new `cmd/builder/adb.go` wires four Cobra subcommands calling the new package.

**Tech Stack:** Go, Cobra, `os/exec` for shelling out to `adb`.

## Global Constraints

- No duplicated ADB logic between `internal/dev` and `internal/adb` — `internal/dev` must import and use `internal/adb`.
- All commands print a helpful error if ADB is not found in PATH (existing message pattern: `"adb not found: %w\nInstall Platform-Tools: https://developer.android.com/tools/releases/platform-tools"`).
- `--device/-d` persistent flag on `adb` command group; falls back to first connected device if omitted.
- `adb install` with no arg auto-detects APK from `dist/` via existing `dev.FindAPK("dist")`.
- `adb logcat --package` resolves PID first via `adb.PIDof`; falls back to `android.packageName` in `builder.json` when `--package` omitted.
- `adb forward <device-port> <host-port>` blocks until Ctrl-C, then runs `adb forward --remove`.

---

### Task 1: Create `internal/adb` package with `Devices`, `Run`, `Install`, `PIDof`

**Files:**
- Create: `internal/adb/adb.go`
- Test: `internal/adb/adb_test.go`

**Interfaces:**
- Produces:
  - `type Device struct { Serial string; State string }`
  - `func Devices() ([]Device, error)`
  - `func Run(ctx context.Context, deviceID string, args ...string) error`
  - `func Install(ctx context.Context, deviceID, apkPath string) error`
  - `func PIDof(ctx context.Context, deviceID, packageName string) (int, error)`
  - `func NotFoundError(err error) error` — wraps an exec error with the standard "adb not found" hint used across all adb commands.

- [ ] **Step 1: Write failing test for parsing `adb devices -l` output**

Create `internal/adb/adb_test.go`:

```go
package adb

import "testing"

func TestParseDevicesOutput(t *testing.T) {
	sample := "List of devices attached\n" +
		"emulator-5554  device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emulator64_x86_64 transport_id:1\n" +
		"R58M12ABCDE    unauthorized\n" +
		"\n"

	devices := parseDevicesOutput(sample)

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d: %+v", len(devices), devices)
	}
	if devices[0].Serial != "emulator-5554" || devices[0].State != "device" {
		t.Errorf("unexpected first device: %+v", devices[0])
	}
	if devices[1].Serial != "R58M12ABCDE" || devices[1].State != "unauthorized" {
		t.Errorf("unexpected second device: %+v", devices[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/adb/... -v`
Expected: FAIL — `parseDevicesOutput` undefined / package internal/adb doesn't exist yet.

- [ ] **Step 3: Implement `internal/adb/adb.go`**

```go
package adb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Device struct {
	Serial string
	State  string
}

func NotFoundError(err error) error {
	return fmt.Errorf("adb not found: %w\nInstall Platform-Tools: https://developer.android.com/tools/releases/platform-tools", err)
}

func parseDevicesOutput(out string) []Device {
	var devices []Device
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			devices = append(devices, Device{Serial: fields[0], State: fields[1]})
		}
	}
	return devices
}

func Devices() ([]Device, error) {
	out, err := exec.Command("adb", "devices", "-l").Output()
	if err != nil {
		return nil, NotFoundError(err)
	}
	return parseDevicesOutput(string(out)), nil
}

func Run(ctx context.Context, deviceID string, args ...string) error {
	fullArgs := append([]string{"-s", deviceID}, args...)
	out, err := exec.CommandContext(ctx, "adb", fullArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Install(ctx context.Context, deviceID, apkPath string) error {
	return Run(ctx, deviceID, "install", "-r", apkPath)
}

func PIDof(ctx context.Context, deviceID, packageName string) (int, error) {
	out, err := exec.CommandContext(ctx, "adb", "-s", deviceID, "shell", "pidof", packageName).Output()
	if err != nil {
		return 0, fmt.Errorf("pidof %s: %w", packageName, err)
	}
	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		return 0, fmt.Errorf("process %s not running on device %s", packageName, deviceID)
	}
	pid, err := strconv.Atoi(strings.Fields(pidStr)[0])
	if err != nil {
		return 0, fmt.Errorf("parse pid %q: %w", pidStr, err)
	}
	return pid, nil
}

func Forward(ctx context.Context, deviceID, devicePort, hostPort string) error {
	return Run(ctx, deviceID, "forward", "tcp:"+devicePort, "tcp:"+hostPort)
}

func ForwardRemove(ctx context.Context, deviceID, devicePort string) error {
	return Run(ctx, deviceID, "forward", "--remove", "tcp:"+devicePort)
}

func Logcat(ctx context.Context, deviceID string, pid int, clear bool, w io.Writer) error {
	if clear {
		if err := Run(ctx, deviceID, "logcat", "-c"); err != nil {
			return fmt.Errorf("clear logcat buffer: %w", err)
		}
	}
	args := []string{"-s", deviceID, "logcat"}
	if pid > 0 {
		args = append(args, "--pid="+strconv.Itoa(pid))
	}
	cmd := exec.CommandContext(ctx, "adb", args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

var _ = bufio.NewReader // keep bufio import if unused helpers are added later
```

Note: remove the `bufio` import and the trailing `var _ = bufio.NewReader` line — it isn't needed. Final imports are `bufio` removed entirely:

```go
import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/adb/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adb/adb.go internal/adb/adb_test.go
git commit -m "feat: add internal/adb package for shared ADB device control"
```

---

### Task 2: Refactor `internal/dev/session.go` to use `internal/adb` (remove duplication)

**Files:**
- Modify: `internal/dev/session.go`

**Interfaces:**
- Consumes: `adb.Devices() ([]adb.Device, error)`, `adb.Run(ctx, deviceID, args...) error` (Task 1).
- Produces: same `Session` public API as before (`NewSession`, `SetSkipInstall`, `Start`) — no signature changes, so `cmd/builder/flutter.go` and future `cmd/builder/rn.go` remain unaffected.

- [ ] **Step 1: Replace `listDevices` and `adbRun` usages with `internal/adb` calls**

In `internal/dev/session.go`, change the import block to add `"github.com/Maortz/android-builder/internal/adb"`, then replace the body of `selectDevice`, `Start`, and delete the now-unused private `listDevices`/`adbRun` funcs:

```go
package dev

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/manifoldco/promptui"
)

// Session manages adb install → app launch → flutter attach.
type Session struct {
	deviceID    string
	apkPath     string
	packageName string
	skipInstall bool
	handler     *FlutterHandler
}

func NewSession(deviceID, apkPath string, handler *FlutterHandler) *Session {
	return &Session{deviceID: deviceID, apkPath: apkPath, handler: handler}
}

func (s *Session) SetSkipInstall(skip bool, packageName string) {
	s.skipInstall = skip
	s.packageName = packageName
}

// FindAPK returns the newest .apk in distDir, prompting if multiple.
func FindAPK(distDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(distDir, "*.apk"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no APK in %s — run 'builder android build' first", distDir)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	prompt := promptui.Select{Label: "Select APK", Items: matches}
	_, selected, err := prompt.Run()
	return selected, err
}

func (s *Session) Start(ctx context.Context) error {
	deviceID, err := s.selectDevice()
	if err != nil {
		return err
	}
	s.deviceID = deviceID

	if !s.skipInstall {
		fmt.Printf("Installing %s...\n", s.apkPath)
		if err := adb.Install(ctx, deviceID, s.apkPath); err != nil {
			return fmt.Errorf("adb install: %w", err)
		}
		fmt.Println("Installed.")

		if s.packageName == "" {
			pkg, err := detectPackageName(s.apkPath)
			if err != nil {
				return fmt.Errorf("%w\nUse --package com.your.app or set android.packageName in builder.json", err)
			}
			s.packageName = pkg
		}

		fmt.Printf("Launching %s...\n", s.packageName)
		if err := adb.Run(ctx, deviceID, "shell", "monkey", "-p", s.packageName, "-c", "android.intent.category.LAUNCHER", "1"); err != nil {
			return fmt.Errorf("launch app: %w", err)
		}
	}

	return s.handler.Attach(ctx, deviceID, s.packageName)
}

func (s *Session) selectDevice() (string, error) {
	if s.deviceID != "" {
		return s.deviceID, nil
	}
	devices, err := adb.Devices()
	if err != nil {
		return "", err
	}
	var serials []string
	for _, d := range devices {
		if d.State == "device" {
			serials = append(serials, d.Serial)
		}
	}
	if len(serials) == 0 {
		return "", fmt.Errorf("no Android devices found\nEnable USB debugging and reconnect, then check: adb devices")
	}
	if len(serials) == 1 {
		fmt.Printf("Device: %s\n", serials[0])
		return serials[0], nil
	}
	prompt := promptui.Select{Label: "Select device", Items: serials}
	_, selected, err := prompt.Run()
	return selected, err
}

func detectPackageName(apkPath string) (string, error) {
	tools := []string{"aapt", "aapt2"}
	var out []byte
	var err error
	for _, tool := range tools {
		out, err = exec.Command(tool, "dump", "badging", apkPath).Output()
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("aapt/aapt2 not found — could not detect package name")
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "package: name=") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "name=") {
				return strings.Trim(strings.TrimPrefix(field, "name="), `'"`), nil
			}
		}
	}
	return "", fmt.Errorf("could not parse package name from aapt output")
}
```

- [ ] **Step 2: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build ./... && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all existing tests (including `watcher_test.go`) still pass.

- [ ] **Step 3: Commit**

```bash
git add internal/dev/session.go
git commit -m "refactor: use internal/adb in dev.Session, remove duplicated adb helpers"
```

---

### Task 3: Add `builder adb devices|install|logcat|forward` Cobra commands

**Files:**
- Create: `cmd/builder/adb.go`
- Modify: `cmd/builder/root.go:19-21` (add `rootCmd.AddCommand(adbCmd)`)

**Interfaces:**
- Consumes: `adb.Devices()`, `adb.Install(ctx, deviceID, apkPath)`, `adb.PIDof(ctx, deviceID, packageName)`, `adb.Logcat(ctx, deviceID, pid, clear, w)`, `adb.Forward(ctx, deviceID, devicePort, hostPort)`, `adb.ForwardRemove(ctx, deviceID, devicePort)` (Task 1); `dev.FindAPK(distDir)` (existing, `internal/dev/session.go`); `loadConfig()` (existing, `cmd/builder/root.go`).
- Produces: `adbCmd *cobra.Command` (package `main`), added to `rootCmd`.

- [ ] **Step 1: Implement `cmd/builder/adb.go`**

```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/Maortz/android-builder/internal/dev"
	"github.com/spf13/cobra"
)

var adbCmd = &cobra.Command{Use: "adb", Short: "ADB device commands"}

var adbDevicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List connected ADB devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		devices, err := adb.Devices()
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			fmt.Println("No devices connected.")
			return nil
		}
		for _, d := range devices {
			fmt.Printf("%s\t%s\n", d.Serial, d.State)
		}
		return nil
	},
}

var adbInstallCmd = &cobra.Command{
	Use:   "install [apk-path]",
	Short: "Install APK on device",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID, err := resolveDevice(cmd)
		if err != nil {
			return err
		}
		apkPath := ""
		if len(args) == 1 {
			apkPath = args[0]
		} else {
			apkPath, err = dev.FindAPK("dist")
			if err != nil {
				return err
			}
		}
		if err := adb.Install(cmd.Context(), deviceID, apkPath); err != nil {
			return fmt.Errorf("adb install: %w", err)
		}
		fmt.Println("installed")
		return nil
	},
}

var adbLogcatCmd = &cobra.Command{
	Use:   "logcat",
	Short: "Stream logcat output",
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID, err := resolveDevice(cmd)
		if err != nil {
			return err
		}
		packageName, _ := cmd.Flags().GetString("package")
		clear, _ := cmd.Flags().GetBool("clear")
		if packageName == "" {
			if cfg, err := loadConfig(); err == nil {
				packageName = cfg.Android.PackageName
			}
		}
		pid := 0
		if packageName != "" {
			pid, err = adb.PIDof(cmd.Context(), deviceID, packageName)
			if err != nil {
				return err
			}
		}
		return adb.Logcat(cmd.Context(), deviceID, pid, clear, os.Stdout)
	},
}

var adbForwardCmd = &cobra.Command{
	Use:   "forward <device-port> <host-port>",
	Short: "Forward a TCP port from device to host",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		deviceID, err := resolveDevice(cmd)
		if err != nil {
			return err
		}
		devicePort, hostPort := args[0], args[1]
		if err := adb.Forward(cmd.Context(), deviceID, devicePort, hostPort); err != nil {
			return fmt.Errorf("adb forward: %w", err)
		}
		fmt.Printf("Forwarding tcp:%s -> tcp:%s. Press Ctrl-C to stop.\n", devicePort, hostPort)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		fmt.Println("\nRemoving forward...")
		return adb.ForwardRemove(cmd.Context(), deviceID, devicePort)
	},
}

func resolveDevice(cmd *cobra.Command) (string, error) {
	deviceID, _ := cmd.Flags().GetString("device")
	if deviceID != "" {
		return deviceID, nil
	}
	if cfg, err := loadConfig(); err == nil && cfg.Android.DeviceID != "" {
		return cfg.Android.DeviceID, nil
	}
	devices, err := adb.Devices()
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.State == "device" {
			return d.Serial, nil
		}
	}
	return "", fmt.Errorf("no Android devices found\nEnable USB debugging and reconnect, then check: adb devices")
}

func init() {
	adbCmd.PersistentFlags().StringP("device", "d", "", "ADB device serial")
	adbCmd.AddCommand(adbDevicesCmd)
	adbCmd.AddCommand(adbInstallCmd)
	adbCmd.AddCommand(adbLogcatCmd)
	adbCmd.AddCommand(adbForwardCmd)
	adbLogcatCmd.Flags().String("package", "", "Filter logcat to this package name")
	adbLogcatCmd.Flags().Bool("clear", false, "Clear logcat buffer before streaming")
}
```

- [ ] **Step 2: Add `Android.DeviceID` config field**

Read `internal/config/types.go` first to find the `Android` struct, then add a `DeviceID` field with a `json:"deviceId,omitempty"` tag alongside the existing `PackageName` field (do not change any other field).

- [ ] **Step 3: Wire `adbCmd` into root**

In `cmd/builder/root.go`, change:

```go
func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(androidCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(devCmd)
}
```

to:

```go
func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(androidCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(adbCmd)
}
```

- [ ] **Step 4: Build and smoke-test**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && ./builder adb devices`
Expected: build succeeds; `adb devices` prints `No devices connected.` (or a device list) without crashing (works even without a real device attached, since `adb.Devices()` just shells out and parses).

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/builder/adb.go cmd/builder/root.go internal/config/types.go
git commit -m "feat: add builder adb devices/install/logcat/forward subcommands"
```
