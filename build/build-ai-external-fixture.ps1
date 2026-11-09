param(
 [Parameter(Mandatory)][string]$SourceDirectory,
 [string]$OutputDirectory='_temp/ai-upstream-20261109/external-resume/worker-fixture',
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe'
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=(Resolve-Path -LiteralPath $SourceDirectory).Path
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$utf8=[Text.UTF8Encoding]::new($false)
New-Item -ItemType Directory -Force -Path $output|Out-Null
# Build the real production entry against a prepared upstream module, without
# mutating that shared source or rerunning plugin preparation beside build-ai.
Copy-Item -Path (Join-Path $source 'cmd/winone-ai/*') -Destination $output -Recurse -Force
Copy-Item -LiteralPath (Join-Path $root 'modules/ai-magpie/main.go'),(Join-Path $root 'modules/ai-magpie/external.go'),(Join-Path $root 'modules/ai-magpie/external_service.go'),(Join-Path $root 'modules/ai-magpie/external_mcp.go') -Destination $output -Force
$replace=$source.Replace('\','/')
[IO.File]::WriteAllText((Join-Path $output 'go.mod'),"module github.com/yetone/magpie/winone-external-fixture`n`ngo 1.27.3`nrequire github.com/yetone/magpie v0.0.0`nreplace github.com/yetone/magpie => `"$replace`"`n",$utf8)
Copy-Item -LiteralPath (Join-Path $source 'go.sum') -Destination (Join-Path $output 'go.sum') -Force
$priorRoot=$env:GOROOT;$priorCache=$env:GOCACHE
try{
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 Push-Location $output
 try{& $go build -mod=mod -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $output 'WinOne.AI.exe') .;if($LASTEXITCODE-ne 0){throw 'External fixture worker build failed'}}finally{Pop-Location}
}finally{$env:GOROOT=$priorRoot;$env:GOCACHE=$priorCache}
Write-Output (Join-Path $output 'WinOne.AI.exe')
