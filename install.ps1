$ErrorActionPreference = "Stop"

$Repo = "Maortz/android-builder"
$Binary = "builder.exe"
$InstallDir = "$env:LOCALAPPDATA\Programs\android-builder"

$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "32-bit Windows not supported"; exit 1
}

$Release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
$Version = $Release.tag_name
if (-not $Version) { Write-Error "Could not get latest version"; exit 1 }

$Url = "https://github.com/$Repo/releases/download/$Version/builder_windows_$Arch.exe"
Write-Host "Installing android-builder $Version (windows/$Arch)..."

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Dest = "$InstallDir\$Binary"
Invoke-WebRequest -Uri $Url -OutFile $Dest

# Add to user PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to PATH (restart your terminal)"
}

Write-Host "Installed: $Dest"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  builder auth github       # save GitHub token"
Write-Host "  builder init              # add android-build.yml + update builder.json"
Write-Host "  builder android build     # trigger GHA build, download APK"
Write-Host "  builder dev flutter       # install APK + hot-reload session"
