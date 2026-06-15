# scripts/setup.ps1 — One-command setup for nexus-engine on Windows.
#
# What it does:
#   1. Verifies Go 1.21+
#   2. Installs ripgrep (winget / scoop / choco)
#   3. Installs uv (Python manager — no system Python needed)
#   4. Creates Python venv + installs docling-serve
#   5. Builds nexus.exe and nexus-grpc.exe to bin\
#   6. Installs git pre-commit hooks
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\setup.ps1
#
# NOTE: macOS and Windows support is not yet fully tested.
# Report issues at https://github.com/EngineerProjects/nexus-engine/issues

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

# ── Runtime root ──────────────────────────────────────────────────────────────
if (-not $env:NEXUS_RUNTIME_ROOT) {
    $env:NEXUS_RUNTIME_ROOT = Join-Path $env:APPDATA "nexus-cli"
}

# ── Helpers ───────────────────────────────────────────────────────────────────
function Write-Ok   { param($msg) Write-Host "  [OK]  $msg" -ForegroundColor Green }
function Write-Info { param($msg) Write-Host "  [ ]  $msg" -ForegroundColor Cyan }
function Write-Warn { param($msg) Write-Host "  [!]  $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg) Write-Host "  [X]  $msg" -ForegroundColor Red; exit 1 }
function Write-Step { param($msg) Write-Host "`n$msg" -ForegroundColor White }

# ── 1. Go ─────────────────────────────────────────────────────────────────────
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

# ── 2. ripgrep ────────────────────────────────────────────────────────────────
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

# ── 3. uv ─────────────────────────────────────────────────────────────────────
Write-Step "Checking uv (Python manager)..."

if (Get-Command uv -ErrorAction SilentlyContinue) {
    Write-Ok "uv $(uv --version)"
} else {
    Write-Info "Installing uv..."
    try {
        Invoke-RestMethod https://astral.sh/uv/install.ps1 | Invoke-Expression
        # Refresh PATH for this session
        $env:PATH = [System.Environment]::GetEnvironmentVariable("PATH", "User") + ";" + $env:PATH
        Write-Ok "uv $(uv --version)"
    } catch {
        Write-Fail "Failed to install uv: $_`nInstall manually: https://docs.astral.sh/uv/getting-started/installation/"
    }
}

# ── 4. Python venv + docling-serve ────────────────────────────────────────────
if ($env:SKIP_PYTHON -eq "1") {
    Write-Warn "Skipping Python/docling setup (SKIP_PYTHON=1)"
} else {
    Write-Step "Setting up Python environment..."

    $venvDir = Join-Path $env:NEXUS_RUNTIME_ROOT ".venv"
    $doclingBin = Join-Path $venvDir "Scripts\docling-serve.exe"

    if (-not (Test-Path $doclingBin)) {
        Write-Info "Creating venv at $venvDir..."
        New-Item -ItemType Directory -Force -Path $env:NEXUS_RUNTIME_ROOT | Out-Null
        uv venv $venvDir --python 3.11 --seed

        $pyBin = Join-Path $venvDir "Scripts\python.exe"
        $pkg = if ($env:DOCLING_EXTRAS) { "docling-serve[$env:DOCLING_EXTRAS]" } else { "docling-serve" }
        Write-Info "Installing $pkg..."
        uv pip install --python $pyBin $pkg
        Write-Ok "docling-serve installed"
    } else {
        Write-Ok "docling-serve (already installed)"
    }
}

# ── 5. Build ──────────────────────────────────────────────────────────────────
Write-Step "Building nexus-engine..."

Set-Location $RepoRoot
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -o "bin\nexus.exe" ".\cmd\cli"
Write-Ok "bin\nexus.exe"
go build -o "bin\nexus-grpc.exe" ".\cmd\grpc"
Write-Ok "bin\nexus-grpc.exe"

# ── 6. Git hooks ──────────────────────────────────────────────────────────────
Write-Step "Installing git hooks..."

$hooksDir = Join-Path $RepoRoot ".githooks"
if (Test-Path $hooksDir) {
    git -C $RepoRoot config core.hooksPath .githooks
    Write-Ok "Git hooks installed from .githooks\"
} else {
    Write-Warn "No .githooks\ directory found — skipping"
}

# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  Setup complete!" -ForegroundColor Green
Write-Host ""
Write-Host "  Runtime data: $env:NEXUS_RUNTIME_ROOT"
Write-Host ""
Write-Host "  Add bin\ to your PATH (current session):"
Write-Host "    `$env:PATH += `";$RepoRoot\bin`""
Write-Host ""
Write-Host "  Configure a provider:"
Write-Host "    nexus config --provider anthropic --api-key sk-ant-..."
Write-Host ""
Write-Host "  Start chatting:"
Write-Host "    nexus chat"
Write-Host ""
