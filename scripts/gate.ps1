# The green gate: gofmt, both builds, vet x3, tests, the ebiten-free check on
# five leaf packages, the headless-safety check on every package that has tests
# (BUG-13), the Article V fixtures check, and mod tidy.
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
# The harness's own unit tests (d2app, 23 Sep 2026): tagged `harness`, so the
# untagged run above -- and CI's -- never builds them (d2app links ebiten's UI;
# see BUG-13 below). This machine has a screen, so they run here.
$o = & go test -count=1 -tags harness ./d2app 2>&1
"test harness exit=$LASTEXITCODE $(($o | Where-Object { $_ -match '^ok|FAIL|panic' }) -join ' ')" | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2world 2>&1
"ebiten in d2world deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2input 2>&1
"ebiten in d2input deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2core/d2map/d2maprenderer 2>&1
"ebiten in d2maprenderer deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
$deps = & go list -deps ./d2game/d2player 2>&1
"ebiten in d2player deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
# d2logfile, added 19 Sep 2026: it exists BECAUSE of this property. It lived in
# d2app, which imports ebiten, and the moment d2app gained its first test file
# `go test ./...` ran a binary there and ebiten's init panicked on the headless
# CI runner -- while this gate passed on a Windows machine with a display. A
# package whose tests must run anywhere may not reach ebiten.
$deps = & go list -deps ./d2common/d2logfile 2>&1
"ebiten in d2logfile deps: $(($deps | Where-Object { $_ -match 'hajimehoshi/ebiten' }).Count)" | Out-File $Log -Append
# BUG-13's GENERAL CASE, closed 20 Sep 2026. A test file in a package that links
# ebiten's UI panics on a headless runner ("glfw: X11: The DISPLAY environment
# variable is missing") before it asserts anything -- and this gate runs on a
# machine WITH a screen, so it cannot see that by testing. It can see it by
# reading the import graph, which is what this does. The five fixed checks above
# pin architectural intent about particular leaf packages; this one pins
# TESTABILITY across every package, including ones that do not exist yet.
$tested = & go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... 2>$null |
  Where-Object { $_ }
$unsafe = @()
foreach ($pkg in $tested) {
  $deps = & go list -deps -test $pkg 2>$null
  if ($deps -contains 'github.com/hajimehoshi/ebiten/v2/internal/ui') { $unsafe += $pkg }
}
if (-not $tested) {
  # THE SILENT ZERO. Both go list calls end in 2>$null, so a broken package or a
  # toolchain fault empties $tested and the ok branch below prints a line that is
  # structurally identical to a pass -- "0 tested package(s), 0 link ...". An
  # enumeration that found nothing has not checked anything.
  "headless-safe tests: RED -- go list returned no tested packages; the check did not run" |
    Out-File $Log -Append
} elseif ($unsafe.Count -gt 0) {
  "headless-safe tests: RED -- has tests AND links ebiten/internal/ui: $($unsafe -join ', ')" |
    Out-File $Log -Append
} else {
  "headless-safe tests: ok ($($tested.Count) tested package(s), 0 link ebiten/internal/ui)" |
    Out-File $Log -Append
}
$o = & go run ./tools/strigoihook check-fixtures 2>&1; "check-fixtures exit=$LASTEXITCODE $o" | Out-File $Log -Append
$o = & go mod tidy -diff 2>&1; "mod tidy -diff exit=$LASTEXITCODE $o" | Out-File $Log -Append
"DONE" | Out-File $Log -Append
