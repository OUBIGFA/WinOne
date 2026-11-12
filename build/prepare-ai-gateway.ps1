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
$load='if gw := served.Load(); gw != nil {'
if(-not $handler.Contains($load)){throw 'Upstream recent-call owner changed'}
$handler=$handler.Replace($load,'if gw != nil {')
$generated="// Generated from Magpie's current providers.go; MIT. Do not edit.`npackage main`nimport (`"net/http`"; `"strconv`"; `"github.com/yetone/magpie/internal/gateway`")`nfunc winOneCallRoutes(mux *http.ServeMux, gw *gateway.Server) {`n$handler`n}`n"
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'cmd/winone-ai/gateway_call.go'),$generated,$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/gateway_backend.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/gateway_admin.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai/gateway_admin.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/gateway_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_gateway.go')
$contract=[IO.File]::ReadAllText((Join-Path $root 'internal/ai/gateway_paths.go')).Replace('package ai','package main')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'cmd/winone-ai/gateway_paths.go'),$contract,$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/claude_mode_test.go') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_claude_mode_test.go')
$file=Join-Path $SourceDirectory 'internal/provider/winone_embedded.go'
$content=[IO.File]::ReadAllText($file).Replace('normalize(p)','normalize(winOneGatewayProvider(p))')
[IO.File]::WriteAllText($file,$content,$encoding)
# Keep the upstream fingerprint algorithm for real keys. The outer router's
# private transport keys use the Host-provided fingerprint of the selected key.
$revision=(& git -C $SourceDirectory rev-parse HEAD).Trim()
$keySource=(& git -C $SourceDirectory show ($revision+':internal/provider/keys.go')) -join "`n"
$hook='func keyID(key string) string {'
if(-not $keySource.Contains($hook)){throw 'Upstream key identity boundary changed'}
$keySource=$keySource.Replace($hook,$hook+' if id,ok:=winOneKeyIdentity(key);ok{return id}; ')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/provider/keys.go'),$keySource+"`n",$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/claude_mode.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_claude_mode.go')
$claudeSource=(& git -C $SourceDirectory show ($revision+':internal/gateway/claude_subscription.go')) -join "`n"
$hook='claudeLast.run = ClaudeLastRun{TUI: why == "", Why: why, At: time.Now()}'
if(-not $claudeSource.Contains($hook)){throw 'Upstream Claude run observation boundary changed'}
$claudeSource=$claudeSource.Replace($hook,$hook+"`n`twinOneRememberClaudeRun(claudeLast.run)")
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/gateway/claude_subscription.go'),$claudeSource+"`n",$encoding)
