# Signing Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `builder signing setup` to base64-encode an Android keystore and upload it + credentials as 4 GitHub Actions secrets, update `builder.json`, and make the workflow template sign release builds when those secrets exist.

**Architecture:** `internal/github` gets two new `Client` methods (`GetActionsPublicKey`, `CreateOrUpdateSecret`) implementing GitHub's [repo secrets API](libsodium sealed-box encryption via `golang.org/x/crypto/nacl/box`). `cmd/builder/signing.go` collects keystore path + credentials (flags or interactive prompts), encrypts and uploads the 4 secrets, sets `android.signing = true` in `builder.json`. The workflow template gets a conditional `Sign APK` step gated on `secrets.ANDROID_KEYSTORE != ''`, and `builder android build` gets an `--unsigned` flag that forces the workflow to skip signing even when secrets exist (via a new `unsigned` workflow input).

**Tech Stack:** Go, Cobra, `golang.org/x/crypto/nacl/box` (new dependency), GitHub REST API, `promptui` (masked password prompts).

## Global Constraints

- Keystore file is never written to disk beyond the source file itself (only read into memory, then discarded).
- All 4 secrets: `ANDROID_KEYSTORE`, `ANDROID_KEY_ALIAS`, `ANDROID_STORE_PASSWORD`, `ANDROID_KEY_PASSWORD`.
- Running setup a second time overwrites existing secrets without error (GitHub's PUT-secret endpoint is already upsert — no special handling needed).
- `key-password` defaults to `store-password` when omitted.
- `builder android build --unsigned` skips signing even when secrets are configured.
- Print a reminder if the workflow file doesn't yet reference the signing secrets (i.e., predates this feature).

---

### Task 1: Add GitHub Actions secrets API to `internal/github`

**Files:**
- Modify: `internal/github/client.go` (none needed beyond existing `do`/`decode` helpers — read first to confirm)
- Create: `internal/github/secrets.go`
- Test: `internal/github/secrets_test.go`
- Modify: `go.mod`, `go.sum` (add `golang.org/x/crypto`)

**Interfaces:**
- Consumes: `(c *Client) do(ctx, method, path string, body io.Reader) (*http.Response, error)` and `(c *Client) decode(resp *http.Response, v any) error` (existing, `internal/github/client.go`).
- Produces:
  - `type PublicKey struct { KeyID string \`json:"key_id"\`; Key string \`json:"key"\` }`
  - `func (c *Client) GetActionsPublicKey(ctx context.Context, owner, repo string) (*PublicKey, error)`
  - `func (c *Client) CreateOrUpdateSecret(ctx context.Context, owner, repo, secretName, plaintext string, pubKey *PublicKey) error`
  - `func encryptSecret(plaintext string, pubKeyB64 string) (string, error)` (base64-encoded sealed box, unexported — tested directly since it's pure and doesn't need a live GitHub API)

- [ ] **Step 1: Add the crypto dependency**

Run: `cd /workspace && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go get golang.org/x/crypto@latest`
Expected: `go.mod`/`go.sum` updated with a `golang.org/x/crypto` entry.

- [ ] **Step 2: Write failing test for `encryptSecret`**

Create `internal/github/secrets_test.go`:

```go
package github

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

func TestEncryptSecretRoundTrip(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(pub[:])

	sealedB64, err := encryptSecret("super-secret-value", pubKeyB64)
	if err != nil {
		t.Fatalf("encryptSecret: %v", err)
	}

	sealed, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		t.Fatalf("decode sealed box: %v", err)
	}

	opened, ok := box.OpenAnonymous(nil, sealed, pub, priv)
	if !ok {
		t.Fatal("failed to open sealed box")
	}
	if string(opened) != "super-secret-value" {
		t.Errorf("got %q, want %q", opened, "super-secret-value")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/github/... -run TestEncryptSecretRoundTrip -v`
Expected: FAIL — `encryptSecret` undefined.

- [ ] **Step 4: Implement `internal/github/secrets.go`**

```go
package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/nacl/box"
)

type PublicKey struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

func (c *Client) GetActionsPublicKey(ctx context.Context, owner, repo string) (*PublicKey, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/secrets/public-key", owner, repo)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var pk PublicKey
	if err := c.decode(resp, &pk); err != nil {
		return nil, fmt.Errorf("get actions public key: %w", err)
	}
	return &pk, nil
}

func encryptSecret(plaintext string, pubKeyB64 string) (string, error) {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	if len(pubKeyBytes) != 32 {
		return "", fmt.Errorf("unexpected public key length %d, want 32", len(pubKeyBytes))
	}
	var pubKey [32]byte
	copy(pubKey[:], pubKeyBytes)

	sealed, err := box.SealAnonymous(nil, []byte(plaintext), &pubKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("seal secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *Client) CreateOrUpdateSecret(ctx context.Context, owner, repo, secretName, plaintext string, pubKey *PublicKey) error {
	encryptedValue, err := encryptSecret(plaintext, pubKey.Key)
	if err != nil {
		return fmt.Errorf("encrypt %s: %w", secretName, err)
	}
	type payload struct {
		EncryptedValue string `json:"encrypted_value"`
		KeyID          string `json:"key_id"`
	}
	b, _ := json.Marshal(payload{EncryptedValue: encryptedValue, KeyID: pubKey.KeyID})
	path := fmt.Sprintf("/repos/%s/%s/actions/secrets/%s", owner, repo, secretName)
	resp, err := c.do(ctx, "PUT", path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("set secret %s: HTTP %d", secretName, resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./internal/github/... -v`
Expected: PASS

- [ ] **Step 6: Run full test suite and build**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build ./... && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/github/secrets.go internal/github/secrets_test.go go.mod go.sum
git commit -m "feat: add GitHub Actions secrets API to internal/github"
```

---

### Task 2: Add `android.signing` config field and `builder signing setup` command

**Files:**
- Modify: `internal/config/types.go`
- Create: `cmd/builder/signing.go`
- Modify: `cmd/builder/root.go`

**Interfaces:**
- Consumes: `getGitHubClient() (*github.Client, error)` and `loadConfig() (*config.Config, error)` (existing, `cmd/builder/root.go`); `(*github.Client).GetActionsPublicKey(ctx, owner, repo) (*github.PublicKey, error)` and `(*github.Client).CreateOrUpdateSecret(ctx, owner, repo, secretName, plaintext string, pubKey *github.PublicKey) error` (Task 1).
- Produces: `signingCmd *cobra.Command` registered on `rootCmd`; `AndroidConfig.Signing bool` field.

- [ ] **Step 1: Add `Signing` field to `AndroidConfig`**

In `internal/config/types.go`:

```go
type AndroidConfig struct {
	BuildType   string `json:"buildType,omitempty"`
	Flavor      string `json:"flavor,omitempty"`
	PackageName string `json:"packageName,omitempty"`
	DeviceID    string `json:"deviceId,omitempty"`
	Signing     bool   `json:"signing,omitempty"`
}
```

- [ ] **Step 2: Implement `cmd/builder/signing.go`**

```go
package main

import (
	"encoding/base64"
	"fmt"
	"os"

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
	if err := configManagerSave(cfg); err != nil {
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
	return contains(string(data), "ANDROID_KEYSTORE")
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
```

Note: `contains`/`indexOf` duplicate `strings.Contains` — replace them by importing `"strings"` and calling `strings.Contains(string(data), "ANDROID_KEYSTORE")` directly in `workflowReferencesSigningSecrets`, then delete the two helper functions entirely. Final imports for this file: `"encoding/base64"`, `"fmt"`, `"os"`, `"strings"`, `"github.com/manifoldco/promptui"`, `"github.com/spf13/cobra"`.

Also, `configManagerSave` isn't a real existing helper — replace that call with the actual pattern used elsewhere in this codebase (`internal/config.NewManager().Save(cfg)`, see `cmd/builder/init.go`). Add `"github.com/Maortz/android-builder/internal/config"` to imports and change the line to:

```go
	if err := config.NewManager().Save(cfg); err != nil {
		return fmt.Errorf("update builder.json: %w", err)
	}
```

- [ ] **Step 3: Wire `signingCmd` into root**

In `cmd/builder/root.go`, add `rootCmd.AddCommand(signingCmd)` inside `init()`.

- [ ] **Step 4: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 5: Smoke test flag registration**

Run: `./builder signing setup --help`
Expected: help lists `--keystore/-k`, `--key-alias`, `--store-password`, `--key-password`.

- [ ] **Step 6: Commit**

```bash
git add internal/config/types.go cmd/builder/signing.go cmd/builder/root.go
git commit -m "feat: add builder signing setup command"
```

---

### Task 3: Conditional signing step in workflow template + `--unsigned` build flag

**Files:**
- Modify: `internal/workflow/templates/android-build.yml`
- Modify: `internal/build/coordinator.go`
- Modify: `cmd/builder/android.go`

**Interfaces:**
- Consumes: `build.BuildOptions` (existing, `internal/build/coordinator.go`) — add an `Unsigned bool` field.
- Produces: workflow input `unsigned` (`"true"`/`"false"` string, GitHub Actions inputs are always strings) consumed by the new conditional step; `--unsigned` CLI flag on `builder android build`.

- [ ] **Step 1: Add the `unsigned` workflow input and conditional signing step**

In `internal/workflow/templates/android-build.yml`, add a new input under `on.workflow_dispatch.inputs` (after `flutter_version`):

```yaml
      unsigned:
        description: 'Force an unsigned build even if signing secrets are configured'
        required: false
        type: string
        default: 'false'
```

Add a new step before `- name: Build APK` (so the keystore is on disk when Gradle runs):

```yaml
      - name: Decode signing keystore
        if: ${{ secrets.ANDROID_KEYSTORE != '' && inputs.unsigned != 'true' }}
        working-directory: app/android/app
        run: echo "${{ secrets.ANDROID_KEYSTORE }}" | base64 --decode > release.keystore

      - name: Configure signing env
        if: ${{ secrets.ANDROID_KEYSTORE != '' && inputs.unsigned != 'true' }}
        run: |
          echo "ANDROID_KEY_ALIAS=${{ secrets.ANDROID_KEY_ALIAS }}" >> "$GITHUB_ENV"
          echo "ANDROID_STORE_PASSWORD=${{ secrets.ANDROID_STORE_PASSWORD }}" >> "$GITHUB_ENV"
          echo "ANDROID_KEY_PASSWORD=${{ secrets.ANDROID_KEY_PASSWORD }}" >> "$GITHUB_ENV"
```

Note: wiring these env vars into the app's own `android/app/build.gradle` signingConfigs block is the app project's responsibility (this workflow only stages the decoded keystore and credentials as env vars); document this in the command's success message rather than trying to rewrite the consuming app's Gradle files, which is out of scope for this CLI.

- [ ] **Step 2: Add `Unsigned` to `BuildOptions` and pass it through to `TriggerWorkflow` inputs**

In `internal/build/coordinator.go`, change:

```go
type BuildOptions struct {
	OutputDir string
	Timeout   time.Duration
	Release   bool
}
```

to:

```go
type BuildOptions struct {
	OutputDir string
	Timeout   time.Duration
	Release   bool
	Unsigned  bool
}
```

And in `Build`, change:

```go
	inputs := map[string]string{"build_id": buildID, "build_type": buildType}
	if c.config.Flutter.Version != "" {
		inputs["flutter_version"] = c.config.Flutter.Version
	}
```

to:

```go
	inputs := map[string]string{"build_id": buildID, "build_type": buildType}
	if c.config.Flutter.Version != "" {
		inputs["flutter_version"] = c.config.Flutter.Version
	}
	if opts.Unsigned {
		inputs["unsigned"] = "true"
	}
```

- [ ] **Step 3: Add `--unsigned` flag to `cmd/builder/android.go`**

In `init()`, add:

```go
	androidBuildCmd.Flags().Bool("unsigned", false, "Skip signing even if signing secrets are configured")
```

In `runAndroidBuild`, read the flag and pass it through:

```go
	unsigned, _ := cmd.Flags().GetBool("unsigned")
```

Update the `triggerBuild` call and function signature to carry it through:

```go
	apkPath, err := triggerBuild(cmd.Context(), cfg, output, timeout, release, unsigned)
```

```go
func triggerBuild(ctx context.Context, cfg *config.Config, outputDir string, timeout time.Duration, release, unsigned bool) (string, error) {
	gh, err := getGitHubClient()
	if err != nil {
		return "", err
	}
	coord := build.NewCoordinator(cfg, gh)
	result, err := coord.Build(ctx, build.BuildOptions{
		OutputDir: outputDir,
		Timeout:   timeout,
		Release:   release,
		Unsigned:  unsigned,
	})
	if err != nil {
		return "", err
	}
	fmt.Printf("APK: %s\n", result.APKPath)
	fmt.Printf("Workflow: %s\n", result.WorkflowURL)
	return result.APKPath, nil
}
```

Update the other call site in `cmd/builder/init.go` (the "Run build now" prompt) to pass `false` for the new `unsigned` parameter:

```go
		_, err := triggerBuild(context.Background(), cfg, "dist", 30*time.Minute, false, false)
		return err
```

- [ ] **Step 4: Build and run full test suite**

Run: `GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go build -o builder ./cmd/builder && GOPATH=/tmp/gopath GOCACHE=/tmp/gocache go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 5: Smoke test flag registration**

Run: `./builder android build --help`
Expected: help output includes `--unsigned`.

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/templates/android-build.yml internal/build/coordinator.go cmd/builder/android.go cmd/builder/init.go
git commit -m "feat: add conditional APK signing step and --unsigned build flag"
```
