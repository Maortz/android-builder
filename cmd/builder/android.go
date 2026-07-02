package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Maortz/android-builder/internal/build"
	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/dev"
	"github.com/spf13/cobra"
)

var androidCmd = &cobra.Command{Use: "android", Short: "Android build commands"}

var androidBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Trigger a remote Android build on GitHub Actions",
	RunE:  runAndroidBuild,
}

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
	devMode, _ := cmd.Flags().GetBool("dev")
	deviceID, _ := cmd.Flags().GetString("device")
	skipInstall, _ := cmd.Flags().GetBool("skip-install")
	showLogs, _ := cmd.Flags().GetBool("logs")

	if devMode && release {
		return fmt.Errorf("--dev only works with debug builds; remove --release")
	}

	apkPath, err := triggerBuild(cmd.Context(), cfg, output, timeout, release)
	if err != nil {
		return err
	}

	if devMode {
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
