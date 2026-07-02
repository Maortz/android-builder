package main

import (
	"fmt"

	"github.com/Maortz/android-builder/internal/auth"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Authentication commands"}

var authGithubCmd = &cobra.Command{
	Use:   "github",
	Short: "Authenticate with GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := auth.DeviceLogin(cmd.Context())
		if err != nil {
			return err
		}
		if err := auth.SetToken(token); err != nil {
			return fmt.Errorf("save token: %w", err)
		}
		fmt.Println("Authenticated.")
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		auth.DeleteToken()
		fmt.Println("Logged out successfully")
		return nil
	},
}

func init() {
	authCmd.AddCommand(authGithubCmd)
	authCmd.AddCommand(authLogoutCmd)
}
