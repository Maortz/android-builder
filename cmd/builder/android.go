package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Maortz/android-builder/internal/build"
	"github.com/Maortz/android-builder/internal/config"
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
	return triggerBuild(cmd.Context(), cfg, output, timeout, release)
}

func triggerBuild(ctx context.Context, cfg *config.Config, outputDir string, timeout time.Duration, release bool) error {
	gh, err := getGitHubClient()
	if err != nil {
		return err
	}
	coord := build.NewCoordinator(cfg, gh)
	result, err := coord.Build(ctx, build.BuildOptions{
		OutputDir: outputDir,
		Timeout:   timeout,
		Release:   release,
	})
	if err != nil {
		return err
	}
	fmt.Printf("APK: %s\n", result.APKPath)
	fmt.Printf("Workflow: %s\n", result.WorkflowURL)
	return nil
}
