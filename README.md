# android-builder

Build Android APKs remotely on GitHub Actions, download them, and start a Flutter hot-reload session on a connected device — from any OS.

Mirror of [MobAI-App/ios-builder](https://github.com/MobAI-App/ios-builder) for Android.

## Install

**macOS / Linux**
```bash
curl -fsSL https://raw.githubusercontent.com/Maortz/android-builder/main/install.sh | bash
```

**Windows (PowerShell)**
```powershell
irm https://raw.githubusercontent.com/Maortz/android-builder/main/install.ps1 | iex
```

Or manually download a binary from [Releases](https://github.com/Maortz/android-builder/releases) and place it in your `PATH`.

## Prerequisites

- **GitHub account** with a repository containing your Flutter app
- **adb** (Android Debug Bridge) — typically installed with Android SDK
- **flutter** — in your PATH
- **Android device** with USB debugging enabled, connected via USB

## Quick start

### 1. Save your GitHub token

```bash
builder auth github
```

Paste a personal access token with `repo` and `actions:read` scopes. Token is stored securely using your OS keyring (or plain file fallback).

### 2. Initialize the project

```bash
builder init
```

Detects your GitHub repository, optionally prompts for Flutter version, and creates `.github/workflows/android-build.yml`. Optionally commits and pushes.

### 3. Trigger a remote build

```bash
builder android build
```

Pushes a workflow run to GitHub Actions, polls for completion, downloads the APK to `dist/` (default), and prints the workflow URL.

Flags:
- `--release` — build release APK instead of debug
- `--output DIR` — save APK to different directory (default: `dist`)
- `--timeout DURATION` — polling timeout (default: 30m)

### 4. Start a hot-reload session

```bash
builder dev flutter
```

Installs the APK to your connected device via adb, finds the app package, then runs `flutter attach` for hot reload on file changes (in `lib/` by default).

Flags:
- `--apk PATH` — explicit APK path (auto-detect from `dist/` if omitted)
- `--device ID` — explicit adb device ID (default: first available)
- `--package PKG` — app package name; required with `--skip-install`
- `--skip-install` — skip adb install, just attach (e.g., if already installed)
- `--no-watch` — disable file-change hot reload
- `--no-attach` — print `flutter attach` command instead of running it

## Commands

| Command | Description |
|---------|-------------|
| `builder auth github` | Save GitHub personal access token |
| `builder init` | Create `android-build.yml` + `builder.json` |
| `builder android build` | Trigger debug build on GHA, download APK |
| `builder android build --release` | Trigger release build on GHA, download APK |
| `builder dev flutter` | Install APK + start hot-reload session |
| `builder dev flutter --no-watch` | Attach without file-change hot reload |
| `builder dev flutter --skip-install --package com.example.app` | Attach to already-installed app |

## Configuration

After `builder init`, edit `builder.json` to customize:

```json
{
  "project": "MyApp",
  "platform": "android",
  "github": {
    "owner": "your-username",
    "repo": "your-repo",
    "branch": "master"
  },
  "android": {
    "buildType": "debug",
    "flavor": "",
    "packageName": "com.example.app"
  },
  "flutter": {
    "version": "3.24.0",
    "watch": {
      "dirs": ["lib"],
      "patterns": [".dart"],
      "ignore": [".g.dart", ".freezed.dart"],
      "debounce": 100
    }
  }
}
```

- **platform** — `"android"` (or `"ios"` if extending to match ios-builder)
- **android.buildType** — `"debug"` or `"release"`
- **android.flavor** — optional build flavor
- **android.packageName** — app package name (e.g., `com.example.allergy_detector`)
- **flutter.version** — Flutter version; empty string = latest stable
- **github.branch** — branch to dispatch workflow on (default: `master`)
- **flutter.watch** — file watcher config for hot reload (dirs, patterns, ignore list, debounce in ms)

## How it works

1. **Init:** `builder init` writes `.github/workflows/android-build.yml` (GitHub Actions workflow) and `builder.json` (config).
2. **Build:** `builder android build` triggers the workflow via GitHub API, polls for completion, and downloads the build artifact (APK).
3. **Dev session:** `builder dev flutter` installs the APK via adb, detects the app package name, then runs `flutter attach` to connect to the running app. With file watching enabled, changes to `.dart` files trigger hot reload.

## Development

Clone the repository and build locally:

```bash
cd android-builder
go build -o builder ./cmd/builder
./builder --help
```

Run tests:

```bash
go test ./...
```

Binaries are released for:
- `linux-amd64`
- `linux-arm64`
- `darwin-amd64` (macOS Intel)
- `darwin-arm64` (macOS Apple Silicon)
- `windows-amd64`

## License

MIT
