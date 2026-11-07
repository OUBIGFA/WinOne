param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$GoExe='go',
 [string[]]$Packages=@('./internal/configuration'),
 [string]$Run='Projection|Lifecycle',
 [string]$Benchmark='',
 [ValidatePattern('^[1-9][0-9]*(x|ms|s)$')][string]$BenchTime='20x',
 [ValidateRange(1,10)][int]$Count=1
)
# Explicit repository-owned save/transaction tests and repeatable durable I/O
# measurement. No native login registration or real profile is ever used.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath (Join-Path $output 'result.json')){throw 'Choose a new output directory to preserve evidence'}
New-Item -ItemType Directory -Path $output -Force|Out-Null
$goPath=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}elseif($GoExe.Contains('/') -or $GoExe.Contains('\')){Join-Path $root $GoExe}else{$GoExe}
$go=(Get-Command $goPath).Source
$prior=@{};foreach($name in @('GOROOT','GOCACHE','TEMP','TMP','HOME')){$prior[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP;$env:HOME=Join-Path $output 'home'
 New-Item -ItemType Directory -Path $env:TEMP,$env:HOME -Force|Out-Null
 $args=@('-C',$root,'test')+$Packages+@('-run',$Run,'-count',"$Count",'-v')
 if($Benchmark){$args+=@('-bench',$Benchmark,'-benchtime',$BenchTime,'-benchmem')}
 $ErrorActionPreference='Continue'
 & $go @args 2>&1|ForEach-Object {$_.ToString()}|Out-File -LiteralPath (Join-Path $output 'tests.log') -Encoding utf8
 $code=$LASTEXITCODE
 $ErrorActionPreference='Stop'
 [IO.File]::WriteAllText((Join-Path $output 'result.json'),(@{exitCode=$code;run=$Run;benchmark=$Benchmark;benchTime=$BenchTime;count=$Count}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 if($code-ne 0){throw "Configuration save checks failed; see $output/tests.log"}
} finally {foreach($name in $prior.Keys){[Environment]::SetEnvironmentVariable($name,$prior[$name],'Process')}}
