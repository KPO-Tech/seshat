# scripts/setup.ps1 - One-command setup for Seshat on Windows.
#
# What it does:
#   1. Verifies Go 1.21+
#   2. Installs ripgrep (winget / scoop / choco)
#   3. Builds seshat.exe and seshat-grpc.exe to bin\
#   4. Installs git pre-commit hooks
#   5. Offers to make shift+Enter / ctrl+Enter insert a newline (Windows Terminal, VS Code)
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\setup.ps1
#
# NOTE: macOS and Windows support is not yet fully tested.
# Report issues at https://github.com/KPO-Tech/seshat-runtime/issues

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

# -- Runtime root --------------------------------------------------------------
if (-not $env:SESHAT_RUNTIME_ROOT) {
    $env:SESHAT_RUNTIME_ROOT = Join-Path $env:APPDATA "seshat-cli"
}

# -- Helpers -------------------------------------------------------------------
function Write-Ok   { param($msg) Write-Host "  [OK]  $msg" -ForegroundColor Green }
function Write-Info { param($msg) Write-Host "  [ ]  $msg" -ForegroundColor Cyan }
function Write-Warn { param($msg) Write-Host "  [!]  $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  [X]  $msg" -ForegroundColor Red; exit 1 }
function Write-Step { param($msg) Write-Host "`n$msg" -ForegroundColor White }

# -- 1. Go ---------------------------------------------------------------------
Write-Step "Checking Go..."

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Fail "Go not found. Install Go 1.21+ from: https://go.dev/dl/"
}

$goVer = (go version) -replace "go version go", "" -replace " .*", ""
$parts = $goVer -split "\."
$major = [int]$parts[0]
$minor = [int]$parts[1]
if ($major -lt 1 -or ($major -eq 1 -and $minor -lt 21)) {
    Write-Fail "Go $goVer found but 1.21+ required. Update at: https://go.dev/dl/"
}
Write-Ok "Go $goVer"

# -- 2. ripgrep ----------------------------------------------------------------
Write-Step "Checking ripgrep..."

if (Get-Command rg -ErrorAction SilentlyContinue) {
    $rgVer = (rg --version | Select-Object -First 1) -replace "ripgrep ", ""
    Write-Ok "ripgrep $rgVer"
} else {
    Write-Info "Installing ripgrep..."
    $installed = $false
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id BurntSushi.ripgrep.MSVC --silent --accept-source-agreements --accept-package-agreements
        $installed = $true
    } elseif (Get-Command scoop -ErrorAction SilentlyContinue) {
        scoop install ripgrep
        $installed = $true
    } elseif (Get-Command choco -ErrorAction SilentlyContinue) {
        choco install ripgrep -y
        $installed = $true
    }
    if ($installed) {
        Write-Ok "ripgrep installed"
    } else {
        Write-Warn "Could not auto-install ripgrep. Download from: https://github.com/BurntSushi/ripgrep/releases"
    }
}

# -- 3. Build ------------------------------------------------------------------
Write-Step "Building Seshat..."

Set-Location $RepoRoot
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -o "bin\seshat.exe" ".\tui\cmd\cli"
Write-Ok "bin\seshat.exe"
go build -o "bin\seshat-grpc.exe" ".\cmd\grpc"
Write-Ok "bin\seshat-grpc.exe"

# -- 4. Git hooks --------------------------------------------------------------
Write-Step "Installing git hooks..."

$hooksDir = Join-Path $RepoRoot ".githooks"
if (Test-Path $hooksDir) {
    git -C $RepoRoot config core.hooksPath .githooks
    Write-Ok "Git hooks installed from .githooks\"
} else {
    Write-Warn "No .githooks\ directory found - skipping"
}

# -- 5. Terminal keys ----------------------------------------------------------
# Windows Terminal and the VS Code terminal send the same thing for Enter,
# shift+Enter and ctrl+Enter, so they have to be told to send what Seshat
# decodes. ctrl+j works without it. Each settings file is backed up first.
Write-Step "Terminal keys (shift+Enter, ctrl+Enter)..."

$answer = "y"
if (-not $env:SESHAT_SETUP_YES) {
    try {
        $answer = Read-Host "  Make shift+Enter and ctrl+Enter insert a newline in Windows Terminal and VS Code? [Y/n]"
    } catch {
        $answer = "n"   # no console to ask on: leave the user's settings alone
    }
}
if ($answer -eq "" -or $answer -match "^[yY]") {
    try {
        & (Join-Path $RepoRoot "bin\seshat.exe") terminal-setup
    } catch {
        Write-Warn "terminal-setup failed: $_"
    }
} else {
    Write-Info "Skipped. Run it later with: seshat terminal-setup   (add --check to preview)"
}

# -- Done ----------------------------------------------------------------------
Write-Host ""
Write-Host "  Setup complete!" -ForegroundColor Green
Write-Host ""
Write-Host "  Runtime data: $env:SESHAT_RUNTIME_ROOT"
Write-Host ""
Write-Host "  Add bin\ to your PATH (current session):"
Write-Host "    `$env:PATH += `";$RepoRoot\bin`""
Write-Host ""
Write-Host "  Configure a provider:"
Write-Host "    seshat config --provider anthropic --api-key sk-ant-..."
Write-Host ""
Write-Host "  Start chatting:"
Write-Host "    seshat chat"
Write-Host ""
