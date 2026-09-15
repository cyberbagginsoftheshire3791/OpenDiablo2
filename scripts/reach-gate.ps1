# Runs the A2 reachability gate and writes everything to a log.
# Moved into the repo (scripts/) 14 Sep 2026; the repo root is anchored via
# $PSScriptRoot and the log stays out of the tree.
#   powershell -NoProfile -ExecutionPolicy Bypass -File ...\scripts\reach-gate.ps1
param(
  [string]$Log  = 'C:\Users\josht\Projects\strigoi-harness-runs\reach-gate.txt',
  [int]   $Jobs = 4
)

$ErrorActionPreference = 'Continue'
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

"reach-gate start $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss K')  jobs=$Jobs" | Out-File -FilePath $Log -Encoding utf8

$t0 = Get-Date
& 'C:\Program Files\Go\bin\go.exe' run ./tools/reachcheck `
    "-deadcode=C:\Users\josht\go\bin\deadcode.exe" "-jobs=$Jobs" 2>&1 |
  Out-File -FilePath $Log -Encoding utf8 -Append
$code = $LASTEXITCODE

"EXIT=$code" | Out-File -FilePath $Log -Encoding utf8 -Append
"reach-gate done in $([int]((Get-Date) - $t0).TotalSeconds) s" | Out-File -FilePath $Log -Encoding utf8 -Append
