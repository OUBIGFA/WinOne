param([Parameter(Mandatory)][string]$OutputDirectory,[string]$GoExe='go',[switch]$SkipTests,[switch]$RunWindowChecks,[string]$LearningResourceDirectory='dist/resources/assets/learning')
# MyGo v0.8.3 with the scoped Windows Shell file-drop adaptation. Selection,
# pointer release and tool-window behavior still use its public APIs.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if(-not(Test-Path -LiteralPath (Join-Path $root 'apps/result-ui/assets/provider-bitmaps/manifest.json'))){throw 'Run build/build-provider-icons.ps1 before building the shared result UI'}
$go=(Get-Command $GoExe -ErrorAction Stop).Source
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'WinOne.Results.exe')
if($LASTEXITCODE -ne 0){throw 'Result UI output must be ignored'}
& node (Join-Path $PSScriptRoot 'generate-result-icons.mjs')
if($LASTEXITCODE -ne 0){throw 'Result icon generation failed'}
$saved=@{GOROOT=$env:GOROOT;CGO_ENABLED=$env:CGO_ENABLED;GOTOOLCHAIN=$env:GOTOOLCHAIN;GOCACHE=$env:GOCACHE}
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go);$env:CGO_ENABLED='0';$env:GOTOOLCHAIN='local'
 & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
 if(-not $env:GOCACHE){$env:GOCACHE=Join-Path $root 'dist/resources/cache/go'}
 Push-Location (Join-Path $root 'apps/result-ui')
 try {
  & (Join-Path $PSScriptRoot 'verify-result-notices.ps1') -GoExe $go -SourceOnly
  if(-not $SkipTests){
   & $go test ./... -count=1;if($LASTEXITCODE -ne 0){throw 'Result UI tests failed'}
  }
  & $go vet ./...;if($LASTEXITCODE -ne 0){throw 'Result UI vet failed'}
  & $go vet -tags mygo_noinspector ./...;if($LASTEXITCODE -ne 0){throw 'Result UI production vet failed'}
  & $go run github.com/akavel/rsrc@v0.10.2 -manifest (Join-Path $root 'build/windows/app.manifest') -ico (Join-Path $root 'internal/branding/icon.ico') -o cmd/winone-results/resource_windows.syso
  if($LASTEXITCODE -ne 0){throw 'Result UI resource generation failed'}
  New-Item -ItemType Directory -Path $output -Force|Out-Null
  $exe=Join-Path $output 'WinOne.Results.exe'
  & $go build -trimpath -tags mygo_noinspector -ldflags '-s -w -H windowsgui -X github.com/egoist/mygo.production=1' -o $exe ./cmd/winone-results
  if($LASTEXITCODE -ne 0){throw 'Result UI build failed'}
  & (Join-Path $PSScriptRoot 'copy-learning.ps1') -OutputDirectory (Join-Path $output 'learning') -ResourceDirectory $LearningResourceDirectory
  & (Join-Path $PSScriptRoot 'verify-result-notices.ps1') -GoExe $go -Executable $exe
  if($RunWindowChecks){
   $priorProbe=$env:WINONE_RESULT_PROBE_EXE;$priorDirectory=$env:WINONE_RESULT_PROBE_DIR
   try{
    $env:WINONE_RESULT_PROBE_EXE=$exe
    $env:WINONE_RESULT_PROBE_DIR=Join-Path (Split-Path -Parent $output) 'native-probe'
    & $go test -run '^TestNativeResult' -count=1 -v .
    if($LASTEXITCODE -ne 0){throw 'Result UI native window checks failed'}
   }finally{$env:WINONE_RESULT_PROBE_EXE=$priorProbe;$env:WINONE_RESULT_PROBE_DIR=$priorDirectory}
  }
 }finally{Pop-Location}
}finally{foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}}
