# Dumps the tracker HTML through headless Chrome and checks it rendered.
# Moved into the repo (scripts/) 14 Sep 2026. It touches no repo files, so it
# needs no repo-root anchor; the tracker, the DOM dump and check-render.js stay
# in their existing absolute locations.
$ErrorActionPreference = 'Stop'
$chrome = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
$tracker = 'C:\Users\josht\Desktop\Video Game\Claude doc outputs\strigoi-tracker.html'
$dom = 'C:\Users\josht\Projects\strigoi-harness-runs\tracker-dom.html'
$uri = ([System.Uri]$tracker).AbsoluteUri
Write-Output "dumping $uri"
& $chrome --headless=new --disable-gpu --virtual-time-budget=5000 --dump-dom $uri 2>$null | Out-File -FilePath $dom -Encoding utf8
Write-Output ("dom bytes = " + (Get-Item $dom).Length)
node 'C:\Users\josht\Projects\strigoi-harness-runs\check-render.js' $dom
