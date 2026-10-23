param(
 [Parameter(Mandatory)][string]$MagpieSourceDirectory,
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$GoExe='go',
 [string]$PluginSourceDirectory=$env:WINONE_AI_PLUGIN_SOURCE_DIRECTORY,
 [string]$BunExe=$env:WINONE_AI_BUN_EXE,
 [switch]$Test
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
if(-not $PluginSourceDirectory -or -not $BunExe){throw 'Pass the pinned PluginSourceDirectory and BunExe for the shared account runtime'}
$root=Split-Path -Parent $PSScriptRoot
$source=(Resolve-Path -LiteralPath $MagpieSourceDirectory).Path
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
# The Magpie source is prepared locally and trusted as-is. WinOne no longer
# pins a fixed upstream revision or a fixed dependency graph for it. The
# source's own HEAD is read for per-file extraction, never a hardcoded value.
$revision=(& git -C $source rev-parse HEAD).Trim()
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'build-output.probe')
if($LASTEXITCODE -ne 0){throw 'AI output must be Git-ignored'}
$encoding=New-Object Text.UTF8Encoding($false)
$entry=Join-Path $source 'cmd/winone-ai'
& node (Join-Path $root 'build/verify-ai-source.mjs') $source
if($LASTEXITCODE -ne 0){throw 'AI source/license verification failed'}
New-Item -ItemType Directory -Force -Path $entry,$output | Out-Null
& node (Join-Path $root 'build/prepare-ai-plugins.mjs') $PluginSourceDirectory $BunExe $output
if($LASTEXITCODE -ne 0){throw 'Bundled account plugin preparation failed'}
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/main.go'),(Join-Path $root 'modules/ai-magpie/main_test.go'),(Join-Path $root 'modules/ai-magpie/accounts.go'),(Join-Path $root 'modules/ai-magpie/stream_test.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/stream.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/icons.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/gateway.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/signin_state_test.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/signin_state.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/failures.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/catalog.go'),(Join-Path $root 'modules/ai-magpie/catalog_test.go'),(Join-Path $root 'modules/ai-magpie/plugin-catalog.json') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugins.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/account_snapshot.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_signin.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_desktop_windows.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/models.go'),(Join-Path $root 'modules/ai-magpie/models_test.go') -Destination $entry
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/provider_accounts_test.go') -Destination (Join-Path $source 'internal/provider/winone_accounts_test.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/plugin_profile.go.txt') -Destination (Join-Path $source 'internal/plugin/winone_profile.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/account_snapshot.go.txt') -Destination (Join-Path $source 'internal/provider/winone_account_snapshot.go') -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/plugin_snapshot_test.go') -Destination (Join-Path $source 'internal/plugin/winone_snapshot_test.go') -Force
# Explicit embedding hooks: no discovery of another product's signed-in accounts.
$provider=Join-Path $source 'internal/provider/provider.go'
$text=[IO.File]::ReadAllText($provider)
if(-not $text.Contains('WinOneProviders != nil')){
 $text=$text.Replace('func All() []Provider {',"func All() []Provider {`n if WinOneProviders != nil { return winOneConfiguredProviders() }")
 [IO.File]::WriteAllText($provider,$text,$encoding)
}
$text=[IO.File]::ReadAllText($provider).Replace('return append([]Provider(nil), WinOneProviders...)','return winOneConfiguredProviders()')
[IO.File]::WriteAllText($provider,$text,$encoding)
[IO.File]::WriteAllText((Join-Path $source 'internal/provider/winone_embedded.go'),"package provider`n// Set once before serving a WinOne request; never persisted.`nvar WinOneProviders []Provider`nvar WinOneSearchModels map[string]bool`nfunc WinOneAccountIDs() []string { return append([]string(nil), accountIDs...) }`nfunc winOneConfiguredProviders() []Provider { result := make([]Provider, len(WinOneProviders)); for i, p := range WinOneProviders { result[i] = normalize(p) }; return result }`n",$encoding)
$models=Join-Path $source 'internal/provider/models.go'
$text=(& git -C $source show "${revision}:internal/provider/models.go") -join "`n"
if(-not $text.Contains('ms := p.available()')){throw 'Upstream provider model metadata boundary changed'}
$text=$text.Replace('ms := p.available()', 'ms := p.available(); if len(WinOneSearchModels)>0 { ms=slices.Clone(ms); for _,id:=range p.Models { if WinOneSearchModels[p.ID+"/"+id] && !slices.ContainsFunc(ms,func(m catalog.Model)bool{return m.ID==id}) { ms=append(ms,catalog.Model{ID:id,Name:id}) } }; for i,m:=range ms { if WinOneSearchModels[p.ID+"/"+m.ID] { ms[i].WebSearch="native" } } }')
[IO.File]::WriteAllText($models,$text+"`n",$encoding)
$accounts=Join-Path $source 'internal/provider/account.go'
$text=[IO.File]::ReadAllText($accounts)
if(-not $text.Contains('WinOneProviders != nil')){
 $text=$text.Replace('func Accounts() []Provider {',"func Accounts() []Provider {`n if WinOneProviders != nil { return nil }")
 [IO.File]::WriteAllText($accounts,$text,$encoding)
}
# A new WinOne login must not cancel a different application's Codex login.
# Keep OAuth's registered callback port, but report a busy port immediately.
$signin=Join-Path $source 'internal/provider/signin.go'
$text=(& git -C $source show "${revision}:internal/provider/signin.go") -join "`n"
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned magpie sign-in source'}
$text=[regex]::Replace($text,'(?s)func listenCodexCallback\(\) \(net.Listener, error\) \{.*?\n\}',"func listenCodexCallback() (net.Listener, error) { return net.Listen(`"tcp`", codexCallbackAddr) }")
# A sign-in click never silently installs another desktop client. Existing
# clients are reused; missing CLI dependencies are reported by the flow.
if(-not $text.Contains('install the official client first')){$text=$text.Replace('cli, install := missingCLI(agent)',"cli, install := missingCLI(agent)`n if install { return SignInState{}, fmt.Errorf(`"%s is required; install the official client first`", cli.Name) }")}
[IO.File]::WriteAllText($signin,$text+"`n",$encoding)
# Plugin-backed accounts obey the same no-implicit-client-install rule.
$pluginSignin=Join-Path $source 'internal/provider/pluginsignin.go'
$text=(& git -C $source show "${revision}:internal/provider/pluginsignin.go") -join "`n"
if($LASTEXITCODE -ne 0){throw 'Cannot read pinned plugin sign-in source'}
if(([regex]::Matches($text,'var installing context.Context')).Count -ne 1){throw 'Upstream plugin sign-in preparation changed'}
$text=$text.Replace('"errors"',('"errors"'+"`n`t"+'"fmt"')).Replace('var installing context.Context',"if install { return SignInState{}, fmt.Errorf(`"%s is required; install the official client first`", cli.Name) }`n var installing context.Context")
# Preserve the full upstream authorization state machine. The desktop entry
# registers it asynchronously and gives preparation the login owner's context.
$begin=@'
func (s *signInFlow) pluginBegin(id string, method int, inputs map[string]string) error {
	ctx, cancel := context.WithTimeout(s.via(context.Background()), time.Minute)
	a, err := plugin.Authorize(ctx, id, method, inputs, plugin.NewAccount)
	cancel()
'@
$contextBegin=@'
func (s *signInFlow) pluginBegin(id string, method int, inputs map[string]string) error {
 ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
 defer cancel()
 return s.pluginBeginContext(ctx, id, method, inputs)
}
func (s *signInFlow) pluginBeginContext(ctx context.Context, id string, method int, inputs map[string]string) error {
 a, err := plugin.Authorize(s.via(ctx), id, method, inputs, plugin.NewAccount)
'@
$begin=$begin.Replace("`r`n","`n");$contextBegin=$contextBegin.Replace("`r`n","`n")
if(-not $text.Contains($begin)){throw 'Upstream plugin authorization context changed'}
$text=$text.Replace($begin,$contextBegin).Replace('s.st.State != "waiting" && s.st.State != "installing"','s.st.State != "waiting" && s.st.State != "installing" && s.st.State != "starting"')
[IO.File]::WriteAllText($pluginSignin,$text+"`n",$encoding)
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/patches/plugin_signin.go.txt') -Destination (Join-Path $source 'internal/provider/winone_plugin_signin.go') -Force
& (Join-Path $PSScriptRoot 'prepare-ai-usage.ps1') -SourceDirectory $source -Revision $revision
& (Join-Path $PSScriptRoot 'prepare-ai-settings.ps1') -SourceDirectory $source -Revision $revision
& (Join-Path $PSScriptRoot 'prepare-ai-plugin-registry.ps1') -SourceDirectory $source -Revision $revision
& (Join-Path $PSScriptRoot 'prepare-ai-routing.ps1') -SourceDirectory $source
Push-Location $source
try {
 if($Test){
  # Both caches and persistent metadata stay inside this test's owned output.
  $priorTestEnvironment=@{}
  try {
   $sharedGoCache=& $GoExe env -json GOPATH GOMODCACHE | ConvertFrom-Json
   if($LASTEXITCODE -ne 0){throw 'Cannot locate the shared Go dependency cache'}
   # Tests belong beside the task's candidate bundle, never inside shared/ai:
   # otherwise packaging a tested build would also copy private test profiles.
   $temporaryRoot=Join-Path $root '_temp'
   $taskName=($output.Substring($temporaryRoot.Length).TrimStart([char[]]@('\','/')) -split '[\\/]')[0]
   $testRoot=Join-Path (Join-Path $temporaryRoot $taskName) ('ai-tests-'+[guid]::NewGuid().ToString('N'))
   $testEnvironment=@{
    XDG_CONFIG_HOME=(Join-Path $testRoot 'config');XDG_CACHE_HOME=(Join-Path $testRoot 'cache')
    USERPROFILE=(Join-Path $testRoot 'home');APPDATA=(Join-Path $testRoot 'home/AppData/Roaming');LOCALAPPDATA=(Join-Path $testRoot 'home/AppData/Local')
    CODEX_HOME=(Join-Path $testRoot 'home/.codex');TEMP=(Join-Path $testRoot 'temporary');TMP=(Join-Path $testRoot 'temporary')
    MAGPIE_NO_STATS='1';MAGPIE_OTEL_ENABLED='false'
    GOPATH=$sharedGoCache.GOPATH;GOMODCACHE=$sharedGoCache.GOMODCACHE
   }
   # Account tests must not inherit credentials or app-specific profile paths
   # from the developer's shell. This changes only this build process.
   foreach($entry in [Environment]::GetEnvironmentVariables().GetEnumerator()) {
    if($entry.Key -match '^(CODEX_|CLAUDE_|ANTHROPIC_|OPENAI_|GEMINI_|COPILOT_|CURSOR_|GROK_|KIRO_|DEVIN_|WORKBUDDY_|QODER_|ZED_|FACTORY_|DROID_|MIMO_|ZCODE_|AWS_|AZURE_)|^(GH_TOKEN|GITHUB_TOKEN|GOOGLE_APPLICATION_CREDENTIALS)$') {
     if(-not $testEnvironment.ContainsKey([string]$entry.Key)){$testEnvironment[[string]$entry.Key]=$null}
    }
   }
   New-Item -ItemType Directory -Force -Path $testEnvironment.USERPROFILE,$testEnvironment.TEMP | Out-Null
   foreach($name in $testEnvironment.Keys){$priorTestEnvironment[$name]=[Environment]::GetEnvironmentVariable($name);[Environment]::SetEnvironmentVariable($name,$testEnvironment[$name])}
   $ErrorActionPreference='Continue'
   & $GoExe test ./cmd/winone-ai -count=1
   $ErrorActionPreference='Stop'
   if($LASTEXITCODE -ne 0){throw 'Magpie adapter tests failed'}
   $ErrorActionPreference='Continue'
   & $GoExe test ./internal/provider -run '^TestWinOneOAuth' -count=1
   $ErrorActionPreference='Stop'
   if($LASTEXITCODE -ne 0){throw 'Magpie OAuth adapter test failed'}
   $ErrorActionPreference='Continue'
   & $GoExe test ./internal/plugin -run '^TestWinOne(AccountSnapshot|Extension|PluginCommand)' -count=1 -v
   $ErrorActionPreference='Stop'
   if($LASTEXITCODE -ne 0){throw 'Magpie account snapshot parity test failed'}
  } finally {foreach($name in $priorTestEnvironment.Keys){[Environment]::SetEnvironmentVariable($name,$priorTestEnvironment[$name])}}
 }
 $ErrorActionPreference='Continue'
 & $GoExe build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $output 'WinOne.AI.exe') ./cmd/winone-ai
 $ErrorActionPreference='Stop'
 if($LASTEXITCODE -ne 0){throw 'Magpie adapter build failed'}
 Copy-Item -LiteralPath (Join-Path $source 'LICENSE') -Destination (Join-Path $output 'magpie-LICENSE.txt')
} finally {Pop-Location}
