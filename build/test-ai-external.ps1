param(
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
 [string]$AIExe,
 [string]$SourceDirectory,
 [switch]$FullAdapter,
 [string]$OutputDirectory='_temp/ai-upstream-20261109/external-resume',
 [string]$Run='TestExternal'
)
# External-client tests only use generated folders under the task's TEMP.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$previous=@{}
$cache=& $go env -json GOPATH GOMODCACHE | ConvertFrom-Json
$environment=@{
 GOROOT=(Split-Path -Parent (Split-Path -Parent $go));GOCACHE=(Join-Path $root 'dist/resources/cache/go')
 GOPATH=$cache.GOPATH;GOMODCACHE=$cache.GOMODCACHE
 USERPROFILE=(Join-Path $output 'home');HOME=(Join-Path $output 'home')
 APPDATA=(Join-Path $output 'home/roaming');LOCALAPPDATA=(Join-Path $output 'home/local')
 XDG_CONFIG_HOME=(Join-Path $output 'config');XDG_CACHE_HOME=(Join-Path $output 'cache')
 XDG_DATA_HOME=(Join-Path $output 'data');XDG_STATE_HOME=(Join-Path $output 'state')
 TEMP=(Join-Path $output 'temporary');TMP=(Join-Path $output 'temporary')
 MAGPIE_NO_STATS='1';WINONE_AI_TEST_EXE=$null
}
# No inherited application location or credential may escape the fixture home.
foreach($item in [Environment]::GetEnvironmentVariables().GetEnumerator()){
 if($item.Key -match '^(CODEX_|CLAUDE_|ANTHROPIC_|OPENAI_|GEMINI_|COPILOT_|CURSOR_|GROK_|KIRO_|DEVIN_|WORKBUDDY_|QODER_|ZED_|FACTORY_|DROID_|MIMO_|ZCODE_|AWS_|AZURE_|PI_|MAGPIE_)|^(GH_TOKEN|GITHUB_TOKEN|GOOGLE_APPLICATION_CREDENTIALS)$'){
  if(-not $environment.ContainsKey([string]$item.Key)){$environment[[string]$item.Key]=$null}
 }
}
# Use the adapter's complete location contract, including less common clients
# whose environment variables do not share a vendor prefix.
$pathContract=[IO.File]::ReadAllText((Join-Path $root 'internal/ai/external_windows.go'))
$pathContract=$pathContract.Substring($pathContract.IndexOf('var externalClientPathVars'))
foreach($match in [regex]::Matches($pathContract,'"([A-Z][A-Z0-9_]*)"')){
 $name=$match.Groups[1].Value
 if(-not $environment.ContainsKey($name)){$environment[$name]=$null}
}
try{
 foreach($name in $environment.Keys){$previous[$name]=[Environment]::GetEnvironmentVariable($name);[Environment]::SetEnvironmentVariable($name,$environment[$name])}
 New-Item -ItemType Directory -Force -Path $environment.USERPROFILE,$environment.APPDATA,$environment.LOCALAPPDATA,$environment.XDG_CONFIG_HOME,$environment.XDG_CACHE_HOME,$environment.TEMP|Out-Null
 if($AIExe){$env:WINONE_AI_TEST_EXE=(Resolve-Path -LiteralPath $AIExe).Path}
 Push-Location $root
 try{& $go test ./internal/ai -run $Run -count=1 -timeout 180s;if($LASTEXITCODE-ne 0){throw 'External client behavior tests failed'}}finally{Pop-Location}
 Push-Location $root
 try{& $go test ./apps/host -run $Run -count=1 -timeout 180s;if($LASTEXITCODE-ne 0){throw 'External Host behavior tests failed'}}finally{Pop-Location}
 if($SourceDirectory){
  # Import the prepared upstream without modifying its source or competing
  # with build-ai. This module owns only the external adapter under test.
  $source=(Resolve-Path -LiteralPath $SourceDirectory).Path.Replace('\','/')
  $harness=Join-Path $output $(if($FullAdapter){'full-adapter-tests'}else{'adapter-tests'})
  New-Item -ItemType Directory -Force -Path $harness|Out-Null
  $utf8=[Text.UTF8Encoding]::new($false)
  $mod="module github.com/yetone/magpie/winone-external-tests`n`ngo 1.27.3`n`nrequire github.com/yetone/magpie v0.0.0`nreplace github.com/yetone/magpie => `"$source`"`n"
  [IO.File]::WriteAllText((Join-Path $harness 'go.mod'),$mod,$utf8)
  Copy-Item -LiteralPath (Join-Path $SourceDirectory 'go.sum') -Destination (Join-Path $harness 'go.sum') -Force
  if($FullAdapter){Copy-Item -Path (Join-Path $SourceDirectory 'cmd/winone-ai/*') -Destination $harness -Recurse -Force;Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/main.go') -Destination $harness -Force}
  Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/external.go'),(Join-Path $root 'modules/ai-magpie/external_service.go'),(Join-Path $root 'modules/ai-magpie/external_test.go'),(Join-Path $root 'modules/ai-magpie/external_mcp.go'),(Join-Path $root 'modules/ai-magpie/external_mcp_test.go') -Destination $harness -Force
  if(-not $FullAdapter){[IO.File]::WriteAllText((Join-Path $harness 'request_test.go'),"package main`ntype request struct{Key string;ExternalGateway *externalGateway}`n// Runtime packaging is checked by the actual worker tests, not this handler harness.`nfunc prepareThirdPartyRuntime(string)error{return nil}`n",$utf8)}
  Push-Location $harness
  try{& $go test -mod=mod . -run $Run -count=1 -timeout 180s -v;if($LASTEXITCODE-ne 0){throw 'External upstream adapter behavior tests failed'}}finally{Pop-Location}
 }
}finally{foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name])}}
