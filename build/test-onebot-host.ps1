param(
 [Parameter(Mandatory)][string]$BunExe,
 [Parameter(Mandatory)][string]$ChatDirectory,
 [Parameter(Mandatory)][string]$AIExe,
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$GoExe='go',
 [string]$Filter='OneBot',
 [string[]]$Packages=@('./apps/host')
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath (Join-Path $output 'result.json')){throw 'Choose a new output directory to preserve evidence'}
New-Item -ItemType Directory -Force $output | Out-Null
$values=@{
 GOPATH=(& $GoExe env GOPATH)
 GOMODCACHE=(& $GoExe env GOMODCACHE)
 WINONE_CHAT_BUN_EXE=(Resolve-Path -LiteralPath $BunExe).Path
 WINONE_CHAT_WORKER=(Resolve-Path -LiteralPath (Join-Path $ChatDirectory 'worker.js')).Path
 WINONE_ONEBOT_WORKER=(Resolve-Path -LiteralPath (Join-Path $ChatDirectory 'onebot.js')).Path
 WINONE_AI_TEST_EXE=(Resolve-Path -LiteralPath $AIExe).Path
 TEMP=$output;TMP=$output;HOME=(Join-Path $output 'home');USERPROFILE=(Join-Path $output 'home')
}
New-Item -ItemType Directory -Force $values.HOME | Out-Null
$previous=@{}
try {
 foreach($name in $values.Keys){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process');[Environment]::SetEnvironmentVariable($name,$values[$name],'Process')}
 # Explicit real-worker test entry. No accounts, installed application or system
 # startup state are used; all native fixtures inherit the isolated roots above.
 $ErrorActionPreference='Continue'
 & $GoExe -C $root test @Packages -run $Filter -count=1 -v 2>&1 | ForEach-Object {$_.ToString()} | Out-File -Encoding utf8 (Join-Path $output 'tests.log')
 $code=$LASTEXITCODE
 $ErrorActionPreference='Stop'
 [IO.File]::WriteAllText((Join-Path $output 'result.json'),(@{exitCode=$code}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 if($code-ne 0){throw "OneBot Host checks failed; see $output/tests.log"}
} finally {foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}
