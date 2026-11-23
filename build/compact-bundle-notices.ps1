param([Parameter(Mandatory)][string]$BundleDirectory,[string]$GoExe='go',[string]$CargoExe='cargo',[string]$DesktopImplementation='MyGo')
$ErrorActionPreference='Stop'
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
$licenses=Join-Path $bundle 'licenses'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $licenses
$work=Join-Path (Split-Path $bundle -Parent) ('notice-build-'+[guid]::NewGuid().ToString('N'))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $work
$output=Join-Path $work 'licenses'
& node (Join-Path $PSScriptRoot 'bundle-notices.mjs') $bundle $output (Get-Command $GoExe).Source (Get-Command $CargoExe).Source $DesktopImplementation
if($LASTEXITCODE -ne 0){throw 'Runtime notice selection failed'}
# The prior directory contains this build's already verified working notices.
# Keep it beside the build as evidence; only the two new user documents ship.
$prior=Join-Path $work 'verified-input-notices'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $prior
Move-Item -LiteralPath $licenses -Destination $prior
Move-Item -LiteralPath $output -Destination $licenses
