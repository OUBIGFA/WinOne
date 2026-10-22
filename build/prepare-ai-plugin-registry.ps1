param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$utf8=[Text.UTF8Encoding]::new($false)
$relative='internal/plugin/store.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream plugin registry'}
$replacements=@{
 'func Dir() string { return filepath.Join(settings.Dir(), "plugins") }'='func Dir() string { if d:=WinOneDirectory();d!=""{return filepath.Join(d,"plugins")}; return filepath.Join(settings.Dir(), "plugins") }'
 'func listPath() string { return filepath.Join(settings.Dir(), "plugins.json") }'='func listPath() string { if d:=WinOneDirectory();d!=""{return filepath.Join(d,"plugins.json")}; return filepath.Join(settings.Dir(), "plugins.json") }'
 'return List{Plugins: clonePlugins(l.Plugins), Config: cloneMap(l.Config), Prefer: maps.Clone(l.Prefer)}'='return winOneEffective(List{Plugins: clonePlugins(l.Plugins), Config: cloneMap(l.Config), Prefer: maps.Clone(l.Prefer)})'
 'func save(l List) error {'='func save(l List) error { l=WinOneManaged(l);'
 'os.MkdirAll(settings.Dir(), 0o700)'='os.MkdirAll(filepath.Dir(listPath()), 0o700)'
}
foreach($key in $replacements.Keys){if(-not $source.Contains($key)){throw "Upstream plugin registry changed: $key"};$source=$source.Replace($key,$replacements[$key])}
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
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
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/plugin_registry.go.txt') -Destination (Join-Path $SourceDirectory 'internal/plugin/winone_registry.go')

# Only the outer gateway owns retries and group Patience. Retrying inside a
# transport worker multiplies calls and defeats an inner group's Off choice.
$relative='internal/gateway/fallback.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream gateway retry policy'}
$from='func passing(status int, header http.Header, body []byte, again int) (time.Duration, bool) {'
if(-not $source.Contains($from)){throw 'Upstream gateway retry boundary changed'}
$source=$source.Replace($from,$from+' if WinOneTransportOnly { return 0,false };')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_registry_test.go') -Destination (Join-Path $SourceDirectory 'internal/plugin/winone_registry_test.go')
# Keep opaque-origin plugin scripts confined to their own authenticated path.
# Stripping /session for routing without carrying it into CSP made the static
# page appear, but blocked its bridge and every P5/P6 interaction.
$relative='internal/gui/pluginpage.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream plugin page policy'}
$from='own = r.Host + pluginPagePrefix + tok + "/"'
if(-not $source.Contains($from)){throw 'Upstream plugin page policy changed'}
$source=$source.Replace($from,'own = r.Host + winOnePagePrefix(r) + pluginPagePrefix + tok + "/"')
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)

# Installed rows belong to the user's registry; verified request overlays are
# internal. The directory picker is delegated to WinOne by the page bridge.
$relative='internal/gui/plugins.go'
$source=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join [Environment]::NewLine
if($LASTEXITCODE -ne 0){throw 'Cannot read upstream plugin manager'}
$from='l := plugin.Load()'
if(-not $source.Contains($from)){throw 'Upstream plugin manager state changed'}
$source=$source.Replace($from,'l := plugin.WinOneManaged(plugin.Load())').Replace('Picker: w != nil && !isWeb(w)','Picker: true')
$changed='if op := r.PathValue("op"); op == "add" || op == "update" || op == "upgrade" {'
if(-not $source.Contains($changed)){throw 'Upstream plugin mutation hook changed'}
# The embedded router stays alive across mutations. Invalidate before replying
# so the very next request uses the new middleware, not the one-second cache.
$source=$source.Replace($changed,('middleware.Reload()'+[Environment]::NewLine+$changed))
[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$source+[Environment]::NewLine,$utf8)
