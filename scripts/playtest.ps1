# Runs the playtest scripts (laptop only, never CI -- Article V). Moved into the
# repo (scripts/) 14 Sep 2026; the repo root is anchored via $PSScriptRoot and
# the log stays out of the tree. -Run '.' runs them all.
#   powershell -NoProfile -ExecutionPolicy Bypass -File ...\scripts\playtest.ps1 -Run .
param([string]$Run = "TestNightAndLight", [string]$Log = "C:\Users\josht\Projects\strigoi-harness-runs\playtest.txt")
$repo = Split-Path -Parent $PSScriptRoot
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
Set-Location $repo
& go test -tags playtest ./playtest/... -run $Run -v -count=1 2>&1 | Out-File -Encoding utf8 -Width 400 $Log
"EXIT=$LASTEXITCODE" | Out-File -Encoding utf8 -Append $Log
