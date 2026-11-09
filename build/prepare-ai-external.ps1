param([Parameter(Mandatory)][string]$SourceDirectory,[Parameter(Mandatory)][string]$Revision)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$encoding=[Text.UTF8Encoding]::new($false)
$files=@{}
foreach($change in @(
 @{Path='internal/agent/gatewaykey.go';From='func keyAt(gw string) string {';To="func keyAt(gw string) string {`n if WinOneExternalKey != `"`" { return WinOneExternalKey }"},
 @{Path='internal/browserext/browserext.go';From='cfg, err := config(gatewayURL, "")';To='cfg, err := config(gatewayURL, WinOneExternalKey)'},
 @{Path='internal/library/mcp_hub.go';From='json.Marshal(map[string]string{"url": u})';To='json.Marshal(map[string]any{"url": u, "headers": map[string]string{"Authorization": "Bearer " + agent.KeyAt(gateway.URL())}})'},
 @{Path='internal/library/mcp_hub.go';From='json.Marshal(map[string]string{"name": HubEntry, "type": "http", "url": u})';To='json.Marshal(map[string]any{"name": HubEntry, "type": "http", "url": u, "headers": map[string]string{"Authorization": "Bearer " + agent.KeyAt(gateway.URL())}})'},
 @{Path='internal/settings/otel.go';From='func OTelExport() (OTel, error) {';To="func OTelExport() (OTel, error) {`n if config := winOneExternalOTel.Load(); config != nil { return *config, config.Check() }"},
 @{Path='internal/gui/browser.go';From='resp, err := http.DefaultClient.Do(req)';To="if key := browserext.WinOneExternalKey; key != `"`" { req.Header.Set(`"Authorization`", `"Bearer `" + key) }`n resp, err := http.DefaultClient.Do(req)"},
 @{Path='internal/gui/browser.go';From="`t`t`tbrowserext.Write(dir, gateway.URL())";To="`t`t`tif !browserext.WinOneExternal { browserext.Write(dir, gateway.URL()) }"},
 @{Path='internal/browsermcp/browsermcp.go';From='"Bearer "+gateway.Token';To='"Bearer "+winOneMCPKey()'},
 @{Path='internal/imagemcp/imagemcp.go';From='"Bearer "+gateway.Token';To='"Bearer "+winOneMCPKey()'},
 @{Path='internal/library/market.go';From="`tif exe, err := selfExe(); err == nil {";To="`tdefer winOneExternalSelfCommand(s)`n`tif exe, err := selfExe(); err == nil {"},
 @{Path='internal/library/market.go';From='func InstallServer(id string, values map[string]string, agents []string) (*Result, error) {';To="func InstallServer(id string, values map[string]string, agents []string) (*Result, error) {`n if err:=winOneExternalCanInstall(id);err!=nil{return nil,err}"},
 @{Path='internal/library/self_here.go';From='return base == magpieCommand || strings.HasPrefix(base, magpieCommand+"-")';To='return base == magpieCommand || base == "winone.ai" || strings.HasPrefix(base, magpieCommand+"-")'},
 @{Path='internal/library/self_here.go';From='if s == nil || !ownServer(s) || runsHere(s.Command) {';To="if s != nil && ownServer(s) { winOneExternalSelfCommand(s) }`n if s == nil || !ownServer(s) || runsHere(s.Command) {"},
 @{Path='internal/gui/sessions_manage.go';From='Terminal: runtime.GOOS == "darwin" && !isWeb(w)';To='Terminal: terminalHere() && !isWeb(w)'},
 @{Path='internal/plugin/tools.go';From='func toolsPath() string { return filepath.Join(settings.Dir(), "plugin-tools.json") }';To='func toolsPath() string { if d:=WinOneDirectory();d!=""{return filepath.Join(d,"plugin-tools.json")}; return filepath.Join(settings.Dir(), "plugin-tools.json") }'},
 @{Path='internal/library/plugin_tools.go';From='plugin.ToolsNoted = func() {';To='plugin.ToolsNoted = func() { if plugin.WinOneDirectory()!=""{return}'},
 @{Path='internal/library/plugin_tools.go';From='func PluginServers() map[string]PluginServer {';To='func PluginServers() map[string]PluginServer { if servers,external:=winOneExternalPluginServers();external{return servers}'},
 @{Path='internal/library/sync.go';From="go func() {`n`t`t`tif _, err := Sync(); err != nil {";To="go func() {`n if os.Getenv(`"WINONE_AI_EXTERNAL_DATA`")!=`"`"{return}`n`t`t`tif _, err := Sync(); err != nil {"}
)){
 if($files.ContainsKey($change.Path)){$text=$files[$change.Path]}else{$text=(& git -C $SourceDirectory show ($Revision+':'+$change.Path))-join "`n";if($LASTEXITCODE-ne 0){throw 'Cannot read external-client upstream adapter source'}}
 if(-not $text.Contains($change.From)){throw ('External-client adapter anchor changed: '+$change.Path)}
 $files[$change.Path]=$text.Replace($change.From,$change.To)
}
foreach($relative in $files.Keys){[IO.File]::WriteAllText((Join-Path $SourceDirectory $relative),$files[$relative]+"`n",$encoding)}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_agent.go.txt') -Destination (Join-Path $SourceDirectory 'internal/agent/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_browser.go.txt') -Destination (Join-Path $SourceDirectory 'internal/browserext/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_settings.go.txt') -Destination (Join-Path $SourceDirectory 'internal/settings/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_gui.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gui/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_appdir.go.txt') -Destination (Join-Path $SourceDirectory 'internal/appdir/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_mcp_key.go.txt') -Destination (Join-Path $SourceDirectory 'internal/browsermcp/winone_external.go') -Force
$keySource=[IO.File]::ReadAllText((Join-Path $root 'modules/ai-magpie/patches/external_mcp_key.go.txt')).Replace('package browsermcp','package imagemcp')
[IO.File]::WriteAllText((Join-Path $SourceDirectory 'internal/imagemcp/winone_external.go'),$keySource,$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_library.go.txt') -Destination (Join-Path $SourceDirectory 'internal/library/winone_external.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/external_gateway.go.txt') -Destination (Join-Path $SourceDirectory 'internal/gateway/winone_external.go') -Force
