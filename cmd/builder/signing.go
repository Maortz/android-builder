package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/Maortz/android-builder/internal/config"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var signingCmd = &cobra.Command{Use: "signing", Short: "Code signing commands"}

var signingSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Upload Android keystore to GitHub Secrets for signed builds",
	RunE:  runSigningSetup,
}

func init() {
	signingCmd.AddCommand(signingSetupCmd)
	signingSetupCmd.Flags().StringP("keystore", "k", "", "Path to .jks or .keystore file")
	signingSetupCmd.Flags().String("key-alias", "", "Key alias in the keystore")
	signingSetupCmd.Flags().String("store-password", "", "Keystore password")
	signingSetupCmd.Flags().String("key-password", "", "Key password (defaults to store password if omitted)")
}

func runSigningSetup(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	keystorePath, _ := cmd.Flags().GetString("keystore")
	if keystorePath == "" {
		p := promptui.Prompt{Label: "Keystore path (.jks/.keystore)"}
		keystorePath, err = p.Run()
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(keystorePath); err != nil {
		return fmt.Errorf("keystore not found: %s", keystorePath)
	}

	keyAlias, _ := cmd.Flags().GetString("key-alias")
	if keyAlias == "" {
		p := promptui.Prompt{Label: "Key alias"}
		keyAlias, err = p.Run()
		if err != nil {
			return err
		}
	}

	storePassword, _ := cmd.Flags().GetString("store-password")
	if storePassword == "" {
		p := promptui.Prompt{Label: "Keystore password", Mask: '*'}
		storePassword, err = p.Run()
		if err != nil {
			return err
		}
	}

	keyPassword, _ := cmd.Flags().GetString("key-password")
	if keyPassword == "" {
		keyPassword = storePassword
	}

	keystoreData, err := os.ReadFile(keystorePath)
	if err != nil {
		return fmt.Errorf("read keystore: %w", err)
	}
	keystoreB64 := base64.StdEncoding.EncodeToString(keystoreData)

	gh, err := getGitHubClient()
	if err != nil {
		return err
	}

	pubKey, err := gh.GetActionsPublicKey(cmd.Context(), cfg.GitHub.Owner, cfg.GitHub.Repo)
	if err != nil {
		return fmt.Errorf("get repo public key: %w", err)
	}

	secrets := map[string]string{
		"ANDROID_KEYSTORE":       keystoreB64,
		"ANDROID_KEY_ALIAS":      keyAlias,
		"ANDROID_STORE_PASSWORD": storePassword,
		"ANDROID_KEY_PASSWORD":   keyPassword,
	}
	for name, value := range secrets {
		if err := gh.CreateOrUpdateSecret(cmd.Context(), cfg.GitHub.Owner, cfg.GitHub.Repo, name, value, pubKey); err != nil {
			return fmt.Errorf("upload secret %s: %w", name, err)
		}
		fmt.Printf("Uploaded secret: %s\n", name)
	}

	cfg.Android.Signing = true
	if err := config.NewManager().Save(cfg); err != nil {
		return fmt.Errorf("update builder.json: %w", err)
	}
	fmt.Println("Updated: builder.json (android.signing = true)")

	if !workflowReferencesSigningSecrets() {
		fmt.Println("\nWarning: .github/workflows/android-build.yml doesn't reference ANDROID_KEYSTORE yet.")
		fmt.Println("Re-run 'builder init' to update the workflow, or add a signing step manually.")
	}

	fmt.Println("\nSigning configured. Use --release on your next build to produce a signed APK.")
	return nil
}

func workflowReferencesSigningSecrets() bool {
	data, err := os.ReadFile(".github/workflows/android-build.yml")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "ANDROID_KEYSTORE")
}
