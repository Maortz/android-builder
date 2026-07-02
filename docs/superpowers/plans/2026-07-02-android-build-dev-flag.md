# Android Build --dev Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--dev` (plus `--device`, `--skip-install`, `--logs`) to `builder android build` so a successful debug build immediately installs the APK and starts a Flutter dev session in one command.

**Architecture:** After `triggerBuild` succeeds and `--dev` is set, `runAndroidBuild` calls a new `runDevSession(ctx, cfg, apkPath, deviceID, skipInstall, showLogs)` helper in `cmd/builder/android.go` that builds a `dev.NewFlutterHandler` + `dev.NewSession` exactly like `cmd/builder/flutter.go` does, reusing all existing dev-session code — no new package.

**Tech Stack:** Go, Cobra, `internal/dev`, `internal/build`.

## Global Constraints

- `--dev` only works with debug builds — error out if `--release` is also set.
- `--dev --skip-install` skips ADB install and goes straight to `flutter attach`.
- `--dev --device <id>` targets a specific ADB device.
- `--dev --logs` streams logcat during the dev session.
- Ctrl-C during the dev phase must cancel cleanly (rely on `cmd.Context()` cancellation already wired by Cobra's default signal handling via `ExecuteContext`/process signal — see Task 2 note on why this differs from `dev react-native`'s manual signal handling).
- Without `--dev`, `builder android build` behaves exactly as before (no regression).

---

### Task 1: Add `--dev`/`--device`/`--skip-install`/`--logs` flags and dev-session handoff

**Files:**
- Modify: `cmd/builder/android.go`

**Interfaces:**
- Consumes: `build.Coordinator.Build(ctx, build.BuildOptions) (*build.Result, error)` (existing, `internal/build/coordinator.go` — confirm `Result` has an `APKPath string` field before writing this task's code, since `runDevSession` needs it); `dev.NewFlutterHandler(noAttach, noWatch, showLogs bool, watchCfg *config.WatchConfig) *dev.FlutterHandler` (existing after issue #2); `dev.NewSession(deviceID, apkPath string, handler *dev.FlutterHandler) *dev.Session`, `(*dev.Session).SetSkipInstall(skip bool, packageName string)`, `(*dev.Session).Start(ctx) error` (existing, `internal/dev/session.go`).
- Produces: `func runDevSession(ctx context.Context, cfg *config.Config, apkPath, deviceID string, skipInstall, showLogs bool) error` in package `main`.

- [ ] **Step 1: Confirm `build.Result` shape**

Run: `grep -n "type Result" -A 6 /workspace/internal/build/coordinator.go`
Expected: a struct with an `APKPath string` field (already used as `result.APKPath` in the existing `triggerBuild`). If the field name differs, use the actual name in Step 3 below instead of `APKPath`.

- [ ] **Step 2: Add flags in `init()`**

Replace the `init()` function in `cmd/builder/android.go`:

```go
func init() {
	androidBuildCmd.Flags().StringP("output", "o", "dist", "Output directory for APK")
	androidBuildCmd.Flags().Duration("timeout", 30*time.Minute, "Build timeout")
	androidBuildCmd.Flags().Bool("release", false, "Build release APK instead of debug")
	androidBuildCmd.Flags().Bool("dev", false, "After build completes, install APK and start dev session")
	androidBuildCmd.Flags().StringP("device", "d", "", "ADB device ID (used with --dev)")
	androidBuildCmd.Flags().Bool("skip-install", false, "Skip APK install when using --dev")
	androidBuildCmd.Flags().Bool("logs", false, "Stream logcat output (used with --dev)")
	androidCmd.AddCommand(androidBuildCmd)
}
```

- [ ] **Step 3: Update `runAndroidBuild` and `triggerBuild`, add `runDevSession`**

Replace the rest of `cmd/builder/android.go` below `init()`:

```go
func runAndroidBuild(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	release, _ := cmd.Flags().GetBool("release")
	dev, _ := cmd.Flags().GetBool("dev")
	deviceID, _ := cmd.Flags().GetString("device")
	skipInstall, _ := cmd.Flags().GetBool("skip-install")
	showLogs, _ := cmd.Flags().GetBool("logs")

	if dev && release {
		return fmt.Errorf("--dev only works with debug builds; remove --release")
	}

	apkPath, err := triggerBuild(cmd.Context(), cfg, output, timeout, release)
	if err != nil {
		return err
	}

	if dev {
		fmt.Println("\n--- Starting dev session ---")
		return runDevSession(cmd.Context(), cfg, apkPath, deviceID, skipInstall, showLogs)
	}
	return nil
}

func triggerBuild(ctx context.Context, cfg *config.Config, outputDir string, timeout time.Duration, release bool) (string, error) {
	gh, err := getGitHubClient()
	if err != nil {
		return "", err
	}
	coord := build.NewCoordinator(cfg, gh)
	result, err := coord.Build(ctx, build.BuildOptions{
		OutputDir: outputDir,
		Timeout:   timeout,
		Release:   release,
	})
	if err != nil {
		return "", err
	}
	fmt.Printf("APK: %s\n", result.APKPath)
	fmt.Printf("Workflow: %s\n", result.WorkflowURL)
	return result.APKPath, nil
}

func runDevSession(ctx context.Context, cfg *config.Config, apkPath, deviceID string, skipInstall, showLogs bool) error {
	packageName := cfg.Android.PackageName

	if skipInstall && packageName == "" {
		return fmt.Errorf("--package (android.packageName in builder.json) is required with --skip-install")
	}

	watchCfg := &config.WatchConfig{
		Dirs:     []string{"lib"},
		Patterns: []string{".dart"},
		Ignore:   []string{".g.dart", ".freezed.dart"},
		Debounce: 100,
	}
	if len(cfg.Flutter.Watch.Dirs) > 0 {
		watchCfg = &cfg.Flutter.Watch
	}

	handler := dev.NewFlutterHandler(false, false, showLogs, watchCfg)
	session := dev.NewSession(deviceID, apkPath, handler)
	session.SetSkipInstall(skipInstall, packageName)
	return session.Start(ctx)
}
```

Update the import block at the top of the file to add `"github.com/Maortz/android-builder/internal/dev"`:

```go
import (
	"context"
	"fmt"
	"time"

	"github.com/Maortz/android-builder/internal/build"
	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/dev"
	"github.com/spf13/cobra"
)
```

Note: the local variable named `dev` in `runAndroidBuild` (`dev, _ := cmd.Flags().GetBool("dev")`) shadows the `dev` package import within that function's scope. Since `runAndroidBuild` doesn't call anything from the `dev` package directly (only `runDevSession` does, in its own scope), this is safe — but rename the local variable to `devMode` to avoid confusion for future readers:

```go
	devMode, _ := cmd.Flags().GetBool("dev")
	...
	if devMode && release {
		return fmt.Errorf("--dev only works with debug builds; remove --release")
	}
	...
	if devMode {
		fmt.Println("\n--- Starting dev session ---")
		return runDevSession(cmd.Context(), cfg, apkPath, deviceID, skipInstall, showLogs)
	}
```

- [ ] **Step 4: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass. (`triggerBuild`'s signature changed from `error` to `(string, error)` — confirm no other file calls it; `grep -rn "triggerBuild(" cmd/` should show only the one call site inside `runAndroidBuild` after this change.)

- [ ] **Step 5: Smoke test flag registration and the release/dev conflict**

Run: `./builder android build --help`
Expected: help lists `--dev`, `--device`/`-d`, `--skip-install`, `--logs` alongside the existing `--output`, `--timeout`, `--release`.

Run (in a directory without `builder.json`, to confirm the conflict check fires before config validation would otherwise fail first — actually config validation runs first in `runAndroidBuild`, so instead test in a directory *with* a valid `builder.json`): with a valid `builder.json` present, run `./builder android build --dev --release`
Expected: prints `Error: --dev only works with debug builds; remove --release` and exits non-zero, without attempting a GitHub API call.

- [ ] **Step 6: Commit**

```bash
git add cmd/builder/android.go
git commit -m "feat: add --dev flag to builder android build"
```
