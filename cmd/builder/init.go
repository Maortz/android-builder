package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/workflow"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Android builds for this repository",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringP("project", "p", "", "Project name (default: current directory name)")
	initCmd.Flags().StringP("remote", "r", "origin", "Git remote name")
}

func detectGitHubRepo(remote string) (owner, repo string, err error) {
	out, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		return "", "", fmt.Errorf("no '%s' remote", remote)
	}
	u := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
	if path, ok := strings.CutPrefix(u, "https://github.com/"); ok {
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}
	if strings.HasPrefix(u, "git@") {
		if i := strings.Index(u, ":"); i > 0 {
			parts := strings.SplitN(u[i+1:], "/", 2)
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
	}
	return "", "", fmt.Errorf("could not parse GitHub URL: %s", u)
}

func getLocalFlutterVersion() string {
	out, err := exec.Command("flutter", "--version", "--machine").Output()
	if err != nil {
		return ""
	}
	s := string(out)
	key := `"frameworkVersion":"`
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	s = s[i+len(key):]
	if j := strings.Index(s, `"`); j >= 0 {
		return s[:j]
	}
	return ""
}

func runInit(cmd *cobra.Command, args []string) error {
	remote, _ := cmd.Flags().GetString("remote")
	owner, repoName, err := detectGitHubRepo(remote)
	if err != nil {
		return err
	}
	fmt.Printf("Repository: %s/%s\n\n", owner, repoName)

	projectName, _ := cmd.Flags().GetString("project")
	if projectName == "" {
		cwd, _ := os.Getwd()
		p := promptui.Prompt{Label: "Project name", Default: filepath.Base(cwd)}
		projectName, err = p.Run()
		if err != nil {
			return err
		}
	}

	localVer := getLocalFlutterVersion()
	fp := promptui.Prompt{Label: "Flutter version (empty = latest stable)", Default: localVer}
	flutterVersion, _ := fp.Run()

	// Write workflow file
	if err := os.MkdirAll(".github/workflows", 0755); err != nil {
		return err
	}
	tmpl, err := workflow.GetWorkflowTemplate()
	if err != nil {
		return err
	}
	workflowPath := ".github/workflows/android-build.yml"
	if err := os.WriteFile(workflowPath, tmpl, 0644); err != nil {
		return err
	}
	fmt.Printf("Created: %s\n", workflowPath)

	// Merge into existing builder.json if present (preserves ios section)
	mgr := config.NewManager()
	cfg, err := mgr.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	cfg.Project = projectName
	cfg.Platform = "android"
	cfg.GitHub = config.GitHubConfig{Owner: owner, Repo: repoName}
	cfg.Android = config.AndroidConfig{BuildType: "debug"}
	cfg.Flutter.Version = flutterVersion
	if err := mgr.Save(cfg); err != nil {
		return err
	}
	fmt.Println("Updated: builder.json")

	// Offer commit+push
	cp := promptui.Prompt{Label: "Commit and push", IsConfirm: true}
	if _, err := cp.Run(); err == nil {
		exec.Command("git", "add", workflowPath, "builder.json").Run()
		exec.Command("git", "commit", "-m", "Add Android build workflow").Run()
		exec.Command("git", "push").Run()
		fmt.Println("Pushed.")
	}

	// Offer immediate build
	bp := promptui.Prompt{Label: "Run build now", IsConfirm: true}
	if _, err := bp.Run(); err == nil {
		return triggerBuild(context.Background(), cfg, "dist", 30*time.Minute, false)
	}

	fmt.Println("\nTo build: builder android build")
	return nil
}
