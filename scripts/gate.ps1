# The green gate: gofmt, both builds, vet x3, tests, the ebiten-free check on
# the four leaf packages, the Article V fixtures check, and mod tidy.
# Moved into the repo (scripts/) 14 Sep 2026 so the instrument is versioned; the
# repo root is anchored via $PSScriptRoot and the log stays out of the tree.
#   powershell -NoProfile -ExecutionPolicy Bypass -File ...\scripts\gate.ps1
param([string]$Log = "C:\Users\josht\Projects\strigoi-harness-runs\gate.txt")
$repo = Split-Path -Parent $PSScriptRoot
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
Set-Location $repo
"HEAD $(git rev-parse --short HEAD) $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss K')" | Out-File $Log
$fmt = gofmt -l . 2>&1
if ($fmt) { "gofmt: UNFORMATTED $fmt" | Out-File $Log -Append } else { "gofmt: clean" | Out-File $Log -Append }
$o = & go build ./... 2>&1; "build untagged exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go build -tags harness ./... 2>&1; "build harness exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go vet ./... 2>&1; "vet exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go vet -tags harness ./... 2>&1; "vet harness exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go vet -tags "harness playtest" ./... 2>&1; "vet harness+playtest exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go test -count=1 ./... 2>&1
$ok = ($o | Where-Object { $_ -match '^ok' }).Count
$fail = ($o | Where-Object { $_ -match 'FAIL' }).Count
"test exit=$LASTEXITCODE ok=$ok fail=$fail" | Out-File $Log -Append
$o | Where-Object { $_ -match 'FAIL|panic|\.go:' } | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2world 2>&1
"ebiten in d2world deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2input 2>&1
"ebiten in d2input deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2map/d2maprenderer 2>&1
"ebiten in d2maprenderer deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2game/d2player 2>&1
"ebiten in d2player deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$o = & go run ./tools/strigoihook check-fixtures 2>&1; "check-fixtures exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go mod tidy -diff 2>&1; "mod tidy -diff exit=$LASTEXITCODE $o" | Out-File $Log -Append
"DONE" | Out-File $Log -Append
