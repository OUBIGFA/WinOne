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
Set-Hook 'internal/settings/settings.go' @{
 'func Shared() Settings {'='func Shared() Settings { if s,ok:=WinOneLoad();ok{return s};'
 'func Load() Settings {'='func Load() Settings { if s,ok:=WinOneLoad();ok{return s.clone()};'
 'func LoadChecked() (Settings, error) {'='func LoadChecked() (Settings, error) { if s,ok:=WinOneLoad();ok{return s.clone(),nil};'
 'func SetRedactModel(on bool) error {'='func SetRedactModel(on bool) error { if _,ok:=WinOneLoad();ok{return winOneSetRedactModel(on)};'
}
# JS cannot round-trip UnixNano record IDs. Stringify only public DTOs;
# the upstream on-disk int64 format and older history remain compatible.
Set-Hook 'internal/gui/masking.go' @{
 'type maskedRecord struct {'='type maskedRecord struct { Key int64 `json:"key,string"`;'
 'out := maskedRecord{Record: r}'='out := maskedRecord{Record: r, Key: r.Key}'
}
Set-Hook 'internal/gateway/masked.go' @{
 '`json:"key,omitempty"`'='`json:"key,omitempty,string"`'
}
Set-Hook 'internal/gateway/claude_room.go' @{
 'if dir == "/" {'='if filepath.Dir(dir) == dir {'
}
Set-Hook 'internal/pluginrt/config.go' @{
 'func Load() (File, bool, error) {'='func Load() (File, bool, error) { if f,ok:=WinOneLoad();ok{return f,true,nil};'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/plugins_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/pluginrt/winone_embed.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/builtin_plugins.go'),(Join-Path $root 'modules/ai-magpie/bundle_selection.go'),(Join-Path $root 'modules/ai-magpie/builtin_plugins_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
Set-Hook 'internal/provider/search_api.go' @{
 'func SearchAPIs() []SearchAPI {'='func SearchAPIs() []SearchAPI { if WinOneSearchAPIs != nil { return slices.DeleteFunc(slices.Clone(WinOneSearchAPIs), func(a SearchAPI) bool { return !a.Ready() }) };'
 'func StoredSearchAPIs() []SearchAPI { return load().Searches }'='func StoredSearchAPIs() []SearchAPI { if WinOneSearchAPIs != nil { return slices.Clone(WinOneSearchAPIs) }; return load().Searches }'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/settings/winone_embed.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_provider.go.txt') -Destination (Join-Path $SourceDirectory 'internal/provider/winone_settings.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_gateway.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_settings.go')
Set-Hook 'internal/pii/find.go' @{
 'func Find(texts []string) ([][]redact.Found, error) {'='func Find(texts []string) ([][]redact.Found, error) { if remote := winOneRemoteFind.Load(); remote != nil { return (*remote)(texts) };'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/privacy_pii.go.txt') -Destination (Join-Path $SourceDirectory 'internal/pii/winone_private.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/privacy_gui.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gui/winone_privacy.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/settings.go'),(Join-Path $root 'modules/ai-magpie/settings_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
