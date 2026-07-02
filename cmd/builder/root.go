package main

import (
	"fmt"

	"github.com/Maortz/android-builder/internal/auth"
	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/github"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "builder",
	Short:        "Build Android apps remotely using GitHub Actions",
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(androidCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(adbCmd)
}

func getGitHubClient() (*github.Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, fmt.Errorf("not authenticated. Run: builder auth github")
	}
	return github.NewClient(token), nil
}

func loadConfig() (*config.Config, error) {
	mgr := config.NewManager()
	cfg, err := mgr.Load()
	if err != nil {
		if err == config.ErrConfigNotFound {
			return nil, fmt.Errorf("builder.json not found. Run: builder init")
		}
		return nil, err
	}
	return cfg, nil
}
