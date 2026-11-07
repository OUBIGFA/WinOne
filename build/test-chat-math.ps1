param([string]$BunExe='dist/resources/tools/bun-1.4.2/bun.exe')
# Focused math-layout regression; no package, font or browser is downloaded.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$bun=if([IO.Path]::IsPathRooted($BunExe)){$BunExe}else{Join-Path $root $BunExe}
& $bun test (Join-Path $root 'modules/chat-pi/math/convert.test.mjs')
if($LASTEXITCODE-ne 0){throw 'Math layout checks failed'}
