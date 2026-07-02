package main

import (
	"fmt"
	"os"

	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/dev"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{Use: "dev", Short: "Development commands"}

var devFlutterCmd = &cobra.Command{
	Use:   "flutter",
	Short: "Install APK and start Flutter hot-reload session",
	RunE:  runDevFlutter,
}

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
