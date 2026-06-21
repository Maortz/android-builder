package main

import (
	"fmt"
	"syscall"

	"github.com/Maortz/android-builder/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Authentication commands"}

var authGithubCmd = &cobra.Command{
	Use:   "github",
	Short: "Save GitHub personal access token",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("GitHub token (needs repo + actions:read scope): ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return err
		}
		fmt.Println()
		if len(b) == 0 {
			return fmt.Errorf("token cannot be empty")
		}
		if err := auth.SetToken(string(b)); err != nil {
			return fmt.Errorf("save token: %w", err)
		}
		fmt.Println("Token saved.")
		return nil
	},
}

func init() { authCmd.AddCommand(authGithubCmd) }
