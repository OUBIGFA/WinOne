param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$utf8=[Text.UTF8Encoding]::new($false)
# Hooks are rebuilt from the pinned source. Private snapshots avoid persisting
# credentials or global model settings in each short-lived account profile.
function Set-Hook([string]$relative,[hashtable]$replacements) {
 $original=(& git -C $SourceDirectory show ($Revision+':'+$relative)) -join "`n"
 if($LASTEXITCODE -ne 0){throw "Cannot read upstream settings source: $relative"}
 foreach($from in $replacements.Keys){
  if(-not $original.Contains($from)){throw "Upstream settings hook changed: $relative / $from"}
  $original=$original.Replace($from,$replacements[$from])
 }
 [IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$original+"`n",$utf8)
}
Set-Hook 'internal/settings/settings.go' @{'func Load() Settings {'='func Load() Settings { if WinOneSettings != nil { return *WinOneSettings };'}
Set-Hook 'internal/provider/search_api.go' @{
 'func SearchAPIs() []SearchAPI {'='func SearchAPIs() []SearchAPI { if WinOneSearchAPIs != nil { return slices.DeleteFunc(slices.Clone(WinOneSearchAPIs), func(a SearchAPI) bool { return !a.Ready() }) };'
 'func StoredSearchAPIs() []SearchAPI { return load().Searches }'='func StoredSearchAPIs() []SearchAPI { if WinOneSearchAPIs != nil { return slices.Clone(WinOneSearchAPIs) }; return load().Searches }'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/settings/winone_embed.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_settings.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_gateway.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_settings.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/settings.go'),(Join-Path $root 'modules/ai-magpie/settings_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
