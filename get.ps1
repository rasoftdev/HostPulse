# HostPulse remote installer for Windows: clones the repository, builds it, installs `hostpulse.exe`
# and deletes everything temporary (the cloned repo and, if needed, a temporary Go).
#
#   irm https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.ps1 | iex
#   $env:HOSTPULSE_ACTION='uninstall'; irm https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.ps1 | iex
#
# It installs into %LOCALAPPDATA%\Programs\HostPulse and adds it to the user PATH (no administrator needed).
# With HOSTPULSE_PREFIX it installs into that folder and leaves the PATH alone.
# Optional variables: HOSTPULSE_REPO (git URL), HOSTPULSE_REF (branch or tag, default main).
$ErrorActionPreference = 'Stop'

$Repo      = if ($env:HOSTPULSE_REPO) { $env:HOSTPULSE_REPO } else { 'https://github.com/rasoftdev/HostPulse.git' }
$Ref       = if ($env:HOSTPULSE_REF)  { $env:HOSTPULSE_REF }  else { 'main' }
$GoVersion = '1.26.0'
$InstallDir = if ($env:HOSTPULSE_PREFIX) { $env:HOSTPULSE_PREFIX } else { Join-Path $env:LOCALAPPDATA 'Programs\HostPulse' }

function Say($m) { Write-Host "==> $m" -ForegroundColor Cyan }
function Die($m) { Write-Host "ERROR: $m" -ForegroundColor Red; throw $m }

function Remove-FromUserPath($dir) {
    $p = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $p) { return }
    $parts = $p -split ';' | Where-Object { $_ -and ($_.TrimEnd('\') -ne $dir.TrimEnd('\')) }
    [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
}

if ($env:HOSTPULSE_ACTION -eq 'uninstall') {
    Say "Uninstalling HostPulse from $InstallDir..."
    Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue
    Remove-FromUserPath $InstallDir
    Say 'Done. HostPulse has been removed (open a new terminal to refresh the PATH).'
    return
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Die 'git is missing (winget install Git.Git).' }
$arch = switch ($env:PROCESSOR_ARCHITECTURE) { 'ARM64' { 'arm64' } 'AMD64' { 'amd64' } default { Die "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" } }

# Everything temporary lives here and is always deleted when finished (success or failure).
$Work = Join-Path ([IO.Path]::GetTempPath()) ("hostpulse-build-" + [Guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $Work | Out-Null
$oldGoCache, $oldGoPath, $oldTc, $oldCgo, $oldPath = $env:GOCACHE, $env:GOPATH, $env:GOTOOLCHAIN, $env:CGO_ENABLED, $env:PATH
try {
    Say "Cloning the repository ($Ref)..."
    git clone --quiet --depth 1 --branch $Ref $Repo (Join-Path $Work 'repo')
    if ($LASTEXITCODE -ne 0) { Die 'Could not clone the repository.' }

    # Go: use the system one if it is recent enough; otherwise a temporary copy inside $Work.
    $needGo = $true
    if (Get-Command go -ErrorAction SilentlyContinue) {
        $have = (go env GOVERSION) -replace '^go', ''
        try { if ([version]($have -replace '[^\d\.].*$', '') -ge [version]$GoVersion) { $needGo = $false } } catch {}
    }
    if ($needGo) {
        Say "Downloading a temporary Go $GoVersion (it will be removed when finished)..."
        $zip = Join-Path $Work 'go.zip'
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -UseBasicParsing "https://go.dev/dl/go$GoVersion.windows-$arch.zip" -OutFile $zip
        Expand-Archive -Path $zip -DestinationPath $Work -Force
        Remove-Item $zip
        $env:PATH = (Join-Path $Work 'go\bin') + ';' + $env:PATH
    }
    $env:GOCACHE = Join-Path $Work 'gocache'; $env:GOPATH = Join-Path $Work 'gopath'
    $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'

    Say 'Building...'
    Push-Location (Join-Path $Work 'repo')
    try { go build -trimpath -ldflags '-s -w' -o (Join-Path $Work 'hostpulse.exe') ./cmd/hostpulse; if ($LASTEXITCODE -ne 0) { Die 'The build failed.' } }
    finally { Pop-Location }

    Say "Installing into $InstallDir..."
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item (Join-Path $Work 'hostpulse.exe') (Join-Path $InstallDir 'hostpulse.exe') -Force

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $env:HOSTPULSE_PREFIX -and -not (($userPath -split ';') | Where-Object { $_.TrimEnd('\') -eq $InstallDir.TrimEnd('\') })) {
        [Environment]::SetEnvironmentVariable('Path', (($userPath, $InstallDir | Where-Object { $_ }) -join ';'), 'User')
    }
    Say 'Cleaning up temporary files...'
}
finally {
    $env:GOCACHE, $env:GOPATH, $env:GOTOOLCHAIN, $env:CGO_ENABLED, $env:PATH = $oldGoCache, $oldGoPath, $oldTc, $oldCgo, $oldPath
    # Read-only files in Go's cache need -Force to be deleted.
    Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}
Write-Host ''
Write-Host "HostPulse installed: $InstallDir\hostpulse.exe"
Write-Host 'Open a NEW terminal (so the PATH is reloaded) and run:  hostpulse'
