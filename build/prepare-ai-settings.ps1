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
 'b, err := json.MarshalIndent(s, "", "  ")'='if _,ok:=WinOneLoad();ok{s.GitHubToken="";s.OTel.Headers=nil}; b, err := json.MarshalIndent(s, "", "  ")'
 'func SetGatewayConversations(on bool) error {'='func SetGatewayConversations(on bool) error { if _,ok:=WinOneLoad();ok{return winOneSetGatewayConversations(on)};'
 'func Shared() Settings {'='func Shared() Settings { if s,ok:=WinOneLoad();ok{return s};'
 'func SharedModelIDs() map[string]string {'='func SharedModelIDs() map[string]string { if s,ok:=WinOneLoad();ok{return s.ModelIDs};'
 'func Load() Settings {'='func Load() Settings { if s,ok:=WinOneLoad();ok{return s.clone()};'
 'func LoadChecked() (Settings, error) {'='func LoadChecked() (Settings, error) { if s,ok:=WinOneLoad();ok{return s.clone(),nil};'
}
# Upstream now walks cgroups with path.Clean/path.Dir and terminates at "/".
# The former WinOne filepath.Dir patch would reintroduce an endless Windows walk.
$room=(& git -C $SourceDirectory show ($Revision+':internal/gateway/claude_room.go')) -join "`n"
if($LASTEXITCODE -ne 0 -or -not $room.Contains('dir = path.Dir(dir)')){throw 'Review upstream cgroup traversal'}
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/gateway/claude_room.go'),$room+"`n",$utf8)
Set-Hook 'internal/pluginrt/config.go' @{
 'func Load() (File, bool, error) {'='func Load() (File, bool, error) { if f,ok:=WinOneLoad();ok{return f,true,nil};'
}
Set-Hook 'internal/gateway/decisions.go' @{
 'func canDecide(p provider.Provider) bool {'='func canDecide(p provider.Provider) bool { if supported,ok:=WinOneDecisionProviders[p.ID];ok{return supported};'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/plugins_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/pluginrt/winone_embed.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_embed.go.txt') -Destination (Join-Path $SourceDirectory 'internal/settings/winone_embed.go')
Set-Hook 'internal/access/access.go' @{
 '"encoding/json"'=''
 'err = json.Unmarshal(b, &keys)'='err = winOneDecodeKeys(b, &keys)'
 'b, err := json.MarshalIndent(keys, "", "  ")'='b, err := winOneEncodeKeys(keys)'
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/access_store_windows.go.txt') -Destination (Join-Path $SourceDirectory 'internal/access/winone_store_windows.go')
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/settings_gateway.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_settings.go')
Set-Hook 'internal/gateway/native.go' @{
 'p := *found'='p, nativeOK := winOneNativeProvider(*found); if !nativeOK {fail(http.StatusForbidden,"Native APIs require an API-key provider");return}'
}
# WinOne's enabled gateway models apply to helper calls too. Keep upstream
# resolution, group checks and errors; only mark the Host-authenticated path.
Set-Hook 'internal/gateway/key_models.go' @{
 'if chose, _ := r.Context().Value(magpieChoseKey{}).(bool); chose {'='if chose, _ := r.Context().Value(magpieChoseKey{}).(bool); chose && !winOneMediaRestricted(r) {'
}
Set-Hook 'internal/gateway/gateway.go' @{
 'if chose {'='if chose && !winOneMediaRestricted(r) {'
}
foreach($media in @('draw','video')){
 Set-Hook ("internal/gateway/$media.go") @{
  'if keyHeld && named &&'='if keyHeld && (named || winOneMediaRestricted(r)) &&'
 }
}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/settings.go'),(Join-Path $root 'modules/ai-magpie/settings_test.go') -Destination (Join-Path $SourceDirectory 'cmd/winone-ai')
