param([Parameter(Mandatory)][string]$SourceDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$encoding=[Text.UTF8Encoding]::new($false)
& node (Join-Path $root 'build/prepare-ai-design.mjs') $SourceDirectory
if($LASTEXITCODE -ne 0){throw 'Cannot prepare the shared WinOne AI design'}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing-host.js') -Destination (Join-Path $SourceDirectory 'internal/gui/assets/winone-routing.js')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing-bridge.js') -Destination (Join-Path $SourceDirectory 'internal/gui/assets/winone-routing-bridge.js')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing.css') -Destination (Join-Path $SourceDirectory 'internal/gui/assets/winone-routing.css')
& node (Join-Path $root 'build/prepare-ai-routing.mjs') $SourceDirectory
if($LASTEXITCODE -ne 0){throw 'Cannot isolate the upstream routing plugin'}
& node (Join-Path $root 'build/prepare-ai-plugin-page.mjs') $SourceDirectory
if($LASTEXITCODE -ne 0){throw 'Cannot isolate the upstream third-party plugin manager'}
foreach($name in @('plugin-bridge.js','plugin-host.js','plugins.css')){
 Copy-Item -LiteralPath (Join-Path $root ('modules/ai-magpie/'+$name)) -Destination (Join-Path $SourceDirectory ('internal/gui/assets/winone-'+$name))
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai/routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_gui.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gui/winone_routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_routing.go')
$file=Join-Path $SourceDirectory 'internal/provider/winone_embedded.go'
$content=[IO.File]::ReadAllText($file).Replace('normalize(p)','normalize(winOneRoutingProvider(p))')
[IO.File]::WriteAllText($file,$content,$encoding)
