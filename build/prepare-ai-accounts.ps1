param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$utf8=[Text.UTF8Encoding]::new($false)
# Normalize the shared plugin subprocess boundary, including browser downloads
# and native login helpers. Do not rewrite individual third-party plugins.
$relative='internal/plugin/bun.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream plugin command environment'}
$from='netproxy.EnvForBun(e)'
if(-not $source.Contains($from)){throw 'Upstream plugin command environment changed'}
# Apply the Windows identity after upstream selects the request/download
# environment, retaining its distinct proxy and verified install cache.
$source=$source.Replace($from,'netproxy.EnvForBun(winOnePluginEnvironment(e))')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_environment_test.go') -Destination (Join-Path $SourceDirectory 'internal/plugin/winone_environment_test.go')
# All provider readers, including Cached, see exactly one verified adapter.
$relative='internal/plugin/store.go'
$text=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join "`n"
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream account registry'}
$pattern='(?ms)^func list\(\) List \{.*?^\}'
if(([regex]::Matches($text,$pattern)).Count -ne 1){throw 'Upstream account registry boundary changed'}
$text=[regex]::Replace($text,$pattern,'func list() List { return winOneAccountList() }')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$text+"`n",$utf8)
# A private transport worker is behind an already processed gateway request.
# Preserve upstream serving and provider hooks, without running user-request
# hooks/middleware twice across WinOne's authenticated provider bridge.
$relative='internal/gateway/middleware.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream gateway middleware'}
$from='func (s *Server) serveAgent(w http.ResponseWriter, r *http.Request, from provider.Protocol, body []byte) {'
if(-not $source.Contains($from)){throw 'Upstream gateway middleware boundary changed'}
$source=$source.Replace($from,$from+' if WinOneTransportOnly { s.serve(w,r,from,body); return };')
$from='to := gwmw.To{ID: p.ID, Name: p.Name, Account: accountAgent(p), Model: model}'
if(-not $source.Contains($from)){throw 'Upstream provider middleware context changed'}
$source=$source.Replace($from,$from+'; if kind:=WinOneMiddlewareAccounts[p.ID];kind!=""{to.Account=kind}')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/account_registry.go.txt') -Destination (Join-Path $SourceDirectory 'internal/plugin/winone_registry.go')

# Only the outer gateway owns retries and group Patience. Retrying inside a
# transport worker multiplies calls and defeats an inner group's Off choice.
$relative='internal/gateway/fallback.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream gateway retry policy'}
$from='func passing(status int, header http.Header, body []byte, again int) (time.Duration, bool) {'
if(-not $source.Contains($from)){throw 'Upstream gateway retry boundary changed'}
$source=$source.Replace($from,$from+' if WinOneTransportOnly { return 0,false };')
$from='func restOf(id string) (Rest, bool) {'
if(-not $source.Contains($from)){throw 'Upstream persisted rest lookup boundary changed'}
# The outer router owns rests as well as retries. Ignore prior transport
# rests so an upgraded worker cannot replay a stale refusal on a new request.
$source=$source.Replace($from,$from+' if WinOneTransportOnly { return Rest{},false };')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
$relative='internal/gateway/routing.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream rest writer'}
$from='func (s *Server) restAfterMarked(c candidate, status int, header http.Header, body []byte, sharedPool bool) Rest {'
if(-not $source.Contains($from)){throw 'Upstream rest writer boundary changed'}
$source=$source.Replace($from,$from+' if WinOneTransportOnly { return Rest{Why:failureOf(c,status,body),Status:status,By:"off",Until:time.Now()} };')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_registry_test.go') -Destination (Join-Path $SourceDirectory 'internal/plugin/winone_registry_test.go')
