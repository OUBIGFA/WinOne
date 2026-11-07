param([Parameter(Mandatory)][string]$SourceDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$encoding=[Text.UTF8Encoding]::new($false)
# Reuse the current upstream read handler whole: route IDs and masked headers
# must match its trace, while unrelated provider mutations stay unregistered.
$providers=[IO.File]::ReadAllText((Join-Path $SourceDirectory 'internal/gui/providers.go')).Replace("`r`n","`n")
$start=$providers.IndexOf('mux.HandleFunc("GET /api/gateway/call",')
$end=$providers.IndexOf("`n`t// the order the Providers tab lists them in",$start)
if($start -lt 0 -or $end -le $start){throw 'Upstream recent-call handler extraction changed'}
$handler=$providers.Substring($start,$end-$start)
$generated="// Generated from Magpie's current providers.go; MIT. Do not edit.`npackage gui`nimport (`"net/http`"; `"strconv`"; `"github.com/yetone/magpie/internal/gateway`")`nfunc winOneCallRoutes(mux *http.ServeMux) {`n$handler`n}`n"
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/gui/winone_call.go'),$generated,$encoding)
# Vite writes timing warnings to stderr even on success. Windows PowerShell
# must judge the native exit code, not turn a diagnostic into a build failure.
$ErrorActionPreference='Continue'
& node (Join-Path $root 'build/prepare-ai-design.mjs') $SourceDirectory
$ErrorActionPreference='Stop'
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
foreach($name in @('masking-host.js','masking-prehost.js','masking.css')){
 Copy-Item -LiteralPath (Join-Path $root ('modules/ai-magpie/'+$name)) -Destination (Join-Path $SourceDirectory ('internal/gui/assets/winone-'+$name))
}
& node (Join-Path $root 'build/prepare-ai-masking.mjs') $SourceDirectory
if($LASTEXITCODE -ne 0){throw 'Cannot isolate the upstream privacy records'}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/routing.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai/routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_gui.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gui/winone_routing.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/routing_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_routing.go')
$file=Join-Path $SourceDirectory 'internal/provider/winone_embedded.go'
$content=[IO.File]::ReadAllText($file).Replace('normalize(p)','normalize(winOneRoutingProvider(p))')
[IO.File]::WriteAllText($file,$content,$encoding)
