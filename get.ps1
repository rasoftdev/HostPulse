# Instalador remoto de HostPulse para Windows: clona el repositorio, compila, instala `hostpulse.exe`
# y borra todo lo temporal (el repo clonado y, si hizo falta, el Go temporal).
#
#   irm https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.ps1 | iex
#   $env:HOSTPULSE_ACTION='uninstall'; irm https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.ps1 | iex
#
# Se instala en %LOCALAPPDATA%\Programs\HostPulse y se añade al PATH del usuario (no necesita administrador).
# Con HOSTPULSE_PREFIX se instala en esa carpeta y no se toca el PATH.
# Variables opcionales: HOSTPULSE_REPO (URL git), HOSTPULSE_REF (rama o etiqueta, por defecto main).
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
    Say "Desinstalando HostPulse de $InstallDir..."
    Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue
    Remove-FromUserPath $InstallDir
    Say 'Listo. HostPulse se ha eliminado (abre una terminal nueva para refrescar el PATH).'
    return
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Die 'Falta git (winget install Git.Git).' }
$arch = switch ($env:PROCESSOR_ARCHITECTURE) { 'ARM64' { 'arm64' } 'AMD64' { 'amd64' } default { Die "Arquitectura no soportada: $env:PROCESSOR_ARCHITECTURE" } }

# Todo lo temporal vive aquí y se borra siempre al terminar (con éxito o con error).
$Work = Join-Path ([IO.Path]::GetTempPath()) ("hostpulse-build-" + [Guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $Work | Out-Null
$oldGoCache, $oldGoPath, $oldTc, $oldCgo, $oldPath = $env:GOCACHE, $env:GOPATH, $env:GOTOOLCHAIN, $env:CGO_ENABLED, $env:PATH
try {
    Say "Clonando el repositorio ($Ref)..."
    git clone --quiet --depth 1 --branch $Ref $Repo (Join-Path $Work 'repo')
    if ($LASTEXITCODE -ne 0) { Die 'No se pudo clonar el repositorio.' }

    # Go: usa el del sistema si es lo bastante reciente; si no, una copia temporal dentro de $Work.
    $needGo = $true
    if (Get-Command go -ErrorAction SilentlyContinue) {
        $have = (go env GOVERSION) -replace '^go', ''
        try { if ([version]($have -replace '[^\d\.].*$', '') -ge [version]$GoVersion) { $needGo = $false } } catch {}
    }
    if ($needGo) {
        Say "Descargando un Go $GoVersion temporal (se borrará al terminar)..."
        $zip = Join-Path $Work 'go.zip'
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -UseBasicParsing "https://go.dev/dl/go$GoVersion.windows-$arch.zip" -OutFile $zip
        Expand-Archive -Path $zip -DestinationPath $Work -Force
        Remove-Item $zip
        $env:PATH = (Join-Path $Work 'go\bin') + ';' + $env:PATH
    }
    $env:GOCACHE = Join-Path $Work 'gocache'; $env:GOPATH = Join-Path $Work 'gopath'
    $env:GOTOOLCHAIN = 'local'; $env:CGO_ENABLED = '0'

    Say 'Compilando...'
    Push-Location (Join-Path $Work 'repo')
    try { go build -trimpath -ldflags '-s -w' -o (Join-Path $Work 'hostpulse.exe') ./cmd/hostpulse; if ($LASTEXITCODE -ne 0) { Die 'Falló la compilación.' } }
    finally { Pop-Location }

    Say "Instalando en $InstallDir..."
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item (Join-Path $Work 'hostpulse.exe') (Join-Path $InstallDir 'hostpulse.exe') -Force

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $env:HOSTPULSE_PREFIX -and -not (($userPath -split ';') | Where-Object { $_.TrimEnd('\') -eq $InstallDir.TrimEnd('\') })) {
        [Environment]::SetEnvironmentVariable('Path', (($userPath, $InstallDir | Where-Object { $_ }) -join ';'), 'User')
    }
    Say 'Limpiando archivos temporales...'
}
finally {
    $env:GOCACHE, $env:GOPATH, $env:GOTOOLCHAIN, $env:CGO_ENABLED, $env:PATH = $oldGoCache, $oldGoPath, $oldTc, $oldCgo, $oldPath
    # Los ficheros de solo lectura del caché de Go impiden borrar sin -Force.
    Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}
Write-Host ''
Write-Host "HostPulse instalado: $InstallDir\hostpulse.exe"
Write-Host 'Abre una terminal NUEVA (para que cargue el PATH) y escribe:  hostpulse'
