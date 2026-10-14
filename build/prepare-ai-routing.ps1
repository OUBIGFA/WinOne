param([Parameter(Mandatory)][string]$SourceDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$encoding=[Text.UTF8Encoding]::new($false)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing-host.js') -Destination (Join-Path $SourceDirectory 'internal/gui/assets/winone-routing.js')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai/routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_gui.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gui/winone_routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_routing.go')
$file=Join-Path $SourceDirectory 'internal/provider/winone_embedded.go'
$content=[IO.File]::ReadAllText($file).Replace('normalize(p)','normalize(winOneRoutingProvider(p))')
[IO.File]::WriteAllText($file,$content,$encoding)
