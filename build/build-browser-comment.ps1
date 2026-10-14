param([Parameter(Mandatory)][string]$OutputDirectory,[string]$BunExe='bun')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force -Path $output | Out-Null
& $BunExe build (Join-Path $root 'apps/desktop/frontend/src/features/cindy/browser/browserCommentPreload.ts') --target browser --format iife --minify --outfile (Join-Path $output 'browser_comment_guest.js')
if($LASTEXITCODE -ne 0){throw 'Browser comment guest compilation failed'}
