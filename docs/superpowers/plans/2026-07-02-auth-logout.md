# Auth Logout Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `builder auth logout` to remove stored GitHub credentials (keyring + plain-text fallback).

**Architecture:** `internal/auth/auth.go` already has `DeleteToken()` which deletes from keyring and removes the fallback file. This task only needs a Cobra subcommand in `cmd/builder/auth.go` that calls it and prints a success message, plus a test proving `GetToken()` fails after logout.

**Tech Stack:** Go, Cobra, github.com/zalando/go-keyring.

## Global Constraints

- Match existing style in `cmd/builder/auth.go` (var-based cobra.Command, wired in `init()`).
- `builder auth logout` must exit cleanly (no error) when nothing is stored (`DeleteToken` already ignores `keyring.ErrNotFound` style errors via `_ = err`).
- Success message: `Logged out successfully`.

---

### Task 1: Add `auth logout` subcommand and test

**Files:**
- Modify: `cmd/builder/auth.go`
- Test: `internal/auth/auth_test.go` (new file)

**Interfaces:**
- Consumes: `auth.DeleteToken()` (existing, `internal/auth/auth.go:52`), `auth.GetToken() (string, error)` (existing, `internal/auth/auth.go:23`), `auth.SetToken(token string) error` (existing, `internal/auth/auth.go:44`).
- Produces: `authLogoutCmd *cobra.Command` in package `main`, registered under `authCmd`.

- [ ] **Step 1: Write failing test for DeleteToken clearing GetToken**

Create `internal/auth/auth_test.go`:

```go
package auth

import "testing"

func TestDeleteTokenClearsFallbackFile(t *testing.T) {
	tmp := t.TempDir()
	old := tokenFile
	tokenFile = tmp + "/token"
	defer func() { tokenFile = old }()

	if err := SetToken("dummy-token"); err != nil {
		t.Fatalf("SetToken failed: %v", err)
	}

	DeleteToken()

	if _, err := GetToken(); err == nil {
		t.Fatal("expected GetToken to fail after DeleteToken, got nil error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails or passes against current code**

Run: `go test ./internal/auth/... -run TestDeleteTokenClearsFallbackFile -v`
Expected: PASS (DeleteToken already implemented) — this test is a regression guard, not a TDD-red step. If it fails, investigate `DeleteToken`/`SetToken` before continuing (do not modify their behavior as part of this task; if broken, that's a pre-existing bug to flag separately).

- [ ] **Step 3: Add `authLogoutCmd` to `cmd/builder/auth.go`**

Replace the full file contents with:

```go
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
```

- [ ] **Step 4: Build and run full test suite**

Run: `go build -o builder ./cmd/builder && go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 5: Manual smoke test**

Run: `./builder auth logout`
Expected output: `Logged out successfully`

- [ ] **Step 6: Commit**

```bash
git add cmd/builder/auth.go internal/auth/auth_test.go
git commit -m "feat: add builder auth logout command"
```
