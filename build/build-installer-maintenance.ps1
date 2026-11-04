[CmdletBinding()]
param(
    [string]$GoExe='go',
    [Parameter(Mandatory)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore --quiet --no-index -- (Join-Path $output 'maintenance.probe')
if($LASTEXITCODE -ne 0){throw 'Maintenance helper output must be Git-ignored'}
[IO.Directory]::CreateDirectory($output)|Out-Null
$priorRoot=$env:GOROOT
$env:GOROOT=''
Push-Location $root
try {
    # Embed the helper's real identity and asInvoker policy. Do not copy Host's
    # UI manifest or carry its tray, account and login entry points into setup.
    & $GoExe run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest build/windows/installer-maintenance.manifest -ico internal/branding/icon.ico -o build/installer-maintenance/resource_windows.syso
    if($LASTEXITCODE -ne 0){throw 'Maintenance helper resources failed'}
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o (Join-Path $output 'WinOne.Maintenance.exe') ./build/installer-maintenance
    if($LASTEXITCODE -ne 0){throw 'Maintenance helper build failed'}
} finally {Pop-Location;$env:GOROOT=$priorRoot}
$helper=Join-Path $output 'WinOne.Maintenance.exe'
$hash=(Get-FileHash -LiteralPath $helper -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText($helper+'.sha256',$hash+'  WinOne.Maintenance.exe'+"`n",[Text.UTF8Encoding]::new($false))
Write-Output "Dedicated maintenance helper built: $helper"
