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
