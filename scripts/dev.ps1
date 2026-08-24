# SPEC-01 dev loop: builds the daemon binary, starts it in the background, then
# runs `wails dev` (the GUI). The daemon is stopped when wails dev exits.
$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$binDir = Join-Path $root "build"
$bin = Join-Path $binDir "heka-dev.exe"
$logOut = Join-Path $root ".heka-dev-daemon.log"
$logErr = Join-Path $root ".heka-dev-daemon.log.err"

New-Item -ItemType Directory -Force -Path $binDir | Out-Null

# main.go embeds all:frontend/dist, so it must exist before `go build`.
$dist = Join-Path $root "frontend\dist"
if (-not (Test-Path $dist)) {
    Write-Host "Building frontend (frontend/dist missing)..." -ForegroundColor Yellow
    Push-Location (Join-Path $root "frontend")
    try { npm run build } finally { Pop-Location }
}

Write-Host "Building daemon binary..."
go build -o $bin .

Write-Host "Starting Heka daemon (logs: $logOut)..."
$daemon = Start-Process -FilePath $bin -ArgumentList "daemon" `
    -WorkingDirectory $root `
    -RedirectStandardOutput $logOut -RedirectStandardError $logErr `
    -PassThru -NoNewWindow
Write-Host "Daemon pid $($daemon.Id) started."

try {
    wails dev
}
finally {
    if (-not $daemon.HasExited) {
        Stop-Process -Id $daemon.Id -Force
        Write-Host "Daemon stopped."
    }
}