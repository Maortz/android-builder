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
