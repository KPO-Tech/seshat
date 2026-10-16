# install.ps1 - Seshat end-user installer for Windows
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/KPO-Tech/seshat-runtime/main/scripts/install.ps1 | iex
#
# Options (set before running, as environment variables):
#   $env:SESHAT_VERSION = "v1.9.0"        Install a specific version (default: latest)
#   $env:SESHAT_INSTALL_DIR = "C:\tools"  Binary destination (default: %LOCALAPPDATA%\Programs\seshat)
#   $env:SESHAT_SKIP_RIPGREP = "1"        Do not try to install ripgrep
#
# What it does:
#   1. Picks the Windows build that matches the processor (amd64 or arm64)
#   2. Downloads it from the GitHub release and checks it against SHA256SUMS.txt
#   3. Installs seshat.exe and adds its folder to the user PATH
#   4. Installs ripgrep (winget, scoop or choco) when it is missing: the glob and
#      grep tools need it
#
# It never asks for administrator rights: everything goes to the current user.
#
# For developers who build from source: scripts\setup.ps1.

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"   # Invoke-WebRequest is much faster without the progress bar

$Repo = "KPO-Tech/seshat-runtime"
$Binary = "seshat"

function Write-Step($text) { Write-Host ""; Write-Host "> $text" -ForegroundColor White }
function Write-Info($text) { Write-Host "[seshat] $text" -ForegroundColor Cyan }
function Write-Ok($text) { Write-Host "[seshat] $text" -ForegroundColor Green }
function Write-Warn($text) { Write-Host "[seshat] $text" -ForegroundColor Yellow }

function Fail($text) {
    Write-Host "[seshat] $text" -ForegroundColor Red
    Write-Host "[seshat] Download a binary manually: https://github.com/$Repo/releases" -ForegroundColor Red
    throw $text
}

# Older Windows PowerShell does not use TLS 1.2 by default, which GitHub requires.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch { }

# -- Platform ------------------------------------------------------------------
Write-Step "Detecting platform"

$arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch ($arch.ToUpperInvariant()) {
    "AMD64" { $suffix = "windows-amd64" }
    "ARM64" { $suffix = "windows-arm64" }
    default { Fail "Unsupported processor: $arch" }
}
Write-Info "Platform: $suffix"

# -- Version -------------------------------------------------------------------
Write-Step "Resolving version"

$version = $env:SESHAT_VERSION
if (-not $version) {
    Write-Info "Fetching latest release..."
    try {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "seshat-installer" }
        $version = $latest.tag_name
    } catch {
        Fail "Could not resolve the latest version: $($_.Exception.Message). Set `$env:SESHAT_VERSION = 'vX.Y.Z'."
    }
}
if (-not $version) { Fail "Could not resolve the version. Set `$env:SESHAT_VERSION = 'vX.Y.Z'." }
Write-Info "Version: $version"

# -- Download ------------------------------------------------------------------
Write-Step "Downloading seshat $version"

# SESHAT_RELEASE_BASE overrides the download location (used to test the script against a local folder).
$baseUrl = if ($env:SESHAT_RELEASE_BASE) { $env:SESHAT_RELEASE_BASE.TrimEnd("/") } else { "https://github.com/$Repo/releases/download/$version" }
$asset = "$Binary-$suffix.exe"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("seshat-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Info "Binary: $asset"
    try {
        Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile (Join-Path $tmp $asset) -UseBasicParsing
        Invoke-WebRequest -Uri "$baseUrl/SHA256SUMS.txt" -OutFile (Join-Path $tmp "SHA256SUMS.txt") -UseBasicParsing
    } catch {
        Fail "Download failed: $($_.Exception.Message). Version $version may not include a Windows build."
    }

    # -- Checksum --------------------------------------------------------------
    Write-Step "Verifying checksum"

    $line = Get-Content (Join-Path $tmp "SHA256SUMS.txt") | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))\s*$" } | Select-Object -First 1
    if (-not $line) { Fail "$asset is not listed in SHA256SUMS.txt." }
    $expected = ($line -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $tmp $asset)).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { Fail "Checksum verification failed - aborting." }
    Write-Ok "Checksum OK"

    # -- Install ---------------------------------------------------------------
    Write-Step "Installing binary"

    $installDir = if ($env:SESHAT_INSTALL_DIR) { $env:SESHAT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\seshat" }
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $target = Join-Path $installDir "$Binary.exe"
    try {
        Copy-Item -Path (Join-Path $tmp $asset) -Destination $target -Force
    } catch {
        Fail "Could not write $target. Is seshat running? Close it and try again."
    }
    Write-Ok "Installed: $target"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# -- PATH (current user only) --------------------------------------------------
Write-Step "Configuring PATH"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = @()
if ($userPath) { $entries = $userPath -split ";" | Where-Object { $_ } }
$normalized = $installDir.TrimEnd("\")
if ($entries | Where-Object { $_.TrimEnd("\") -ieq $normalized }) {
    Write-Info "$installDir is already in the user PATH"
} else {
    [Environment]::SetEnvironmentVariable("Path", (($entries + $installDir) -join ";"), "User")
    Write-Ok "Added $installDir to the user PATH"
}
# Make seshat usable in this very session too.
if (-not (($env:Path -split ";") | Where-Object { $_.TrimEnd("\") -ieq $normalized })) {
    $env:Path = "$env:Path;$installDir"
}

# -- ripgrep -------------------------------------------------------------------
Write-Step "Checking ripgrep"

if ($env:SESHAT_SKIP_RIPGREP -eq "1") {
    Write-Info "Skipped (SESHAT_SKIP_RIPGREP=1)"
} elseif (Get-Command rg -ErrorAction SilentlyContinue) {
    Write-Ok "ripgrep is already installed"
} else {
    $installed = $false
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        Write-Info "Installing ripgrep with winget"
        & winget install --id BurntSushi.ripgrep.MSVC --exact --silent --accept-package-agreements --accept-source-agreements | Out-Null
        $installed = ($LASTEXITCODE -eq 0)
    } elseif (Get-Command scoop -ErrorAction SilentlyContinue) {
        Write-Info "Installing ripgrep with scoop"
        & scoop install ripgrep | Out-Null
        $installed = ($LASTEXITCODE -eq 0)
    } elseif (Get-Command choco -ErrorAction SilentlyContinue) {
        Write-Info "Installing ripgrep with choco"
        & choco install ripgrep -y | Out-Null
        $installed = ($LASTEXITCODE -eq 0)
    }
    if ($installed) {
        Write-Ok "ripgrep installed (open a new terminal to use it)"
    } else {
        Write-Warn "Could not install ripgrep. The glob and grep tools need it: https://github.com/BurntSushi/ripgrep#installation"
    }
}

# -- Done ----------------------------------------------------------------------
$runtimeRoot = if ($env:SESHAT_RUNTIME_ROOT) { $env:SESHAT_RUNTIME_ROOT } else { Join-Path $env:APPDATA "seshat-cli" }
Write-Host ""
Write-Host "Seshat $version installed successfully" -ForegroundColor Green
Write-Host ""
Write-Host "  Binary:  $target"
Write-Host "  Runtime: $runtimeRoot"
Write-Host "           (database and sessions are created on first run)"
Write-Host ""
Write-Host "  Get started (in a new terminal if seshat is not found):"
Write-Host "    seshat config      # configure your AI provider and API key"
Write-Host "    seshat chat        # start chatting"
Write-Host "    seshat terminal-setup   # make shift+Enter insert a newline (Windows Terminal, VS Code)"
Write-Host ""
Write-Host "  Docs: https://github.com/$Repo"
Write-Host ""
